package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverChunks_ValidChunks(t *testing.T) {
	// Create a temporary task directory structure.
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")
	transcriptsDir := filepath.Join(taskDir, "transcripts")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some chunk files.
	for i := 1; i <= 3; i++ {
		chunkPath := filepath.Join(chunksDir, filepath.Base("part_"+pad(i)+".wav"))
		if err := os.WriteFile(chunkPath, []byte("audio data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	chunks, err := DiscoverChunks(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// Verify chunks are sorted by number.
	for i, chunk := range chunks {
		expected := i + 1
		if chunk.Number != expected {
			t.Errorf("chunk %d: expected number %d, got %d", i, expected, chunk.Number)
		}
	}
}

func TestDiscoverChunks_MixedFormats(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mixed format chunks.
	if err := os.WriteFile(filepath.Join(chunksDir, "part_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunksDir, "part_002.mp3"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := DiscoverChunks(taskDir)
	if err == nil {
		t.Fatal("expected error for mixed formats, got nil")
	}
}

func TestDiscoverChunks_NonContiguous(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create non-contiguous chunks (1, 3 - missing 2).
	if err := os.WriteFile(filepath.Join(chunksDir, "part_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunksDir, "part_003.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := DiscoverChunks(taskDir)
	if err == nil {
		t.Fatal("expected error for non-contiguous chunks, got nil")
	}
}

func TestDiscoverChunks_InvalidNames(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create invalid chunk names.
	if err := os.WriteFile(filepath.Join(chunksDir, "chunk_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := DiscoverChunks(taskDir)
	if err == nil {
		t.Fatal("expected error for invalid chunk names, got nil")
	}
}

func TestDiscoverChunks_StrictThreeDigitNumbering(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create chunk with non-strict numbering (part_1.wav instead of part_001.wav).
	if err := os.WriteFile(filepath.Join(chunksDir, "part_1.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := DiscoverChunks(taskDir)
	if err == nil {
		t.Fatal("expected error for non-strict 3-digit numbering, got nil")
	}
}

func TestDiscoverChunks_InvalidExtension(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create chunk with invalid extension.
	if err := os.WriteFile(filepath.Join(chunksDir, "part_001.txt"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := DiscoverChunks(taskDir)
	if err == nil {
		t.Fatal("expected error for invalid extension, got nil")
	}
}

func TestDiscoverChunks_SkipsCompletedTranscripts(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")
	transcriptsDir := filepath.Join(taskDir, "transcripts")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create chunks.
	for i := 1; i <= 3; i++ {
		chunkPath := filepath.Join(chunksDir, "part_"+pad(i)+".wav")
		if err := os.WriteFile(chunkPath, []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create transcript for chunk 1 and 3 (skip 2).
	if err := os.WriteFile(filepath.Join(transcriptsDir, "part_001.txt"), []byte("transcript"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(transcriptsDir, "part_003.txt"), []byte("transcript"), 0644); err != nil {
		t.Fatal(err)
	}

	chunks, err := DiscoverChunks(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify chunk 1 and 3 have transcripts, chunk 2 does not.
	for _, chunk := range chunks {
		switch chunk.Number {
		case 1, 3:
			if !chunk.HasTranscript {
				t.Errorf("chunk %d should have transcript", chunk.Number)
			}
		case 2:
			if chunk.HasTranscript {
				t.Errorf("chunk %d should not have transcript", chunk.Number)
			}
		}
	}
}

func TestRunResumeInheritsManifestOutputMode(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(dir, "task")
	if err := os.MkdirAll(filepath.Join(taskDir, "chunks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(taskDir, "transcripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "chunks", "part_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "transcripts", "part_001.txt"), []byte("done"), 0644); err != nil {
		t.Fatal(err)
	}
	m := CreateManifest(taskDir, "/audio.wav", "minutes", 5, 10, "mimo", "mimo-v2.5-asr")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	err := RunResume([]string{"--env-file", envPath, taskDir})
	if err == nil || !strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Fatalf("expected inherited minutes mode to require DeepSeek key, got %v", err)
	}
}

func TestRunResumeExplicitOutputModeUpdatesManifest(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=test-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(dir, "task")
	if err := os.MkdirAll(filepath.Join(taskDir, "chunks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(taskDir, "transcripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "chunks", "part_001.wav"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "transcripts", "part_001.txt"), []byte("done"), 0644); err != nil {
		t.Fatal(err)
	}
	m := CreateManifest(taskDir, "/audio.wav", "minutes", 5, 10, "mimo", "old-model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	if err := RunResume([]string{"--env-file", envPath, "--output-mode", "transcript", taskDir}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OutputMode != "transcript" || loaded.Status != "completed" {
		t.Fatalf("unexpected completed manifest: %+v", loaded)
	}
}

func pad(n int) string {
	if n < 10 {
		return "00" + string(rune('0'+n))
	}
	return "0" + string(rune('0'+n/10)) + string(rune('0'+n%10))
}
