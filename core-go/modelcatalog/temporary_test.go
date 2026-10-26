package modelcatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTemporaryCleanupOnlyExpiredManagedEntries(t *testing.T) {
	root := t.TempDir()
	paths, _ := NewPaths(root)
	if err := paths.prepare(); err != nil {
		t.Fatal(err)
	}
	catalog, _ := Load()
	id := catalog.Models[0].ModelID
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	stale := filepath.Join(paths.Downloads, id+"."+strings.Repeat("a", 32)+".partial")
	recent := filepath.Join(paths.Downloads, id+"."+strings.Repeat("b", 32)+".partial")
	unknown := filepath.Join(paths.Downloads, "user-file.partial")
	for _, p := range []string{stale, recent, unknown} {
		os.WriteFile(p, []byte("download"), 0600)
	}
	os.Chtimes(stale, old, old)
	os.Chtimes(unknown, old, old)
	stage := filepath.Join(paths.Staging, id+"."+strings.Repeat("c", 32))
	os.Mkdir(stage, 0700)
	child := filepath.Join(stage, "model.bin")
	os.WriteFile(child, []byte("staged"), 0600)
	os.Chtimes(child, old, old)
	os.Chtimes(stage, old, old)
	installed := filepath.Join(paths.Models, "keep.onnx")
	os.WriteFile(installed, []byte("installed"), 0600)
	os.Chtimes(installed, old, old)
	r, err := CleanupTemporary(root, now, 7*24*time.Hour)
	if err != nil || r.RemovedCount != 2 || r.RemovedBytes != 14 {
		t.Fatalf("cleanup %+v %v", r, err)
	}
	for _, p := range []string{recent, unknown, installed} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("retained file removed: %s", p)
		}
	}
	for _, p := range []string{stale, stage} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("stale entry remains: %s", p)
		}
	}
}

func TestTemporaryCleanupSkipsLiveModelAndSymlinks(t *testing.T) {
	root := t.TempDir()
	paths, _ := NewPaths(root)
	paths.prepare()
	catalog, _ := Load()
	id := catalog.Models[0].ModelID
	now := time.Now()
	old := now.Add(-9 * 24 * time.Hour)
	stale := filepath.Join(paths.Downloads, id+"."+strings.Repeat("a", 32)+".partial")
	os.WriteFile(stale, []byte("download"), 0600)
	os.Chtimes(stale, old, old)
	lock, err := acquireModelLock(paths, id)
	if err != nil {
		t.Fatal(err)
	}
	r, err := CleanupTemporary(root, now, 7*24*time.Hour)
	lock.release()
	if err != nil || r.RemovedCount != 0 {
		t.Fatalf("active model cleanup %+v %v", r, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatal("active download removed")
	}
	stage := filepath.Join(paths.Staging, id+"."+strings.Repeat("b", 32))
	os.Mkdir(stage, 0700)
	outside := filepath.Join(t.TempDir(), "keep")
	os.WriteFile(outside, []byte("keep"), 0600)
	if err := os.Symlink(outside, filepath.Join(stage, "linked")); err != nil {
		t.Skip(err)
	}
	os.Chtimes(stage, old, old)
	r, err = CleanupTemporary(root, now, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); err != nil {
		t.Fatal("symlink-containing staging was removed")
	}
	if b, _ := os.ReadFile(outside); string(b) != "keep" {
		t.Fatal("external file changed")
	}
}

func TestTemporaryCleanupDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if r, err := CleanupTemporary(root, time.Now(), 7*24*time.Hour); err != nil || r.RemovedCount != 0 {
		t.Fatalf("missing root %+v %v", r, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("maintenance created storage")
	}
	if _, err := CleanupTemporary(root, time.Now(), time.Hour); err == nil {
		t.Fatal("unsafe retention accepted")
	}
}
