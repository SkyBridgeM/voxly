package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

type CacheFile struct {
	Reference string `json:"reference"`
	SizeBytes int64  `json:"size_bytes"`
}

type CachePreview struct {
	Scope        string      `json:"scope,omitempty"`
	CanClean     bool        `json:"can_clean"`
	Reason       string      `json:"reason,omitempty"`
	Files        []CacheFile `json:"files"`
	TotalBytes   int64       `json:"total_bytes"`
	Token        string      `json:"token,omitempty"`
	CacheStatus  string      `json:"cache_status"`
	RemovedBytes int64       `json:"removed_bytes"`
}

// Cache operations accept only a direct task folder under the configured root.
// No path from the frontend or manifest is ever used as a deletion target.
func cacheTaskDirectory(root, folder string) (string, error) {
	if !filepath.IsAbs(root) || folder == "." || folder == ".." || strings.ContainsAny(folder, `/\`) || strings.TrimSpace(folder) == "" {
		return "", errors.New("invalid task folder")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, folder)
	for _, p := range []string{dir, ManifestPath(dir)} {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("unsafe task folder")
		}
	}
	if info, err := os.Lstat(filepath.Join(dir, ".resume.lock")); err == nil && !info.Mode().IsRegular() {
		return "", errors.New("unsafe task lock")
	}
	return dir, nil
}

func regularCacheInput(taskDir, path string) bool {
	if !filepath.IsAbs(path) {
		path = filepath.Join(taskDir, path)
	}
	rel, err := filepath.Rel(taskDir, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	current := taskDir
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func previewCacheLocked(dir string, m *Manifest) (CachePreview, error) {
	return previewAudioLocked(dir, m, false)
}

func previewAudioLocked(dir string, m *Manifest, textOnly bool) (CachePreview, error) {
	p := CachePreview{Files: []CacheFile{}, CacheStatus: "present", Scope: "chunks"}
	if textOnly {
		p.Scope = "text_only"
	}
	var err error
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return p, err
	}
	if m.CacheCleanup != nil {
		p.CacheStatus = m.CacheCleanup.Status
	}
	block := func(reason string) (CachePreview, error) { p.Reason = reason; return p, nil }
	if m.State != TaskCompleted || (m.Status != "completed" && m.Status != "completed_with_errors") || (textOnly && m.Status != "completed") {
		return block("task_not_completed")
	}
	for _, g := range m.Generations {
		if g.Status == "processing" || g.Status == "pending" {
			return block("task_busy")
		}
	}
	// Only the one recorded, direct audio child is eligible, never an external
	// import or another file under audio/. Persisted intent allows crash recovery
	// if that source was already unlinked before the final manifest write.
	sourceDir, sourceDirErr := filepath.EvalSymlinks(filepath.Dir(m.Source.Path))
	sourcePath := filepath.Join(sourceDir, filepath.Base(m.Source.Path))
	sourceRef := filepath.ToSlash(filepath.Join("audio", filepath.Base(sourcePath)))
	if sourceDirErr != nil || !filepath.IsAbs(m.Source.Path) || filepath.Dir(sourcePath) != filepath.Join(dir, "audio") {
		return block("source_unavailable")
	}
	audioInfo, audioErr := os.Lstat(filepath.Dir(m.Source.Path))
	if audioErr != nil || !audioInfo.IsDir() || audioInfo.Mode()&os.ModeSymlink != 0 {
		return block("source_unavailable")
	}
	source, sourceErr := os.Lstat(sourcePath)
	removedIntentionally := m.CacheCleanup != nil && m.CacheCleanup.Scope == "text_only" && m.CacheCleanup.SourceReference == sourceRef && (m.CacheCleanup.Status == "cleaning" || m.CacheCleanup.Status == "cleaned")
	sourceMissing := errors.Is(sourceErr, os.ErrNotExist) && removedIntentionally
	if !sourceMissing && (sourceErr != nil || !source.Mode().IsRegular() || source.Size() != m.Source.SizeBytes || !regularCacheInput(dir, sourcePath)) {
		return block("source_unavailable")
	}
	transcript := filepath.Join(dir, "transcript.md")
	if !regularCacheInput(dir, transcript) || m.Artifacts.Transcript.State != ArtifactReady || !artifactMatches(transcript, &m.Artifacts.Transcript) {
		return block("transcript_unverified")
	}
	if c := m.CanonicalTranscript; c != nil {
		h, err := fileSHA256(transcript)
		if err != nil || c.Status != "completed" || c.ContentSHA256 != h {
			return block("transcript_unverified")
		}
	}
	if m.Chunks.VAD != nil && !vadArtifactsValid(dir, m) {
		return block("transcript_unverified")
	}
	chunksDir := filepath.Join(dir, "chunks")
	info, err := os.Lstat(chunksDir)
	chunksMissing := errors.Is(err, os.ErrNotExist)
	if !chunksMissing && (err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return block("unsafe_cache")
	}
	allowed := make(map[string]bool)
	for _, item := range m.Chunks.Items {
		if !strings.HasPrefix(item.ID, "part_") || strings.ContainsAny(item.ID, `/\.`) || (m.Chunks.Format != "wav" && m.Chunks.Format != "mp3") {
			return block("unsafe_cache")
		}
		allowed[item.ID+"."+m.Chunks.Format] = true
	}
	var entries []os.DirEntry
	if !chunksMissing {
		entries, err = os.ReadDir(chunksDir)
		if err != nil {
			return p, err
		}
	}
	hash := sha256.New()
	manifestBytes, err := os.ReadFile(ManifestPath(dir))
	if err != nil {
		return p, err
	}
	hash.Write(manifestBytes)
	fmt.Fprintln(hash, p.Scope)
	if !sourceMissing {
		sourceHash, err := fileSHA256(sourcePath)
		if err != nil {
			return block("source_unavailable")
		}
		fmt.Fprintln(hash, sourceHash)
	}
	for _, entry := range entries {
		path := filepath.Join(chunksDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || !allowed[entry.Name()] {
			return block("unsafe_cache")
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return p, err
		}
		p.Files = append(p.Files, CacheFile{Reference: "chunks/" + entry.Name(), SizeBytes: info.Size()})
		p.TotalBytes += info.Size()
		fmt.Fprintf(hash, "%s:%d:%s\n", entry.Name(), info.Size(), digest)
	}
	// Source is last: on interruption remaining audio can still be inspected.
	if textOnly && !sourceMissing {
		p.Files = append(p.Files, CacheFile{Reference: sourceRef, SizeBytes: source.Size()})
		p.TotalBytes += source.Size()
	}
	if len(p.Files) == 0 && (m.CacheCleanup == nil || m.CacheCleanup.Status != "cleaning") {
		return block("no_cache")
	}
	p.CanClean = true
	p.Token = hex.EncodeToString(hash.Sum(nil))
	return p, nil
}

func taskCache(root, folder, expectedToken string, clean bool) (CachePreview, error) {
	return taskAudioCache(root, folder, expectedToken, clean, false)
}

func taskAudioCache(root, folder, expectedToken string, clean, textOnly bool) (CachePreview, error) {
	dir, err := cacheTaskDirectory(root, folder)
	if err != nil {
		return CachePreview{}, err
	}
	lock, err := AcquireTaskLock(dir)
	if err != nil {
		return CachePreview{Files: []CacheFile{}, Reason: "task_busy", CacheStatus: "unknown"}, nil
	}
	defer lock.Release()
	m, err := LoadManifest(dir)
	if err != nil {
		return CachePreview{}, err
	}
	p, err := previewAudioLocked(dir, m, textOnly)
	if err != nil || !clean || !p.CanClean {
		return p, err
	}
	if expectedToken == "" || expectedToken != p.Token {
		p.CanClean, p.Reason, p.Token = false, "preview_changed", ""
		return p, nil
	}
	return cleanCacheLocked(dir, m, p)
}

// Caller owns the Task lock, including automatic cleanup at workflow completion.
func cleanCacheLocked(dir string, m *Manifest, p CachePreview) (CachePreview, error) {
	// Persist intent before the first unlink. A crash leaves a retryable cleaning
	// record; no transcript, generation or checkpoint is discarded.
	if m.CacheCleanup == nil {
		m.CacheCleanup = &taskmanifest.CacheCleanup{}
	}
	if p.Scope == "text_only" {
		m.CacheCleanup.Scope = "text_only"
		m.CacheCleanup.SourceReference = filepath.ToSlash(filepath.Join("audio", filepath.Base(m.Source.Path)))
	} else if m.CacheCleanup.Scope != "text_only" {
		m.CacheCleanup.Scope = "chunks"
	}
	m.CacheCleanup.Status = "cleaning"
	m.CacheCleanup.StartedAt = nowFunc().UTC().Format(time.RFC3339)
	m.CacheCleanup.CleanedAt = ""
	if err := SaveManifest(dir, m); err != nil {
		return p, err
	}
	for _, file := range p.Files {
		path := filepath.Join(dir, filepath.FromSlash(file.Reference))
		if !regularCacheInput(dir, path) {
			return p, errors.New("cache changed during cleanup")
		}
		if err := os.Remove(path); err != nil {
			return p, err
		}
		p.RemovedBytes += file.SizeBytes
	}
	m.CacheCleanup.Status = "cleaned"
	m.CacheCleanup.CleanedAt = nowFunc().UTC().Format(time.RFC3339)
	if err := SaveManifest(dir, m); err != nil {
		return p, err
	}
	p.CanClean, p.Token, p.CacheStatus = false, "", "cleaned"
	return p, nil
}

func autoCleanTaskCacheLocked(dir string, m *Manifest, enabled bool) {
	if !enabled || m.Status != "completed" {
		return
	}
	p, err := previewCacheLocked(dir, m)
	if err == nil && p.CanClean {
		_, err = cleanCacheLocked(dir, m, p)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Processing cache cleanup did not complete; it can be retried from storage settings.")
	}
}

func runTaskCache(command string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("output-root", "", "Application-owned output root")
	folder := fs.String("task-folder", "", "Direct task folder")
	token := fs.String("expected-token", "", "Preview token")
	scope := fs.String("scope", "chunks", "chunks or text_only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected cache command arguments")
	}
	if *scope != "chunks" && *scope != "text_only" {
		return errors.New("invalid cleanup scope")
	}
	p, err := taskAudioCache(*root, *folder, *token, command == "clean-task-cache", *scope == "text_only")
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(p)
}
