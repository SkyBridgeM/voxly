package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var chunkNameRE = regexp.MustCompile(`^part_(\d{3})\.(mp3|wav)$`)

// RunResume handles the resume command.
func RunResume(args []string) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	outputMode := fs.String("output-mode", "", "Output mode (transcript, notes, minutes)")
	templateID := fs.String("template-id", "structured-notes", "Built-in generation template")
	temporaryPromptStdin := fs.Bool("temporary-prompt-stdin", false, "Read one-time generation instructions from stdin")
	envFile := fs.String("env-file", "", "Explicit path to .env file")
	projectRoot := fs.String("project-root", "", "Explicit project root for .env lookup")
	promptPath := fs.String("prompt-path", "", "Path to prompt template")
	obsidianTemplatePath := fs.String("obsidian-template-path", "", "Path to Obsidian template")
	asrProvider := fs.String("asr-provider", string(ASRProviderMimo), "Speech-recognition provider")
	asrModel := fs.String("asr-model", string(ASRModelMimoV25), "Speech-recognition model")
	llmProvider := fs.String("llm-provider", string(LLMProviderDeepSeek), "Generated-content provider")
	llmModel := fs.String("llm-model", string(LLMModelDeepSeekV4), "Generated-content model")
	templatesFile := fs.String("templates-file", "", "Application-owned custom template storage")
	appDataRoot := fs.String("app-data-root", "", "Voxly app data root for managed local models")

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
	absTaskDir, err := filepath.Abs(taskDirArg)
	if err != nil {
		return fmt.Errorf("resolving task directory: %w", err)
	}
	if _, err := os.Stat(absTaskDir); err != nil {
		return fmt.Errorf("task directory not found: %s", absTaskDir)
	}

	// 1. Acquire exclusive task lock.
	lock, err := AcquireTaskLock(absTaskDir)
	if err != nil {
		return fmt.Errorf("acquiring task lock: %w", err)
	}
	defer lock.Release()

	// 2. Load and validate manifest.
	m, err := LoadManifest(absTaskDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("loading manifest: %w", err)
		}

		legacy, legacyErr := LoadLegacyManifest(absTaskDir)
		if legacyErr != nil {
			if errors.Is(legacyErr, os.ErrNotExist) {
				return fmt.Errorf("no manifest found in %s — cannot resume without a task_manifest.json", absTaskDir)
			}
			return fmt.Errorf("loading legacy manifest: %w", legacyErr)
		}
		m, err = migrateLegacyManifest(absTaskDir, legacy)
		if err != nil {
			return fmt.Errorf("migrating legacy manifest: %w", err)
		}
	}

	if err := ValidateManifest(absTaskDir, m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	// Inherit output mode from manifest unless explicitly overridden.
	if outputModeExplicit && *outputMode != "" {
		if *temporaryPromptStdin {
			if *outputMode != "transcript_and_generate" {
				return fmt.Errorf("temporary prompt requires --output-mode transcript_and_generate")
			}
			prompt, err := readTemporaryPrompt(os.Stdin)
			if err != nil {
				return err
			}
			m.OutputMode = "transcript_and_generate"
			m.RequestedTemplateID = ""
			m.RequestedPrompt = prompt
		} else {
			mode, template, err := normalizeProcessingSelection(*outputMode, *templateID)
			if err != nil {
				return err
			}
			m.OutputMode, m.RequestedTemplateID = mode, template
			m.RequestedPrompt = ""
		}
	} else if *temporaryPromptStdin {
		return fmt.Errorf("temporary prompt requires an explicit --output-mode transcript_and_generate")
	} else if m.OutputMode == "" {
		m.OutputMode = "transcript"
	}
	*outputMode = m.OutputMode

	// Load settings.
	settings, err := loadResumeSettings(*envFile, *projectRoot)
	if err != nil {
		return err
	}
	if *promptPath != "" {
		settings.MinutesPromptPath = resolveCLIPath(*promptPath)
	}
	if *obsidianTemplatePath != "" {
		settings.ObsidianTemplatePath = resolveCLIPath(*obsidianTemplatePath)
	}
	settings.ASRProvider, settings.MimoModel = *asrProvider, *asrModel
	settings.LLMProvider, settings.DeepSeekModel = *llmProvider, *llmModel
	settings.TemplateStorePath = *templatesFile
	settings.AppDataRoot = *appDataRoot
	m.Provider = settings.ASRProvider
	m.Chunks.ASRProvider = settings.ASRProvider
	m.Chunks.ASRModel = settings.MimoModel
	if m.SourceKind == "microphone_recording" && m.Recording != nil && m.Recording.FinalAudioReference != "" && (m.Recording.Status == "cancelled" || m.Recording.Status == "interrupted") {
		m.Recording.Status = "recovered"
	}
	if err := ValidateASRProviderConfig(settings.ASRConfig()); err != nil {
		return err
	}
	if m.OutputMode == "transcript_and_generate" {
		if err := ValidateLLMProviderConfig(settings.LLMConfig()); err != nil {
			return err
		}
	}

	// API key validation is deferred to the stages that need it:
	// - executeTranscribing validates MIMO_API_KEY
	// - executeMinutesGenerating validates DEEPSEEK_API_KEY
	// This allows local-only stages (merge, notes) and no-op resume
	// to proceed without API keys.

	// 3. Reconcile manifest and filesystem.
	ReconcileChunks(absTaskDir, m)
	ReconcileArtifacts(absTaskDir, m)
	if err := PrepareArtifactsForResume(m); err != nil {
		return fmt.Errorf("preparing artifacts for resume: %w", err)
	}

	// Update chunk counts from reconciled state.
	m.Chunks.Total = len(m.Chunks.Items)

	// Persist reconciled state.
	if err := SaveManifest(absTaskDir, m); err != nil {
		return fmt.Errorf("saving reconciled manifest: %w", err)
	}

	// 4. Choose earliest incomplete stage.
	stage, complete := ResumeStage(m)
	if complete {
		fmt.Println("Status: All outputs are valid — nothing to resume")
		printResumeResult(absTaskDir, m)
		return nil
	}

	// 5. Execute stages idempotently.
	return executeResume(absTaskDir, m, settings, *outputMode, stage)
}

