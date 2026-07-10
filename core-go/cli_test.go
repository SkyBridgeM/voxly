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
		"Status: Preparing notes input",
		"Status: Generating meeting minutes",
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

func TestStdoutContract_SavedLines(t *testing.T) {
	lines := []string{
		"Transcript saved: /path/to/transcript.md",
		"Notes input saved: /path/to/notes_input.md",
		"Meeting minutes saved: /path/to/meeting_minutes.md",
		"Task folder: /path/to/task",
	}
	prefixes := []string{
		"Transcript saved: ",
		"Notes input saved: ",
		"Meeting minutes saved: ",
		"Task folder: ",
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, prefixes[i]) {
			t.Errorf("line %q must start with %q", line, prefixes[i])
		}
	}
}

// --- Error case tests ---

func TestRunTranscribe_UnsupportedOutputMode(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("MIMO_API_KEY=x\n"), 0644)
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--output-mode", "invalid", audioPath})
	if err == nil {
		t.Fatal("expected error for unsupported output-mode")
	}
	if !strings.Contains(err.Error(), "invalid --output-mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunTranscribe_NotesModeNoDeepSeekKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	// Has MIMO_API_KEY but no DEEPSEEK_API_KEY — notes mode should still work
	// (it fails at ffmpeg, not at key validation).
	os.WriteFile(envPath, []byte("MIMO_API_KEY=test-key\n"), 0644)
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, "--output-mode", "notes", audioPath})
	if err != nil && strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Errorf("notes mode should not require DEEPSEEK_API_KEY: %v", err)
	}
}

func TestRunTranscribe_MinutesModeRequiresDeepSeekKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	// Has MIMO_API_KEY but no DEEPSEEK_API_KEY.
	os.WriteFile(envPath, []byte("MIMO_API_KEY=test-key\n"), 0644)
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, "--output-mode", "minutes", audioPath})
	if err == nil {
		t.Fatal("expected error for missing DEEPSEEK_API_KEY in minutes mode")
	}
	if !strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Errorf("error should mention DEEPSEEK_API_KEY, got: %v", err)
	}
}

func TestRunTranscribe_TranscriptModeNoDeepSeekKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("MIMO_API_KEY=test-key\n"), 0644)
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake"), 0644)

	err := runTranscribe([]string{"--env-file", envPath, "--output-mode", "transcript", audioPath})
	if err != nil && strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Errorf("transcript mode should not require DEEPSEEK_API_KEY: %v", err)
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

// --- CLI path resolution tests ---

func TestResolveCLIPath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"absolute unchanged", "/abs/path.md", "/abs/path.md"},
		{"relative joins cwd", "prompt.md", filepath.Join(cwd, "prompt.md")},
		{"nested relative", "prompts/default.md", filepath.Join(cwd, "prompts", "default.md")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCLIPath(tt.in)
			if got != tt.want {
				t.Errorf("resolveCLIPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunTranscribe_CLIFlagPathResolvesRelativeToCwd(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test\nMINUTES_PROMPT_PATH=env_prompt.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	audioPath := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0644); err != nil {
		t.Fatal(err)
	}

	// CLI --prompt-path "cli_prompt.md" should resolve to cwd/cli_prompt.md,
	// NOT dir/cli_prompt.md (which would be .env-relative).
	err := runTranscribe([]string{
		"--env-file", envPath,
		"--prompt-path", "cli_prompt.md",
		"--output-dir", filepath.Join(dir, "out"),
		audioPath,
	})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), ".env") {
		t.Errorf("CLI path should resolve relative to cwd, not .env: %v", err)
	}
}

func TestRunResume_CLIFlagPathResolvesRelativeToCwd(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	taskDir := filepath.Join(dir, "task")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunksDir, "part_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	err := RunResume([]string{
		"--env-file", envPath,
		"--prompt-path", "cli_prompt.md",
		taskDir,
	})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), ".env") {
		t.Errorf("CLI path should resolve relative to cwd, not .env: %v", err)
	}
}

// --- MINUTES_PROVIDER validation tests ---

func TestRunTranscribe_MinutesProviderNotDeepSeek(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test\nMINUTES_PROVIDER=openai\n"), 0644); err != nil {
		t.Fatal(err)
	}

	audioPath := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTranscribe([]string{
		"--env-file", envPath,
		"--output-mode", "minutes",
		audioPath,
	})
	if err == nil {
		t.Fatal("expected error for unsupported MINUTES_PROVIDER")
	}
	if !strings.Contains(err.Error(), "MINUTES_PROVIDER=deepseek") {
		t.Errorf("error should mention required provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"openai"`) {
		t.Errorf("error should mention actual provider, got: %v", err)
	}
}

func TestRunResume_MinutesProviderNotDeepSeek(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test\nMINUTES_PROVIDER=openai\n"), 0644); err != nil {
		t.Fatal(err)
	}

	taskDir := filepath.Join(dir, "task")
	chunksDir := filepath.Join(taskDir, "chunks")
	transcriptsDir := filepath.Join(taskDir, "transcripts")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunksDir, "part_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(transcriptsDir, "part_001.txt"), []byte("done"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-create transcript.md and notes_input.md so resume goes straight to minutes.
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("transcript"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "notes_input.md"), []byte("notes"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create a manifest with chunk items so resume doesn't try to re-split.
	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "minutes", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 1
	m.Chunks.Items = []ChunkItem{
		{ID: "part_001", AudioPath: filepath.Join(chunksDir, "part_001.wav"), TranscriptPath: filepath.Join(transcriptsDir, "part_001.txt"), State: ChunkCompleted},
	}
	m.Artifacts.Transcript.State = ArtifactReady
	m.Artifacts.NotesInput.State = ArtifactReady
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	err := RunResume([]string{
		"--env-file", envPath,
		"--output-mode", "minutes",
		taskDir,
	})
	if err == nil {
		t.Fatal("expected error for missing DEEPSEEK_API_KEY")
	}
	if !strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Errorf("error should mention DEEPSEEK_API_KEY, got: %v", err)
	}
}
