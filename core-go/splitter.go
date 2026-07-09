package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CalculateRetryChunkSeconds returns a descending sequence of chunk durations
// to try when chunks exceed the upload limit. Halves each time, minimum 60s.
func CalculateRetryChunkSeconds(initialSeconds int) []int {
	if initialSeconds < minChunkSeconds {
		return []int{initialSeconds}
	}
	values := []int{initialSeconds}
	current := initialSeconds
	for current > minChunkSeconds {
		next := current / 2
		if next < minChunkSeconds {
			next = minChunkSeconds
		}
		if next == values[len(values)-1] {
			break
		}
		values = append(values, next)
		current = next
	}
	return values
}

// SplitAudio splits an audio file into chunks using ffmpeg.
// Returns the sorted list of chunk file paths.
func SplitAudio(inputPath, chunksDir string, chunkMinutes int, maxUploadBytes int64) ([]string, error) {
	if err := ensureFFmpeg(); err != nil {
		return nil, err
	}
	if err := ValidateAudioFormat(inputPath); err != nil {
		return nil, err
	}
	if _, err := os.Stat(inputPath); err != nil {
		return nil, fmt.Errorf("audio file not found: %s", inputPath)
	}

	inputFormat := strings.TrimPrefix(strings.ToLower(filepath.Ext(inputPath)), ".")
	chunkFormat := chunkAudioFormat(inputFormat)
	initialSeconds := chunkMinutes * 60
	if initialSeconds < minChunkSeconds {
		initialSeconds = minChunkSeconds
	}

	var attempted []int
	for _, chunkSeconds := range CalculateRetryChunkSeconds(initialSeconds) {
		attempted = append(attempted, chunkSeconds)
		clearChunks(chunksDir, chunkFormat)
		if err := splitOnce(inputPath, chunksDir, inputFormat, chunkSeconds); err != nil {
			return nil, err
		}

		chunks, err := findChunks(chunksDir, chunkFormat)
		if err != nil {
			return nil, err
		}

		oversize := filterOversize(chunks, maxUploadBytes)
		if len(oversize) == 0 {
			return chunks, nil
		}

		if chunkSeconds <= minChunkSeconds {
			details := make([]string, 0, min(len(oversize), 5))
			for _, c := range oversize[:min(len(oversize), 5)] {
				mb, _ := FileSizeMB(c)
				details = append(details, fmt.Sprintf("%s=%.2fMB", filepath.Base(c), mb))
			}
			return nil, fmt.Errorf(
				"one or more chunks exceed the upload limit even at the 60-second minimum: %s. "+
					"Increase --max-upload-mb or provide smaller/lower-bitrate audio",
				strings.Join(details, ", "),
			)
		}
	}

	return nil, fmt.Errorf("unable to split audio below upload limit. Attempted chunk lengths: %v", attempted)
}

func chunkAudioFormat(inputFormat string) string {
	if inputFormat == "m4a" {
		return "wav"
	}
	return inputFormat
}

func clearChunks(chunksDir, format string) {
	_ = os.MkdirAll(chunksDir, 0755)
	entries, _ := os.ReadDir(chunksDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "part_") && strings.HasSuffix(e.Name(), "."+format) {
			_ = os.Remove(filepath.Join(chunksDir, e.Name()))
		}
	}
}

func splitOnce(inputPath, chunksDir, inputFormat string, chunkSeconds int) error {
	chunkFmt := chunkAudioFormat(inputFormat)
	outputPattern := filepath.Join(chunksDir, fmt.Sprintf("part_%%03d.%s", chunkFmt))

	var args []string
	if inputFormat == "m4a" {
		args = []string{
			"-hide_banner", "-loglevel", "error", "-y",
			"-i", inputPath,
			"-f", "segment", "-segment_time", fmt.Sprint(chunkSeconds),
			"-reset_timestamps", "1",
			"-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le",
			outputPattern,
		}
	} else {
		// Try stream copy first.
		args = []string{
			"-hide_banner", "-loglevel", "error", "-y",
			"-i", inputPath,
			"-f", "segment", "-segment_time", fmt.Sprint(chunkSeconds),
			"-reset_timestamps", "1",
			"-c", "copy",
			outputPattern,
		}
		if err := runFFmpeg(args); err == nil {
			renumberChunks(chunksDir, chunkFmt)
			return nil
		}
		// Fallback: re-encode.
		args = []string{
			"-hide_banner", "-loglevel", "error", "-y",
			"-i", inputPath,
			"-f", "segment", "-segment_time", fmt.Sprint(chunkSeconds),
			"-reset_timestamps", "1",
			outputPattern,
		}
	}

	if err := runFFmpeg(args); err != nil {
		return err
	}
	renumberChunks(chunksDir, chunkFmt)
	return nil
}

// renumberChunks renames ffmpeg's 0-based part files to 1-based.
func renumberChunks(chunksDir, format string) {
	entries, _ := filepath.Glob(filepath.Join(chunksDir, fmt.Sprintf("part_*.%s", format)))
	sort.Strings(entries)

	type rename struct {
		temp   string
		target string
	}
	var renames []rename

	for i, path := range entries {
		base := filepath.Base(path)
		// Skip already-renamed 1-based files.
		expected := fmt.Sprintf("part_%03d.%s", i+1, format)
		if base == expected {
			continue
		}
		temp := filepath.Join(chunksDir, fmt.Sprintf(".renaming_%03d.%s", i+1, format))
		renames = append(renames, rename{temp: path, target: temp})
	}

	// Phase 1: rename originals to temp names.
	for _, r := range renames {
		_ = os.Rename(r.temp, r.target)
	}

	// Phase 2: collect all files and rename to 1-based.
	allFiles, _ := filepath.Glob(filepath.Join(chunksDir, fmt.Sprintf("part_*.%s", format)))
	// Also include temp files.
	tempFiles, _ := filepath.Glob(filepath.Join(chunksDir, fmt.Sprintf(".renaming_*.%s", format)))
	allFiles = append(allFiles, tempFiles...)
	sort.Strings(allFiles)

	for i, path := range allFiles {
		target := filepath.Join(chunksDir, fmt.Sprintf("part_%03d.%s", i+1, format))
		if path != target {
			_ = os.Rename(path, target)
		}
	}
}

func findChunks(chunksDir, format string) ([]string, error) {
	pattern := filepath.Join(chunksDir, fmt.Sprintf("part_*.%s", format))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("ffmpeg did not create any audio chunks")
	}
	return matches, nil
}

func filterOversize(chunks []string, maxBytes int64) []string {
	var oversize []string
	for _, c := range chunks {
		info, err := os.Stat(c)
		if err != nil {
			continue
		}
		if info.Size() > maxBytes {
			oversize = append(oversize, c)
		}
	}
	return oversize
}

func ensureFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg was not found. Install it with: brew install ffmpeg")
	}
	return nil
}

func runFFmpeg(args []string) error {
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %s\n%s", err, string(output))
	}
	return nil
}
