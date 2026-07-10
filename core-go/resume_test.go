package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper to create a task directory with the standard layout.
func createTestTaskDir(t *testing.T) string {
	t.Helper()
	taskDir := t.TempDir()
	for _, sub := range []string{"audio", "chunks", "transcripts"} {
		if err := os.Mkdir(filepath.Join(taskDir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return taskDir
}

// Helper to create chunk audio files.
func createChunks(t *testing.T, chunksDir string, count int, format string) {
	t.Helper()
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("part_%03d.%s", i, format)
		if err := os.WriteFile(filepath.Join(chunksDir, name), []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// Helper to create transcript files.
func createTranscripts(t *testing.T, transcriptsDir string, numbers []int) {
	t.Helper()
	for _, n := range numbers {
		name := fmt.Sprintf("part_%03d.txt", n)
		if err := os.WriteFile(filepath.Join(transcriptsDir, name), []byte("transcript text"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// --- Scenario 1: Killed during chunk 5/10 → retranscribes only missing chunks ---

func TestResume_KilledDuringTranscription_RetranscribesOnlyMissing(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 10, "wav")
	// Chunks 1-4 have transcripts, 5-10 do not (simulating crash at chunk 5).
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2, 3, 4})

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 10
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("part_%03d", i)
		state := ChunkPending
		if i <= 4 {
			state = ChunkCompleted
		}
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:             id,
			AudioPath:      filepath.Join(taskDir, "chunks", id+".wav"),
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          state,
		})
	}
	m.State = TaskTranscribing
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// Reconcile: should keep completed for 1-4, reset 5-10 to PENDING.
	ReconcileChunks(taskDir, m)

	completed := 0
	pending := 0
	for _, item := range m.Chunks.Items {
		switch item.State {
		case ChunkCompleted:
			completed++
		case ChunkPending:
			pending++
		}
	}
	if completed != 4 {
		t.Errorf("expected 4 completed chunks, got %d", completed)
	}
	if pending != 6 {
		t.Errorf("expected 6 pending chunks, got %d", pending)
	}

	// ResumeStage should say TRANSCRIBING.
	stage, _ := ResumeStage(m)
	if stage != TaskTranscribing {
		t.Errorf("expected stage %q, got %q", TaskTranscribing, stage)
	}
}

func TestReconcileChunks_HashMismatchRequiresRetranscription(t *testing.T) {
	taskDir := createTestTaskDir(t)
	chunkPath := filepath.Join(taskDir, "chunks", "part_001.wav")
	transcriptPath := filepath.Join(taskDir, "transcripts", "part_001.txt")
	if err := os.WriteFile(chunkPath, []byte("original audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcriptPath, []byte("old transcript"), 0600); err != nil {
		t.Fatal(err)
	}
	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 1
	m.Chunks.Items = []ChunkItem{{
		ID:               "part_001",
		AudioPath:        chunkPath,
		TranscriptPath:   transcriptPath,
		State:            ChunkCompleted,
		AudioSHA256:      mustFileSHA256(chunkPath),
		TranscriptSHA256: mustFileSHA256(transcriptPath),
	}}
	if err := os.WriteFile(chunkPath, []byte("changed audio"), 0600); err != nil {
		t.Fatal(err)
	}

	ReconcileChunks(taskDir, m)
	if m.Chunks.Items[0].State != ChunkPending {
		t.Fatalf("changed audio must require retranscription, got %s", m.Chunks.Items[0].State)
	}
}

// --- Scenario 2: Merge failure → reruns merge only ---

func TestResume_MergeFailure_RerunsMergeOnly(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2, 3})

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 3
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("part_%03d", i)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:             id,
			AudioPath:      filepath.Join(taskDir, "chunks", id+".wav"),
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          ChunkCompleted,
		})
	}
	// All chunks completed, but transcript.md is missing (merge failed).
	m.Artifacts.Transcript.State = ArtifactPending
	m.State = TaskFailed
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	ReconcileChunks(taskDir, m)
	ReconcileArtifacts(taskDir, m)

	stage, _ := ResumeStage(m)
	if stage != TaskMerging {
		t.Errorf("expected stage %q, got %q", TaskMerging, stage)
	}

	// All chunks should still be completed.
	for _, item := range m.Chunks.Items {
		if item.State != ChunkCompleted {
			t.Errorf("chunk %s should be COMPLETED, got %s", item.ID, item.State)
		}
	}
}

// --- Scenario 3: DeepSeek failure → reruns minutes only ---

