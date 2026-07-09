package main

import (
	"os"
	"path/filepath"
	"testing"
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
