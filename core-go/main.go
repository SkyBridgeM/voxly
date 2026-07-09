package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const version = "0.1.0-spike"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "transcribe":
		if err := runTranscribe(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("meeting-asr-go %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `meeting-asr-go %s — experimental Go CLI for meeting-asr

Usage:
  meeting-asr-go transcribe <audio_path> [flags]

Flags:
  --output-mode transcript   Output mode (only "transcript" supported in spike)
  --output-dir output        Base output directory
  --chunk-minutes 5          Chunk length in minutes
  --max-upload-mb 10         Max upload size in MB
  --env-file path            Explicit path to .env file (default: search upward from cwd)
  --project-root path        Explicit project root for .env lookup (default: cwd)

`, version)
}

// reorderArgs moves all flags (and their values) before positional arguments.
// This allows flags to appear before or after the audio path:
//
//	"input.mp3 --chunk-minutes 3" → "--chunk-minutes 3 input.mp3"
//	"--chunk-minutes 3 input.mp3" → "--chunk-minutes 3 input.mp3" (no-op)
func reorderArgs(args []string) []string {
	var flags []string
	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// If this flag takes a value (not a boolean flag), consume the next arg.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	result := make([]string, 0, len(flags)+len(positional))
	result = append(result, flags...)
	result = append(result, positional...)
	return result
}

func runTranscribe(args []string) error {
	fs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
	outputMode := fs.String("output-mode", "transcript", "Output mode")
	outputDir := fs.String("output-dir", "output", "Base output directory")
	chunkMinutes := fs.Int("chunk-minutes", 0, "Chunk length in minutes (0 = use .env or default)")
	maxUploadMB := fs.Float64("max-upload-mb", 0, "Max upload size in MB (0 = use .env or default)")
	envFile := fs.String("env-file", "", "Explicit path to .env file")
	projectRoot := fs.String("project-root", "", "Explicit project root for .env lookup")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	if *outputMode != "transcript" {
		return fmt.Errorf("only --output-mode transcript is supported in this spike, got %q", *outputMode)
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: meeting-asr-go transcribe <audio_path>")
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("expected exactly one audio path, got %d: %v", fs.NArg(), fs.Args())
	}

	audioPath := fs.Arg(0)

	// Resolve audio path.
	absAudio, err := filepath.Abs(audioPath)
	if err != nil {
		return fmt.Errorf("resolving audio path: %w", err)
	}
	if _, err := os.Stat(absAudio); err != nil {
		return fmt.Errorf("audio file not found: %s", absAudio)
	}
	if err := ValidateAudioFormat(absAudio); err != nil {
		return err
	}

	// Load .env: explicit --env-file takes priority, then --project-root, then cwd search.
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

	// Override from flags.
	if *chunkMinutes > 0 {
		settings.DefaultChunkMin = *chunkMinutes
	}
	if *maxUploadMB > 0 {
		settings.MaxUploadMB = *maxUploadMB
	}

	// Validate API key.
	if _, err := settings.RequireMimoAPIKey(); err != nil {
		return err
	}

	// Resolve output directory.
	absOutput, err := filepath.Abs(*outputDir)
	if err != nil {
		return fmt.Errorf("resolving output dir: %w", err)
	}

	// Run the workflow.
	return runWorkflow(absAudio, absOutput, settings)
}

func runWorkflow(audioPath, outputDir string, settings *Settings) error {
	// 1. Create task directory.
	audioStem := filepath.Base(audioPath)
	audioStem = audioStem[:len(audioStem)-len(filepath.Ext(audioStem))]

	taskDir, err := CreateTaskDir(outputDir, audioStem)
	if err != nil {
		return fmt.Errorf("creating task dir: %w", err)
	}

	// 2. Copy source audio to task/audio/.
	sourceCopy := filepath.Join(taskDir, "audio", filepath.Base(audioPath))
	if err := CopyFile(audioPath, sourceCopy); err != nil {
		return fmt.Errorf("copying audio: %w", err)
	}

	// 3. Split audio.
	fmt.Println("Status: Splitting audio")
	chunks, err := SplitAudio(sourceCopy, filepath.Join(taskDir, "chunks"), settings.DefaultChunkMin, settings.MaxUploadBytes())
	if err != nil {
		return fmt.Errorf("splitting audio: %w", err)
	}

	// 4. Transcribe chunks.
	fmt.Printf("Status: Transcribing %d chunk(s)\n", len(chunks))
	transcriptsDir := filepath.Join(taskDir, "transcripts")
	progressFn := func(action string, index, total int) {
		if action == "transcribing" || action == "completed" {
			fmt.Printf("Progress: %d/%d\n", index, total)
		}
	}
	if _, err := TranscribeChunks(chunks, transcriptsDir, settings, progressFn); err != nil {
		return fmt.Errorf("transcribing: %w", err)
	}

	// 5. Merge transcripts.
	fmt.Println("Status: Merging transcript")
	transcriptPath := filepath.Join(taskDir, "transcript.md")
	if err := MergeTranscripts(transcriptsDir, transcriptPath); err != nil {
		return fmt.Errorf("merging transcripts: %w", err)
	}

	// 6. Done.
	fmt.Println("Status: Done")
	fmt.Printf("Transcript saved: %s\n", transcriptPath)
	fmt.Printf("Task folder: %s\n", taskDir)

	return nil
}
