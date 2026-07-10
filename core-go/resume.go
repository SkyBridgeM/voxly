package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var chunkNameRE = regexp.MustCompile(`^part_(\d{3})\.(mp3|wav)$`)

// ChunkInfo holds information about a discovered chunk file.
type ChunkInfo struct {
	Number        int
	Path          string
	Format        string
	HasTranscript bool
}

// DiscoverChunks scans a task directory for chunks and validates them.
func DiscoverChunks(taskDir string) ([]ChunkInfo, error) {
	chunksDir := filepath.Join(taskDir, "chunks")
	transcriptsDir := filepath.Join(taskDir, "transcripts")

	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, fmt.Errorf("reading chunks directory: %w", err)
	}

	var chunks []ChunkInfo
	formats := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if this is a part_* file with invalid naming.
		if strings.HasPrefix(entry.Name(), "part_") {
			match := chunkNameRE.FindStringSubmatch(entry.Name())
			if match == nil {
				return nil, fmt.Errorf("invalid chunk filename %q: expected part_NNN.wav or part_NNN.mp3 (strict 3-digit numbering)", entry.Name())
			}

			num, err := strconv.Atoi(match[1])
			if err != nil {
				return nil, fmt.Errorf("invalid chunk number in %q: %w", entry.Name(), err)
			}

			format := match[2]
			formats[format] = true

			// Check if transcript exists and is non-empty.
			transcriptPath := filepath.Join(transcriptsDir, fmt.Sprintf("part_%03d.txt", num))
			hasTranscript := isCompleteFile(transcriptPath)

			chunks = append(chunks, ChunkInfo{
				Number:        num,
				Path:          filepath.Join(chunksDir, entry.Name()),
				Format:        format,
				HasTranscript: hasTranscript,
			})
		}
		// Skip non-part files (like .DS_Store, etc.)
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("no valid chunk files found in %s", chunksDir)
	}

	// Check for mixed formats.
	if len(formats) > 1 {
		formatList := make([]string, 0, len(formats))
		for f := range formats {
			formatList = append(formatList, f)
		}
		sort.Strings(formatList)
		return nil, fmt.Errorf("mixed chunk formats found: %s. Use only one format", strings.Join(formatList, ", "))
	}

	// Sort by number.
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Number < chunks[j].Number
	})

	// Validate contiguous numbering starting at 1.
	for i, chunk := range chunks {
		expected := i + 1
		if chunk.Number != expected {
			return nil, fmt.Errorf("chunk numbering must be contiguous starting at 1: expected part_%03d, got part_%03d", expected, chunk.Number)
		}
	}

	return chunks, nil
}

