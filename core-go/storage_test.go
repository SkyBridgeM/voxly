package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageSummaryCountsCategoriesWithoutFollowingLinks(t *testing.T) {
	root, folder, m := cacheFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.wav")
	os.WriteFile(outside, bytes.Repeat([]byte("x"), 999), 0600)
	if err := os.Symlink(outside, filepath.Join(root, folder, "audio", "linked.wav")); err != nil {
		t.Skip(err)
	}
	s, err := storageSummary(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tasks) != 1 || s.Tasks[0].ChunkBytes != 10 || s.Categories["source_audio"] != m.Source.SizeBytes || s.Categories["chunk_audio"] != 10 || s.Categories["transcripts"] != 18 || s.IssueCount < 1 {
		t.Fatalf("summary %+v", s)
	}
	var total int64
	for _, size := range s.Categories {
		total += size
	}
	if total != s.TotalBytes {
		t.Fatal("category total mismatch")
	}
	appRoot := t.TempDir()
	os.MkdirAll(filepath.Join(appRoot, "models"), 0700)
	os.MkdirAll(filepath.Join(appRoot, "downloads"), 0700)
	os.WriteFile(filepath.Join(appRoot, "models", "model.onnx"), bytes.Repeat([]byte("m"), 128), 0600)
	os.WriteFile(filepath.Join(appRoot, "downloads", "model.partial"), bytes.Repeat([]byte("p"), 23), 0600)
	app, err := storageSummary("", appRoot)
	if err != nil || app.Categories["models"] != 128 || app.Categories["temporary"] != 23 || app.TotalBytes != 151 {
		t.Fatalf("app summary %+v %v", app, err)
	}
	combined, err := storageSummary(root, appRoot)
	if err != nil {
		t.Fatal(err)
	}
	rootOnly, _ := storageSummary(root, "")
	if combined.TotalBytes != rootOnly.TotalBytes {
		t.Fatal("overlapping roots double-counted")
	}
}

func TestBatchCacheRevalidatesEachTaskAndContinues(t *testing.T) {
	root, first, m1 := cacheFixture(t)
	secondRoot, second, m2 := cacheFixture(t)
	if root != secondRoot {
		t.Fatal("expected shared fixture root")
	}
	_, third, m3 := cacheFixture(t)
	m3.State = TaskTranscribing
	SaveManifest(filepath.Join(root, third), m3)
	_, locked, _ := cacheFixture(t)
	lock, err := AcquireTaskLock(filepath.Join(root, locked))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	p, err := previewCacheBatch(root)
	if err != nil {
		t.Fatal(err)
	}
	request := BatchCacheRequest{OutputRoot: p.OutputRoot}
	blocked := map[string]string{}
	for _, task := range p.Tasks {
		if task.Preview.CanClean {
			request.Tasks = append(request.Tasks, BatchCacheSelection{Folder: task.Folder, Token: task.Preview.Token})
		} else {
			blocked[task.Folder] = task.Preview.Reason
		}
	}
	if len(request.Tasks) != 2 || blocked[third] != "task_not_completed" || blocked[locked] != "task_busy" {
		t.Fatalf("preview %+v", p)
	}
	os.WriteFile(m1.Source.Path, []byte("other"), 0600)
	r, err := cleanCacheBatch(root, request)
	if err != nil || r.CleanedCount != 1 || r.SkippedCount != 1 || r.FailedCount != 0 || r.RemovedBytes != 10 {
		t.Fatalf("batch %+v %v", r, err)
	}
	if _, err := os.Stat(m1.Chunks.Items[0].AudioPath); err != nil {
		t.Fatal("changed task was cleaned")
	}
	if _, err := os.Stat(m2.Chunks.Items[0].AudioPath); !os.IsNotExist(err) {
		t.Fatal("eligible task kept chunks")
	}
	if _, err := os.Stat(m3.Chunks.Items[0].AudioPath); err != nil {
		t.Fatal("incomplete task changed")
	}
	_ = first
	_ = second
}

