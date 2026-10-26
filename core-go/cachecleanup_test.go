package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

func cacheFixture(t *testing.T) (string, string, *Manifest) {
	t.Helper()
	dir, m := completedV2Manifest(t)
	for _, sub := range []string{"audio", "chunks", "transcripts"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(dir, "audio", "source.wav")
	if err := os.Rename(m.Source.Path, source); err != nil {
		t.Fatal(err)
	}
	m.Source.Path = source
	m.Chunks.Format, m.Chunks.Total = "wav", 2
	m.Chunks.ASRModel = defaultMimoModel
	for _, id := range []string{"part_001", "part_002"} {
		audio := filepath.Join(dir, "chunks", id+".wav")
		text := filepath.Join(dir, "transcripts", id+".txt")
		if err := os.WriteFile(audio, []byte("audio"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(text, []byte("hello\n"), 0600); err != nil {
			t.Fatal(err)
		}
		h, _ := fileSHA256(audio)
		th, _ := fileSHA256(text)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{ID: id, AudioPath: audio, TranscriptPath: text, State: ChunkCompleted, SizeBytes: 5, AudioSHA256: h, TranscriptSHA256: th})
	}
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(dir), filepath.Base(dir), m
}

func TestCacheCleanupPreservesOutputsAndGeneration(t *testing.T) {
	root, folder, m := cacheFixture(t)
	dir := filepath.Join(root, folder)
	previous, err := GenerateContent(dir, m, "structured-notes", DefaultSettings(), fakeContentGenerator{text: "previous summary"})
	if err != nil {
		t.Fatal(err)
	}
	previousPath := filepath.Join(dir, filepath.FromSlash(previous.ArtifactReference))
	before := map[string]string{}
	for _, p := range []string{m.Source.Path, m.Artifacts.Transcript.Path, m.Chunks.Items[0].TranscriptPath, previousPath} {
		before[p], _ = fileSHA256(p)
	}
	p, err := taskCache(root, folder, "", false)
	if err != nil || !p.CanClean || p.TotalBytes != 10 || len(p.Files) != 2 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if _, err := os.Stat(m.Chunks.Items[0].AudioPath); err != nil {
		t.Fatal("preview deleted cache")
	}
	result, err := taskCache(root, folder, p.Token, true)
	if err != nil || result.CacheStatus != "cleaned" || result.RemovedBytes != 10 {
		t.Fatalf("cleanup: %+v %v", result, err)
	}
	for path, h := range before {
		if after, _ := fileSHA256(path); after != h {
			t.Fatalf("changed retained file %s", path)
		}
	}
	m, err = LoadManifest(dir)
	if err != nil || m.CacheCleanup == nil || m.CacheCleanup.Status != "cleaned" || m.CacheCleanup.CleanedAt == "" {
		t.Fatalf("cleanup not durable: %v", err)
	}
	ReconcileChunks(dir, m)
	ReconcileArtifacts(dir, m)
	if m.Chunks.Items[0].State != ChunkCompleted {
		t.Fatal("intentional removal invalidated ASR provenance")
	}
	if _, complete := ResumeStage(m); !complete {
		t.Fatal("cleaned task restarted ASR")
	}
	emptyEnv := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(emptyEnv, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunResume([]string{dir, "--env-file", emptyEnv}); err != nil {
		t.Fatalf("completed resume after cleanup: %v", err)
	}
	g, err := GenerateContent(dir, m, "structured-notes", DefaultSettings(), fakeContentGenerator{text: "summary"})
	if err != nil || g.Status != "completed" {
		t.Fatalf("generation after cleanup: %+v %v", g, err)
	}
	p, err = taskCache(root, folder, "", false)
	if err != nil || p.CanClean || p.Reason != "no_cache" {
		t.Fatalf("repeat cleanup: %+v %v", p, err)
	}
}

func TestCacheCleanupBlocksUnsafeAndIncompleteTasks(t *testing.T) {
	tests := []struct {
		name, reason string
		mutate       func(string, *Manifest)
	}{
		{"incomplete", "task_not_completed", func(_ string, m *Manifest) { m.State = TaskTranscribing }},
		{"missing_source", "source_unavailable", func(_ string, m *Manifest) { os.Remove(m.Source.Path) }},
		{"changed_source_size", "source_unavailable", func(_ string, m *Manifest) { os.WriteFile(m.Source.Path, []byte("changed"), 0600) }},
		{"source_inside_cache", "source_unavailable", func(_ string, m *Manifest) { m.Source.Path = m.Chunks.Items[0].AudioPath }},
		{"changed_transcript", "transcript_unverified", func(_ string, m *Manifest) { os.WriteFile(m.Artifacts.Transcript.Path, []byte("edited"), 0600) }},
		{"unknown_file", "unsafe_cache", func(d string, _ *Manifest) {
			os.WriteFile(filepath.Join(d, "chunks", "keep.wav"), []byte("keep"), 0600)
		}},
		{"busy_generation", "task_not_completed", func(_ string, m *Manifest) {
			_, err := GenerateContent(filepath.Dir(m.Artifacts.Transcript.Path), m, "structured-notes", DefaultSettings(), fakeContentGenerator{text: "summary"})
			if err != nil {
				t.Fatal(err)
			}
			m.Generations[0].Status = "processing"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, folder, m := cacheFixture(t)
			dir := filepath.Join(root, folder)
			tt.mutate(dir, m)
			if err := SaveManifest(dir, m); err != nil {
				t.Fatal(err)
			}
			p, err := taskCache(root, folder, "", false)
			if err != nil || p.CanClean || p.Reason != tt.reason {
				t.Fatalf("got %+v %v", p, err)
			}
			if _, err := os.Stat(m.Chunks.Items[1].AudioPath); err != nil {
				t.Fatal("blocked preview deleted a chunk")
			}
		})
	}
}

func TestCacheCleanupRejectsStalePreviewAndLocks(t *testing.T) {
	root, folder, m := cacheFixture(t)
	dir := filepath.Join(root, folder)
	p, err := taskCache(root, folder, "", false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(m.Chunks.Items[0].AudioPath, []byte("altered"), 0600)
	result, err := taskCache(root, folder, p.Token, true)
	if err != nil || result.Reason != "preview_changed" {
		t.Fatalf("stale preview: %+v %v", result, err)
	}
	lock, err := AcquireTaskLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	p, err = taskCache(root, folder, "", false)
	if err != nil || p.CanClean || p.Reason != "task_busy" {
		t.Fatalf("lock conflict: %+v %v", p, err)
	}
	var out bytes.Buffer
	if err := runUpdateTaskMetadata([]string{"--output-root", root, "--task-id", m.TaskID}, bytes.NewBufferString(`{"title":"other","description":"","tags":[]}`), &out); err == nil {
		t.Fatal("metadata update ignored task lock")
	}
}

func TestCacheCleanupInterruptedRetryAndEmptyIntent(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "all_removed"}[all], func(t *testing.T) {
			root, folder, m := cacheFixture(t)
			dir := filepath.Join(root, folder)
			m.CacheCleanup = &taskmanifest.CacheCleanup{Status: "cleaning", StartedAt: m.CreatedAt}
			if err := SaveManifest(dir, m); err != nil {
				t.Fatal(err)
			}
			os.Remove(m.Chunks.Items[0].AudioPath)
			if all {
				os.Remove(m.Chunks.Items[1].AudioPath)
			}
			p, err := taskCache(root, folder, "", false)
			if err != nil || !p.CanClean {
				t.Fatalf("retry preview %+v %v", p, err)
			}
			p, err = taskCache(root, folder, p.Token, true)
			if err != nil || p.CacheStatus != "cleaned" {
				t.Fatalf("retry cleanup %+v %v", p, err)
			}
		})
	}
}

func TestCacheCleanupRejectsTraversalAndSymlinks(t *testing.T) {
	root, folder, m := cacheFixture(t)
	dir := filepath.Join(root, folder)
	for _, bad := range []string{"..", ".", "../" + folder, dir, ""} {
		if _, err := taskCache(root, bad, "", false); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.wav")
	os.WriteFile(outside, []byte("outside"), 0600)
	p := m.Chunks.Items[0].AudioPath
	os.Remove(p)
	if err := os.Symlink(outside, p); err != nil {
		t.Skip(err)
	}
	preview, err := taskCache(root, folder, "", false)
	if err != nil || preview.CanClean || preview.Reason != "unsafe_cache" {
		t.Fatalf("symlink preview %+v %v", preview, err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "outside" {
		t.Fatal("external file changed")
	}
	link := filepath.Join(root, "linked-task")
	os.Symlink(dir, link)
	if _, err := taskCache(root, "linked-task", "", false); err == nil {
		t.Fatal("accepted symlinked task")
	}
}

func TestCacheCleanupCLIAndManifestCompatibility(t *testing.T) {
	root, folder, m := cacheFixture(t)
	var out bytes.Buffer
	if err := runTaskCache("preview-task-cache", []string{"--output-root", root, "--task-folder", folder}, &out); err != nil {
		t.Fatal(err)
	}
	var p CachePreview
	if err := json.Unmarshal(out.Bytes(), &p); err != nil || !p.CanClean {
		t.Fatalf("CLI output %s %v", out.String(), err)
	}
	if m.UnknownFields == nil {
		m.UnknownFields = map[string]json.RawMessage{}
	}
	m.UnknownFields["future_field"] = json.RawMessage(`{"keep":true}`)
	m.CacheCleanup = &taskmanifest.CacheCleanup{Status: "cleaning", StartedAt: m.CreatedAt, UnknownFields: map[string]json.RawMessage{"future_cache_flag": json.RawMessage(`true`)}}
	if err := SaveManifest(filepath.Join(root, folder), m); err != nil {
		t.Fatal(err)
	}
	p, err := taskCache(root, folder, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taskCache(root, folder, p.Token, true); err != nil {
		t.Fatal(err)
	}
	m, err = LoadManifest(filepath.Join(root, folder))
	if m.CacheCleanup == nil || string(m.CacheCleanup.UnknownFields["future_cache_flag"]) != "true" {
		t.Fatal("unknown cleanup field lost")
	}
	var future map[string]bool
	if err != nil || json.Unmarshal(m.UnknownFields["future_field"], &future) != nil || !future["keep"] {
		t.Fatalf("unknown field lost: %v", err)
	}
}

func TestCacheCleanupPreviewTracksSourceAndManifest(t *testing.T) {
	for _, change := range []string{"source", "metadata", "missing_token"} {
		t.Run(change, func(t *testing.T) {
			root, folder, m := cacheFixture(t)
			dir := filepath.Join(root, folder)
			p, err := taskCache(root, folder, "", false)
			if err != nil || !p.CanClean {
				t.Fatalf("preview %+v %v", p, err)
			}
			token := p.Token
			switch change {
			case "source":
				if err := os.WriteFile(m.Source.Path, []byte("other"), 0600); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				m.UserMetadata = &taskmanifest.UserMetadata{Title: "changed"}
				if err := SaveManifest(dir, m); err != nil {
					t.Fatal(err)
				}
			case "missing_token":
				token = ""
			}
			p, err = taskCache(root, folder, token, true)
			if err != nil || p.CanClean || p.Reason != "preview_changed" {
				t.Fatalf("changed preview %+v %v", p, err)
			}
			if _, err := os.Stat(m.Chunks.Items[0].AudioPath); err != nil {
				t.Fatal("stale preview removed audio")
			}
			loaded, err := LoadManifest(dir)
			if err != nil || loaded.CacheCleanup != nil {
				t.Fatal("stale preview changed manifest")
			}
		})
	}
}
