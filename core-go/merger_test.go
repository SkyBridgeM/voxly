package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeTranscripts(t *testing.T) {
	dir := t.TempDir()
	transcriptsDir := filepath.Join(dir, "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write part files out of order to test sorting.
	os.WriteFile(filepath.Join(transcriptsDir, "part_002.txt"), []byte("Second part text\n"), 0644)
	os.WriteFile(filepath.Join(transcriptsDir, "part_001.txt"), []byte("First part text\n"), 0644)
	os.WriteFile(filepath.Join(transcriptsDir, "part_003.txt"), []byte("Third part text\n"), 0644)

	outputPath := filepath.Join(dir, "transcript.md")
	if err := MergeTranscripts(transcriptsDir, outputPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	got := string(data)

	// Check structure.
	if !strings.Contains(got, "[Part 001]") {
		t.Error("missing [Part 001]")
	}
	if !strings.Contains(got, "[Part 002]") {
		t.Error("missing [Part 002]")
	}
	if !strings.Contains(got, "[Part 003]") {
		t.Error("missing [Part 003]")
	}

	// Check order: Part 001 before Part 002.
	idx1 := strings.Index(got, "[Part 001]")
	idx2 := strings.Index(got, "[Part 002]")
	idx3 := strings.Index(got, "[Part 003]")
	if idx1 >= idx2 || idx2 >= idx3 {
		t.Errorf("parts not in order: idx1=%d, idx2=%d, idx3=%d", idx1, idx2, idx3)
	}

	// Check content.
	if !strings.Contains(got, "First part text") {
		t.Error("missing first part text")
	}
	if !strings.Contains(got, "Second part text") {
		t.Error("missing second part text")
	}

	// Should end with newline.
	if !strings.HasSuffix(got, "\n") {
		t.Error("output should end with newline")
	}
}

func TestMergeTranscriptsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	transcriptsDir := filepath.Join(dir, "empty")
	os.MkdirAll(transcriptsDir, 0755)

	err := MergeTranscripts(transcriptsDir, filepath.Join(dir, "out.md"))
	if err == nil {
		t.Fatal("expected error for empty transcripts dir")
	}
	if !strings.Contains(err.Error(), "no valid transcript part files") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeTranscriptsNoPartsDir(t *testing.T) {
	err := MergeTranscripts("/nonexistent/dir", "/tmp/out.md")
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}
