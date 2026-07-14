package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const currentSchemaVersion = taskmanifest.CurrentSchemaVersion

// TaskManifest is the runtime's only durable task model. Manifest remains an
// alias for existing workflow helper signatures; it is not a legacy format.
type TaskManifest = taskmanifest.TaskManifest
type Manifest = TaskManifest
type SourceInfo = taskmanifest.SourceInfo
type ChunksInfo = taskmanifest.ChunksInfo
type ChunkItem = taskmanifest.ChunkItem
type Artifacts = taskmanifest.Artifacts
type ArtifactInfo = taskmanifest.Artifact
type ArtifactError = taskmanifest.ArtifactError
type ErrorInfo = taskmanifest.ErrorInfo

// legacyManifest is the flat manifest format written before the resume state
// machine was introduced. Keep it separate from Manifest so new manifests
// never re-emit deprecated fields.
type legacyManifest struct {
	Version      string  `json:"version"`
	SourceAudio  string  `json:"source_audio"`
	OutputMode   string  `json:"output_mode"`
	ChunkMinutes int     `json:"chunk_minutes"`
	MaxUploadMB  float64 `json:"max_upload_mb"`
	ASRProvider  string  `json:"asr_provider"`
	ASRModel     string  `json:"asr_model"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	Status       string  `json:"status"`
	Error        *string `json:"error"`
}

// ManifestPath returns the path to task_manifest.json in a task directory.
func ManifestPath(taskDir string) string {
	return filepath.Join(taskDir, taskmanifest.ManifestFilename)
}

func legacyManifestPath(taskDir string) string {
	return filepath.Join(taskDir, "manifest.json")
}

// LoadManifest reads and validates task_manifest.json from a task directory.
func LoadManifest(taskDir string) (*Manifest, error) {
	m, err := taskmanifest.LoadManifest(ManifestPath(taskDir))
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return m, nil
}

// LoadLegacyManifest loads the pre-state-machine manifest format. Callers
// must migrate it before using it as a task checkpoint.
func LoadLegacyManifest(taskDir string) (*legacyManifest, error) {
	data, err := os.ReadFile(legacyManifestPath(taskDir))
	if err != nil {
		return nil, fmt.Errorf("reading legacy manifest: %w", err)
	}
	var m legacyManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing legacy manifest: %w", err)
	}
	return &m, nil
}

// SaveManifest atomically writes task_manifest.json to a task directory.
func SaveManifest(taskDir string, m *Manifest) error {
	m.SchemaVersion = currentSchemaVersion
	m.UpdatedAt = nowFunc().Format(time.RFC3339)
	if m.Backend == "" {
		m.Backend = "go"
	}
	if m.Provider == "" {
		m.Provider = m.Chunks.ASRProvider
		if m.Provider == "" {
			m.Provider = "unknown"
		}
	}
	m.Status = taskStatus(m)
	if m.SourceOwnership == "" {
		m.SourceOwnership = "task_copy"
	}
	if m.InputFormat == "" {
		m.InputFormat = inputFormat(m.Source.Path)
	}
	if err := taskmanifest.ValidateManifest(m); err != nil {
		return fmt.Errorf("validating task manifest: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	temp, err := os.CreateTemp(taskDir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary manifest: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("setting manifest permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("writing temporary manifest: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("syncing temporary manifest: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing temporary manifest: %w", err)
	}
	if err := replaceFileAtomically(tempPath, ManifestPath(taskDir)); err != nil {
		return fmt.Errorf("replacing manifest: %w", err)
	}
	return nil
}

// CreateManifest creates a new manifest for a fresh transcription task.
func CreateManifest(taskDir, sourceAudio, outputMode string, chunkMinutes int, maxUploadMB float64, asrProvider, asrModel string) *Manifest {
	now := nowFunc().Format(time.RFC3339)
	info, _ := os.Stat(sourceAudio)
	var sizeBytes int64
	if info != nil {
		sizeBytes = info.Size()
	}

	return &Manifest{
		SchemaVersion:    currentSchemaVersion,
		IntegrityVersion: 1,
		TaskID:           filepath.Base(taskDir),
		OutputMode:       outputMode,
		Backend:          "go",
		Provider:         asrProvider,
		Status:           "created",
		Source: SourceInfo{
			Path:         sourceAudio,
			OriginalName: filepath.Base(sourceAudio),
			SizeBytes:    sizeBytes,
		},
		SourceOwnership: "task_copy",
		InputFormat:     inputFormat(sourceAudio),
		State:           TaskCreated,
		Attempt:         1,
		Chunks: ChunksInfo{
			Format:       "",
			ChunkMinutes: chunkMinutes,
			MaxUploadMB:  maxUploadMB,
			ASRProvider:  asrProvider,
			ASRModel:     asrModel,
			Total:        0,
			Items:        nil,
		},
		Artifacts: Artifacts{
			Transcript: ArtifactInfo{Path: filepath.Join(taskDir, "transcript.md"), State: ArtifactPending},
			NotesInput: ArtifactInfo{Path: filepath.Join(taskDir, "notes_input.md"), State: requestedArtifactState(outputMode == "notes" || outputMode == "minutes")},
			Minutes:    ArtifactInfo{Path: filepath.Join(taskDir, "meeting_minutes.md"), State: requestedArtifactState(outputMode == "minutes")},
		},
		LastError:   nil,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: nil,
	}
}

func requestedArtifactState(requested bool) ArtifactState {
	if requested {
		return ArtifactPending
	}
	return ArtifactInvalid
}

func taskStatus(m *Manifest) string {
	if m.State == TaskCreated {
		return "created"
	}
	if m.State == TaskCompleted {
		for _, artifact := range requiredArtifacts(m) {
			if artifact.State != ArtifactReady {
				return "processing"
			}
		}
		return "completed"
	}
	if m.State == TaskFailed {
		completed := false
		failed := false
		for _, artifact := range requiredArtifacts(m) {
			completed = completed || artifact.State == ArtifactReady
			failed = failed || artifact.State == ArtifactFailed
		}
		if completed && failed {
			return "completed_with_errors"
		}
		return "failed"
	}
	return "processing"
}

func requiredArtifacts(m *Manifest) []ArtifactInfo {
	artifacts := []ArtifactInfo{m.Artifacts.Transcript}
	if m.OutputMode == "notes" || m.OutputMode == "minutes" {
		artifacts = append(artifacts, m.Artifacts.NotesInput)
	}
	if m.OutputMode == "minutes" {
		artifacts = append(artifacts, m.Artifacts.Minutes)
	}
	return artifacts
}

func inputFormat(path string) string {
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if format == "" {
		return "unknown"
	}
	return format
}

// SetTaskState updates the task state and persists the manifest.
func SetTaskState(taskDir string, m *Manifest, state TaskState) error {
	m.State = string(state)
	return SaveManifest(taskDir, m)
}

// RecordTaskError records an error and transitions the task to FAILED.
func RecordTaskError(taskDir string, m *Manifest, stage TaskState, err error, retryable bool) error {
	now := nowFunc().Format(time.RFC3339)
	m.State = string(TaskFailed)
	m.LastError = &ErrorInfo{
		Stage:     string(stage),
		Message:   err.Error(),
		Retryable: retryable,
		At:        now,
	}
	markArtifactFailed(m, stage, err, retryable)
	return SaveManifest(taskDir, m)
}

func markArtifactProcessing(artifact *ArtifactInfo) {
	artifact.State = ArtifactProcessing
	artifact.Error = nil
}

func markArtifactFailed(m *Manifest, stage TaskState, err error, retryable bool) {
	var artifact *ArtifactInfo
	switch stage {
	case TaskMerging:
		artifact = &m.Artifacts.Transcript
		m.Artifacts.NotesInput.State = ArtifactInvalid
		m.Artifacts.NotesInput.Error = nil
		m.Artifacts.Minutes.State = ArtifactInvalid
		m.Artifacts.Minutes.Error = nil
	case TaskNotesGenerating:
		artifact = &m.Artifacts.NotesInput
		m.Artifacts.Minutes.State = ArtifactInvalid
		m.Artifacts.Minutes.Error = nil
	case TaskMinutesGenerating:
		artifact = &m.Artifacts.Minutes
	default:
		return
	}
	artifact.State = ArtifactFailed
	artifact.Error = &taskmanifest.ArtifactError{Code: string(stage), Message: err.Error(), Retryable: &retryable}
}

// PrepareArtifactsForResume is the only path allowed to transition a failed
// artifact back to pending. Non-retryable failures remain durable blockers.
func PrepareArtifactsForResume(m *Manifest) error {
	requested := []struct {
		name     string
		required bool
		artifact *ArtifactInfo
	}{
		{name: "transcript", required: true, artifact: &m.Artifacts.Transcript},
		{name: "notes", required: m.OutputMode == "notes" || m.OutputMode == "minutes", artifact: &m.Artifacts.NotesInput},
		{name: "minutes", required: m.OutputMode == "minutes", artifact: &m.Artifacts.Minutes},
	}
	for _, item := range requested {
		if !item.required || item.artifact.State != ArtifactFailed {
			continue
		}
		if item.artifact.Error == nil || item.artifact.Error.Retryable == nil || !*item.artifact.Error.Retryable {
			return fmt.Errorf("%s artifact failed and is not retryable", item.name)
		}
		item.artifact.State = ArtifactPending
		item.artifact.Error = nil
	}
	return nil
}

func IsRetryableError(err error) bool {
	var clientErr *httpClientError
	if errors.As(err, &clientErr) {
		return clientErr.isRetriable()
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return networkErr.Timeout() || networkErr.Temporary()
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "rate limit") || strings.Contains(message, "server error") || strings.Contains(message, "timeout")
}

// MarkTaskCompleted transitions the task to COMPLETED and records the completion time.
func MarkTaskCompleted(taskDir string, m *Manifest) error {
	now := nowFunc().Format(time.RFC3339)
	m.State = string(TaskCompleted)
	m.CompletedAt = &now
	m.LastError = nil
	return SaveManifest(taskDir, m)
}

// ValidateManifest checks that the manifest has valid schema, required fields,
// and safe task-local paths. All paths must remain within the task directory.
func ValidateManifest(taskDir string, m *Manifest) error {
	if m.SchemaVersion != currentSchemaVersion {
		return fmt.Errorf("unsupported schema version %q (expected %q)", m.SchemaVersion, currentSchemaVersion)
	}
	if m.IntegrityVersion != 0 && m.IntegrityVersion != 1 {
		return fmt.Errorf("unsupported integrity_version %d", m.IntegrityVersion)
	}
	if m.TaskID == "" {
		return fmt.Errorf("manifest missing task_id")
	}
	if m.OutputMode != "transcript" && m.OutputMode != "notes" && m.OutputMode != "minutes" {
		return fmt.Errorf("invalid output_mode %q", m.OutputMode)
	}
	if !validTaskStates[m.State] {
		return fmt.Errorf("invalid task state %q", m.State)
	}

	absTaskDir, err := filepath.Abs(taskDir)
	if err != nil {
		return fmt.Errorf("resolving task directory: %w", err)
	}

	// Validate source path is within task directory.
	if m.Source.Path != "" {
		if err := pathIsInside(absTaskDir, m.Source.Path, "source"); err != nil {
			return err
		}
	}

	// Validate chunk IDs, states, integrity metadata, and paths.
	seenChunkIDs := make(map[string]struct{}, len(m.Chunks.Items))
	for _, item := range m.Chunks.Items {
		if err := validateChunkID(item.ID); err != nil {
			return err
		}
		if _, exists := seenChunkIDs[item.ID]; exists {
			return fmt.Errorf("duplicate chunk ID %q", item.ID)
		}
		seenChunkIDs[item.ID] = struct{}{}
		if !validChunkStates[item.State] {
			return fmt.Errorf("invalid state %q for chunk %s", item.State, item.ID)
		}
		if err := validateSHA256(item.AudioSHA256, "audio hash for "+item.ID); err != nil {
			return err
		}
		if err := validateSHA256(item.TranscriptSHA256, "transcript hash for "+item.ID); err != nil {
			return err
		}
		if item.AudioPath != "" {
			if err := pathIsInside(absTaskDir, item.AudioPath, "chunk audio"); err != nil {
				return err
			}
		}
		if item.TranscriptPath != "" {
			if err := pathIsInside(absTaskDir, item.TranscriptPath, "chunk transcript"); err != nil {
				return err
			}
		}
	}
	if m.Chunks.Total != len(m.Chunks.Items) {
		return fmt.Errorf("chunks.total is %d but manifest contains %d items", m.Chunks.Total, len(m.Chunks.Items))
	}
	if len(m.Chunks.Items) > 0 && m.Chunks.Format != "mp3" && m.Chunks.Format != "wav" {
		return fmt.Errorf("invalid chunk format %q", m.Chunks.Format)
	}

	// Validate artifact states, hashes, and paths.
	for name, artifact := range map[string]ArtifactInfo{
		"transcript":  m.Artifacts.Transcript,
		"notes_input": m.Artifacts.NotesInput,
		"minutes":     m.Artifacts.Minutes,
	} {
		if artifact.State != "" && !validArtifactStates[artifact.State] {
			return fmt.Errorf("invalid state %q for %s artifact", artifact.State, name)
		}
		if err := validateSHA256(artifact.SHA256, name+" artifact hash"); err != nil {
			return err
		}
		if artifact.Path != "" {
			if err := pathIsInside(absTaskDir, artifact.Path, name); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateSHA256(value, label string) error {
	if value == "" {
		return nil
	}
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("invalid %s: expected 64 hexadecimal characters", label)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("invalid %s: %w", label, err)
	}
	return nil
}

// validateChunkID rejects chunk IDs that could escape the task directory.
func validateChunkID(id string) error {
	if id == "" {
		return fmt.Errorf("empty chunk ID")
	}
	if strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("chunk ID %q contains path separator", id)
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("chunk ID %q contains path traversal", id)
	}
	return nil
}

// pathIsInside verifies that a path resolves inside the expected parent directory.
func pathIsInside(parentDir, child, label string) error {
	absChild, err := filepath.Abs(child)
	if err != nil {
		return fmt.Errorf("resolving %s path %q: %w", label, child, err)
	}
	rel, err := filepath.Rel(parentDir, absChild)
	if err != nil {
		return fmt.Errorf("%s path %q is not relative to task directory", label, child)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%s path %q escapes task directory", label, child)
	}
	return nil
}
