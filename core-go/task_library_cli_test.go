package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yudi/meet-asr/core-go/tasklibrary"
)

func taskLibraryFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "docs", "fixtures", "task_manifest", name)
}

func copyTaskLibraryFixture(t *testing.T, root, folder, fixture string) {
	t.Helper()
	data, err := os.ReadFile(taskLibraryFixturePath(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, folder)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tasklibrary.ManifestFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunListTasksWritesMachineReadableResultWithIssues(t *testing.T) {
	root := t.TempDir()
	copyTaskLibraryFixture(t, root, "valid", "task_manifest_completed.json")
	invalid := filepath.Join(root, "invalid")
	if err := os.Mkdir(invalid, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalid, tasklibrary.ManifestFilename), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runListTasks([]string{"--output-root", root}, &stdout); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&stdout)
	var result tasklibrary.DiscoveryResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout contains data after the JSON result: %v", err)
	}
	if len(result.Tasks) != 1 || len(result.Issues) != 1 {
		t.Fatalf("result = %#v, want one task and one issue", result)
	}
	if result.Issues[0].Code != tasklibrary.IssueInvalidManifest {
		t.Fatalf("issue = %#v", result.Issues[0])
	}
}

func TestRunListTasksMissingRootIsTopLevelFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	var stdout bytes.Buffer
	if err := runListTasks([]string{"--output-root", missing}, &stdout); err == nil {
		t.Fatal("expected missing root error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on top-level failure", stdout.String())
	}
}

func TestRunListTasksUnreadableRootIsTopLevelFailure(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(rootFile, []byte("not a task root"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := runListTasks([]string{"--output-root", rootFile}, &stdout); err == nil {
		t.Fatal("expected unreadable root error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on top-level failure", stdout.String())
	}
}

func TestRunListTasksRequiresApplicationRoot(t *testing.T) {
	for _, args := range [][]string{nil, {"--output-root", "relative"}} {
		var stdout bytes.Buffer
		if err := runListTasks(args, &stdout); err == nil {
			t.Fatalf("runListTasks(%v) succeeded", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q, want empty", stdout.String())
		}
	}
}
