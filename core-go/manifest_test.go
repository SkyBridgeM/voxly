package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateManifest(t *testing.T) {
	taskDir := t.TempDir()
	sourceAudio := "/path/to/audio.wav"
	outputMode := "transcript"
	chunkMinutes := 5
	maxUploadMB := 10.0
	asrProvider := "mimo"
	asrModel := "mimo-v2.5-asr"

	manifest := CreateManifest(taskDir, sourceAudio, outputMode, chunkMinutes, maxUploadMB, asrProvider, asrModel)

	if manifest.SchemaVersion != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, manifest.SchemaVersion)
	}
	if manifest.TaskID != filepath.Base(taskDir) {
		t.Errorf("expected task ID %q, got %q", filepath.Base(taskDir), manifest.TaskID)
	}
	if manifest.OutputMode != outputMode {
		t.Errorf("expected output mode %q, got %q", outputMode, manifest.OutputMode)
	}
	if manifest.Source.Path != sourceAudio {
		t.Errorf("expected source path %q, got %q", sourceAudio, manifest.Source.Path)
	}
	if manifest.State != TaskCreated {
		t.Errorf("expected state %q, got %q", TaskCreated, manifest.State)
	}
	if manifest.Chunks.ChunkMinutes != chunkMinutes {
		t.Errorf("expected chunk minutes %d, got %d", chunkMinutes, manifest.Chunks.ChunkMinutes)
	}
	if manifest.Chunks.MaxUploadMB != maxUploadMB {
		t.Errorf("expected max upload MB %f, got %f", maxUploadMB, manifest.Chunks.MaxUploadMB)
	}
	if manifest.Chunks.ASRProvider != asrProvider {
		t.Errorf("expected ASR provider %q, got %q", asrProvider, manifest.Chunks.ASRProvider)
	}
	if manifest.Chunks.ASRModel != asrModel {
		t.Errorf("expected ASR model %q, got %q", asrModel, manifest.Chunks.ASRModel)
	}
	if manifest.Chunks.Total != 0 {
		t.Errorf("expected total chunks 0, got %d", manifest.Chunks.Total)
	}
	if len(manifest.Chunks.Items) != 0 {
		t.Errorf("expected 0 chunk items, got %d", len(manifest.Chunks.Items))
	}
	if manifest.Artifacts.Transcript.State != ArtifactPending {
		t.Errorf("expected transcript artifact pending, got %q", manifest.Artifacts.Transcript.State)
	}
	if manifest.LastError != nil {
		t.Errorf("expected nil last error, got %v", manifest.LastError)
	}
	if manifest.CompletedAt != nil {
		t.Errorf("expected nil completed_at, got %v", manifest.CompletedAt)
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	taskDir := t.TempDir()

	manifest := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "mimo-v2.5-asr")
	manifest.Chunks.Items = []ChunkItem{
		{ID: "part_001", AudioPath: "/chunks/part_001.wav", State: ChunkCompleted},
	}

	if err := SaveManifest(taskDir, manifest); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version mismatch: expected %d, got %d", currentSchemaVersion, loaded.SchemaVersion)
	}
	if loaded.TaskID != manifest.TaskID {
		t.Errorf("task ID mismatch: expected %q, got %q", manifest.TaskID, loaded.TaskID)
	}
	if loaded.Source.Path != manifest.Source.Path {
		t.Errorf("source path mismatch: expected %q, got %q", manifest.Source.Path, loaded.Source.Path)
	}
	if loaded.State != manifest.State {
		t.Errorf("state mismatch: expected %q, got %q", manifest.State, loaded.State)
	}
	if loaded.Chunks.Total != manifest.Chunks.Total {
		t.Errorf("total chunks mismatch: expected %d, got %d", manifest.Chunks.Total, loaded.Chunks.Total)
	}
	if len(loaded.Chunks.Items) != 1 {
		t.Fatalf("expected 1 chunk item, got %d", len(loaded.Chunks.Items))
	}
	if loaded.Chunks.Items[0].ID != "part_001" {
		t.Errorf("chunk item ID mismatch: %q", loaded.Chunks.Items[0].ID)
	}
}

func TestSetTaskState(t *testing.T) {
	taskDir := t.TempDir()
	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	if err := SetTaskState(taskDir, m, TaskTranscribing); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != TaskTranscribing {
		t.Errorf("expected state %q, got %q", TaskTranscribing, loaded.State)
	}
}