// executeResume runs the resume pipeline starting from the given stage.
func executeResume(taskDir string, m *Manifest, settings *Settings, outputMode string, startStage TaskState) error {
	stage := startStage
	for {
		var err error
		switch stage {
		case TaskSplitting:
			err = executeSplitting(taskDir, m, settings)
		case TaskTranscribing:
			err = executeTranscribing(taskDir, m, settings)
		case TaskMerging:
			err = executeMerging(taskDir, m)
		case TaskNotesGenerating:
			err = executeNotesGenerating(taskDir, m, settings)
		case TaskMinutesGenerating:
			err = executeMinutesGenerating(taskDir, m, settings)
		case TaskContentGenerating:
			err = executeContentGenerating(taskDir, m, settings)
		case TaskValidatingOutputs:
			err = executeValidatingOutputs(taskDir, m)
		default:
			return fmt.Errorf("unexpected resume stage: %s", stage)
		}

		if err != nil {
			if stage == TaskContentGenerating {
				m.State = TaskCompleted
				_ = SaveManifest(taskDir, m)
				return fmt.Errorf("%s failed: %w", stage, err)
			}
			_ = RecordTaskError(taskDir, m, stage, err, IsRetryableError(err))
			return fmt.Errorf("%s failed: %w", stage, err)
		}

		// Advance to next stage.
		next, complete := ResumeStage(m)
		if complete {
			if err := MarkTaskCompleted(taskDir, m); err != nil {
				return fmt.Errorf("marking task completed: %w", err)
			}
			fmt.Println("Status: Done")
			printResumeResult(taskDir, m)
			return nil
		}
		if next == stage {
			return fmt.Errorf("stage %s did not progress — possible idempotency issue", stage)
		}
		stage = next
	}
}

func executeContentGenerating(taskDir string, m *Manifest, settings *Settings) error {
	if err := ValidateLLMProviderConfig(settings.LLMConfig()); err != nil {
		return err
	}
	if _, err := settings.RequireDeepSeekAPIKey(); err != nil {
		return err
	}
	if err := SetTaskState(taskDir, m, TaskContentGenerating); err != nil {
		return err
	}
	fmt.Printf("LLM provider=%s model=%s\n", settings.LLMProvider, settings.DeepSeekModel)
	fmt.Println("Status: Generating content")
	generator, err := ResolveLLMContentGenerator(settings.LLMConfig())
	if err != nil {
		return err
	}
	return ResumeOrGenerateContent(taskDir, m, settings, generator)
}

