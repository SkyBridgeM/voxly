package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/modelcatalog"
	"github.com/yudi/meet-asr/core-go/tasklibrary"
	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const version = "0.2.0-minutes"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "transcribe":
		if err := runTranscribe(os.Args[2:]); err != nil {
			exitWithError(err)
		}
	case "resume":
		if err := RunResume(os.Args[2:]); err != nil {
			exitWithError(err)
		}
	case "list-tasks":
		if err := runListTasks(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "storage-summary", "preview-cache-batch", "clean-cache-batch", "cleanup-model-temporary":
		if err := runStorageCommand(os.Args[1], os.Args[2:], os.Stdin, os.Stdout); err != nil {
			exitWithError(err)
		}
	case "preview-task-cache", "clean-task-cache":
		if err := runTaskCache(os.Args[1], os.Args[2:], os.Stdout); err != nil {
			exitWithError(err)
		}
	case "get-task-metadata":
		if err := runGetTaskMetadata(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "update-task-metadata":
		if err := runUpdateTaskMetadata(os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "list-templates":
		if err := runListTemplates(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "create-custom-template":
		if err := runCreateCustomTemplate(os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "update-custom-template":
		if err := runUpdateCustomTemplate(os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "delete-custom-template":
		if err := runDeleteCustomTemplate(os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "list-providers":
		if err := json.NewEncoder(os.Stdout).Encode(BuiltInProviderCatalog()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "model-catalog", "model-status", "download-model", "import-model", "delete-model", "normalize-hf-endpoint", "test-hf-endpoint":
		if err := runModelCommand(os.Args[1], os.Args[2:], os.Stdout, os.Stderr); err != nil {
			encodeModelCommandError(os.Stderr, err)
			os.Exit(1)
		}
	case "generate-content":
		if err := runGenerateContent(os.Args[2:]); err != nil {
			exitWithError(err)
		}
	case "retry-generation":
		if err := runRetryGeneration(os.Args[2:], os.Stdout); err != nil {
			exitWithError(err)
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

// exitWithError reports a failure with the exit status the desktop uses to tell
// a requested stop apart from a workflow or provider failure.
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	if errors.Is(err, errTaskCancelled) {
		os.Exit(cancelledExitCode)
	}
	os.Exit(1)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `meeting-asr-go %s — experimental Go CLI for meeting-asr

Usage:
  meeting-asr-go transcribe <audio_path> [flags]
  meeting-asr-go resume <task_dir> [flags]
  meeting-asr-go preview-task-cache --output-root <path> --task-folder <folder>
  meeting-asr-go clean-task-cache --output-root <path> --task-folder <folder> --expected-token <token>
  meeting-asr-go list-tasks --output-root <path>
  meeting-asr-go get-task-metadata --output-root <path> --task-id <id>
  meeting-asr-go update-task-metadata --output-root <path> --task-id <id>
  meeting-asr-go list-templates [--templates-file <path>]
  meeting-asr-go create-custom-template --templates-file <path>
  meeting-asr-go update-custom-template --templates-file <path>
  meeting-asr-go delete-custom-template --templates-file <path>
  meeting-asr-go list-providers
	meeting-asr-go model-catalog
	meeting-asr-go model-status --app-data-root <path> --model-id sensevoice-small-int8
	meeting-asr-go download-model --app-data-root <path> --model-id sensevoice-small-int8
	meeting-asr-go import-model --app-data-root <path> --model-id sensevoice-small-int8 --source-archive <path>
	meeting-asr-go delete-model --app-data-root <path> --model-id sensevoice-small-int8
  meeting-asr-go generate-content --task-dir <path> --template-id <id>
  meeting-asr-go retry-generation --task-dir <path> --generation-id <id>

Flags:
  --output-mode transcript_only   Output mode: transcript_only or transcript_and_generate
  --template-id id           Built-in template for transcript_and_generate
  --asr-provider mimo        Speech-recognition provider
  --asr-model model          Speech-recognition model
  --llm-provider deepseek    Generated-content provider
  --llm-model model          Generated-content model
  --output-dir output        Base output directory
  --chunk-minutes 5          Chunk length in minutes
  --max-upload-mb 10         Max upload size in MB
  --env-file path            Explicit path to .env file (default: search upward from cwd)
  --project-root path        Explicit project root for .env lookup (default: cwd)
  --prompt-path path         Path to prompt template
  --obsidian-template-path path  Path to Obsidian template

`, version)
}

func runModelCommand(command string, args []string, stdout, stderr io.Writer) error {
	catalog, err := modelcatalog.Load()
	if err != nil {
		return err
	}
	if command == "model-catalog" {
		if len(args) != 0 {
			return fmt.Errorf("model-catalog accepts no arguments")
		}
		return json.NewEncoder(stdout).Encode(catalog)
	}
	if command == "normalize-hf-endpoint" {
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		endpoint := fs.String("endpoint", "", "Hugging Face origin")
		if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
			return fmt.Errorf("invalid endpoint arguments")
		}
		normalized, err := modelcatalog.NormalizeHuggingFaceEndpoint(*endpoint)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(map[string]string{"endpoint": normalized})
	}
	if command == "test-hf-endpoint" {
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		endpoint := fs.String("endpoint", "", "Hugging Face origin")
		if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
			return fmt.Errorf("invalid endpoint arguments")
		}
		normalized, err := modelcatalog.NormalizeHuggingFaceEndpoint(*endpoint)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, normalized, nil)
		client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return fmt.Errorf("too many redirects")
			}
			redirect, normalizeErr := modelcatalog.NormalizeHuggingFaceEndpoint(request.URL.String())
			if normalizeErr != nil || redirect != normalized {
				return fmt.Errorf("unsafe endpoint redirect")
			}
			return nil
		}}
		response, err := client.Do(req)
		if err != nil {
			return &modelcatalog.Error{Code: "network_unavailable", Message: "Could not connect to the Hugging Face endpoint.", Retryable: true}
		}
		response.Body.Close()
		return json.NewEncoder(stdout).Encode(map[string]any{"endpoint": normalized, "reachable": response.StatusCode < 500})
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("app-data-root", "", "Voxly app data root")
	modelID := fs.String("model-id", "sensevoice-small-int8", "Managed model ID")
	source := fs.String("source-archive", "", "Official archive selected for import")
	controlStdin := fs.Bool("control-stdin", false, "Accept a graceful stop request on stdin")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !filepath.IsAbs(*root) {
		return fmt.Errorf("--app-data-root must be absolute")
	}
	model, err := catalog.Model(*modelID)
	if err != nil {
		return err
	}
	if command == "model-status" {
		return json.NewEncoder(stdout).Encode(modelcatalog.Status(*root, catalog.CatalogRevision, model))
	}
	ctx, stop := watchCancellation(*controlStdin)
	defer stop()
	progress := func(state string, completed, total int64) {
		fmt.Fprintf(stderr, "Model-Progress: %s %d %d\n", state, completed, total)
	}
	var result modelcatalog.Readiness
	switch command {
	case "download-model":
		result, err = modelcatalog.DownloadAndInstall(ctx, *root, catalog.CatalogRevision, model, modelcatalog.DefaultHTTPClient(45*time.Minute), progress)
	case "import-model":
		if !filepath.IsAbs(*source) {
			return fmt.Errorf("--source-archive must be absolute")
		}
		result, err = modelcatalog.ImportArchive(ctx, *root, catalog.CatalogRevision, model, *source, progress)
	case "delete-model":
		result, err = modelcatalog.Delete(*root, catalog.CatalogRevision, model)
	default:
		return fmt.Errorf("unknown model command")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(result)
}

func encodeModelCommandError(stderr io.Writer, err error) {
	var managed *modelcatalog.Error
	if errors.As(err, &managed) {
		_ = json.NewEncoder(stderr).Encode(managed)
		return
	}
	_ = json.NewEncoder(stderr).Encode(modelcatalog.Error{Code: "internal_error", Message: "The local model operation failed.", Retryable: false})
}

func runGenerateContent(args []string) error {
	fs := flag.NewFlagSet("generate-content", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	taskDir := fs.String("task-dir", "", "Validated task directory")
	templateID := fs.String("template-id", "", "Built-in template ID")
	llmProvider := fs.String("llm-provider", string(LLMProviderDeepSeek), "Generated-content provider")
	llmModel := fs.String("llm-model", string(LLMModelDeepSeekV4), "Generated-content model")
	envFile := fs.String("env-file", "", "Explicit .env path")
	projectRoot := fs.String("project-root", "", "Project root")
	templatesFile := fs.String("templates-file", "", "Application-owned custom template storage")
	controlStdin := fs.Bool("control-stdin", false, "Accept a graceful stop request on stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || !filepath.IsAbs(*taskDir) {
		return fmt.Errorf("--task-dir must be an absolute path")
	}
	ctx, stop := watchCancellation(*controlStdin)
	defer stop()
	lock, err := AcquireTaskLock(*taskDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	m, err := LoadManifest(*taskDir)
	if err != nil {
		return err
	}
	if err := ensureCanonicalTranscript(*taskDir, m); err != nil {
		return err
	}
	settings, err := loadResumeSettings(*envFile, *projectRoot)
	if err != nil {
		return err
	}
	settings.LLMProvider = *llmProvider
	settings.DeepSeekModel = *llmModel
	settings.TemplateStorePath = *templatesFile
	if _, err := LookupPromptTemplateWithStore(settings.TemplateStorePath, *templateID); err != nil {
		return err
	}
	if err := ValidateLLMProviderConfig(settings.LLMConfig()); err != nil {
		return err
	}
	if _, err := settings.RequireDeepSeekAPIKey(); err != nil {
		return err
	}
	fmt.Printf("LLM provider=%s model=%s\n", settings.LLMProvider, settings.DeepSeekModel)
	generator, err := ResolveLLMContentGenerator(settings.LLMConfig())
	if err != nil {
		return err
	}
	record, err := GenerateContentContext(ctx, *taskDir, m, *templateID, settings, generator)
	if record != nil {
		fmt.Printf("Generation ID: %s\n", record.GenerationID)
	}
	if err != nil {
		markTaskCancelledIfRequested(ctx, *taskDir, m, err)
		return err
	}
	fmt.Printf("Generated artifact: %s\n", record.ArtifactReference)
	return nil
}

// runRetryGeneration retries exactly one generation the user selected. Unlike
// generate-content it creates no new generation, and unlike resume it does not
// touch other pending work; the record keeps its provider, model, template
// snapshot and source transcript revision.
func runRetryGeneration(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("retry-generation", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	taskDir := fs.String("task-dir", "", "Validated task directory")
	generationID := fs.String("generation-id", "", "Generation identity to retry")
	envFile := fs.String("env-file", "", "Explicit .env path")
	projectRoot := fs.String("project-root", "", "Project root")
	controlStdin := fs.Bool("control-stdin", false, "Accept a graceful stop request on stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || !filepath.IsAbs(*taskDir) {
		return fmt.Errorf("--task-dir must be an absolute path")
	}
	if strings.TrimSpace(*generationID) == "" {
		return fmt.Errorf("--generation-id is required")
	}
	ctx, stop := watchCancellation(*controlStdin)
	defer stop()
	lock, err := AcquireTaskLock(*taskDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	m, err := LoadManifest(*taskDir)
	if err != nil {
		return err
	}
	target := generationIndex(m, *generationID)
	if target < 0 {
		return fmt.Errorf("generation %s was not found in this task", *generationID)
	}
	if !generationRetryable(&m.Generations[target]) {
		return fmt.Errorf("generation %s is not retryable", *generationID)
	}
	settings, err := loadResumeSettings(*envFile, *projectRoot)
	if err != nil {
		return err
	}
	// The retried generation owns its provider and model; the current
	// configuration never overrides them.
	effective, err := generationExecutionSettings(&m.Generations[target], settings)
	if err != nil {
		return err
	}
	if err := ValidateLLMProviderConfig(effective.LLMConfig()); err != nil {
		return err
	}
	if _, err := effective.RequireDeepSeekAPIKey(); err != nil {
		return err
	}
	generator, err := ResolveLLMContentGenerator(effective.LLMConfig())
	if err != nil {
		return err
	}
	fmt.Println("Status: Retrying generation")
	fmt.Fprintf(stdout, "Generation ID: %s\n", *generationID)
	fmt.Printf("LLM provider=%s model=%s\n", effective.LLMProvider, effective.DeepSeekModel)
	if err := RetryGeneration(ctx, *taskDir, m, *generationID, settings, generator); err != nil {
		markTaskCancelledIfRequested(ctx, *taskDir, m, err)
		return err
	}
	record := m.Generations[target]
	fmt.Fprintf(stdout, "Generation status: %s\n", record.Status)
	if record.ArtifactReference != "" {
		fmt.Fprintf(stdout, "Generated artifact: %s\n", record.ArtifactReference)
	}
	return nil
}

func parseTemplatesFile(args []string, command string) (string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("templates-file", "", "Application-owned custom template storage")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("%s does not accept positional arguments", command)
	}
	if strings.TrimSpace(*path) != "" && !filepath.IsAbs(*path) {
		return "", fmt.Errorf("--templates-file must be an absolute path")
	}
	return *path, nil
}

func decodeTemplateInput(stdin io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(stdin, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid_template_input: request JSON is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid_template_input: exactly one JSON document is required")
	}
	return nil
}

func runListTemplates(args []string, stdout io.Writer) error {
	path, err := parseTemplatesFile(args, "list-templates")
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(ListPromptTemplateCatalog(path))
}

func runCreateCustomTemplate(args []string, stdin io.Reader, stdout io.Writer) error {
	path, err := parseTemplatesFile(args, "create-custom-template")
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("--templates-file is required")
	}
	var input CustomTemplateInput
	if err := decodeTemplateInput(stdin, &input); err != nil {
		return err
	}
	template, err := CreateCustomPromptTemplate(path, input)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(template)
}

func runUpdateCustomTemplate(args []string, stdin io.Reader, stdout io.Writer) error {
	path, err := parseTemplatesFile(args, "update-custom-template")
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("--templates-file is required")
	}
	var input UpdateCustomTemplateInput
	if err := decodeTemplateInput(stdin, &input); err != nil {
		return err
	}
	template, err := UpdateCustomPromptTemplate(path, input)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(template)
}

func runDeleteCustomTemplate(args []string, stdin io.Reader, stdout io.Writer) error {
	path, err := parseTemplatesFile(args, "delete-custom-template")
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("--templates-file is required")
	}
	var input DeleteCustomTemplateInput
	if err := decodeTemplateInput(stdin, &input); err != nil {
		return err
	}
	if err := DeleteCustomPromptTemplate(path, input); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(map[string]bool{"deleted": true})
}

func runListTasks(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("list-tasks", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outputRoot := fs.String("output-root", "", "Application-owned output root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("list-tasks does not accept positional arguments")
	}
	if strings.TrimSpace(*outputRoot) == "" {
		return fmt.Errorf("--output-root is required")
	}
	if !filepath.IsAbs(*outputRoot) {
		return fmt.Errorf("--output-root must be absolute")
	}

	result, err := tasklibrary.DiscoverWithDescriber(*outputRoot, taskActionDescriber())
	if err != nil {
		return fmt.Errorf("discovering tasks: %w", err)
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return fmt.Errorf("encoding task library result: %w", err)
	}
	return nil
}

func metadataFlags(command string, args []string) (string, string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outputRoot := fs.String("output-root", "", "Application-owned output root")
	taskID := fs.String("task-id", "", "Stable Task identity")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if fs.NArg() != 0 || !filepath.IsAbs(*outputRoot) || strings.TrimSpace(*taskID) == "" {
		return "", "", fmt.Errorf("--output-root must be absolute and --task-id is required")
	}
	return *outputRoot, *taskID, nil
}

func runGetTaskMetadata(args []string, stdout io.Writer) error {
	root, taskID, err := metadataFlags("get-task-metadata", args)
	if err != nil {
		return err
	}
	metadata, err := tasklibrary.ReadMetadata(root, taskID)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(metadata)
}

func runUpdateTaskMetadata(args []string, stdin io.Reader, stdout io.Writer) error {
	root, taskID, err := metadataFlags("update-task-metadata", args)
	if err != nil {
		return err
	}
	var update tasklibrary.MetadataUpdate
	decoder := json.NewDecoder(io.LimitReader(stdin, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&update); err != nil {
		return fmt.Errorf("decode metadata update: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("metadata update must contain exactly one JSON document")
	}
	discovery, err := tasklibrary.Discover(root)
	if err != nil {
		return err
	}
	var folder string
	for _, task := range discovery.Tasks {
		if task.TaskID == taskID {
			if folder != "" {
				return errors.New("task identity is ambiguous")
			}
			folder = task.TaskFolder
		}
	}
	dir, err := cacheTaskDirectory(root, folder)
	if err != nil {
		return err
	}
	lock, err := AcquireTaskLock(dir)
	if err != nil {
		return err
	}
	defer lock.Release()
	metadata, err := tasklibrary.UpdateMetadata(root, taskID, update)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(metadata)
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
			if !strings.Contains(arg, "=") && arg != "--auto-clean-chunks" && arg != "--vad" && arg != "--control-stdin" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
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
	templateID := fs.String("template-id", "structured-notes", "Built-in generation template")
	templatesFile := fs.String("templates-file", "", "Application-owned custom template storage")
	outputDir := fs.String("output-dir", "output", "Base output directory")
	chunkMinutes := fs.Int("chunk-minutes", 0, "Chunk length in minutes (0 = use .env or default)")
	maxUploadMB := fs.Float64("max-upload-mb", 0, "Max upload size in MB (0 = use .env or default)")
	envFile := fs.String("env-file", "", "Explicit path to .env file")
	projectRoot := fs.String("project-root", "", "Explicit project root for .env lookup")
	promptPath := fs.String("prompt-path", "", "Path to prompt template")
	obsidianTemplatePath := fs.String("obsidian-template-path", "", "Path to Obsidian template")
	asrProvider := fs.String("asr-provider", string(ASRProviderMimo), "Speech-recognition provider")
	asrModel := fs.String("asr-model", string(ASRModelMimoV25), "Speech-recognition model")
	useVAD := fs.Bool("vad", false, "Use Silero VAD for local ASR")
	localLanguage := fs.String("local-asr-language", "zh", "Local ASR language: zh or auto")
	appDataRoot := fs.String("app-data-root", "", "Managed local model storage (absolute path)")
	controlStdin := fs.Bool("control-stdin", false, "Accept a graceful stop request on stdin")
	autoCleanChunks := fs.Bool("auto-clean-chunks", false, "Clean chunks after successful completion")
	llmProvider := fs.String("llm-provider", string(LLMProviderDeepSeek), "Generated-content provider")
	llmModel := fs.String("llm-model", string(LLMModelDeepSeekV4), "Generated-content model")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	normalizedMode, normalizedTemplate, err := normalizeProcessingSelection(*outputMode, *templateID)
	if err != nil {
		return err
	}
	*outputMode, *templateID = normalizedMode, normalizedTemplate

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
	if *promptPath != "" {
		settings.MinutesPromptPath = resolveCLIPath(*promptPath)
	}
	if *obsidianTemplatePath != "" {
		settings.ObsidianTemplatePath = resolveCLIPath(*obsidianTemplatePath)
	}
	settings.AutoCleanChunks = *autoCleanChunks
	settings.LocalModelRoot = *appDataRoot
	settings.LocalASRLanguage = *localLanguage
	settings.UseVAD = *useVAD
	settings.ASRProvider = *asrProvider
	settings.MimoModel = *asrModel
	settings.LLMProvider = *llmProvider
	settings.DeepSeekModel = *llmModel
	if err := ValidateASRProviderConfig(settings.ASRConfig()); err != nil {
		return err
	}
	if *outputMode == "transcript_and_generate" {
		if err := ValidateLLMProviderConfig(settings.LLMConfig()); err != nil {
			return err
		}
	}

	// Cloud ASR requires a credential; the local provider does not.
	if settings.ASRConfig().ProviderID == ASRProviderMimo {
		if _, err := settings.RequireMimoAPIKey(); err != nil {
			return err
		}
	}

	// Fail fast on unsupported generated-content configuration before expensive work.
	if *outputMode == "transcript_and_generate" {
		if _, err := settings.RequireDeepSeekAPIKey(); err != nil {
			return err
		}
	}

	// Resolve output directory.
	absOutput, err := filepath.Abs(*outputDir)
	if err != nil {
		return fmt.Errorf("resolving output dir: %w", err)
	}

	settings.TemplateStorePath = *templatesFile
	if *outputMode == "transcript_and_generate" {
		if _, err := LookupPromptTemplateWithStore(settings.TemplateStorePath, *templateID); err != nil {
			return err
		}
	}
	// Run the workflow under a cancellable context: a signal, a control-channel
	// stop request, or the desktop disappearing all end the run gracefully.
	ctx, stop := watchCancellation(*controlStdin)
	defer stop()
	return runWorkflowV2(ctx, absAudio, absOutput, settings, *outputMode, *templateID)
}

func normalizeProcessingSelection(mode, templateID string) (string, string, error) {
	switch mode {
	case "transcript", "transcript_only":
		return "transcript_only", "", nil
	case "notes":
		return "transcript_and_generate", "structured-notes", nil
	case "minutes":
		return "transcript_and_generate", "meeting-minutes", nil
	case "transcript_and_generate":
		if strings.TrimSpace(templateID) == "" {
			return "", "", fmt.Errorf("template_id is required")
		}
		return mode, templateID, nil
	default:
		return "", "", fmt.Errorf("invalid --output-mode %q", mode)
	}
}

func runWorkflowV2(ctx context.Context, audioPath, outputDir string, settings *Settings, outputMode, templateID string) error {
	return runWorkflowContext(ctx, audioPath, outputDir, settings, outputMode, templateID)
}

func runWorkflow(audioPath, outputDir string, settings *Settings, outputMode string, selectedTemplate ...string) error {
	return runWorkflowContext(context.Background(), audioPath, outputDir, settings, outputMode, selectedTemplate...)
}

// runWorkflowContext runs first execution under the run's cancellation context.
func runWorkflowContext(ctx context.Context, audioPath, outputDir string, settings *Settings, outputMode string, selectedTemplate ...string) error {
	capabilities, err := ResolveASRCapabilities(settings.ASRConfig())
	if err != nil {
		return err
	}
	// Local readiness must fail before creating a Task or copying audio.
	if settings.ASRConfig().ProviderID == ASRProviderLocalSenseVoice {
		if _, err := normalizeLocalASRLanguage(settings.LocalASRLanguage); err != nil {
			return err
		}
		if _, err := localASRStatus(settings.LocalModelRoot); err != nil {
			return err
		}
	}
	if settings.UseVAD {
		if settings.ASRConfig().ProviderID != ASRProviderLocalSenseVoice {
			return fmt.Errorf("VAD requires local SenseVoice ASR")
		}
		if _, err := vadModelPath(); err != nil {
			return err
		}
	}
	// 1. Create task directory.
	audioStem := filepath.Base(audioPath)
	audioStem = audioStem[:len(audioStem)-len(filepath.Ext(audioStem))]

	taskDir, err := CreateTaskDir(outputDir, audioStem)
	if err != nil {
		return fmt.Errorf("creating task dir: %w", err)
	}

	// Hold the Task lock for the whole run so discovery can tell "being
	// processed" from "abandoned", and so two operations cannot share one Task.
	lock, err := AcquireTaskLock(taskDir)
	if err != nil {
		return fmt.Errorf("acquiring task lock: %w", err)
	}
	defer lock.Release()

	// 2. Copy source audio to task/audio/.
	sourceCopy := filepath.Join(taskDir, "audio", filepath.Base(audioPath))
	if err := CopyFile(audioPath, sourceCopy); err != nil {
		return fmt.Errorf("copying audio: %w", err)
	}

	// 3. Create manifest.
	manifest := CreateManifest(taskDir, sourceCopy, outputMode, settings.DefaultChunkMin, settings.MaxUploadMB, settings.ASRProvider, settings.MimoModel)
	if settings.UseVAD {
		manifest.Chunks.VAD = &taskmanifest.VADPlan{Version: 1, Language: settings.LocalASRLanguage}
	}
	if outputMode == "transcript_and_generate" && len(selectedTemplate) > 0 {
		manifest.RequestedTemplateID = selectedTemplate[0]
	}
	if err := SetTaskState(taskDir, manifest, TaskValidatingAudio); err != nil {
		return fmt.Errorf("creating manifest: %w", err)
	}
	fmt.Printf("ASR provider=%s model=%s\n", settings.ASRProvider, settings.MimoModel)
	if outputMode == "transcript_and_generate" {
		fmt.Printf("LLM provider=%s model=%s\n", settings.LLMProvider, settings.DeepSeekModel)
	}

	// 4. Split audio.
	if err := SetTaskState(taskDir, manifest, TaskSplitting); err != nil {
		return fmt.Errorf("updating task state: %w", err)
	}
	fmt.Println("Status: Splitting audio")
	if manifest.Chunks.VAD != nil {
		if err := prepareVADChunks(ctx, taskDir, manifest, settings); err != nil {
			if stopErr := stopIfCancelled(ctx, taskDir, manifest, err); stopErr != nil {
				return stopErr
			}
			_ = RecordTaskError(taskDir, manifest, TaskSplitting, err, false)
			return err
		}
	} else {
		chunks, err := SplitAudioWithCapabilities(
			ctx,
			sourceCopy,
			filepath.Join(taskDir, "chunks"),
			settings.DefaultChunkMin,
			settings.MaxUploadBytes(),
			capabilities,
		)
		if err != nil {
			if stopErr := stopIfCancelled(ctx, taskDir, manifest, err); stopErr != nil {
				return stopErr
			}
			_ = RecordTaskError(taskDir, manifest, TaskSplitting, err, IsRetryableError(err))
			return fmt.Errorf("splitting audio: %w", err)
		}

		// Build chunk items from split results.
		chunkItems, err := buildChunkItems(taskDir, chunks)
		if err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskSplitting, err, IsRetryableError(err))
			return err
		}
		chunkFormat := ""
		if len(chunks) > 0 {
			chunkFormat = strings.TrimPrefix(filepath.Ext(chunks[0]), ".")
		}
		manifest.Chunks.Format = chunkFormat
		manifest.Chunks.Total = len(chunkItems)
		manifest.Chunks.Items = chunkItems
		if err := SaveManifest(taskDir, manifest); err != nil {
			return fmt.Errorf("saving chunk info: %w", err)
		}

	}

	// 5. Transcribe chunks.
	if err := SetTaskState(taskDir, manifest, TaskTranscribing); err != nil {
		return fmt.Errorf("updating task state: %w", err)
	}
	fmt.Printf("Status: Transcribing %d chunk(s)\n", len(manifest.Chunks.Items))
	progressFn := func(action string, index, total int) {
		if action == "transcribing" || action == "completed" {
			fmt.Printf("Progress: %d/%d\n", index, total)
		}
	}
	// First execution and resume share one chunk executor, which owns chunk
	// state, transcript hashing and manifest persistence. The context is the
	// future cancellation boundary; wiring user-facing cancellation is out of
	// scope for this work package.
	pendingItems := make([]*ChunkItem, 0, len(manifest.Chunks.Items))
	for i := range manifest.Chunks.Items {
		pendingItems = append(pendingItems, &manifest.Chunks.Items[i])
	}
	if err := transcribeChunkItems(ctx, taskDir, manifest, pendingItems, settings, progressFn); err != nil {
		if stopErr := stopIfCancelled(ctx, taskDir, manifest, err); stopErr != nil {
			return stopErr
		}
		_ = RecordTaskError(taskDir, manifest, TaskTranscribing, err, IsRetryableError(err))
		return fmt.Errorf("transcribing: %w", err)
	}

	// 6. Merge transcripts.
	if err := SetTaskState(taskDir, manifest, TaskMerging); err != nil {
		return fmt.Errorf("updating task state: %w", err)
	}
	markArtifactProcessing(&manifest.Artifacts.Transcript)
	if err := SaveManifest(taskDir, manifest); err != nil {
		return fmt.Errorf("saving transcript artifact state: %w", err)
	}
	fmt.Println("Status: Merging transcript")
	transcriptPath := filepath.Join(taskDir, "transcript.md")
	if err := mergeTaskTranscript(taskDir, manifest, transcriptPath); err != nil {
		_ = RecordTaskError(taskDir, manifest, TaskMerging, err, IsRetryableError(err))
		return fmt.Errorf("merging transcripts: %w", err)
	}
	if err := markArtifactReady(transcriptPath, &manifest.Artifacts.Transcript); err != nil {
		_ = RecordTaskError(taskDir, manifest, TaskMerging, err, IsRetryableError(err))
		return err
	}
	if err := completeCanonicalTranscript(taskDir, transcriptPath, manifest); err != nil {
		_ = RecordTaskError(taskDir, manifest, TaskMerging, err, false)
		return err
	}
	if err := SaveManifest(taskDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	if manifest.Chunks.VAD != nil && len(manifest.Chunks.Items) == 0 && outputMode != "transcript_only" {
		return fmt.Errorf("no speech detected; content generation was not started")
	}

	// 7. Legacy notes/minutes branches are retained only for direct internal
	// compatibility tests. New CLI selections always use Processing Model V2.
	var notesInputPath string
	if outputMode == "notes" || outputMode == "minutes" {
		if err := SetTaskState(taskDir, manifest, TaskNotesGenerating); err != nil {
			return fmt.Errorf("updating task state: %w", err)
		}
		markArtifactProcessing(&manifest.Artifacts.NotesInput)
		if err := SaveManifest(taskDir, manifest); err != nil {
			return fmt.Errorf("saving notes artifact state: %w", err)
		}
		fmt.Println("Status: Preparing notes input")
		promptPath := settings.ResolvePath(settings.MinutesPromptPath)
		notesInputPath = filepath.Join(taskDir, "notes_input.md")
		if err := CreateNotesInput(promptPath, transcriptPath, notesInputPath); err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskNotesGenerating, err, IsRetryableError(err))
			return fmt.Errorf("creating notes input: %w", err)
		}
		if err := markArtifactReady(notesInputPath, &manifest.Artifacts.NotesInput); err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskNotesGenerating, err, IsRetryableError(err))
			return err
		}
		if err := SaveManifest(taskDir, manifest); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	var generatedContentPath string
	if outputMode == "transcript_and_generate" {
		templateID := "structured-notes"
		if len(selectedTemplate) > 0 && selectedTemplate[0] != "" {
			templateID = selectedTemplate[0]
		}
		fmt.Println("Status: Generating content")
		generator, resolveErr := ResolveLLMContentGenerator(settings.LLMConfig())
		if resolveErr != nil {
			return resolveErr
		}
		record, err := GenerateContentContext(ctx, taskDir, manifest, templateID, settings, generator)
		if err != nil {
			if stopErr := stopIfCancelled(ctx, taskDir, manifest, err); stopErr != nil {
				return stopErr
			}
			// Transcript remains valid; failed generation metadata is durable.
			manifest.State = TaskCompleted
			_ = SaveManifest(taskDir, manifest)
			return fmt.Errorf("generating content: %w", err)
		}
		generatedContentPath = filepath.Join(taskDir, filepath.FromSlash(record.ArtifactReference))
	}

	// 8. meeting_minutes.md (minutes mode only).
	var minutesPath string
	if outputMode == "minutes" {
		if err := SetTaskState(taskDir, manifest, TaskMinutesGenerating); err != nil {
			return fmt.Errorf("updating task state: %w", err)
		}
		markArtifactProcessing(&manifest.Artifacts.Minutes)
		if err := SaveManifest(taskDir, manifest); err != nil {
			return fmt.Errorf("saving minutes artifact state: %w", err)
		}
		fmt.Println("Status: Generating meeting minutes")
		rawMinutes, err := GenerateMinutesContext(ctx, notesInputPath, settings)
		if err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskMinutesGenerating, err, IsRetryableError(err))
			return fmt.Errorf("generating meeting minutes: %w", err)
		}

		// Apply Obsidian template.
		templatePath := settings.ResolvePath(settings.ObsidianTemplatePath)
		title := ExtractTitle(filepath.Base(taskDir))
		taskName := filepath.Base(taskDir)

		transcriptData, err := os.ReadFile(transcriptPath)
		if err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskMinutesGenerating, err, IsRetryableError(err))
			return fmt.Errorf("reading transcript for template: %w", err)
		}

		wrapped, err := ApplyObsidianTemplate(templatePath, rawMinutes, title, taskName, string(transcriptData))
		if err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskMinutesGenerating, err, IsRetryableError(err))
			return fmt.Errorf("applying Obsidian template: %w", err)
		}

		minutesPath = filepath.Join(taskDir, "meeting_minutes.md")
		if err := WriteFileAtomically(minutesPath, []byte(wrapped), 0600); err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskMinutesGenerating, err, IsRetryableError(err))
			return fmt.Errorf("writing meeting minutes: %w", err)
		}
		if err := markArtifactReady(minutesPath, &manifest.Artifacts.Minutes); err != nil {
			_ = RecordTaskError(taskDir, manifest, TaskMinutesGenerating, err, IsRetryableError(err))
			return err
		}
		if err := SaveManifest(taskDir, manifest); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	// 9. Validate final outputs before completing the task.
	if err := executeValidatingOutputs(taskDir, manifest); err != nil {
		_ = RecordTaskError(taskDir, manifest, TaskValidatingOutputs, err, IsRetryableError(err))
		return fmt.Errorf("validating outputs: %w", err)
	}

	// 10. Mark task completed.
	if err := MarkTaskCompleted(taskDir, manifest); err != nil {
		return fmt.Errorf("marking task completed: %w", err)
	}

	autoCleanTaskCacheLocked(taskDir, manifest, settings.AutoCleanChunks)

	// 10. Done.
	fmt.Println("Status: Done")
	fmt.Printf("Transcript saved: %s\n", transcriptPath)
	if notesInputPath != "" {
		fmt.Printf("Notes input saved: %s\n", notesInputPath)
	}
	if minutesPath != "" {
		fmt.Printf("Meeting minutes saved: %s\n", minutesPath)
	}
	if generatedContentPath != "" {
		fmt.Printf("Generated content saved: %s\n", generatedContentPath)
	}
	fmt.Printf("Task folder: %s\n", taskDir)

	return nil
}

// buildChunkItems creates ChunkItem records from split audio chunk paths.
func buildChunkItems(taskDir string, chunks []string) ([]ChunkItem, error) {
	items := make([]ChunkItem, len(chunks))
	for i, chunkPath := range chunks {
		id := strings.TrimSuffix(filepath.Base(chunkPath), filepath.Ext(chunkPath))
		info, _ := os.Stat(chunkPath)
		var sizeBytes int64
		if info != nil {
			sizeBytes = info.Size()
		}
		hash, err := fileSHA256(chunkPath)
		if err != nil {
			return nil, fmt.Errorf("hashing audio chunk %s: %w", chunkPath, err)
		}
		items[i] = ChunkItem{
			ID:             id,
			AudioPath:      chunkPath,
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          ChunkPending,
			SizeBytes:      sizeBytes,
			AudioSHA256:    hash,
		}
	}
	return items, nil
}

func strPtr(s string) *string {
	return &s
}

// resolveCLIPath resolves a CLI flag path relative to the current working directory.
// This ensures CLI flags override .env paths with cwd-relative semantics,
// not .env-file-relative semantics.
func resolveCLIPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		// Fallback: return as-is if we can't get cwd.
		return p
	}
	return filepath.Join(cwd, p)
}
