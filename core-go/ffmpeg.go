package main

import (
	"os"
	"path/filepath"
)

// Packaged desktop processing must use its own FFmpeg, even if another version
// happens to be installed on PATH. Standalone development keeps PATH fallback.
func ffmpegBinaryAt(executable string) string {
	dir := filepath.Dir(executable)
	bundled := filepath.Join(dir, "ffmpeg")
	if info, err := os.Lstat(bundled); err == nil && info.Mode().IsRegular() {
		return bundled
	}
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		return bundled
	}
	return "ffmpeg"
}

func ffmpegBinary() string {
	executable, err := os.Executable()
	if err != nil {
		return "ffmpeg"
	}
	return ffmpegBinaryAt(executable)
}