// executeSplitting re-runs splitting if chunks are missing or invalid.
func executeSplitting(taskDir string, m *Manifest, settings *Settings) error {
	if err := SetTaskState(taskDir, m, TaskSplitting); err != nil {
		return err
	}
	fmt.Println("Status: Splitting audio")

	sourceAudio := m.Source.Path
	if !isCompleteFile(sourceAudio) {
		return fmt.Errorf("source audio not found: %s", sourceAudio)
	}

	// Clean stale transcript parts before re-splitting.
	// Old part_*.txt files may not correspond to the new chunk set.
	if err := cleanTranscriptParts(taskDir); err != nil {
		return err
	}

	// Invalidate all downstream artifacts since chunks are being rebuilt.
	m.Artifacts.Transcript.State = ArtifactPending
	m.Artifacts.Transcript.Error = nil
	m.Artifacts.NotesInput.State = ArtifactPending
	m.Artifacts.NotesInput.Error = nil
	m.Artifacts.Minutes.State = ArtifactPending
	m.Artifacts.Minutes.Error = nil
	for _, path := range []string{m.Artifacts.Transcript.Path, m.Artifacts.NotesInput.Path, m.Artifacts.Minutes.Path} {
		if err := removeArtifactFile(path); err != nil {
			return err
		}
	}

	chunksDir := filepath.Join(taskDir, "chunks")
	capabilities, err := ResolveASRCapabilities(settings.ASRConfig())
	if err != nil {
		return err
	}
	chunks, err := SplitAudioWithCapabilities(
		sourceAudio,
		chunksDir,
		m.Chunks.ChunkMinutes,
		int64(m.Chunks.MaxUploadMB*1024*1024),
		capabilities,
	)
	if err != nil {
		return err
	}

	// Rebuild chunk items.
	items := make([]ChunkItem, len(chunks))
	for i, chunkPath := range chunks {
		id := strings.TrimSuffix(filepath.Base(chunkPath), filepath.Ext(chunkPath))
		info, _ := os.Stat(chunkPath)
		var sizeBytes int64
		if info != nil {
			sizeBytes = info.Size()
		}
		audioHash, err := fileSHA256(chunkPath)
		if err != nil {
			return fmt.Errorf("hashing audio chunk %s: %w", chunkPath, err)
		}
		items[i] = ChunkItem{
			ID:             id,
			AudioPath:      chunkPath,
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          ChunkPending,
			SizeBytes:      sizeBytes,
			AudioSHA256:    audioHash,
		}
	}

	format := ""
	if len(chunks) > 0 {
		format = strings.TrimPrefix(filepath.Ext(chunks[0]), ".")
	}

	m.Chunks.Format = format
	m.Chunks.Total = len(items)
	m.Chunks.Items = items
	return SaveManifest(taskDir, m)
}

// cleanTranscriptParts removes all part_*.txt files from the transcripts directory.
func cleanTranscriptParts(taskDir string) error {
	transcriptsDir := filepath.Join(taskDir, "transcripts")
	entries, err := os.ReadDir(transcriptsDir)
	if err != nil {
		return fmt.Errorf("reading transcripts directory for cleanup: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "part_") && strings.HasSuffix(e.Name(), ".txt") {
			path := filepath.Join(transcriptsDir, e.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("removing stale transcript %s: %w", path, err)
			}
		}
	}
	return nil
}

// removeArtifactFile removes an artifact file if it exists.
func removeArtifactFile(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale artifact %s: %w", path, err)
	}
	return nil
}

