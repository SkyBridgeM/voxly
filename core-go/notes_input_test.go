package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateNotesInputSnapshot(t *testing.T) {
	dir := t.TempDir()

	promptPath := filepath.Join(dir, "prompt.md")
	transcriptPath := filepath.Join(dir, "transcript.md")
	outputPath := filepath.Join(dir, "notes_input.md")

	os.WriteFile(promptPath, []byte("Analyze this meeting.\n"), 0644)
	os.WriteFile(transcriptPath, []byte("[Part 001]\n\nHello world.\n"), 0644)

	if err := CreateNotesInput(promptPath, transcriptPath, outputPath); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	// Exact snapshot — must match Python create_notes_input() format.
	want := "Analyze this meeting.\n\n---\n\n## Transcript\n\n[Part 001]\n\nHello world.\n"
	if string(got) != want {
		t.Errorf("notes_input.md snapshot mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestCreateNotesInputTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()

	promptPath := filepath.Join(dir, "prompt.md")
	transcriptPath := filepath.Join(dir, "transcript.md")
	outputPath := filepath.Join(dir, "notes_input.md")

	os.WriteFile(promptPath, []byte("  prompt text  \n\n"), 0644)
	os.WriteFile(transcriptPath, []byte("\n\n  transcript text  \n"), 0644)

	if err := CreateNotesInput(promptPath, transcriptPath, outputPath); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(got), "prompt text") {
		t.Error("prompt text should be present")
	}
	if !strings.Contains(string(got), "transcript text") {
		t.Error("transcript text should be present")
	}
	if strings.HasPrefix(string(got), "  ") {
		t.Error("should not have leading whitespace")
	}
}

func TestCreateNotesInputMissingPrompt(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "transcript.md")
	os.WriteFile(transcriptPath, []byte("text"), 0644)

	err := CreateNotesInput("/nonexistent/prompt.md", transcriptPath, filepath.Join(dir, "out.md"))
	if err == nil {
		t.Fatal("expected error for missing prompt file")
	}
	if !strings.Contains(err.Error(), "prompt file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateNotesInputMissingTranscript(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	os.WriteFile(promptPath, []byte("text"), 0644)

	err := CreateNotesInput(promptPath, "/nonexistent/transcript.md", filepath.Join(dir, "out.md"))
	if err == nil {
		t.Fatal("expected error for missing transcript file")
	}
	if !strings.Contains(err.Error(), "transcript file") {
		t.Errorf("unexpected error: %v", err)
	}
}
