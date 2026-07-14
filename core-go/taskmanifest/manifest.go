// Package taskmanifest provides the versioned Task manifest data model used by
// the runtime as its durable checkpoint and resume source of truth.
package taskmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const CurrentSchemaVersion = "1.0"

var ErrUnsupportedSchemaVersion = errors.New("unsupported task manifest schema version")

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
	Execution     ExecutionMetadata          `json:"execution"`
	Input         InputMetadata              `json:"input"`
	Artifacts     Artifacts                  `json:"artifacts"`
	Checkpoint    *CheckpointMetadata        `json:"checkpoint,omitempty"`
	UserMetadata  *UserMetadata              `json:"user_metadata,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`

	// Runtime checkpoint accessors are excluded from JSON. The custom codec
	// maps them to the versioned execution/input/checkpoint sections so runtime
	// code and persisted TaskManifest state cannot diverge.
	IntegrityVersion int        `json:"-"`
	OutputMode       string     `json:"-"`
	Source           SourceInfo `json:"-"`
	State            string     `json:"-"`
	Attempt          int        `json:"-"`
	Chunks           ChunksInfo `json:"-"`
	LastError        *ErrorInfo `json:"-"`
	CompletedAt      *string    `json:"-"`
}

type SourceInfo struct {
	Path         string `json:"path"`
	OriginalName string `json:"original_name"`
	SizeBytes    int64  `json:"size_bytes"`
}

type ErrorInfo struct {
	Stage     string `json:"stage"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	At        string `json:"at"`
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
	Title         string                     `json:"title,omitempty"`
	Description   string                     `json:"description,omitempty"`
	Tags          []string                   `json:"tags,omitempty"`
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
	if manifest.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("%w: %q", ErrUnsupportedSchemaVersion, manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.TaskID) == "" {
		return errors.New("task manifest missing task_id")
	}
	if strings.TrimSpace(manifest.CreatedAt) == "" || strings.TrimSpace(manifest.UpdatedAt) == "" {
		return errors.New("task manifest missing required timestamps")
	}
	if err := manifest.Execution.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.Input.SourceReference) == "" {
		return errors.New("task manifest missing input.source_reference")
	}
	if strings.TrimSpace(manifest.Input.SourceOwnership) == "" {
		return errors.New("task manifest missing input.source_ownership")
	}
	if strings.TrimSpace(manifest.Input.Format) == "" {
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
	if err := rejectSensitiveFields(manifest); err != nil {
		return err
	}
	return nil
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
	type plain TaskManifest
	unknown, err := unmarshalWithUnknown(data, (*plain)(manifest), "schema_version", "task_id", "created_at", "updated_at", "execution", "input", "artifacts", "checkpoint", "user_metadata")
	manifest.UnknownFields = unknown
	manifest.syncRuntime()
	return err
}

func (manifest TaskManifest) MarshalJSON() ([]byte, error) {
	manifest.syncCanonical()
	type plain TaskManifest
	return marshalWithUnknown(plain(manifest), manifest.UnknownFields)
}

func (manifest *TaskManifest) syncCanonical() {
	if manifest.OutputMode != "" {
		manifest.Execution.OutputMode = manifest.OutputMode
	}
	if manifest.State != "" {
		manifest.Execution.Stage = manifest.State
		manifest.Execution.Status = aggregateStatus(manifest.State)
	}
	if manifest.Attempt != 0 {
		manifest.Execution.Attempt = manifest.Attempt
	}
	if manifest.CompletedAt != nil {
		manifest.Execution.CompletedAt = manifest.CompletedAt
	}
	if manifest.LastError != nil {
		retryable := manifest.LastError.Retryable
		manifest.Execution.ErrorSummary = &ArtifactError{Code: manifest.LastError.Stage, Message: manifest.LastError.Message, Retryable: &retryable}
	} else if manifest.State != "FAILED" {
		manifest.Execution.ErrorSummary = nil
	}
	if manifest.Source.Path != "" {
		manifest.Input.SourceReference = manifest.Source.Path
		manifest.Input.OriginalName = manifest.Source.OriginalName
		manifest.Input.SizeBytes = manifest.Source.SizeBytes
	}
	if manifest.Input.SourceOwnership == "" {
		manifest.Input.SourceOwnership = "task_copy"
	}
	if manifest.Input.Format == "" {
		manifest.Input.Format = "unknown"
	}
	if manifest.Checkpoint == nil {
		manifest.Checkpoint = &CheckpointMetadata{}
	}
	manifest.Checkpoint.IntegrityVersion = manifest.IntegrityVersion
	manifest.Checkpoint.Chunks = manifest.Chunks
}

func (manifest *TaskManifest) syncRuntime() {
	manifest.OutputMode = manifest.Execution.OutputMode
	manifest.State = manifest.Execution.Stage
	manifest.Attempt = manifest.Execution.Attempt
	manifest.CompletedAt = manifest.Execution.CompletedAt
	manifest.Source = SourceInfo{Path: manifest.Input.SourceReference, OriginalName: manifest.Input.OriginalName, SizeBytes: manifest.Input.SizeBytes}
	if manifest.Checkpoint != nil {
		manifest.IntegrityVersion = manifest.Checkpoint.IntegrityVersion
		manifest.Chunks = manifest.Checkpoint.Chunks
	}
	if manifest.Execution.ErrorSummary != nil {
		retryable := manifest.Execution.ErrorSummary.Retryable != nil && *manifest.Execution.ErrorSummary.Retryable
		manifest.LastError = &ErrorInfo{Stage: manifest.Execution.ErrorSummary.Code, Message: manifest.Execution.ErrorSummary.Message, Retryable: retryable, At: manifest.UpdatedAt}
	}
}

func aggregateStatus(stage string) string {
	switch stage {
	case "CREATED":
		return "created"
	case "COMPLETED":
		return "completed"
	case "FAILED":
		return "failed"
	default:
		return "processing"
	}
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
	unknown, err := unmarshalWithUnknown(data, (*plain)(metadata), "backend", "provider", "output_mode", "status", "stage", "attempt", "completed_at", "error_summary")
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
	return marshalWithUnknown(plain(metadata), metadata.UnknownFields)
}
