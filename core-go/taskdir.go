package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	slugStripExtRe = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)
	slugCleanRe    = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// SafeSlug converts a filename stem into a safe directory name component.
// Matches Python's safe_slug behavior.
func SafeSlug(value string) string {
	normalized := strings.ReplaceAll(value, "/", "_")
	normalized = strings.ReplaceAll(normalized, "\\", "_")
	stem := slugStripExtRe.ReplaceAllString(normalized, "")
	slug := slugCleanRe.ReplaceAllString(stem, "_")
	slug = strings.Trim(slug, "._-")
	if slug == "" {
		return "meeting"
	}
	return slug
}

// Timestamp returns the current local time formatted as YYYYMMDD_HHMMSS.
func Timestamp() string {
	return time.Now().Format("20060102_150405")
}

// CreateTaskDir creates a task directory with the standard layout:
//
//	<outputDir>/<timestamp>_<safeSlug>/
//	    audio/
//	    chunks/
//	    transcripts/
func CreateTaskDir(outputDir, name string) (string, error) {
	dirName := fmt.Sprintf("%s_%s", Timestamp(), SafeSlug(name))
	taskDir := filepath.Join(outputDir, dirName)

	for _, sub := range []string{"audio", "chunks", "transcripts"} {
		if err := os.MkdirAll(filepath.Join(taskDir, sub), 0755); err != nil {
			return "", fmt.Errorf("creating task subdir %s: %w", sub, err)
		}
	}

	return taskDir, nil
}

// CopyFile copies a file from src to dst, preserving modification time.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := in.Stat()
	if err == nil {
		_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	}

	return out.Close()
}

// AudioFormat returns the format string for the given audio file path.
// For .m4a, the output format is "wav" (converted before upload).
// For .mp3/.wav, the format is kept as-is.
func AudioFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".m4a":
		return "wav"
	case ".mp3":
		return "mp3"
	case ".wav":
		return "wav"
	default:
		return ""
	}
}

// ValidateAudioFormat checks if the file extension is a supported audio format.
func ValidateAudioFormat(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if !supportedAudioSuffixes[ext] {
		return fmt.Errorf("unsupported audio format %q. Supported formats: .m4a, .mp3, .wav", ext)
	}
	return nil
}

// FileSizeMB returns the file size in megabytes.
func FileSizeMB(path string) (float64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return float64(info.Size()) / (1024 * 1024), nil
}
