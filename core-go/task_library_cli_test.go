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

func TestTaskMetadataCLIReadAndUpdate(t *testing.T) {
	root := t.TempDir()
	copyTaskLibraryFixture(t, root, "valid", "task_manifest_completed.json")

	var readOutput bytes.Buffer
	if err := runGetTaskMetadata([]string{"--output-root", root, "--task-id", "task_fixture_completed_001"}, &readOutput); err != nil {
		t.Fatal(err)
	}
	var initial tasklibrary.TaskMetadata
	if err := json.Unmarshal(readOutput.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Title == "" || initial.Description != "" || len(initial.Tags) != 0 {
		t.Fatalf("initial metadata = %#v", initial)
	}

	input := bytes.NewBufferString(`{"title":"Project review","description":"Key decisions","tags":["项目","工作"]}`)
	var updateOutput bytes.Buffer
	if err := runUpdateTaskMetadata([]string{"--output-root", root, "--task-id", initial.TaskID}, input, &updateOutput); err != nil {
		t.Fatal(err)
	}
	var updated tasklibrary.TaskMetadata
	if err := json.Unmarshal(updateOutput.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Project review" || updated.Description != "Key decisions" || len(updated.Tags) != 2 {
		t.Fatalf("updated metadata = %#v", updated)
	}
}

func TestTaskMetadataCLIRejectsUnknownInputAndTraversal(t *testing.T) {
	root := t.TempDir()
	copyTaskLibraryFixture(t, root, "valid", "task_manifest_completed.json")
	var stdout bytes.Buffer
	if err := runUpdateTaskMetadata(
		[]string{"--output-root", root, "--task-id", "../valid"},
		bytes.NewBufferString(`{"title":"x","description":"","tags":[]}`),
		&stdout,
	); err == nil {
		t.Fatal("traversal task ID accepted")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", stdout.String())
	}
	if err := runUpdateTaskMetadata(
		[]string{"--output-root", root, "--task-id", "task_fixture_completed_001"},
		bytes.NewBufferString(`{"title":"x","description":"","tags":[],"secret":"no"}`),
		&stdout,
	); err == nil {
		t.Fatal("unknown metadata field accepted")
	}
}
