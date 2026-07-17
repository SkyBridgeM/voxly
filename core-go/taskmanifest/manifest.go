// Package taskmanifest provides the versioned Task manifest data model used by
// the runtime as its durable checkpoint and resume source of truth.
package taskmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	CurrentSchemaVersion = "2.0"
	LatestSchemaVersion  = CurrentSchemaVersion
	LegacySchemaVersion  = "1.0"
	ManifestFilename     = "task_manifest.json"
)

var ErrUnsupportedSchemaVersion = errors.New("unsupported task manifest schema version")
var safeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

var artifactStatuses = map[string]struct{}{
	"pending": {}, "processing": {}, "completed": {}, "failed": {}, "unavailable": {},
}

var taskStatuses = map[string]struct{}{
	"created": {}, "processing": {}, "completed": {}, "completed_with_errors": {},
	"failed": {}, "cleaned": {}, "archived": {},
}

type TaskManifest struct {
	SchemaVersion string                     `json:"schema_version"`
	TaskID        string                     `json:"task_id"`
	CreatedAt     string                     `json:"created_at"`
	UpdatedAt     string                     `json:"updated_at"`
	UserMetadata  *UserMetadata              `json:"user_metadata,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`

	Backend                 string                     `json:"-"`
	Provider                string                     `json:"-"`
	OutputMode              string                     `json:"-"`
	RequestedTemplateID     string                     `json:"-"`
	Status                  string                     `json:"-"`
	State                   string                     `json:"-"`
	Attempt                 int                        `json:"-"`
	LastError               *ErrorInfo                 `json:"-"`
	CompletedAt             *string                    `json:"-"`
	Source                  SourceInfo                 `json:"-"`
	SourceOwnership         string                     `json:"-"`
	DurationSeconds         *float64                   `json:"-"`
	InputFormat             string                     `json:"-"`
	SourceMetadata          map[string]json.RawMessage `json:"-"`
	Artifacts               Artifacts                  `json:"-"`
	IntegrityVersion        int                        `json:"-"`
	Chunks                  ChunksInfo                 `json:"-"`
	ExecutionUnknownFields  map[string]json.RawMessage `json:"-"`
	InputUnknownFields      map[string]json.RawMessage `json:"-"`
	CheckpointUnknownFields map[string]json.RawMessage `json:"-"`
	CanonicalTranscript     *CanonicalTranscript       `json:"-"`
	Generations             []GenerationRecord         `json:"-"`
}

// CanonicalTranscript identifies the immutable transcript revision used by
// generated content. Hashes cover the exact canonical file bytes.
type CanonicalTranscript struct {
	TranscriptID        string                     `json:"transcript_id"`
	RevisionID          string                     `json:"revision_id"`
	ArtifactReference   string                     `json:"artifact_reference"`
	ContentSHA256       string                     `json:"content_sha256"`
	HashContractVersion int                        `json:"hash_contract_version"`
	CreatedAt           string                     `json:"created_at"`
	ASRProvider         string                     `json:"asr_provider"`
	ASRModel            string                     `json:"asr_model"`
	Status              string                     `json:"status"`
	UnknownFields       map[string]json.RawMessage `json:"-"`
}

// GenerationRecord is one append-only user-requested generation. Technical
// retries remain attempts within the same record; Generate again appends one.
type GenerationRecord struct {
	GenerationID               string                     `json:"generation_id"`
	TemplateID                 string                     `json:"template_id"`
	TemplateRevision           int                        `json:"template_revision"`
	TemplateDisplayName        string                     `json:"template_display_name"`
	TemplateInstructions       string                     `json:"template_instructions"`
	CreatedAt                  string                     `json:"created_at"`
	UpdatedAt                  string                     `json:"updated_at"`
	SourceTranscriptRevisionID string                     `json:"source_transcript_revision_id"`
	SourceTranscriptSHA256     string                     `json:"source_transcript_sha256"`
	LLMProvider                string                     `json:"llm_provider"`
	LLMModel                   string                     `json:"llm_model"`
	PromptCompositionVersion   int                        `json:"prompt_composition_version"`
	PromptHash                 string                     `json:"prompt_hash"`
	Status                     string                     `json:"status"`
	ArtifactReference          string                     `json:"artifact_reference,omitempty"`
	Error                      *ArtifactError             `json:"error,omitempty"`
	Attempts                   []GenerationAttempt        `json:"attempts,omitempty"`
	UnknownFields              map[string]json.RawMessage `json:"-"`
}