func TestBatchCacheRejectsRootSwitchAndDuplicateSelection(t *testing.T) {
	root, folder, m := cacheFixture(t)
	p, err := taskCache(root, folder, "", false)
	if err != nil {
		t.Fatal(err)
	}
	request := BatchCacheRequest{OutputRoot: t.TempDir(), Tasks: []BatchCacheSelection{{Folder: folder, Token: p.Token}}}
	if _, err := cleanCacheBatch(root, request); err == nil {
		t.Fatal("accepted changed output root")
	}
	request.OutputRoot = root
	request.Tasks = append(request.Tasks, request.Tasks[0])
	if _, err := cleanCacheBatch(root, request); err == nil {
		t.Fatal("accepted duplicate selection")
	}
	if _, err := os.Stat(m.Chunks.Items[0].AudioPath); err != nil {
		t.Fatal("invalid batch changed files")
	}
}

func TestAutomaticCleanupIsOptInAndOnlyNewSuccess(t *testing.T) {
	root, folder, m := cacheFixture(t)
	dir := filepath.Join(root, folder)
	autoCleanTaskCacheLocked(dir, m, false)
	if _, err := os.Stat(m.Chunks.Items[0].AudioPath); err != nil {
		t.Fatal("default policy cleaned chunks")
	}
	for _, state := range []string{TaskCancelled, TaskFailed, TaskTranscribing} {
		m.State = state
		SaveManifest(dir, m)
		autoCleanTaskCacheLocked(dir, m, true)
		if _, err := os.Stat(m.Chunks.Items[0].AudioPath); err != nil {
			t.Fatalf("%s task was cleaned", state)
		}
	}
	m.State = TaskCompleted
	SaveManifest(dir, m)
	env := filepath.Join(t.TempDir(), "empty.env")
	os.WriteFile(env, nil, 0600)
	if err := RunResume([]string{dir, "--env-file", env, "--auto-clean-chunks=true"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Chunks.Items[0].AudioPath); err != nil {
		t.Fatal("no-op resume swept a historical task")
	}
	m.State = TaskValidatingOutputs
	SaveManifest(dir, m)
	if err := RunResume([]string{dir, "--env-file", env, "--auto-clean-chunks=true"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(dir)
	if err != nil || loaded.CacheCleanup == nil || loaded.CacheCleanup.Status != "cleaned" {
		t.Fatalf("automatic resume cleanup failed: %v", err)
	}
	if _, err := os.Stat(m.Source.Path); err != nil {
		t.Fatal("automatic cleanup removed source")
	}
}

func TestStorageCLIJSONContracts(t *testing.T) {
	root, _, _ := cacheFixture(t)
	var out bytes.Buffer
	if err := runStorageCommand("preview-cache-batch", []string{"--output-root", root}, bytes.NewReader(nil), &out); err != nil {
		t.Fatal(err)
	}
	var preview BatchCacheResult
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil || len(preview.Tasks) != 1 || !preview.Tasks[0].Preview.CanClean {
		t.Fatalf("preview %s %v", out.String(), err)
	}
	request := BatchCacheRequest{OutputRoot: preview.OutputRoot, Tasks: []BatchCacheSelection{{Folder: preview.Tasks[0].Folder, Token: preview.Tasks[0].Preview.Token}}}
	body, _ := json.Marshal(request)
	out.Reset()
	if err := runStorageCommand("clean-cache-batch", []string{"--output-root", root}, bytes.NewReader(body), &out); err != nil {
		t.Fatal(err)
	}
	var result BatchCacheResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.CleanedCount != 1 {
		t.Fatalf("cleanup %s %v", out.String(), err)
	}
	if err := runStorageCommand("clean-cache-batch", []string{"--output-root", root}, bytes.NewBufferString(`{"unknown":true}`), &out); err == nil {
		t.Fatal("accepted unknown cleanup fields")
	}
}