func TestResume_DeepSeekFailure_RerunsMinutesOnly(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2, 3})

	// Write transcript.md and notes_input.md.
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("transcript"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "notes_input.md"), []byte("notes"), 0600); err != nil {
		t.Fatal(err)
	}

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "minutes", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 3
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("part_%03d", i)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:        id,
			AudioPath: filepath.Join(taskDir, "chunks", id+".wav"),
			State:     ChunkCompleted,
		})
	}
	m.Artifacts.Transcript.State = ArtifactReady
	m.Artifacts.Transcript.Path = filepath.Join(taskDir, "transcript.md")
	m.Artifacts.NotesInput.State = ArtifactReady
	m.Artifacts.NotesInput.Path = filepath.Join(taskDir, "notes_input.md")
	m.Artifacts.Minutes.State = ArtifactPending
	m.State = TaskFailed
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	ReconcileChunks(taskDir, m)
	ReconcileArtifacts(taskDir, m)

	stage, _ := ResumeStage(m)
	if stage != TaskMinutesGenerating {
		t.Errorf("expected stage %q, got %q", TaskMinutesGenerating, stage)
	}
}

// --- Scenario 4: Deleted transcript chunk → invalidates downstream ---

func TestResume_DeletedTranscriptChunk_InvalidatesDownstream(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	// Only chunks 1 and 3 have transcripts (chunk 2 deleted).
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 3})

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 3
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("part_%03d", i)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:             id,
			AudioPath:      filepath.Join(taskDir, "chunks", id+".wav"),
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          ChunkCompleted,
		})
	}
	m.Artifacts.Transcript.State = ArtifactReady
	m.State = TaskCompleted
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// Reconcile should detect missing transcript for chunk 2.
	ReconcileChunks(taskDir, m)
	ReconcileArtifacts(taskDir, m)

	// Chunk 2 should be PENDING.
	if m.Chunks.Items[1].State != ChunkPending {
		t.Errorf("chunk 2 should be PENDING, got %s", m.Chunks.Items[1].State)
	}

	stage, _ := ResumeStage(m)
	if stage != TaskTranscribing {
		t.Errorf("expected stage %q, got %q", TaskTranscribing, stage)
	}
}

// --- Scenario 5: Completed task → no API calls needed ---

func TestResume_CompletedTask_NoWorkNeeded(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2, 3})
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("transcript"), 0600); err != nil {
		t.Fatal(err)
	}

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 3
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("part_%03d", i)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:    id,
			State: ChunkCompleted,
		})
	}
	m.Artifacts.Transcript.State = ArtifactReady
	m.State = TaskCompleted
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	ReconcileChunks(taskDir, m)
	ReconcileArtifacts(taskDir, m)

	stage, complete := ResumeStage(m)
	if !complete {
		t.Error("expected task to be complete")
	}
	if stage != TaskCompleted {
		t.Errorf("expected stage %q, got %q", TaskCompleted, stage)
	}
}

// --- Scenario 6: Manifest counters reconciled from disk ---

func TestResume_CountersReconciledFromDisk(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 5, "wav")
	// Only 2 transcripts on disk.
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 3})

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 5
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("part_%03d", i)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:             id,
			AudioPath:      filepath.Join(taskDir, "chunks", id+".wav"),
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          ChunkCompleted, // Stale: says all completed.
		})
	}
	// Stale manifest says 5 completed, but only 2 exist on disk.
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	ReconcileChunks(taskDir, m)

	completed := m.CountCompletedChunks()
	if completed != 2 {
		t.Errorf("expected completed_chunks = 2 after reconciliation, got %d", completed)
	}
}

// --- Scenario 6b: Reset to zero ---

func TestResume_CountersReconciledToZero(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	// No transcripts on disk.

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	m.Chunks.Format = "wav"
	m.Chunks.Total = 3
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("part_%03d", i)
		m.Chunks.Items = append(m.Chunks.Items, ChunkItem{
			ID:             id,
			AudioPath:      filepath.Join(taskDir, "chunks", id+".wav"),
			TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"),
			State:          ChunkCompleted, // Stale.
		})
	}
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	ReconcileChunks(taskDir, m)

	completed := m.CountCompletedChunks()
	if completed != 0 {
		t.Errorf("expected completed_chunks = 0 after reconciliation, got %d", completed)
	}
}

// --- Scenario 7: Corrupted manifest handling ---

