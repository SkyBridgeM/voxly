package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSafeSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"meeting_notes", "meeting_notes"},
		{"2024 Q4 planning", "2024_Q4_planning"},
		{"file.mp3", "file"},
		{"../etc/passwd", "etc_passwd"},
		{"a/b\\c", "a_b_c"},
		{"", "meeting"},
		{"...", "meeting"},
		{"---test---", "test"},
		{"hello@world#123", "hello_world_123"},
		{"my.file.name.wav", "my.file.name"},
		{"会议记录", "meeting"},
		{"会议_record", "record"},
		{"café résumé", "caf_r_sum"},
		{"file (copy)", "file_copy"},
		{"a", "a"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		got := SafeSlug(tt.input)
		if got != tt.want {
			t.Errorf("SafeSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCreateTaskDir(t *testing.T) {
	outputDir := t.TempDir()
	taskDir, err := CreateTaskDir(outputDir, "test_meeting.mp3")
	if err != nil {
		t.Fatal(err)
	}

	// Directory should exist.
	info, err := os.Stat(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("task dir is not a directory")
	}

	// Subdirectories should exist.
	for _, sub := range []string{"audio", "chunks", "transcripts"} {
		subPath := filepath.Join(taskDir, sub)
		info, err := os.Stat(subPath)
		if err != nil {
			t.Errorf("subdir %s: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("subdir %s is not a directory", sub)
		}
	}

	// Name should contain timestamp and slug.
	base := filepath.Base(taskDir)
	if len(base) < 15 {
		t.Errorf("task dir name too short: %q", base)
	}
}

func TestCreateTaskDirDoesNotReuseExistingDirectory(t *testing.T) {
	originalNow := nowFunc
	nowFunc = func() time.Time {
		return time.Date(2026, 7, 10, 12, 0, 0, 0, time.Local)
	}
	defer func() { nowFunc = originalNow }()

	outputDir := t.TempDir()
	first, err := CreateTaskDir(outputDir, "test_meeting.mp3")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(first, "transcripts", "part_001.txt")
	if err := os.WriteFile(marker, []byte("existing transcript"), 0644); err != nil {
		t.Fatal(err)
	}

	second, err := CreateTaskDir(outputDir, "test_meeting.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected a unique task directory, got the same path")
	}
	if filepath.Base(first) != "20260710_120000_test_meeting" {
		t.Fatalf("unexpected first task directory: %s", first)
	}
	if filepath.Base(second) != "20260710_120000_test_meeting_2" {
		t.Fatalf("unexpected second task directory: %s", second)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("existing task content was changed: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "sub", "dest.txt")

	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestCreateTaskDirPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	outputDir := t.TempDir()
	taskDir, err := CreateTaskDir(outputDir, "test_meeting.mp3")
	if err != nil {
		t.Fatal(err)
	}

	// Task directory should be 0700 (owner-only).
	info, err := os.Stat(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("task dir permissions = %o, want 0700", perm)
	}

	// Subdirectories should be 0700.
	for _, sub := range []string{"audio", "chunks", "transcripts"} {
		subPath := filepath.Join(taskDir, sub)
		info, err := os.Stat(subPath)
		if err != nil {
			t.Errorf("subdir %s: %v", sub, err)
			continue
		}
		perm := info.Mode().Perm()
		if perm != 0700 {
			t.Errorf("subdir %s permissions = %o, want 0700", sub, perm)
		}
	}
}

func TestCopyFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "sub", "dest.txt")

	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("copied file permissions = %o, want 0600", perm)
	}
}

func TestAudioFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"test.mp3", "mp3"},
		{"test.wav", "wav"},
		{"test.m4a", "wav"},
		{"test.flac", ""},
		{"TEST.MP3", "mp3"},
	}
	for _, tt := range tests {
		got := AudioFormat(tt.path)
		if got != tt.want {
			t.Errorf("AudioFormat(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestValidateAudioFormat(t *testing.T) {
	if err := ValidateAudioFormat("test.mp3"); err != nil {
		t.Errorf("mp3 should be valid: %v", err)
	}
	if err := ValidateAudioFormat("test.m4a"); err != nil {
		t.Errorf("m4a should be valid: %v", err)
	}
	if err := ValidateAudioFormat("test.flac"); err == nil {
		t.Error("flac should be invalid")
	}
}