// executeTranscribing transcribes only the PENDING chunks.
func executeTranscribing(taskDir string, m *Manifest, settings *Settings) error {
	if err := SetTaskState(taskDir, m, TaskTranscribing); err != nil {
		return err
	}

	transcriptsDir := filepath.Join(taskDir, "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0700); err != nil {
		return err
	}

	// Collect pending chunk paths.
	var pendingPaths []string
	for i := range m.Chunks.Items {
		if m.Chunks.Items[i].State == ChunkPending {
			pendingPaths = append(pendingPaths, m.Chunks.Items[i].AudioPath)
		}
	}

	if len(pendingPaths) == 0 {
		fmt.Println("Status: All chunks already transcribed")
		return nil
	}

	fmt.Printf("Status: Transcribing %d chunk(s)\n", len(pendingPaths))

	apiKey := ""
	if settings.ASRConfig().ProviderID == ASRProviderMimo {
		var err error
		apiKey, err = settings.RequireMimoAPIKey()
		if err != nil {
			return err
		}
	}
	provider, err := ResolveASRProvider(settings.ASRConfig(), settings, apiKey)
	if err != nil {
		return err
	}
	if settings.ASRConfig().ProviderID == ASRProviderLocalSenseVoice && m.ASRRuntime == nil {
		_, snapshot, err := prepareLocalSenseVoice(settings)
		if err != nil {
			return err
		}
		m.ASRRuntime = snapshot
	}

	for _, chunkPath := range pendingPaths {
		chunkID := strings.TrimSuffix(filepath.Base(chunkPath), filepath.Ext(chunkPath))
		chunk := m.ChunkByID(chunkID)
		if chunk == nil {
			continue
		}

		// Mark IN_PROGRESS.
		chunk.State = ChunkInProgress
		chunk.Attempts++
		if err := SaveManifest(taskDir, m); err != nil {
			return fmt.Errorf("saving manifest before transcription: %w", err)
		}

		fmt.Printf("Transcribing %s...\n", filepath.Base(chunkPath))
		result, err := provider.Transcribe(context.Background(), ASRRequest{AudioPath: chunkPath, Language: "auto"})
		if err != nil {
			chunk.State = ChunkFailed
			msg := err.Error()
			chunk.LastError = &msg
			_ = SaveManifest(taskDir, m)
			return fmt.Errorf("transcribing %s: %w", filepath.Base(chunkPath), err)
		}

		// Write transcript atomically.
		transcriptPath := filepath.Join(transcriptsDir, chunkID+".txt")
		if err := WriteFileAtomically(transcriptPath, []byte(strings.TrimSpace(result.Text)+"\n"), 0600); err != nil {
			chunk.State = ChunkFailed
			msg := err.Error()
			chunk.LastError = &msg
			_ = SaveManifest(taskDir, m)
			return fmt.Errorf("writing transcript for %s: %w", chunkID, err)
		}

		chunk.State = ChunkCompleted
		chunk.TranscriptPath = transcriptPath
		transcriptHash, err := fileSHA256(transcriptPath)
		if err != nil {
			chunk.State = ChunkFailed
			msg := err.Error()
			chunk.LastError = &msg
			_ = SaveManifest(taskDir, m)
			return fmt.Errorf("hashing transcript for %s: %w", chunkID, err)
		}
		chunk.TranscriptSHA256 = transcriptHash
		chunk.LastError = nil
		if err := SaveManifest(taskDir, m); err != nil {
			return fmt.Errorf("saving manifest after transcription: %w", err)
		}
	}

	return nil
}

// executeMerging merges all chunk transcripts into transcript.md.
func executeMerging(taskDir string, m *Manifest) error {
	if err := SetTaskState(taskDir, m, TaskMerging); err != nil {
		return err
	}
	markArtifactProcessing(&m.Artifacts.Transcript)
	if err := SaveManifest(taskDir, m); err != nil {
		return err
	}
	fmt.Println("Status: Merging transcript")

	transcriptsDir := filepath.Join(taskDir, "transcripts")
	transcriptPath := filepath.Join(taskDir, "transcript.md")

	if err := MergeTranscripts(transcriptsDir, transcriptPath); err != nil {
		return err
	}

	if err := markArtifactReady(transcriptPath, &m.Artifacts.Transcript); err != nil {
		return err
	}
	return SaveManifest(taskDir, m)
}

// executeNotesGenerating creates notes_input.md from prompt + transcript.
func executeNotesGenerating(taskDir string, m *Manifest, settings *Settings) error {
	if err := SetTaskState(taskDir, m, TaskNotesGenerating); err != nil {
		return err
	}
	markArtifactProcessing(&m.Artifacts.NotesInput)
	if err := SaveManifest(taskDir, m); err != nil {
		return err
	}
	fmt.Println("Status: Preparing notes input")

	promptPath := settings.ResolvePath(settings.MinutesPromptPath)
	transcriptPath := m.Artifacts.Transcript.Path
	notesPath := filepath.Join(taskDir, "notes_input.md")

	if err := CreateNotesInput(promptPath, transcriptPath, notesPath); err != nil {
		return err
	}

	if err := markArtifactReady(notesPath, &m.Artifacts.NotesInput); err != nil {
		return err
	}
	return SaveManifest(taskDir, m)
}

