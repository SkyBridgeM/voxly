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
)

const currentSchemaVersion = 1

// Manifest represents the task manifest stored in each task directory.
// This is the durable checkpoint for resume operations.
type Manifest struct {
	SchemaVersion    int        `json:"schema_version"`
	IntegrityVersion int        `json:"integrity_version"`
	TaskID           string     `json:"task_id"`
	OutputMode       string     `json:"output_mode"`
	Source           SourceInfo `json:"source"`
	State            TaskState  `json:"state"`
	Attempt          int        `json:"attempt"`
	Chunks           ChunksInfo `json:"chunks"`
	Artifacts        Artifacts  `json:"artifacts"`
	LastError        *ErrorInfo `json:"last_error"`
	CreatedAt        string     `json:"created_at"`
	UpdatedAt        string     `json:"updated_at"`
	CompletedAt      *string    `json:"completed_at"`
}

// SourceInfo describes the input audio file.
type SourceInfo struct {
	Path         string `json:"path"`
	OriginalName string `json:"original_name"`
	SizeBytes    int64  `json:"size_bytes"`
}

// ChunksInfo describes the chunk plan and per-chunk state.
type ChunksInfo struct {
	Format       string      `json:"format"`
	ChunkMinutes int         `json:"chunk_minutes"`
	MaxUploadMB  float64     `json:"max_upload_mb"`
	ASRProvider  string      `json:"asr_provider"`
	ASRModel     string      `json:"asr_model"`
	Total        int         `json:"total"`
	Items        []ChunkItem `json:"items"`
}

// ChunkItem describes one audio chunk and its transcription state.
type ChunkItem struct {
	ID               string     `json:"id"`
	AudioPath        string     `json:"audio_path"`
	TranscriptPath   string     `json:"transcript_path"`
	State            ChunkState `json:"state"`
	Attempts         int        `json:"attempts"`
	SizeBytes        int64      `json:"size_bytes"`
	AudioSHA256      string     `json:"audio_sha256"`
	TranscriptSHA256 string     `json:"transcript_sha256"`
	LastError        *string    `json:"last_error"`
}

// Artifacts describes the generated output files.
type Artifacts struct {
	Transcript ArtifactInfo `json:"transcript"`
	NotesInput ArtifactInfo `json:"notes_input"`
	Minutes    ArtifactInfo `json:"minutes"`
}

// ArtifactInfo describes one generated output file.
type ArtifactInfo struct {
	Path   string        `json:"path"`
	State  ArtifactState `json:"state"`
	SHA256 string        `json:"sha256"`
}

// ErrorInfo records the last error that occurred during the task.
type ErrorInfo struct {
	Stage     TaskState `json:"stage"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
	At        string    `json:"at"`
}

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
	return filepath.Join(taskDir, "task_manifest.json")
}

func legacyManifestPath(taskDir string) string {
	return filepath.Join(taskDir, "manifest.json")
}

// LoadManifest reads and validates task_manifest.json from a task directory.
func LoadManifest(taskDir string) (*Manifest, error) {
	path := ManifestPath(taskDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &m, nil
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
		Source: SourceInfo{
			Path:         sourceAudio,
			OriginalName: filepath.Base(sourceAudio),
			SizeBytes:    sizeBytes,
		},
		State:   TaskCreated,
		Attempt: 1,
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
			NotesInput: ArtifactInfo{Path: filepath.Join(taskDir, "notes_input.md"), State: ArtifactPending},
			Minutes:    ArtifactInfo{Path: filepath.Join(taskDir, "meeting_minutes.md"), State: ArtifactPending},
		},
		LastError:   nil,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: nil,
	}
}

// SetTaskState updates the task state and persists the manifest.
func SetTaskState(taskDir string, m *Manifest, state TaskState) error {
	m.State = state
	return SaveManifest(taskDir, m)
}

// RecordTaskError records an error and transitions the task to FAILED.
func RecordTaskError(taskDir string, m *Manifest, stage TaskState, err error, retryable bool) error {
	now := nowFunc().Format(time.RFC3339)
	m.State = TaskFailed
	m.LastError = &ErrorInfo{
		Stage:     stage,
		Message:   err.Error(),
		Retryable: retryable,
		At:        now,
	}
	return SaveManifest(taskDir, m)
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
	m.State = TaskCompleted
	m.CompletedAt = &now
	m.LastError = nil
	return SaveManifest(taskDir, m)
}

// ChunkByID returns a pointer to the chunk item with the given ID, or nil.
func (m *Manifest) ChunkByID(id string) *ChunkItem {
	for i := range m.Chunks.Items {
		if m.Chunks.Items[i].ID == id {
			return &m.Chunks.Items[i]
		}
	}
	return nil
}

// CountCompletedChunks returns the number of chunks in COMPLETED state.
func (m *Manifest) CountCompletedChunks() int {
	count := 0
	for i := range m.Chunks.Items {
		if m.Chunks.Items[i].State == ChunkCompleted {
			count++
		}
	}
	return count
}

// ValidateManifest checks that the manifest has valid schema, required fields,
// and safe task-local paths. All paths must remain within the task directory.
func ValidateManifest(taskDir string, m *Manifest) error {
	if m.SchemaVersion != currentSchemaVersion {
		return fmt.Errorf("unsupported schema version %d (expected %d)", m.SchemaVersion, currentSchemaVersion)
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
