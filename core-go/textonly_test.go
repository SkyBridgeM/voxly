package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

func TestTextOnlyKeepsResultsAndExternalOriginal(t *testing.T) {
	dir, m, source := vadFixture(t, false)
	finishVADFixture(t, dir, m)
	root, folder := filepath.Dir(dir), filepath.Base(dir)
	external := filepath.Join(t.TempDir(), "original.wav")
	data, _ := os.ReadFile(source)
	if err := os.WriteFile(external, data, 0600); err != nil {
		t.Fatal(err)
	}
	generation, err := GenerateContent(dir, m, "structured-notes", DefaultSettings(), fakeContentGenerator{text: "saved summary"})
	if err != nil {
		t.Fatal(err)
	}
	retained := []string{external, filepath.Join(dir, "transcript.md"), filepath.Join(dir, vadResults), filepath.Join(dir, vadTimed), m.Chunks.Items[0].TranscriptPath, filepath.Join(dir, generation.ArtifactReference)}
	before := map[string]string{}
	for _, path := range retained {
		before[path], err = fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Works even when chunks were cleaned separately first.
	chunks, err := taskCache(root, folder, "", false)
	if err != nil || !chunks.CanClean {
		t.Fatalf("chunks preview: %+v %v", chunks, err)
	}
	if _, err = taskCache(root, folder, chunks.Token, true); err != nil {
		t.Fatal(err)
	}
	p, err := taskAudioCache(root, folder, "", false, true)
	if err != nil || !p.CanClean || len(p.Files) != 1 || p.Files[0].Reference != "audio/source.wav" {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if _, err = os.Stat(source); err != nil {
		t.Fatal("preview removed source")
	}
	result, err := taskAudioCache(root, folder, p.Token, true, true)
	if err != nil || result.CacheStatus != "cleaned" || result.RemovedBytes != int64(len(data)) {
		t.Fatalf("clean: %+v %v", result, err)
	}
	if _, err = os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("source retained")
	}
	for path, want := range before {
		if got, e := fileSHA256(path); e != nil || got != want {
			t.Fatalf("retained output changed: %s", path)
		}
	}
	m, err = LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	ReconcileChunks(dir, m)
	ReconcileArtifacts(dir, m)
	if _, done := ResumeStage(m); !done {
		t.Fatal("text-only task became incomplete")
	}
	if err = RunResume([]string{dir}); err != nil {
		t.Fatal(err)
	}
	if _, err = GenerateContent(dir, m, "structured-notes", DefaultSettings(), fakeContentGenerator{text: "new summary"}); err != nil {
		t.Fatal(err)
	}
	p, err = taskAudioCache(root, folder, "", false, true)
	if err != nil || p.CanClean || p.Reason != "no_cache" {
		t.Fatalf("repeated preview: %+v %v", p, err)
	}
}

func TestTextOnlyGuardsAndPreviewScope(t *testing.T) {
	for _, kind := range []string{"external", "symlink", "failed", "changed", "scope", "bad_transcript", "busy", "bad_timing"} {
		t.Run(kind, func(t *testing.T) {
			dir, m, source := vadFixture(t, false)
			finishVADFixture(t, dir, m)
			root, folder := filepath.Dir(dir), filepath.Base(dir)
			p, err := taskAudioCache(root, folder, "", false, true)
			if err != nil || !p.CanClean {
				t.Fatalf("preview: %+v %v", p, err)
			}
			switch kind {
			case "external":
				m.Source.Path = filepath.Join(t.TempDir(), "original.wav")
				os.WriteFile(m.Source.Path, []byte("outside"), 0600)
				SaveManifest(dir, m)
			case "symlink":
				external := filepath.Join(t.TempDir(), "original.wav")
				data, _ := os.ReadFile(source)
				os.WriteFile(external, data, 0600)
				os.Remove(source)
				os.Symlink(external, source)
			case "failed":
				m.State = TaskFailed
				m.Status = "failed"
				SaveManifest(dir, m)
			case "changed":
				data, _ := os.ReadFile(source)
				data[50] ^= 1
				os.WriteFile(source, data, 0600)
			case "scope":
				p, err = taskCache(root, folder, "", false)
				if err != nil {
					t.Fatal(err)
				}
			case "bad_transcript":
				os.WriteFile(filepath.Join(dir, "transcript.md"), []byte("modified"), 0600)
			case "bad_timing":
				os.WriteFile(filepath.Join(dir, vadResults), []byte("modified"), 0600)
			case "busy":
				lock, e := AcquireTaskLock(dir)
				if e != nil {
					t.Fatal(e)
				}
				defer lock.Release()
			}
			result, err := taskAudioCache(root, folder, p.Token, true, true)
			if err != nil {
				t.Fatal(err)
			}
			if result.CanClean || result.RemovedBytes != 0 || result.Reason == "" {
				t.Fatalf("unsafe cleanup permitted: %+v", result)
			}
			if _, err = os.Lstat(source); err != nil {
				t.Fatal("source removed on refused cleanup")
			}
		})
	}
}

func TestTextOnlyInterruptedCleanupRetry(t *testing.T) {
	dir, m, source := vadFixture(t, false)
	finishVADFixture(t, dir, m)
	m.CacheCleanup = &taskmanifest.CacheCleanup{Scope: "text_only", SourceReference: "audio/source.wav", Status: "cleaning"}
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after deleting all audio but before committing cleaned.
	for _, item := range m.Chunks.Items {
		if err := os.Remove(item.AudioPath); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	root, folder := filepath.Dir(dir), filepath.Base(dir)
	p, err := taskAudioCache(root, folder, "", false, true)
	if err != nil || !p.CanClean || len(p.Files) != 0 {
		t.Fatalf("retry preview: %+v %v", p, err)
	}
	result, err := taskAudioCache(root, folder, p.Token, true, true)
	if err != nil || result.CacheStatus != "cleaned" {
		t.Fatalf("retry: %+v %v", result, err)
	}
}

func TestTextOnlyDeletesChunksAndSourceTogether(t *testing.T) {
	root, folder, m := cacheFixture(t)
	p, err := taskAudioCache(root, folder, "", false, true)
	if err != nil || !p.CanClean || len(p.Files) != 3 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	expected := p.TotalBytes
	result, err := taskAudioCache(root, folder, p.Token, true, true)
	if err != nil || result.RemovedBytes != expected {
		t.Fatalf("cleanup: %+v %v", result, err)
	}
	for _, item := range m.Chunks.Items {
		if _, err = os.Stat(item.AudioPath); !os.IsNotExist(err) {
			t.Fatal("chunk retained")
		}
	}
	if _, err = os.Stat(m.Source.Path); !os.IsNotExist(err) {
		t.Fatal("source retained")
	}
	if _, err = os.Stat(filepath.Join(root, folder, "transcript.md")); err != nil {
		t.Fatal(err)
	}
}