// RunResume handles the resume command.
func RunResume(args []string) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	outputMode := fs.String("output-mode", "transcript", "Output mode")
	envFile := fs.String("env-file", "", "Explicit path to .env file")
	projectRoot := fs.String("project-root", "", "Explicit project root for .env lookup")
	promptPath := fs.String("prompt-path", "", "Path to prompt template")
	obsidianTemplatePath := fs.String("obsidian-template-path", "", "Path to Obsidian template")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		return err
	}
	outputModeExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "output-mode" {
			outputModeExplicit = true
		}
	})

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: meeting-asr-go resume <task_dir>")
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("expected exactly one task directory, got %d: %v", fs.NArg(), fs.Args())
	}

	taskDirArg := fs.Arg(0)

	// Resolve task directory.
	absTaskDir, err := filepath.Abs(taskDirArg)
	if err != nil {
		return fmt.Errorf("resolving task directory: %w", err)
	}

	// Validate task directory exists.
	if _, err := os.Stat(absTaskDir); err != nil {
		return fmt.Errorf("task directory not found: %s", absTaskDir)
	}

	// Resume inherits the original output mode unless the caller explicitly overrides it.
	var existingManifest *Manifest
	existingManifest, err = LoadManifest(absTaskDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("loading task manifest: %w", err)
		}
		existingManifest = nil
	} else if !outputModeExplicit && existingManifest.OutputMode != "" {
		*outputMode = existingManifest.OutputMode
	}

	if *outputMode != "transcript" && *outputMode != "notes" && *outputMode != "minutes" {
		return fmt.Errorf("invalid --output-mode %q. Supported: transcript, notes, minutes", *outputMode)
	}

	// Load settings.
	settings := DefaultSettings()
	if *envFile != "" {
		absEnv, err := filepath.Abs(*envFile)
		if err != nil {
			return fmt.Errorf("resolving env-file path: %w", err)
		}
		loaded, err := LoadSettings(absEnv)
		if err != nil {
			return fmt.Errorf("loading .env: %w", err)
		}
		settings = loaded
	} else {
		searchDir := ""
		if *projectRoot != "" {
			absRoot, err := filepath.Abs(*projectRoot)
			if err != nil {
				return fmt.Errorf("resolving project-root: %w", err)
			}
			searchDir = absRoot
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting cwd: %w", err)
			}
			searchDir = cwd
		}

		envPath, ok := FindEnvFile(searchDir)
		if ok {
			loaded, err := LoadSettings(envPath)
			if err != nil {
				return fmt.Errorf("loading .env: %w", err)
			}
			settings = loaded
		} else {
			fmt.Fprintf(os.Stderr, "Warning: no .env file found, using defaults\n")
		}
	}

	// Override settings from CLI flags.
	if *promptPath != "" {
		settings.MinutesPromptPath = resolveCLIPath(*promptPath)
	}
	if *obsidianTemplatePath != "" {
		settings.ObsidianTemplatePath = resolveCLIPath(*obsidianTemplatePath)
	}

	// Validate API key.
	if _, err := settings.RequireMimoAPIKey(); err != nil {
		return err
	}

	// Fail fast on unsupported minutes provider before expensive work.
	if *outputMode == "minutes" {
		if err := ValidateMinutesProvider(settings); err != nil {
			return err
		}
		if _, err := settings.RequireDeepSeekAPIKey(); err != nil {
			return err
		}
	}

	// Discover and validate chunks.
	chunks, err := DiscoverChunks(absTaskDir)
	if err != nil {
		return fmt.Errorf("discovering chunks: %w", err)
	}

	// Update the manifest before work so it reflects this resume run's effective settings.
	if err := PrepareManifestForResume(absTaskDir, *outputMode, settings.MimoModel, existingManifest); err != nil {
		return fmt.Errorf("preparing task manifest: %w", err)
	}

	// Filter chunks that need transcription.
	var chunksToTranscribe []string
	for _, chunk := range chunks {
		if !chunk.HasTranscript {
			chunksToTranscribe = append(chunksToTranscribe, chunk.Path)
		} else {
			fmt.Printf("Skipping existing transcript: part_%03d.txt\n", chunk.Number)
		}
	}

	// Transcribe missing chunks.
	if len(chunksToTranscribe) > 0 {
		fmt.Printf("Status: Transcribing %d chunk(s)\n", len(chunksToTranscribe))
		transcriptsDir := filepath.Join(absTaskDir, "transcripts")
		progressFn := func(action string, index, total int) {
			if action == "transcribing" || action == "completed" {
				fmt.Printf("Progress: %d/%d\n", index, total)
			}
		}
		if _, err := TranscribeChunks(chunksToTranscribe, transcriptsDir, settings, progressFn); err != nil {
			return failTask(absTaskDir, fmt.Errorf("transcribing: %w", err))
		}
	} else {
		fmt.Println("Status: All chunks already transcribed")
	}

	// Merge transcripts.
	fmt.Println("Status: Merging transcript")
	transcriptPath := filepath.Join(absTaskDir, "transcript.md")
	transcriptsDir := filepath.Join(absTaskDir, "transcripts")
	if err := MergeTranscripts(transcriptsDir, transcriptPath); err != nil {
		return failTask(absTaskDir, fmt.Errorf("merging transcripts: %w", err))
	}

	// Generate notes_input.md (notes and minutes modes).
	var notesInputPath string
	if *outputMode == "notes" || *outputMode == "minutes" {
		fmt.Println("Status: Preparing notes input")
		promptPathResolved := settings.ResolvePath(settings.MinutesPromptPath)
		notesInputPath = filepath.Join(absTaskDir, "notes_input.md")
		if err := CreateNotesInput(promptPathResolved, transcriptPath, notesInputPath); err != nil {
			return failTask(absTaskDir, fmt.Errorf("creating notes input: %w", err))
		}
	}

	// Generate meeting_minutes.md (minutes mode only).
	var minutesPath string
	if *outputMode == "minutes" {
		fmt.Println("Status: Generating meeting minutes")
		rawMinutes, err := GenerateMinutes(notesInputPath, settings)
		if err != nil {
			return failTask(absTaskDir, fmt.Errorf("generating meeting minutes: %w", err))
		}

		// Apply Obsidian template.
		templatePath := settings.ResolvePath(settings.ObsidianTemplatePath)
		title := ExtractTitle(filepath.Base(absTaskDir))
		taskName := filepath.Base(absTaskDir)

		transcriptData, err := os.ReadFile(transcriptPath)
		if err != nil {
			return failTask(absTaskDir, fmt.Errorf("reading transcript for template: %w", err))
		}

		wrapped, err := ApplyObsidianTemplate(templatePath, rawMinutes, title, taskName, string(transcriptData))
		if err != nil {
			return failTask(absTaskDir, fmt.Errorf("applying Obsidian template: %w", err))
		}

		minutesPath = filepath.Join(absTaskDir, "meeting_minutes.md")
		if err := os.WriteFile(minutesPath, []byte(wrapped), 0644); err != nil {
			return failTask(absTaskDir, fmt.Errorf("writing meeting minutes: %w", err))
		}
	}

	// Update manifest status to completed.
	if err := UpdateManifestStatus(absTaskDir, "completed", nil); err != nil {
		return fmt.Errorf("marking task completed: %w", err)
	}

	// Done.
	fmt.Println("Status: Done")
	fmt.Printf("Transcript saved: %s\n", transcriptPath)
	if notesInputPath != "" {
		fmt.Printf("Notes input saved: %s\n", notesInputPath)
	}
	if minutesPath != "" {
		fmt.Printf("Meeting minutes saved: %s\n", minutesPath)
	}
	fmt.Printf("Task folder: %s\n", absTaskDir)

	return nil
}