// executeMinutesGenerating generates meeting_minutes.md via LLM + Obsidian template.
func executeMinutesGenerating(taskDir string, m *Manifest, settings *Settings) error {
	if err := SetTaskState(taskDir, m, TaskMinutesGenerating); err != nil {
		return err
	}
	markArtifactProcessing(&m.Artifacts.Minutes)
	if err := SaveManifest(taskDir, m); err != nil {
		return err
	}
	fmt.Println("Status: Generating meeting minutes")

	// Validate DeepSeek key before attempting generation.
	if _, err := settings.RequireDeepSeekAPIKey(); err != nil {
		return err
	}

	notesPath := m.Artifacts.NotesInput.Path
	if notesPath == "" || !isCompleteFile(notesPath) {
		return fmt.Errorf("notes_input.md not found — cannot generate minutes")
	}

	rawMinutes, err := GenerateMinutes(notesPath, settings)
	if err != nil {
		return err
	}

	templatePath := settings.ResolvePath(settings.ObsidianTemplatePath)
	title := ExtractTitle(filepath.Base(taskDir))
	taskName := filepath.Base(taskDir)

	transcriptData, err := os.ReadFile(m.Artifacts.Transcript.Path)
	if err != nil {
		return fmt.Errorf("reading transcript for template: %w", err)
	}

	wrapped, err := ApplyObsidianTemplate(templatePath, rawMinutes, title, taskName, string(transcriptData))
	if err != nil {
		return err
	}

	minutesPath := filepath.Join(taskDir, "meeting_minutes.md")
	if err := WriteFileAtomically(minutesPath, []byte(wrapped), 0600); err != nil {
		return err
	}

	if err := markArtifactReady(minutesPath, &m.Artifacts.Minutes); err != nil {
		return err
	}
	return SaveManifest(taskDir, m)
}

// executeValidatingOutputs verifies all required artifacts are present and valid.
func executeValidatingOutputs(taskDir string, m *Manifest) error {
	if err := SetTaskState(taskDir, m, TaskValidatingOutputs); err != nil {
		return err
	}

	ReconcileArtifacts(taskDir, m)

	// Check that all required artifacts are ready.
	if m.Artifacts.Transcript.State != ArtifactReady {
		return fmt.Errorf("transcript.md is not ready")
	}
	if (m.OutputMode == "notes" || m.OutputMode == "minutes") && m.Artifacts.NotesInput.State != ArtifactReady {
		return fmt.Errorf("notes_input.md is not ready")
	}
	if m.OutputMode == "minutes" && m.Artifacts.Minutes.State != ArtifactReady {
		return fmt.Errorf("meeting_minutes.md is not ready")
	}

	m.IntegrityVersion = 1
	return SaveManifest(taskDir, m)
}

// printResumeResult prints the final output paths.
func printResumeResult(taskDir string, m *Manifest) {
	if m.Artifacts.Transcript.Path != "" {
		fmt.Printf("Transcript saved: %s\n", m.Artifacts.Transcript.Path)
	}
	if m.Artifacts.NotesInput.Path != "" && m.Artifacts.NotesInput.State == ArtifactReady {
		fmt.Printf("Notes input saved: %s\n", m.Artifacts.NotesInput.Path)
	}
	if m.Artifacts.Minutes.Path != "" && m.Artifacts.Minutes.State == ArtifactReady {
		fmt.Printf("Meeting minutes saved: %s\n", m.Artifacts.Minutes.Path)
	}
	fmt.Printf("Task folder: %s\n", taskDir)
}

// loadResumeSettings loads settings from .env file.
func loadResumeSettings(envFile, projectRoot string) (*Settings, error) {
	settings := DefaultSettings()
	if envFile != "" {
		absEnv, err := filepath.Abs(envFile)
		if err != nil {
			return nil, fmt.Errorf("resolving env-file path: %w", err)
		}
		loaded, err := LoadSettings(absEnv)
		if err != nil {
			return nil, fmt.Errorf("loading .env: %w", err)
		}
		settings = loaded
	} else {
		searchDir := ""
		if projectRoot != "" {
			absRoot, err := filepath.Abs(projectRoot)
			if err != nil {
				return nil, fmt.Errorf("resolving project-root: %w", err)
			}
			searchDir = absRoot
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getting cwd: %w", err)
			}
			searchDir = cwd
		}
		envPath, ok := FindEnvFile(searchDir)
		if ok {
			loaded, err := LoadSettings(envPath)
			if err != nil {
				return nil, fmt.Errorf("loading .env: %w", err)
			}
			settings = loaded
		} else {
			fmt.Fprintf(os.Stderr, "Warning: no .env file found, using defaults\n")
		}
	}
	return settings, nil
}

