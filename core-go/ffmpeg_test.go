package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFFmpegDiscoveryPrefersBundleAndFailsClosed(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "meeting-asr-go")
	if got := ffmpegBinaryAt(executable); got != "ffmpeg" {
		t.Fatalf("development fallback: %s", got)
	}
	bundled := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(bundled, []byte("runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := ffmpegBinaryAt(executable); got != bundled {
		t.Fatalf("sibling ignored: %s", got)
	}
	packaged := filepath.Join(dir, "Voxly.app", "Contents", "MacOS", "meeting-asr-go")
	if got := ffmpegBinaryAt(packaged); got != filepath.Join(filepath.Dir(packaged), "ffmpeg") {
		t.Fatalf("production fell back to PATH: %s", got)
	}
}