type GenerationAttempt struct {
	Attempt   int            `json:"attempt"`
	CreatedAt string         `json:"created_at"`
	Status    string         `json:"status"`
	Error     *ArtifactError `json:"error,omitempty"`
}

type SourceInfo struct {
	Path         string `json:"path"`
	OriginalName string `json:"original_name"`
	SizeBytes    int64  `json:"size_bytes"`
}

type ErrorInfo struct {
	Stage         string                     `json:"stage"`
	Message       string                     `json:"message"`
	Retryable     bool                       `json:"retryable"`
	At            string                     `json:"at"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

// Artifacts is the fixed set of runtime deliverables. Unknown artifact kinds
// are retained for forward-compatible reads.
type Artifacts struct {
	Transcript    Artifact                   `json:"transcript"`
	NotesInput    Artifact                   `json:"notes,omitempty"`
	Minutes       Artifact                   `json:"minutes,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

func (artifacts Artifacts) Get(name string) (Artifact, bool) {
	switch name {
	case "transcript":
		return artifacts.Transcript, true
	case "notes", "notes_input":
		return artifacts.NotesInput, true
	case "minutes":
		return artifacts.Minutes, true
	default:
		data, ok := artifacts.UnknownFields[name]
		if !ok {
			return Artifact{}, false
		}
		var artifact Artifact
		if json.Unmarshal(data, &artifact) != nil {
			return Artifact{}, false
		}
		return artifact, true
	}
}

type ExecutionMetadata struct {
	Backend       string                     `json:"backend"`
	Provider      string                     `json:"provider"`
	OutputMode    string                     `json:"output_mode"`
	TemplateID    string                     `json:"template_id,omitempty"`
	Status        string                     `json:"status"`
	Stage         string                     `json:"stage,omitempty"`
	Attempt       int                        `json:"attempt,omitempty"`
	CompletedAt   *string                    `json:"completed_at,omitempty"`
	ErrorSummary  *ArtifactError             `json:"error_summary,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

type InputMetadata struct {
	SourceReference string                     `json:"source_reference"`
	SourceOwnership string                     `json:"source_ownership"`
	DurationSeconds *float64                   `json:"duration_seconds,omitempty"`
	Format          string                     `json:"format"`
	OriginalName    string                     `json:"original_name,omitempty"`
	SizeBytes       int64                      `json:"size_bytes,omitempty"`
	SourceMetadata  map[string]json.RawMessage `json:"metadata,omitempty"`
	UnknownFields   map[string]json.RawMessage `json:"-"`
}

type Artifact struct {
	State         string                     `json:"status"`
	Path          string                     `json:"path,omitempty"`
	SHA256        string                     `json:"sha256,omitempty"`
	Error         *ArtifactError             `json:"error,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

// CheckpointMetadata contains runtime-only progress needed for idempotent
// resume. It is execution truth, not user-editable organization metadata.
type CheckpointMetadata struct {
	IntegrityVersion int                        `json:"integrity_version,omitempty"`
	Chunks           ChunksInfo                 `json:"chunks"`
	UnknownFields    map[string]json.RawMessage `json:"-"`
}

type ChunksInfo struct {
	Format       string      `json:"format,omitempty"`
	ChunkMinutes int         `json:"chunk_minutes"`
	MaxUploadMB  float64     `json:"max_upload_mb"`
	ASRProvider  string      `json:"asr_provider"`
	ASRModel     string      `json:"asr_model"`
	Total        int         `json:"total"`
	Items        []ChunkItem `json:"items"`
}

type ChunkItem struct {
	ID               string  `json:"id"`
	AudioPath        string  `json:"audio_path"`
	TranscriptPath   string  `json:"transcript_path"`
	State            string  `json:"status"`
	Attempts         int     `json:"attempts"`
	SizeBytes        int64   `json:"size_bytes"`
	AudioSHA256      string  `json:"audio_sha256,omitempty"`
	TranscriptSHA256 string  `json:"transcript_sha256,omitempty"`
	LastError        *string `json:"last_error,omitempty"`
}

type ArtifactError struct {
	Code          string                     `json:"code"`
	Message       string                     `json:"message"`
	Retryable     *bool                      `json:"retryable,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

type UserMetadata struct {
	Title         string                     `json:"title"`
	Description   string                     `json:"description"`
	Tags          []string                   `json:"tags"`
	Favorite      *bool                      `json:"favorite,omitempty"`
	Annotations   []string                   `json:"annotations,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

// LoadManifest reads and validates a Task manifest at path.
func LoadManifest(path string) (*TaskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task manifest: %w", err)
	}
	var manifest TaskManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse task manifest: %w", err)
	}
	if err := manifest.ValidateManifest(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// ValidateManifest validates a Task manifest without modifying it.
func ValidateManifest(manifest *TaskManifest) error {
	if manifest == nil {
		return errors.New("task manifest is nil")
	}
	return manifest.ValidateManifest()
}

// ValidateManifest validates required fields, schema compatibility, and artifact states.
func (manifest *TaskManifest) ValidateManifest() error {
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		return errors.New("task manifest missing schema_version")
	}
	if manifest.SchemaVersion != CurrentSchemaVersion && manifest.SchemaVersion != LegacySchemaVersion {
		return fmt.Errorf("%w: %q", ErrUnsupportedSchemaVersion, manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.TaskID) == "" {
		return errors.New("task manifest missing task_id")
	}
	if strings.TrimSpace(manifest.CreatedAt) == "" || strings.TrimSpace(manifest.UpdatedAt) == "" {
		return errors.New("task manifest missing required timestamps")
	}
	execution := ExecutionMetadata{
		Backend: manifest.Backend, Provider: manifest.Provider, OutputMode: manifest.OutputMode,
		TemplateID: manifest.RequestedTemplateID,
		Status:     manifest.Status, Stage: manifest.State, Attempt: manifest.Attempt,
		CompletedAt: manifest.CompletedAt, ErrorSummary: manifest.errorSummary(),
	}
	if err := execution.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.Source.Path) == "" {
		return errors.New("task manifest missing input.source_reference")
	}
	if strings.TrimSpace(manifest.SourceOwnership) == "" {
		return errors.New("task manifest missing input.source_ownership")
	}
	if strings.TrimSpace(manifest.InputFormat) == "" {
		return errors.New("task manifest missing input.format")
	}
	for name, artifact := range map[string]Artifact{
		"transcript": manifest.Artifacts.Transcript,
		"notes":      manifest.Artifacts.NotesInput,
		"minutes":    manifest.Artifacts.Minutes,
	} {
		if err := artifact.validate(name); err != nil {
			return err
		}
	}
	for name, raw := range manifest.Artifacts.UnknownFields {
		var artifact Artifact
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return fmt.Errorf("parse artifact %q: %w", name, err)
		}
		if err := artifact.validate(name); err != nil {
			return err
		}
	}
	if manifest.SchemaVersion == LatestSchemaVersion && (manifest.OutputMode == "transcript_only" || manifest.OutputMode == "transcript_and_generate" || len(manifest.Generations) > 0) {
		if manifest.CanonicalTranscript == nil {
			return errors.New("task manifest v2 missing canonical_transcript")
		}
		if err := manifest.CanonicalTranscript.validate(); err != nil {
			return err
		}
		for i := range manifest.Generations {
			if err := manifest.Generations[i].validate(); err != nil {
				return fmt.Errorf("generation[%d]: %w", i, err)
			}
		}
	}
	if err := rejectSensitiveFields(manifest); err != nil {
		return err
	}
	return nil
}

func (transcript CanonicalTranscript) validate() error {
	if strings.TrimSpace(transcript.TranscriptID) == "" || strings.TrimSpace(transcript.RevisionID) == "" {
		return errors.New("canonical transcript missing identity")
	}
	if transcript.Status != "pending" && transcript.Status != "processing" && transcript.Status != "completed" && transcript.Status != "failed" {
		return fmt.Errorf("canonical transcript has invalid status %q", transcript.Status)
	}
	if transcript.Status == "completed" {
		if strings.TrimSpace(transcript.ArtifactReference) == "" || strings.TrimSpace(transcript.ContentSHA256) == "" || transcript.HashContractVersion != 1 {
			return errors.New("completed canonical transcript missing reference or hash contract")
		}
		if !safeRelativeReference(transcript.ArtifactReference) {
			return errors.New("canonical transcript has unsafe artifact reference")
		}
	}
	return nil
}

func (generation GenerationRecord) validate() error {
	if !safeIDPattern.MatchString(generation.GenerationID) || !safeIDPattern.MatchString(generation.TemplateID) || generation.TemplateRevision < 1 {
		return errors.New("generation missing identity or template revision")
	}
	if generation.Status != "pending" && generation.Status != "processing" && generation.Status != "completed" && generation.Status != "failed" {
		return fmt.Errorf("generation has invalid status %q", generation.Status)
	}
	if strings.TrimSpace(generation.SourceTranscriptRevisionID) == "" || strings.TrimSpace(generation.SourceTranscriptSHA256) == "" {
		return errors.New("generation missing source transcript identity")
	}
	if strings.TrimSpace(generation.TemplateDisplayName) == "" || strings.TrimSpace(generation.TemplateInstructions) == "" || strings.TrimSpace(generation.CreatedAt) == "" || strings.TrimSpace(generation.UpdatedAt) == "" {
		return errors.New("generation missing template snapshot or timestamps")
	}
	if strings.TrimSpace(generation.LLMProvider) == "" || strings.TrimSpace(generation.LLMModel) == "" || generation.PromptCompositionVersion < 1 || strings.TrimSpace(generation.PromptHash) == "" {
		return errors.New("generation missing provider or prompt provenance")
	}
	if generation.Status == "completed" && strings.TrimSpace(generation.ArtifactReference) == "" {
		return errors.New("completed generation missing artifact reference")
	}
	if generation.ArtifactReference != "" && !safeRelativeReference(generation.ArtifactReference) {
		return errors.New("generation has unsafe artifact reference")
	}
	if generation.Status == "failed" && generation.Error == nil {
		return errors.New("failed generation missing structured error")
	}
	if generation.Status != "failed" && generation.Error != nil {
		return errors.New("non-failed generation must not contain error")
	}
	for i, attempt := range generation.Attempts {
		if attempt.Attempt < 1 || strings.TrimSpace(attempt.CreatedAt) == "" {
			return fmt.Errorf("attempt[%d] missing identity", i)
		}
		if attempt.Status != "processing" && attempt.Status != "completed" && attempt.Status != "failed" {
			return fmt.Errorf("attempt[%d] has invalid status", i)
		}
		if attempt.Status == "failed" && attempt.Error == nil {
			return fmt.Errorf("attempt[%d] missing error", i)
		}
		if attempt.Status != "failed" && attempt.Error != nil {
			return fmt.Errorf("attempt[%d] has unexpected error", i)
		}
	}
	return nil
}

func safeRelativeReference(value string) bool {
	return value != "" && !strings.ContainsAny(value, `\\:`) && !strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func (execution ExecutionMetadata) validate() error {
	if strings.TrimSpace(execution.Backend) == "" {
		return errors.New("task manifest missing execution.backend")
	}
	if strings.TrimSpace(execution.Provider) == "" {
		return errors.New("task manifest missing execution.provider")
	}
	if strings.TrimSpace(execution.OutputMode) == "" {
		return errors.New("task manifest missing execution.output_mode")
	}
	if execution.OutputMode == "transcript_and_generate" && !safeIDPattern.MatchString(execution.TemplateID) {
		return errors.New("task manifest missing execution.template_id")
	}
	if _, ok := taskStatuses[execution.Status]; !ok {
		return fmt.Errorf("task manifest has invalid execution.status %q", execution.Status)
	}
	if execution.ErrorSummary != nil {
		if err := execution.ErrorSummary.validate("execution.error_summary", false); err != nil {
			return err
		}
	}
	return nil
}

func (artifact Artifact) validate(name string) error {
	if _, ok := artifactStatuses[artifact.State]; !ok {
		return fmt.Errorf("artifact %q has invalid status %q", name, artifact.State)
	}
	if artifact.State == "completed" && strings.TrimSpace(artifact.Path) == "" {
		return fmt.Errorf("completed artifact %q missing path", name)
	}
	if artifact.State == "failed" {
		if artifact.Error == nil {
			return fmt.Errorf("failed artifact %q missing structured error", name)
		}
		if err := artifact.Error.validate("artifact "+name+".error", true); err != nil {
			return err
		}
	} else if artifact.Error != nil {
		return fmt.Errorf("artifact %q with status %q must not have an error", name, artifact.State)
	}
	return nil
}

func (artifactError ArtifactError) validate(field string, requireRetryable bool) error {
	if strings.TrimSpace(artifactError.Code) == "" || strings.TrimSpace(artifactError.Message) == "" {
		return fmt.Errorf("%s must contain code and message", field)
	}
	if requireRetryable && artifactError.Retryable == nil {
		return fmt.Errorf("%s must contain code, message, and retryable", field)
	}
	return nil
}

// WriteManifest validates and creates manifest files with owner-only permissions.
func WriteManifest(path string, manifest *TaskManifest) error {
	if manifest == nil {
		return errors.New("task manifest is nil")
	}
	return manifest.WriteManifest(path)
}

// WriteManifest validates and writes the receiver to path.
func (manifest *TaskManifest) WriteManifest(path string) error {
	if err := manifest.ValidateManifest(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write task manifest: %w", err)
	}
	return nil
}

// WriteManifestAtomic validates and replaces a manifest through an owner-only
// temporary file in the same directory. Existing files are never edited in place.
func WriteManifestAtomic(path string, manifest *TaskManifest) error {
	if manifest == nil {
		return errors.New("task manifest is nil")
	}
	if err := manifest.ValidateManifest(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task manifest: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".task-manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary task manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary task manifest: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary task manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary task manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary task manifest: %w", err)
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace task manifest: %w", err)
	}
	return nil
}

func rejectSensitiveFields(manifest *TaskManifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("inspect task manifest fields: %w", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("inspect task manifest fields: %w", err)
	}
	return inspectSensitiveKeys(value, "")
}

func inspectSensitiveKeys(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			if normalized == "api_key" || normalized == "apikey" || normalized == "api_secret" || normalized == "access_token" || normalized == "token" || normalized == "secret" || normalized == "password" || strings.Contains(normalized, "credential") {
				return fmt.Errorf("task manifest contains forbidden sensitive field %q", joinPath(path, key))
			}
			if err := inspectSensitiveKeys(child, joinPath(path, key)); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := inspectSensitiveKeys(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func unmarshalWithUnknown(data []byte, target any, knownKeys ...string) (map[string]json.RawMessage, error) {
	if err := json.Unmarshal(data, target); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, key := range knownKeys {
		delete(fields, key)
	}
	return fields, nil
}

func marshalWithUnknown(known any, unknown map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range unknown {
		if _, exists := fields[key]; !exists {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

func (manifest *TaskManifest) UnmarshalJSON(data []byte) error {
	type wireManifest struct {
		SchemaVersion       string               `json:"schema_version"`
		TaskID              string               `json:"task_id"`
		CreatedAt           string               `json:"created_at"`
		UpdatedAt           string               `json:"updated_at"`
		Execution           ExecutionMetadata    `json:"execution"`
		Input               InputMetadata        `json:"input"`
		Artifacts           Artifacts            `json:"artifacts"`
		Checkpoint          *CheckpointMetadata  `json:"checkpoint,omitempty"`
		UserMetadata        *UserMetadata        `json:"user_metadata,omitempty"`
		CanonicalTranscript *CanonicalTranscript `json:"canonical_transcript,omitempty"`
		Generations         []GenerationRecord   `json:"generations,omitempty"`
	}
	var wire wireManifest
	unknown, err := unmarshalWithUnknown(data, &wire, "schema_version", "task_id", "created_at", "updated_at", "execution", "input", "artifacts", "checkpoint", "user_metadata", "canonical_transcript", "generations")
	if err != nil {
		return err
	}
	manifest.SchemaVersion = wire.SchemaVersion
	manifest.TaskID = wire.TaskID
	manifest.CreatedAt = wire.CreatedAt
	manifest.UpdatedAt = wire.UpdatedAt
	manifest.UserMetadata = wire.UserMetadata
	manifest.UnknownFields = unknown
	manifest.CanonicalTranscript = wire.CanonicalTranscript
	manifest.Generations = wire.Generations
	manifest.Backend = wire.Execution.Backend
	manifest.Provider = wire.Execution.Provider
	manifest.OutputMode = wire.Execution.OutputMode
	manifest.RequestedTemplateID = wire.Execution.TemplateID
	manifest.Status = wire.Execution.Status
	manifest.State = wire.Execution.Stage
	manifest.Attempt = wire.Execution.Attempt
	manifest.CompletedAt = wire.Execution.CompletedAt
	manifest.ExecutionUnknownFields = wire.Execution.UnknownFields
	if wire.Execution.ErrorSummary != nil {
		retryable := wire.Execution.ErrorSummary.Retryable != nil && *wire.Execution.ErrorSummary.Retryable
		manifest.LastError = &ErrorInfo{Stage: wire.Execution.ErrorSummary.Code, Message: wire.Execution.ErrorSummary.Message, Retryable: retryable, At: wire.UpdatedAt, UnknownFields: wire.Execution.ErrorSummary.UnknownFields}
	}
	manifest.Source = SourceInfo{Path: wire.Input.SourceReference, OriginalName: wire.Input.OriginalName, SizeBytes: wire.Input.SizeBytes}
	manifest.SourceOwnership = wire.Input.SourceOwnership
	manifest.DurationSeconds = wire.Input.DurationSeconds
	manifest.InputFormat = wire.Input.Format
	manifest.SourceMetadata = wire.Input.SourceMetadata
	manifest.InputUnknownFields = wire.Input.UnknownFields
	manifest.Artifacts = wire.Artifacts
	if wire.Checkpoint != nil {
		manifest.IntegrityVersion = wire.Checkpoint.IntegrityVersion
		manifest.Chunks = wire.Checkpoint.Chunks
		manifest.CheckpointUnknownFields = wire.Checkpoint.UnknownFields
	}
	return nil
}

func (manifest TaskManifest) MarshalJSON() ([]byte, error) {
	type wireManifest struct {
		SchemaVersion       string               `json:"schema_version"`
		TaskID              string               `json:"task_id"`
		CreatedAt           string               `json:"created_at"`
		UpdatedAt           string               `json:"updated_at"`
		Execution           ExecutionMetadata    `json:"execution"`
		Input               InputMetadata        `json:"input"`
		Artifacts           Artifacts            `json:"artifacts"`
		Checkpoint          *CheckpointMetadata  `json:"checkpoint,omitempty"`
		UserMetadata        *UserMetadata        `json:"user_metadata,omitempty"`
		CanonicalTranscript *CanonicalTranscript `json:"canonical_transcript,omitempty"`
		Generations         []GenerationRecord   `json:"generations,omitempty"`
	}
	wire := wireManifest{
		SchemaVersion: manifest.SchemaVersion,
		TaskID:        manifest.TaskID,
		CreatedAt:     manifest.CreatedAt,
		UpdatedAt:     manifest.UpdatedAt,
		Execution: ExecutionMetadata{
			Backend: manifest.Backend, Provider: manifest.Provider, OutputMode: manifest.OutputMode,
			TemplateID: manifest.RequestedTemplateID,
			Status:     manifest.Status, Stage: manifest.State, Attempt: manifest.Attempt,
			CompletedAt: manifest.CompletedAt, ErrorSummary: manifest.errorSummary(),
			UnknownFields: manifest.ExecutionUnknownFields,
		},
		Input: InputMetadata{
			SourceReference: manifest.Source.Path, SourceOwnership: manifest.SourceOwnership,
			DurationSeconds: manifest.DurationSeconds, Format: manifest.InputFormat,
			OriginalName: manifest.Source.OriginalName, SizeBytes: manifest.Source.SizeBytes,
			SourceMetadata: manifest.SourceMetadata, UnknownFields: manifest.InputUnknownFields,
		},
		Artifacts: manifest.Artifacts,
		Checkpoint: &CheckpointMetadata{
			IntegrityVersion: manifest.IntegrityVersion, Chunks: manifest.Chunks,
			UnknownFields: manifest.CheckpointUnknownFields,
		},
		UserMetadata:        manifest.UserMetadata,
		CanonicalTranscript: manifest.CanonicalTranscript,
		Generations:         manifest.Generations,
	}
	return marshalWithUnknown(wire, manifest.UnknownFields)
}

func (manifest TaskManifest) errorSummary() *ArtifactError {
	if manifest.LastError == nil {
		return nil
	}
	retryable := manifest.LastError.Retryable
	return &ArtifactError{Code: manifest.LastError.Stage, Message: manifest.LastError.Message, Retryable: &retryable, UnknownFields: manifest.LastError.UnknownFields}
}

func (manifest *TaskManifest) ChunkByID(id string) *ChunkItem {
	for i := range manifest.Chunks.Items {
		if manifest.Chunks.Items[i].ID == id {
			return &manifest.Chunks.Items[i]
		}
	}
	return nil
}

func (manifest *TaskManifest) CountCompletedChunks() int {
	count := 0
	for i := range manifest.Chunks.Items {
		if manifest.Chunks.Items[i].State == "COMPLETED" {
			count++
		}
	}
	return count
}

func (metadata *ExecutionMetadata) UnmarshalJSON(data []byte) error {
	type plain ExecutionMetadata
	unknown, err := unmarshalWithUnknown(data, (*plain)(metadata), "backend", "provider", "output_mode", "template_id", "status", "stage", "attempt", "completed_at", "error_summary")
	metadata.UnknownFields = unknown
	return err
}

func (metadata ExecutionMetadata) MarshalJSON() ([]byte, error) {
	type plain ExecutionMetadata
	return marshalWithUnknown(plain(metadata), metadata.UnknownFields)
}

func (metadata *InputMetadata) UnmarshalJSON(data []byte) error {
	type plain InputMetadata
	unknown, err := unmarshalWithUnknown(data, (*plain)(metadata), "source_reference", "source_ownership", "duration_seconds", "format", "original_name", "size_bytes", "metadata")
	metadata.UnknownFields = unknown
	return err
}

func (metadata InputMetadata) MarshalJSON() ([]byte, error) {
	type plain InputMetadata
	return marshalWithUnknown(plain(metadata), metadata.UnknownFields)
}

func (artifact *Artifact) UnmarshalJSON(data []byte) error {
	type plain Artifact
	unknown, err := unmarshalWithUnknown(data, (*plain)(artifact), "status", "path", "sha256", "error")
	artifact.UnknownFields = unknown
	return err
}

func (artifact Artifact) MarshalJSON() ([]byte, error) {
	type plain Artifact
	return marshalWithUnknown(plain(artifact), artifact.UnknownFields)
}

func (artifacts *Artifacts) UnmarshalJSON(data []byte) error {
	type plain Artifacts
	unknown, err := unmarshalWithUnknown(data, (*plain)(artifacts), "transcript", "notes", "notes_input", "minutes")
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil && artifacts.NotesInput.State == "" {
		if raw, ok := fields["notes_input"]; ok {
			if err := json.Unmarshal(raw, &artifacts.NotesInput); err != nil {
				return err
			}
		}
	}
	artifacts.UnknownFields = unknown
	return nil
}

func (artifacts Artifacts) MarshalJSON() ([]byte, error) {
	type plain Artifacts
	return marshalWithUnknown(plain(artifacts), artifacts.UnknownFields)
}

func (artifactError *ArtifactError) UnmarshalJSON(data []byte) error {
	type plain ArtifactError
	unknown, err := unmarshalWithUnknown(data, (*plain)(artifactError), "code", "message", "retryable")
	artifactError.UnknownFields = unknown
	return err
}

func (artifactError ArtifactError) MarshalJSON() ([]byte, error) {
	type plain ArtifactError
	return marshalWithUnknown(plain(artifactError), artifactError.UnknownFields)
}

func (metadata *UserMetadata) UnmarshalJSON(data []byte) error {
	type plain UserMetadata
	unknown, err := unmarshalWithUnknown(data, (*plain)(metadata), "title", "description", "tags", "favorite", "annotations")
	metadata.UnknownFields = unknown
	return err
}

func (metadata UserMetadata) MarshalJSON() ([]byte, error) {
	type plain UserMetadata
	if metadata.Tags == nil {
		metadata.Tags = make([]string, 0)
	}
	return marshalWithUnknown(plain(metadata), metadata.UnknownFields)
}

func (transcript *CanonicalTranscript) UnmarshalJSON(data []byte) error {
	type plain CanonicalTranscript
	unknown, err := unmarshalWithUnknown(data, (*plain)(transcript), "transcript_id", "revision_id", "artifact_reference", "content_sha256", "hash_contract_version", "created_at", "asr_provider", "asr_model", "status")
	transcript.UnknownFields = unknown
	return err
}

func (transcript CanonicalTranscript) MarshalJSON() ([]byte, error) {
	type plain CanonicalTranscript
	return marshalWithUnknown(plain(transcript), transcript.UnknownFields)
}

func (generation *GenerationRecord) UnmarshalJSON(data []byte) error {
	type plain GenerationRecord
	unknown, err := unmarshalWithUnknown(data, (*plain)(generation), "generation_id", "template_id", "template_revision", "template_display_name", "template_instructions", "created_at", "updated_at", "source_transcript_revision_id", "source_transcript_sha256", "llm_provider", "llm_model", "prompt_composition_version", "prompt_hash", "status", "artifact_reference", "error", "attempts")
	generation.UnknownFields = unknown
	return err
}

func (generation GenerationRecord) MarshalJSON() ([]byte, error) {
	type plain GenerationRecord
	return marshalWithUnknown(plain(generation), generation.UnknownFields)
}