func TestResume_CorruptManifest_RefusesRecovery(t *testing.T) {
	taskDir := createTestTaskDir(t)
	path := ManifestPath(taskDir)
	if err := os.WriteFile(path, []byte("{corrupt json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(taskDir)
	if err == nil {
		t.Fatal("expected error for corrupt manifest")
	}
	if !strings.Contains(err.Error(), "parsing manifest") {
		t.Errorf("expected parsing error, got: %v", err)
	}

	// Verify the corrupt file was not overwritten.
	data, _ := os.ReadFile(path)
	if string(data) != "{corrupt json" {
		t.Fatalf("corrupt manifest was overwritten: %q", data)
	}
}

// --- Scenario 8: Concurrent resume rejected by lock ---

func TestResume_ConcurrentResumeRejected(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 1, "wav")

	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	// Acquire first lock.
	lock1, err := AcquireTaskLock(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock1.Release()

	// Second lock attempt should fail.
	_, err = AcquireTaskLock(taskDir)
	if err == nil {
		t.Fatal("expected error for concurrent lock acquisition")
	}
	if !strings.Contains(err.Error(), "already being resumed") {
		t.Errorf("expected 'already being resumed' error, got: %v", err)
	}
}

// --- Persistent lock file is reusable after a process exits ---

func TestTaskLock_ReusesPersistentLockFile(t *testing.T) {
	taskDir := createTestTaskDir(t)

	// A lock file from a previous process is only metadata; the OS lock is what
	// controls ownership and is released automatically when that process exits.
	lockPath := filepath.Join(taskDir, ".resume.lock")
	if err := os.WriteFile(lockPath, []byte("99999999\n2026-01-01T00:00:00Z\n"), 0600); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireTaskLock(taskDir)
	if err != nil {
		t.Fatalf("expected persistent lock file to be reusable, got: %v", err)
	}
	defer lock.Release()

	// Verify lock file exists and contains our PID.
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), fmt.Sprintf("%d", os.Getpid())) {
		t.Errorf("lock file should contain our PID, got: %q", data)
	}
}

// --- Lock release ---

func TestTaskLock_Release(t *testing.T) {
	taskDir := createTestTaskDir(t)

	lock, err := AcquireTaskLock(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(taskDir, ".resume.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatal("lock file should exist")
	}

	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatal("lock file should remain available for future locking")
	}
}

// --- Migration from legacy manifest ---

func TestMigrateManifest_BuildsFromDisk(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2})

	// Write a legacy manifest (simulated by schema_version 0).
	old := &Manifest{
		TaskID:     filepath.Base(taskDir),
		OutputMode: "transcript",
		CreatedAt:  "2026-01-01T00:00:00Z",
	}
	if err := SaveManifest(taskDir, old); err != nil {
		t.Fatal(err)
	}
	// Overwrite schema_version to 0 to simulate legacy.
	old.SchemaVersion = 0
	if err := SaveManifest(taskDir, old); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateManifest(taskDir, old)
	if err != nil {
		t.Fatal(err)
	}

	if migrated.SchemaVersion != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, migrated.SchemaVersion)
	}
	if migrated.Chunks.Total != 3 {
		t.Errorf("expected 3 chunks, got %d", migrated.Chunks.Total)
	}
	completed := migrated.CountCompletedChunks()
	if completed != 0 {
		t.Errorf("legacy chunks without recorded hashes must be retranscribed, got %d completed", completed)
	}
}

func TestRunResume_MigratesLegacyManifestFile(t *testing.T) {
	taskDir := createTestTaskDir(t)
	taskSourcePath := filepath.Join(taskDir, "audio", "meeting.wav")
	if err := os.WriteFile(taskSourcePath, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	originalSourcePath := filepath.Join(t.TempDir(), "original.wav")
	if err := os.WriteFile(originalSourcePath, []byte("original audio"), 0600); err != nil {
		t.Fatal(err)
	}
	createChunks(t, filepath.Join(taskDir, "chunks"), 1, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1})
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("merged transcript"), 0600); err != nil {
		t.Fatal(err)
	}

	legacy := legacyManifest{
		Version:      "1",
		SourceAudio:  originalSourcePath,
		OutputMode:   "transcript",
		ChunkMinutes: 5,
		MaxUploadMB:  10,
		ASRProvider:  "mimo",
		ASRModel:     "model",
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyManifestPath(taskDir), data, 0600); err != nil {
		t.Fatal(err)
	}

	legacyInput, err := LoadLegacyManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := migrateLegacyManifest(taskDir, legacyInput)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.SchemaVersion != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, migrated.SchemaVersion)
	}
	if migrated.Source.Path != taskSourcePath {
		t.Errorf("source path should use the task-local copy: got %q", migrated.Source.Path)
	}
	if migrated.Chunks.ChunkMinutes != legacy.ChunkMinutes {
		t.Errorf("chunk minutes were not preserved: got %d", migrated.Chunks.ChunkMinutes)
	}
	if migrated.CountCompletedChunks() != 0 {
		t.Error("legacy transcripts without hashes must be retranscribed")
	}
}

