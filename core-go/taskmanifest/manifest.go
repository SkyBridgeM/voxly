// Package taskmanifest provides the versioned Task manifest data model.
// It is intentionally not wired into the existing runtime manifest yet.
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
	Artifacts     map[string]Artifact        `json:"artifacts"`
	UserMetadata  *UserMetadata              `json:"user_metadata,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

type ExecutionMetadata struct {
	Backend       string                     `json:"backend"`
	Provider      string                     `json:"provider"`
	OutputMode    string                     `json:"output_mode"`
	Status        string                     `json:"status"`
	ErrorSummary  *ArtifactError             `json:"error_summary,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
}

type InputMetadata struct {
	SourceReference string                     `json:"source_reference"`
	SourceOwnership string                     `json:"source_ownership"`
	DurationSeconds *float64                   `json:"duration_seconds,omitempty"`
	Format          string                     `json:"format"`
	SourceMetadata  map[string]json.RawMessage `json:"metadata,omitempty"`
	UnknownFields   map[string]json.RawMessage `json:"-"`
}

type Artifact struct {
	Status        string                     `json:"status"`
	Path          string                     `json:"path,omitempty"`
	Error         *ArtifactError             `json:"error,omitempty"`
	UnknownFields map[string]json.RawMessage `json:"-"`
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
	if len(manifest.Artifacts) == 0 {
		return errors.New("task manifest missing artifacts")
	}
	for name, artifact := range manifest.Artifacts {
		if strings.TrimSpace(name) == "" {
			return errors.New("task manifest contains artifact with empty name")
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
	if _, ok := artifactStatuses[artifact.Status]; !ok {
		return fmt.Errorf("artifact %q has invalid status %q", name, artifact.Status)
	}
	if artifact.Status == "completed" && strings.TrimSpace(artifact.Path) == "" {
		return fmt.Errorf("completed artifact %q missing path", name)
	}
	if artifact.Status != "completed" && artifact.Path != "" {
		return fmt.Errorf("artifact %q with status %q must not have a completed path", name, artifact.Status)
	}
	if artifact.Status == "failed" {
		if artifact.Error == nil {
			return fmt.Errorf("failed artifact %q missing structured error", name)
		}
		if err := artifact.Error.validate("artifact "+name+".error", true); err != nil {
			return err
		}
	} else if artifact.Error != nil {
		return fmt.Errorf("artifact %q with status %q must not have an error", name, artifact.Status)
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
	unknown, err := unmarshalWithUnknown(data, (*plain)(manifest), "schema_version", "task_id", "created_at", "updated_at", "execution", "input", "artifacts", "user_metadata")
	manifest.UnknownFields = unknown
	return err
}

func (manifest TaskManifest) MarshalJSON() ([]byte, error) {
	type plain TaskManifest
	return marshalWithUnknown(plain(manifest), manifest.UnknownFields)
}

func (metadata *ExecutionMetadata) UnmarshalJSON(data []byte) error {
	type plain ExecutionMetadata
	unknown, err := unmarshalWithUnknown(data, (*plain)(metadata), "backend", "provider", "output_mode", "status", "error_summary")
	metadata.UnknownFields = unknown
	return err
}

func (metadata ExecutionMetadata) MarshalJSON() ([]byte, error) {
	type plain ExecutionMetadata
	return marshalWithUnknown(plain(metadata), metadata.UnknownFields)
}

func (metadata *InputMetadata) UnmarshalJSON(data []byte) error {
	type plain InputMetadata
	unknown, err := unmarshalWithUnknown(data, (*plain)(metadata), "source_reference", "source_ownership", "duration_seconds", "format", "metadata")
	metadata.UnknownFields = unknown
	return err
}

func (metadata InputMetadata) MarshalJSON() ([]byte, error) {
	type plain InputMetadata
	return marshalWithUnknown(plain(metadata), metadata.UnknownFields)
}

func (artifact *Artifact) UnmarshalJSON(data []byte) error {
	type plain Artifact
	unknown, err := unmarshalWithUnknown(data, (*plain)(artifact), "status", "path", "error")
	artifact.UnknownFields = unknown
	return err
}

func (artifact Artifact) MarshalJSON() ([]byte, error) {
	type plain Artifact
	return marshalWithUnknown(plain(artifact), artifact.UnknownFields)
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
