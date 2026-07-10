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
	nowFunc        = time.Now
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
	return nowFunc().Format("20060102_150405")
}

// CreateTaskDir creates a task directory with the standard layout:
//
//	<outputDir>/<timestamp>_<safeSlug>/
//	    audio/
//	    chunks/
//	    transcripts/
func CreateTaskDir(outputDir, name string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	baseName := fmt.Sprintf("%s_%s", Timestamp(), SafeSlug(name))
	var taskDir string
	for suffix := 1; suffix <= 1000; suffix++ {
		dirName := baseName
		if suffix > 1 {
			dirName = fmt.Sprintf("%s_%d", baseName, suffix)
		}
		candidate := filepath.Join(outputDir, dirName)
		err := os.Mkdir(candidate, 0700)
		if err == nil {
			taskDir = candidate
			break
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("creating task dir: %w", err)
		}
	}
	if taskDir == "" {
		return "", fmt.Errorf("could not allocate a unique task folder under %s", outputDir)
	}

	for _, sub := range []string{"audio", "chunks", "transcripts"} {
		if err := os.Mkdir(filepath.Join(taskDir, sub), 0700); err != nil {
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

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
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
