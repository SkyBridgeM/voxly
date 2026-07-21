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
	return SplitAudioWithCapabilities(
		inputPath,
		chunksDir,
		chunkMinutes,
		maxUploadBytes,
		MimoCapabilities(defaultMimoModel),
	)
}

// SplitAudioWithCapabilities executes the deterministic provider-aware plan.
// The planner owns chunk duration, overlap, concurrency, and retry durations;
// this function only realizes that plan with ffmpeg and validates produced
// chunk sizes.
func SplitAudioWithCapabilities(inputPath, chunksDir string, chunkMinutes int, maxUploadBytes int64, capabilities ASRProviderCapabilities) ([]string, error) {
	if err := ensureFFmpeg(); err != nil {
		return nil, err
	}
	if err := ValidateAudioFormat(inputPath); err != nil {
		return nil, err
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("audio file not found: %s", inputPath)
	}

	inputFormat := strings.TrimPrefix(strings.ToLower(filepath.Ext(inputPath)), ".")
	chunkFormat := chunkAudioFormat(inputFormat, capabilities.PreferredChunkFormat)
	requestedChunkDurationMS, err := chunkMinutesToMilliseconds(chunkMinutes)
	if err != nil {
		return nil, err
	}
	plan, err := PlanChunks(ChunkPlannerInput{
		Audio: AudioMetadata{
			SizeBytes: info.Size(),
			Format:    inputFormat,
		},
		Capabilities:             capabilities,
		Strategy:                 ChunkStrategyProviderAuto,
		RequestedChunkDurationMS: requestedChunkDurationMS,
		MaximumChunkBytes:        maxUploadBytes,
		RequestedConcurrency:     1,
	})
	if err != nil {
		return nil, fmt.Errorf("planning audio chunks: %w", err)
	}
	if plan.OverlapMS != 0 || plan.EffectiveConcurrency != 1 {
		return nil, fmt.Errorf("current Mimo splitter cannot execute overlap or concurrent chunk plans")
	}

	var attempted []int
	for _, chunkDurationMS := range plan.RetryChunkDurationsMS {
		if chunkDurationMS%1000 != 0 {
			return nil, fmt.Errorf("planned chunk duration must use whole seconds")
		}
		chunkSeconds := int(chunkDurationMS / 1000)
		attempted = append(attempted, chunkSeconds)
		if err := clearChunks(chunksDir, chunkFormat); err != nil {
			return nil, err
		}
		if err := splitOnce(inputPath, chunksDir, inputFormat, chunkFormat, chunkSeconds); err != nil {
			return nil, err
		}

		chunks, err := findChunks(chunksDir, chunkFormat)
		if err != nil {
			return nil, err
		}

		oversize := filterOversize(chunks, plan.MaximumChunkBytes)
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

func chunkMinutesToMilliseconds(minutes int) (int64, error) {
	if minutes < 0 {
		return 0, fmt.Errorf("chunk minutes cannot be negative")
	}
	const millisecondsPerMinute = int64(60 * 1000)
	const maxInt64 = int64(^uint64(0) >> 1)
	if int64(minutes) > maxInt64/millisecondsPerMinute {
		return 0, fmt.Errorf("chunk duration overflows milliseconds")
	}
	return int64(minutes) * millisecondsPerMinute, nil
}

func chunkAudioFormat(inputFormat, preferred string) string {
	if preferred == "wav" || inputFormat == "m4a" {
		return "wav"
	}
	return inputFormat
}

func clearChunks(chunksDir, format string) error {
	if err := os.MkdirAll(chunksDir, 0700); err != nil {
		return fmt.Errorf("creating chunks directory: %w", err)
	}
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return fmt.Errorf("reading chunks directory for cleanup: %w", err)
	}
	for _, e := range entries {
		isChunkFile := strings.HasPrefix(e.Name(), "part_") || strings.HasPrefix(e.Name(), ".renaming_")
		if !e.IsDir() && isChunkFile && strings.HasSuffix(e.Name(), "."+format) {
			path := filepath.Join(chunksDir, e.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("removing stale chunk %s: %w", path, err)
			}
		}
	}
	return nil
}

func splitOnce(inputPath, chunksDir, inputFormat, chunkFmt string, chunkSeconds int) error {
	outputPattern := filepath.Join(chunksDir, fmt.Sprintf("part_%%03d.%s", chunkFmt))

	var args []string
	if chunkFmt == "wav" {
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
			return renumberChunks(chunksDir, chunkFmt)
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
	return renumberChunks(chunksDir, chunkFmt)
}

// renumberChunks renames ffmpeg's 0-based part files to 1-based.
func renumberChunks(chunksDir, format string) error {
	entries, err := filepath.Glob(filepath.Join(chunksDir, fmt.Sprintf("part_*.%s", format)))
	if err != nil {
		return fmt.Errorf("listing chunks for renumbering: %w", err)
	}
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
		if err := os.Rename(r.temp, r.target); err != nil {
			return fmt.Errorf("preparing chunk renumber %s: %w", r.temp, err)
		}
	}

	// Phase 2: collect all files and rename to 1-based.
	allFiles, err := filepath.Glob(filepath.Join(chunksDir, fmt.Sprintf("part_*.%s", format)))
	if err != nil {
		return fmt.Errorf("listing chunks after renumber preparation: %w", err)
	}
	// Also include temp files.
	tempFiles, err := filepath.Glob(filepath.Join(chunksDir, fmt.Sprintf(".renaming_*.%s", format)))
	if err != nil {
		return fmt.Errorf("listing temporary chunk names: %w", err)
	}
	allFiles = append(allFiles, tempFiles...)
	sort.Strings(allFiles)

	for i, path := range allFiles {
		target := filepath.Join(chunksDir, fmt.Sprintf("part_%03d.%s", i+1, format))
		if path != target {
			if err := os.Rename(path, target); err != nil {
				return fmt.Errorf("renumbering chunk %s: %w", path, err)
			}
		}
	}
	return nil
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
		return fmt.Errorf("ffmpeg was not found on PATH. Install it:\n" +
			"  macOS:   brew install ffmpeg\n" +
			"  Ubuntu:  sudo apt install ffmpeg\n" +
			"  Windows: choco install ffmpeg  (or scoop install ffmpeg)\n" +
			"  Docs:    https://ffmpeg.org/download.html")
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