func TestRecordTaskError(t *testing.T) {
	taskDir := t.TempDir()
	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	testErr := os.ErrNotExist
	if err := RecordTaskError(taskDir, m, TaskTranscribing, testErr, true); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != TaskFailed {
		t.Errorf("expected state %q, got %q", TaskFailed, loaded.State)
	}
	if loaded.LastError == nil {
		t.Fatal("expected non-nil last error")
	}
	if loaded.LastError.Stage != TaskTranscribing {
		t.Errorf("expected error stage %q, got %q", TaskTranscribing, loaded.LastError.Stage)
	}
	if !loaded.LastError.Retryable {
		t.Error("expected retryable error")
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "rate limit", err: &httpClientError{statusCode: 429, message: "rate limit"}, want: true},
		{name: "request timeout", err: &httpClientError{statusCode: 408, message: "timeout"}, want: true},
		{name: "authentication", err: &httpClientError{statusCode: 401, message: "unauthorized"}, want: false},
		{name: "server error", err: fmt.Errorf("DeepSeek server error 503"), want: true},
		{name: "validation", err: fmt.Errorf("invalid output mode"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.want {
				t.Fatalf("IsRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarkTaskCompleted(t *testing.T) {
	taskDir := t.TempDir()
	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	if err := MarkTaskCompleted(taskDir, m); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != TaskCompleted {
		t.Errorf("expected state %q, got %q", TaskCompleted, loaded.State)
	}
	if loaded.CompletedAt == nil {
		t.Error("expected non-nil completed_at")
	}
	if loaded.LastError != nil {
		t.Errorf("expected nil last error, got %v", loaded.LastError)
	}
}

func TestManifestPath(t *testing.T) {
	taskDir := "/path/to/task"
	expected := filepath.Join(taskDir, "task_manifest.json")
	result := ManifestPath(taskDir)

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestLoadManifest_NotFound(t *testing.T) {
	taskDir := t.TempDir()

	_, err := LoadManifest(taskDir)
	if err == nil {
		t.Fatal("expected error for non-existent manifest, got nil")
	}
}

func TestSaveManifestReplacesFileWithoutLeavingTemporaryFile(t *testing.T) {
	taskDir := t.TempDir()
	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10, "mimo", "model-a")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}
	m.State = TaskCompleted
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != TaskCompleted {
		t.Fatalf("state = %q, want %q", loaded.State, TaskCompleted)
	}
	temps, err := filepath.Glob(filepath.Join(taskDir, ".manifest-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary manifest files were not cleaned up: %v", temps)
	}
}

func TestSaveManifestDoesNotOverwriteCorruptManifest(t *testing.T) {
	taskDir := t.TempDir()
	path := ManifestPath(taskDir)
	original := "{not-json"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(taskDir)
	if err == nil || !strings.Contains(err.Error(), "parsing manifest") {
		t.Fatalf("expected parsing error, got %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != original {
		t.Fatalf("corrupt manifest was unexpectedly overwritten: %q", data)
	}
}

func TestManifestAtomicWrite(t *testing.T) {
	taskDir := t.TempDir()
	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(ManifestPath(taskDir))
	if err != nil {
		t.Fatal(err)
	}
	var loaded Manifest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}

	temps, _ := filepath.Glob(filepath.Join(taskDir, ".manifest-*.tmp"))
	if len(temps) != 0 {
		t.Fatalf("temporary files found: %v", temps)
	}
}

func TestManifestProgressTracking_FullLifecycle(t *testing.T) {
	taskDir := t.TempDir()

	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// After splitting.
	m.Chunks.Total = 4
	m.Chunks.Items = []ChunkItem{
		{ID: "part_001", State: ChunkPending},
		{ID: "part_002", State: ChunkPending},
		{ID: "part_003", State: ChunkPending},
		{ID: "part_004", State: ChunkPending},
	}
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// After each transcription.
	for i := range m.Chunks.Items {
		m.Chunks.Items[i].State = ChunkCompleted
	}
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// After merge.
	m.Artifacts.Transcript.State = ArtifactReady
	m.Artifacts.Transcript.Path = filepath.Join(taskDir, "transcript.md")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// Mark completed.
	if err := MarkTaskCompleted(taskDir, m); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Chunks.Total != 4 {
		t.Errorf("total chunks: expected 4, got %d", loaded.Chunks.Total)
	}
	completed := loaded.CountCompletedChunks()
	if completed != 4 {
		t.Errorf("completed chunks: expected 4, got %d", completed)
	}
	if loaded.Artifacts.Transcript.State != ArtifactReady {
		t.Errorf("transcript state: expected %q, got %q", ArtifactReady, loaded.Artifacts.Transcript.State)
	}
	if loaded.State != TaskCompleted {
		t.Errorf("state: expected %q, got %q", TaskCompleted, loaded.State)
	}
}

func TestValidateManifest(t *testing.T) {
	taskDir := t.TempDir()
	tests := []struct {
		name    string
		m       *Manifest
		wantErr string
	}{
		{
			name:    "valid",
			m:       &Manifest{SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated},
			wantErr: "",
		},
		{
			name:    "wrong schema version",
			m:       &Manifest{SchemaVersion: 99, TaskID: "t1", OutputMode: "transcript", State: TaskCreated},
			wantErr: "unsupported schema version",
		},
		{
			name:    "missing task_id",
			m:       &Manifest{SchemaVersion: currentSchemaVersion, OutputMode: "transcript", State: TaskCreated},
			wantErr: "missing task_id",
		},
		{
			name:    "invalid output mode",
			m:       &Manifest{SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "invalid", State: TaskCreated},
			wantErr: "invalid output_mode",
		},
		{
			name:    "invalid state",
			m:       &Manifest{SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: "BOGUS"},
			wantErr: "invalid task state",
		},
		{
			name:    "invalid integrity version",
			m:       &Manifest{SchemaVersion: currentSchemaVersion, IntegrityVersion: 2, TaskID: "t1", OutputMode: "transcript", State: TaskCreated},
			wantErr: "unsupported integrity_version",
		},
		{
			name: "duplicate chunk ID",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Chunks: ChunksInfo{Format: "wav", Total: 2, Items: []ChunkItem{{ID: "part_001", State: ChunkPending}, {ID: "part_001", State: ChunkPending}}},
			},
			wantErr: "duplicate chunk ID",
		},
		{
			name: "invalid chunk state",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Chunks: ChunksInfo{Format: "wav", Total: 1, Items: []ChunkItem{{ID: "part_001", State: "BOGUS"}}},
			},
			wantErr: "invalid state",
		},
		{
			name: "chunk count mismatch",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Chunks: ChunksInfo{Format: "wav", Total: 2, Items: []ChunkItem{{ID: "part_001", State: ChunkPending}}},
			},
			wantErr: "chunks.total",
		},
		{
			name: "invalid chunk hash",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Chunks: ChunksInfo{Format: "wav", Total: 1, Items: []ChunkItem{{ID: "part_001", State: ChunkPending, AudioSHA256: "bad"}}},
			},
			wantErr: "invalid audio hash",
		},
		{
			name: "invalid artifact state",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Artifacts: Artifacts{Transcript: ArtifactInfo{State: "BOGUS"}},
			},
			wantErr: "invalid state",
		},
		{
			name: "chunk ID with path traversal",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Chunks: ChunksInfo{Items: []ChunkItem{{ID: "../../etc/passwd"}}},
			},
			wantErr: "path separator",
		},
		{
			name: "chunk ID with path separator",
			m: &Manifest{
				SchemaVersion: currentSchemaVersion, TaskID: "t1", OutputMode: "transcript", State: TaskCreated,
				Chunks: ChunksInfo{Items: []ChunkItem{{ID: "sub/part_001"}}},
			},
			wantErr: "path separator",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifest(taskDir, tt.m)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestCountCompletedChunks(t *testing.T) {
	m := &Manifest{
		Chunks: ChunksInfo{
			Items: []ChunkItem{
				{ID: "part_001", State: ChunkCompleted},
				{ID: "part_002", State: ChunkPending},
				{ID: "part_003", State: ChunkCompleted},
				{ID: "part_004", State: ChunkFailed},
			},
		},
	}
	if got := m.CountCompletedChunks(); got != 2 {
		t.Errorf("CountCompletedChunks() = %d, want 2", got)
	}
}

func TestChunkByID(t *testing.T) {
	m := &Manifest{
		Chunks: ChunksInfo{
			Items: []ChunkItem{
				{ID: "part_001", State: ChunkCompleted},
				{ID: "part_002", State: ChunkPending},
			},
		},
	}
	chunk := m.ChunkByID("part_002")
	if chunk == nil {
		t.Fatal("expected to find part_002")
	}
	if chunk.State != ChunkPending {
		t.Errorf("expected state %q, got %q", ChunkPending, chunk.State)
	}
	if m.ChunkByID("part_999") != nil {
		t.Error("expected nil for non-existent chunk")
	}
}