func TestRunResume_RejectsInvalidOutputModeOverride(t *testing.T) {
	taskDir := createTestTaskDir(t)
	m := CreateManifest(taskDir, filepath.Join(taskDir, "audio", "meeting.wav"), "transcript", 5, 10, "mimo", "model")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	err := RunResume([]string{"--output-mode", "invalid", taskDir})
	if err == nil || !strings.Contains(err.Error(), "invalid --output-mode") {
		t.Fatalf("expected invalid output mode error, got %v", err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OutputMode != "transcript" {
		t.Errorf("invalid override should not be persisted, got %q", loaded.OutputMode)
	}
}

// --- ResumeStage edge cases ---

func TestResumeStage_NoChunks(t *testing.T) {
	m := &Manifest{
		OutputMode: "transcript",
		Chunks:     ChunksInfo{Total: 0},
	}
	stage, _ := ResumeStage(m)
	if stage != TaskSplitting {
		t.Errorf("expected %q, got %q", TaskSplitting, stage)
	}
}

func TestResumeStage_ReadyArtifactsRequireFinalValidation(t *testing.T) {
	taskDir := createTestTaskDir(t)
	chunkPath := filepath.Join(taskDir, "chunks", "part_001.wav")
	transcriptPath := filepath.Join(taskDir, "transcript.md")
	if err := os.WriteFile(chunkPath, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcriptPath, []byte("merged transcript"), 0600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		OutputMode: "transcript",
		State:      TaskMerging,
		Chunks: ChunksInfo{Total: 1, Items: []ChunkItem{
			{ID: "part_001", AudioPath: chunkPath, State: ChunkCompleted},
		}},
		Artifacts: Artifacts{Transcript: ArtifactInfo{Path: transcriptPath, State: ArtifactReady}},
	}

	stage, complete := ResumeStage(m)
	if complete || stage != TaskValidatingOutputs {
		t.Errorf("expected final validation, got stage=%q complete=%v", stage, complete)
	}

	m.State = TaskValidatingOutputs
	stage, complete = ResumeStage(m)
	if !complete || stage != TaskCompleted {
		t.Errorf("expected completion after final validation, got stage=%q complete=%v", stage, complete)
	}
}

func TestResumeStage_ChunksExistButAudioMissing(t *testing.T) {
	m := &Manifest{
		OutputMode: "transcript",
		Chunks: ChunksInfo{
			Total: 2,
			Items: []ChunkItem{
				{ID: "part_001", AudioPath: "/nonexistent/part_001.wav", State: ChunkCompleted},
				{ID: "part_002", AudioPath: "/nonexistent/part_002.wav", State: ChunkCompleted},
			},
		},
	}
	stage, _ := ResumeStage(m)
	if stage != TaskSplitting {
		t.Errorf("expected %q, got %q", TaskSplitting, stage)
	}
}

func TestResumeStage_TranscriptMdMissing(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 2, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2})

	m := &Manifest{
		OutputMode: "transcript",
		Chunks: ChunksInfo{
			Total: 2,
			Items: []ChunkItem{
				{ID: "part_001", AudioPath: filepath.Join(taskDir, "chunks", "part_001.wav"), State: ChunkCompleted},
				{ID: "part_002", AudioPath: filepath.Join(taskDir, "chunks", "part_002.wav"), State: ChunkCompleted},
			},
		},
		Artifacts: Artifacts{
			Transcript: ArtifactInfo{Path: filepath.Join(taskDir, "transcript.md"), State: ArtifactPending},
		},
	}
	stage, _ := ResumeStage(m)
	if stage != TaskMerging {
		t.Errorf("expected %q, got %q", TaskMerging, stage)
	}
}

func TestResumeStage_NotesInputMissing(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 2, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 2})
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("transcript"), 0600); err != nil {
		t.Fatal(err)
	}

	m := &Manifest{
		OutputMode: "notes",
		Chunks: ChunksInfo{
			Total: 2,
			Items: []ChunkItem{
				{ID: "part_001", AudioPath: filepath.Join(taskDir, "chunks", "part_001.wav"), State: ChunkCompleted},
				{ID: "part_002", AudioPath: filepath.Join(taskDir, "chunks", "part_002.wav"), State: ChunkCompleted},
			},
		},
		Artifacts: Artifacts{
			Transcript: ArtifactInfo{Path: filepath.Join(taskDir, "transcript.md"), State: ArtifactReady},
			NotesInput: ArtifactInfo{Path: filepath.Join(taskDir, "notes_input.md"), State: ArtifactPending},
		},
	}
	stage, _ := ResumeStage(m)
	if stage != TaskNotesGenerating {
		t.Errorf("expected %q, got %q", TaskNotesGenerating, stage)
	}
}

