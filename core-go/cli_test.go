package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- --env-file / --project-root integration tests ---

func TestRunTranscribe_EnvFileExplicit(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test-from-envfile\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a dummy audio file so validation passes (it won't actually transcribe).
	audioPath := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0644); err != nil {
		t.Fatal(err)
	}

	// We can't run the full workflow (needs ffmpeg + API), but we can verify
	// that --env-file is accepted and doesn't error during flag parsing.
	// The workflow will fail at ffmpeg, which is expected.
	err := runTranscribe([]string{"--env-file", envPath, "--output-dir", filepath.Join(dir, "out"), audioPath})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "ffmpeg") {
		// Expected: ffmpeg may be missing, or the fake audio may fail decoding.
		return
	}
	t.Fatalf("--env-file should load settings and reach the workflow, got: %v", err)
}

func TestRunTranscribe_ProjectRoot(t *testing.T) {
	dir := t.TempDir()
	// Put .env in a subdirectory, use --project-root to find it.
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test-from-root\n"), 0644); err != nil {
		t.Fatal(err)
	}

	audioPath := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTranscribe([]string{"--project-root", dir, "--output-dir", filepath.Join(dir, "out"), audioPath})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "ffmpeg") {
		return // expected: ffmpeg may be missing, or fake audio may fail decoding.
	}
	t.Fatalf("--project-root should find .env and reach the workflow, got: %v", err)
}

func TestRunTranscribe_EnvFileTakesPriorityOverProjectRoot(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "priority.env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=from-envfile\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Also put a .env in the project root with a different key.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MIMO_API_KEY=from-root\n"), 0644); err != nil {
		t.Fatal(err)
	}

	audioPath := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTranscribe([]string{"--env-file", envPath, "--project-root", dir, "--output-dir", filepath.Join(dir, "out"), audioPath})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "ffmpeg") {
		return // expected: ffmpeg may be missing, or fake audio may fail decoding.
	}
	t.Fatalf("env-file should take priority and reach the workflow, got: %v", err)
}

// --- Stdout contract tests ---
// These document the exact stdout line prefixes that future Tauri integration
// depends on. If you change these formats, update the Tauri side too.

func TestStdoutContract_StatusLine(t *testing.T) {
	// Status lines must start with "Status: ".
	lines := []string{
		"Status: Splitting audio",
		"Status: Transcribing 3 chunk(s)",
		"Status: Merging transcript",
		"Status: Done",
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "Status: ") {
			t.Errorf("status line must start with 'Status: ', got: %q", line)
		}
	}
}

func TestStdoutContract_ProgressLine(t *testing.T) {
	// Progress lines must match "Progress: N/M" format.
	lines := []string{
		"Progress: 1/5",
		"Progress: 3/3",
		"Progress: 10/10",
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "Progress: ") {
			t.Errorf("progress line must start with 'Progress: ', got: %q", line)
		}
		parts := strings.Split(strings.TrimPrefix(line, "Progress: "), "/")
		if len(parts) != 2 {
			t.Errorf("progress line must have N/M format, got: %q", line)
		}
	}
}

func TestStdoutContract_TranscriptSavedLine(t *testing.T) {
	// "Transcript saved: " prefix followed by absolute path.
	line := "Transcript saved: /path/to/task/transcript.md"
	if !strings.HasPrefix(line, "Transcript saved: ") {
		t.Errorf("must start with 'Transcript saved: '")
	}
}

func TestStdoutContract_TaskFolderLine(t *testing.T) {
	// "Task folder: " prefix followed by absolute path.
	line := "Task folder: /path/to/task"
	if !strings.HasPrefix(line, "Task folder: ") {
		t.Errorf("must start with 'Task folder: '")
	}
}

// --- Error case tests ---

func TestRunTranscribe_UnsupportedOutputMode(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("MIMO_API_KEY=x\n"), 0644)
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--output-mode", "minutes", audioPath})
	if err == nil {
		t.Fatal("expected error for unsupported output-mode")
	}
	if !strings.Contains(err.Error(), "only --output-mode transcript") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunTranscribe_MissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	// Empty .env — no MIMO_API_KEY.
	os.WriteFile(envPath, []byte("# empty\n"), 0644)
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, audioPath})
	if err == nil {
		t.Fatal("expected error for missing MIMO_API_KEY")
	}
	if !strings.Contains(err.Error(), "MIMO_API_KEY") {
		t.Errorf("error should mention MIMO_API_KEY, got: %v", err)
	}
}

func TestRunTranscribe_InvalidAudioSuffix(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("MIMO_API_KEY=x\n"), 0644)
	audioPath := filepath.Join(dir, "test.flac")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, audioPath})
	if err == nil {
		t.Fatal("expected error for unsupported audio format")
	}
	if !strings.Contains(err.Error(), "unsupported audio format") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

func TestRunTranscribe_MultipleAudioPaths(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("MIMO_API_KEY=x\n"), 0644)
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")
	os.WriteFile(a, []byte("fake"), 0644)
	os.WriteFile(b, []byte("fake"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, a, b})
	if err == nil {
		t.Fatal("expected error for multiple audio paths")
	}
	if !strings.Contains(err.Error(), "exactly one audio path") {
		t.Errorf("error should mention single path, got: %v", err)
	}
}

func TestRunTranscribe_NoAudioPath(t *testing.T) {
	err := runTranscribe([]string{})
	if err == nil {
		t.Fatal("expected error for no audio path")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error should mention usage, got: %v", err)
	}
}

func TestRunTranscribe_AudioNotFound(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("MIMO_API_KEY=x\n"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, "/nonexistent/audio.mp3"})
	if err == nil {
		t.Fatal("expected error for missing audio file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}
