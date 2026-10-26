package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/modelcatalog"
	"github.com/yudi/meet-asr/core-go/tasklibrary"
)

type StorageTask struct {
	Folder     string `json:"folder"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	TotalBytes int64  `json:"total_bytes"`
	ChunkBytes int64  `json:"chunk_bytes"`
}

type StorageSummary struct {
	OutputRoot string           `json:"output_root"`
	Categories map[string]int64 `json:"categories"`
	TotalBytes int64            `json:"total_bytes"`
	Tasks      []StorageTask    `json:"tasks"`
	IssueCount int              `json:"issue_count"`
}

// Logical sizes only. Never follow symlinks, including the requested tree root.
func storageTree(root string, visit func(string, fs.FileInfo)) int {
	if info, err := os.Lstat(root); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 1
	}
	issues := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			issues++
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			issues++
			return nil
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				issues++
			} else if info.Mode().IsRegular() {
				visit(path, info)
			}
		}
		return nil
	})
	return issues
}

func taskStorageCategory(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 2 {
		return "other"
	}
	switch parts[1] {
	case ".vad-work":
		return "temporary"
	case "audio":
		return "source_audio"
	case "chunks":
		return "chunk_audio"
	case "transcripts", "transcript.md":
		return "transcripts"
	case "generated", "meeting_minutes.md", "notes_input.md":
		return "generated"
	case "task_manifest.json", "manifest.json", ".resume.lock":
		return "metadata"
	default:
		return "other"
	}
}

func storageSummary(root, appRoot string) (StorageSummary, error) {
	s := StorageSummary{OutputRoot: root, Categories: map[string]int64{}, Tasks: []StorageTask{}}
	for _, key := range []string{"source_audio", "chunk_audio", "transcripts", "generated", "metadata", "models", "temporary", "other"} {
		s.Categories[key] = 0
	}
	if root != "" {
		if !filepath.IsAbs(root) {
			return s, errors.New("output root must be absolute")
		}
		if _, err := os.Stat(root); err == nil {
			canonical, err := filepath.EvalSymlinks(root)
			if err != nil {
				return s, err
			}
			root = canonical
			s.OutputRoot = root
			discovery, err := tasklibrary.Discover(root)
			if err != nil {
				return s, err
			}
			s.IssueCount += len(discovery.Issues)
			byFolder := map[string]int{}
			for _, task := range discovery.Tasks {
				byFolder[task.TaskFolder] = len(s.Tasks)
				s.Tasks = append(s.Tasks, StorageTask{Folder: task.TaskFolder, Title: task.Title, Status: task.Status})
			}
			s.IssueCount += storageTree(root, func(path string, info fs.FileInfo) {
				rel, _ := filepath.Rel(root, path)
				key := taskStorageCategory(rel)
				s.Categories[key] += info.Size()
				s.TotalBytes += info.Size()
				folder := strings.Split(rel, string(filepath.Separator))[0]
				if i, exists := byFolder[folder]; exists {
					s.Tasks[i].TotalBytes += info.Size()
					if key == "chunk_audio" {
						s.Tasks[i].ChunkBytes += info.Size()
					}
				}
			})
		} else if !errors.Is(err, os.ErrNotExist) {
			return s, err
		}
	}
	if appRoot != "" {
		if !filepath.IsAbs(appRoot) {
			return s, errors.New("app data root must be absolute")
		}
		if _, err := os.Stat(appRoot); errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		canonical, err := filepath.EvalSymlinks(appRoot)
		if err != nil {
			return s, err
		}
		appRoot = canonical
		// An overlapping custom output root is already counted above.
		if root != "" {
			rel, _ := filepath.Rel(root, appRoot)
			if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				s.IssueCount++
				return s, nil
			}
		}
		s.IssueCount += storageTree(appRoot, func(path string, info fs.FileInfo) {
			if root != "" {
				rel, _ := filepath.Rel(root, path)
				if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return
				}
			}
			rel, _ := filepath.Rel(appRoot, path)
			first := strings.Split(rel, string(filepath.Separator))[0]
			key := "other"
			if first == "models" {
				key = "models"
			} else if first == "downloads" || first == "staging" {
				key = "temporary"
			}
			s.Categories[key] += info.Size()
			s.TotalBytes += info.Size()
		})
	}
	return s, nil
}

type BatchCacheTask struct {
	Folder  string       `json:"folder"`
	Title   string       `json:"title"`
	Preview CachePreview `json:"preview"`
}
type BatchCacheResult struct {
	OutputRoot   string           `json:"output_root"`
	Tasks        []BatchCacheTask `json:"tasks"`
	RemovedBytes int64            `json:"removed_bytes"`
	CleanedCount int              `json:"cleaned_count"`
	SkippedCount int              `json:"skipped_count"`
	FailedCount  int              `json:"failed_count"`
}
type BatchCacheSelection struct {
	Folder string `json:"folder"`
	Token  string `json:"token"`
}
type BatchCacheRequest struct {
	OutputRoot string                `json:"output_root"`
	Tasks      []BatchCacheSelection `json:"tasks"`
}

func previewCacheBatch(root string) (BatchCacheResult, error) {
	r := BatchCacheResult{OutputRoot: root, Tasks: []BatchCacheTask{}}
	if root == "" {
		return r, nil
	}
	canonical, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	r.OutputRoot = canonical
	discovery, err := tasklibrary.Discover(canonical)
	if err != nil {
		return r, err
	}
	for _, task := range discovery.Tasks {
		p, err := taskCache(canonical, task.TaskFolder, "", false)
		if err != nil {
			p = CachePreview{Files: []CacheFile{}, Reason: "operation_failed", CacheStatus: "unknown"}
		}
		r.Tasks = append(r.Tasks, BatchCacheTask{Folder: task.TaskFolder, Title: task.Title, Preview: p})
	}
	return r, nil
}

func cleanCacheBatch(root string, request BatchCacheRequest) (BatchCacheResult, error) {
	if !filepath.IsAbs(root) {
		return BatchCacheResult{}, errors.New("output root must be absolute")
	}
	r := BatchCacheResult{OutputRoot: root, Tasks: []BatchCacheTask{}}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return r, err
	}
	expected, err := filepath.EvalSymlinks(request.OutputRoot)
	if err != nil || expected != canonical {
		return r, errors.New("output root changed; refresh preview")
	}
	if len(request.Tasks) > 5000 {
		return r, errors.New("too many tasks in one cleanup")
	}
	seen := map[string]bool{}
	for _, task := range request.Tasks {
		if seen[task.Folder] || task.Token == "" {
			return r, errors.New("invalid cleanup selection")
		}
		seen[task.Folder] = true
	}
	for _, task := range request.Tasks {
		p, err := taskCache(canonical, task.Folder, task.Token, true)
		if err != nil {
			p = CachePreview{Files: []CacheFile{}, Reason: "operation_failed", CacheStatus: "unknown"}
			r.FailedCount++
		} else if p.CacheStatus == "cleaned" && p.Reason == "" {
			r.CleanedCount++
			r.RemovedBytes += p.RemovedBytes
		} else {
			r.SkippedCount++
		}
		r.Tasks = append(r.Tasks, BatchCacheTask{Folder: task.Folder, Preview: p})
	}
	return r, nil
}

func runStorageCommand(command string, args []string, input io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("output-root", "", "Output root")
	appRoot := flags.String("app-data-root", "", "App data root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected storage arguments")
	}
	var result any
	var err error
	switch command {
	case "storage-summary":
		result, err = storageSummary(*root, *appRoot)
	case "preview-cache-batch":
		if *root != "" && !filepath.IsAbs(*root) {
			return errors.New("output root must be absolute")
		}
		result, err = previewCacheBatch(*root)
	case "clean-cache-batch":
		var request BatchCacheRequest
		decoder := json.NewDecoder(io.LimitReader(input, 4<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return errors.New("expected one JSON request")
		}
		result, err = cleanCacheBatch(*root, request)
	case "cleanup-model-temporary":
		result, err = modelcatalog.CleanupTemporary(*appRoot, time.Now(), 7*24*time.Hour)
	default:
		return errors.New("unknown storage command")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