// --- ReconcileArtifacts ---

func TestReconcileArtifacts_TranscriptMissing_InvalidatesDownstream(t *testing.T) {
	taskDir := createTestTaskDir(t)

	m := &Manifest{
		OutputMode:       "minutes",
		IntegrityVersion: 1,
		Artifacts: Artifacts{
			Transcript: ArtifactInfo{Path: filepath.Join(taskDir, "transcript.md"), State: ArtifactReady},
			NotesInput: ArtifactInfo{Path: filepath.Join(taskDir, "notes_input.md"), State: ArtifactReady},
			Minutes:    ArtifactInfo{Path: filepath.Join(taskDir, "meeting_minutes.md"), State: ArtifactReady},
		},
	}

	// transcript.md doesn't exist on disk.
	ReconcileArtifacts(taskDir, m)

	if m.Artifacts.Transcript.State != ArtifactPending {
		t.Errorf("transcript should be PENDING, got %s", m.Artifacts.Transcript.State)
	}
	if m.Artifacts.NotesInput.State != ArtifactInvalid {
		t.Errorf("notes should be INVALID, got %s", m.Artifacts.NotesInput.State)
	}
	if m.Artifacts.Minutes.State != ArtifactInvalid {
		t.Errorf("minutes should be INVALID, got %s", m.Artifacts.Minutes.State)
	}
}

func TestReconcileArtifacts_NotesInputMissing_InvalidatesMinutes(t *testing.T) {
	taskDir := createTestTaskDir(t)
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("transcript"), 0600); err != nil {
		t.Fatal(err)
	}

	m := &Manifest{
		OutputMode:       "minutes",
		IntegrityVersion: 1,
		Artifacts: Artifacts{
			Transcript: ArtifactInfo{Path: filepath.Join(taskDir, "transcript.md"), State: ArtifactReady},
			NotesInput: ArtifactInfo{Path: filepath.Join(taskDir, "notes_input.md"), State: ArtifactReady},
			Minutes:    ArtifactInfo{Path: filepath.Join(taskDir, "meeting_minutes.md"), State: ArtifactReady},
		},
	}

	ReconcileArtifacts(taskDir, m)

	if m.Artifacts.Transcript.State != ArtifactReady {
		t.Errorf("transcript should be READY, got %s", m.Artifacts.Transcript.State)
	}
	if m.Artifacts.NotesInput.State != ArtifactPending {
		t.Errorf("notes should be PENDING, got %s", m.Artifacts.NotesInput.State)
	}
	if m.Artifacts.Minutes.State != ArtifactInvalid {
		t.Errorf("minutes should be INVALID, got %s", m.Artifacts.Minutes.State)
	}
}

// --- WriteFileAtomically ---

func TestWriteFileAtomically_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := WriteFileAtomically(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("expected %q, got %q", "hello", string(data))
	}
}

func TestWriteFileAtomically_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomically(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("expected %q, got %q", "new", string(data))
	}

	// No temp files left.
	temps, _ := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if len(temps) != 0 {
		t.Errorf("temp files found: %v", temps)
	}
}

// --- BuildChunkItemsFromDisk ---

func TestBuildChunkItemsFromDisk(t *testing.T) {
	taskDir := createTestTaskDir(t)
	createChunks(t, filepath.Join(taskDir, "chunks"), 3, "wav")
	createTranscripts(t, filepath.Join(taskDir, "transcripts"), []int{1, 3})

	items, format, err := BuildChunkItemsFromDisk(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if format != "wav" {
		t.Errorf("expected format %q, got %q", "wav", format)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].State != ChunkPending {
		t.Errorf("item 1: expected PENDING until its hash is recorded, got %s", items[0].State)
	}
	if items[1].State != ChunkPending {
		t.Errorf("item 2: expected PENDING, got %s", items[1].State)
	}
	if items[2].State != ChunkPending {
		t.Errorf("item 3: expected PENDING until its hash is recorded, got %s", items[2].State)
	}
}

// Helper for pad function used in chunk naming.
func pad(n int) string {
	if n < 10 {
		return "00" + string(rune('0'+n))
	}
	return "0" + string(rune('0'+n/10)) + string(rune('0'+n%10))
}