// migrateManifest converts a legacy manifest (schema_version 0) to the current
// schema by building chunk items and artifact state from disk. The old flat JSON
// fields are not directly accessible in the new struct, so we rebuild from what's
// available on disk.
func migrateManifest(taskDir string, old *Manifest) (*Manifest, error) {
	now := nowFunc().Format(time.RFC3339)

	m := &Manifest{
		SchemaVersion:    currentSchemaVersion,
		IntegrityVersion: old.IntegrityVersion,
		TaskID:           filepath.Base(taskDir),
		OutputMode:       old.OutputMode,
		Backend:          "go",
		Provider:         old.Chunks.ASRProvider,
		Status:           "created",
		State:            TaskCreated,
		Attempt:          1,
		Source:           old.Source,
		Chunks:           old.Chunks,
		Artifacts: Artifacts{
			Transcript: ArtifactInfo{Path: filepath.Join(taskDir, "transcript.md"), State: ArtifactPending},
			NotesInput: ArtifactInfo{Path: filepath.Join(taskDir, "notes_input.md"), State: ArtifactPending},
			Minutes:    ArtifactInfo{Path: filepath.Join(taskDir, "meeting_minutes.md"), State: ArtifactPending},
		},
		CreatedAt: old.CreatedAt,
		UpdatedAt: now,
	}
	if m.Provider == "" {
		m.Provider = "unknown"
	}
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	if m.OutputMode == "" {
		m.OutputMode = "transcript"
	}

	// Try to discover source audio from the audio/ subdirectory only when the
	// old manifest did not retain it.
	audioDir := filepath.Join(taskDir, "audio")
	if m.Source.Path == "" {
		entries, err := os.ReadDir(audioDir)
		if err == nil && len(entries) > 0 {
			for _, e := range entries {
				if !e.IsDir() {
					m.Source.Path = filepath.Join(audioDir, e.Name())
					m.Source.OriginalName = e.Name()
					info, _ := os.Stat(m.Source.Path)
					if info != nil {
						m.Source.SizeBytes = info.Size()
					}
					break
				}
			}
		}
	}

	// Try to discover chunks from disk.
	items, format, err := BuildChunkItemsFromDisk(taskDir)
	if err == nil {
		m.Chunks.Format = format
		m.Chunks.Total = len(items)
		m.Chunks.Items = items
	}

	// Migrate old error info.
	if old.LastError != nil {
		m.LastError = &ErrorInfo{
			Stage:     TaskFailed,
			Message:   old.LastError.Message,
			Retryable: false,
			At:        now,
		}
	}

	// Save the migrated manifest.
	if err := SaveManifest(taskDir, m); err != nil {
		return nil, fmt.Errorf("saving migrated manifest: %w", err)
	}

	return m, nil
}

// migrateLegacyManifest converts the old flat manifest.json format into the
// current schema before handing it to the common filesystem reconciliation.
func migrateLegacyManifest(taskDir string, old *legacyManifest) (*Manifest, error) {
	m := &Manifest{
		SchemaVersion: "",
		TaskID:        filepath.Base(taskDir),
		OutputMode:    old.OutputMode,
		Source: SourceInfo{
			OriginalName: filepath.Base(old.SourceAudio),
		},
		State: TaskCreated,
		Chunks: ChunksInfo{
			ChunkMinutes: old.ChunkMinutes,
			MaxUploadMB:  old.MaxUploadMB,
			ASRProvider:  old.ASRProvider,
			ASRModel:     old.ASRModel,
		},
		CreatedAt: old.CreatedAt,
		UpdatedAt: old.UpdatedAt,
	}
	if old.Error != nil {
		m.LastError = &ErrorInfo{Stage: TaskFailed, Message: *old.Error}
	}
	// Old manifests point at the caller's original source path, which can lie
	// outside the task directory. The workflow already copied the source into
	// task/audio/, so let migrateManifest discover that durable task-local copy.
	return migrateManifest(taskDir, m)
}
