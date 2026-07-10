package main

// TaskState represents the current pipeline stage for a task.
type TaskState string

const (
	TaskCreated           TaskState = "CREATED"
	TaskValidatingAudio   TaskState = "VALIDATING_AUDIO"
	TaskSplitting         TaskState = "SPLITTING"
	TaskTranscribing      TaskState = "TRANSCRIBING"
	TaskMerging           TaskState = "MERGING"
	TaskNotesGenerating   TaskState = "NOTES_GENERATING"
	TaskMinutesGenerating TaskState = "MINUTES_GENERATING"
	TaskValidatingOutputs TaskState = "VALIDATING_OUTPUTS"
	TaskCompleted         TaskState = "COMPLETED"
	TaskFailed            TaskState = "FAILED"
)

// ChunkState represents the state of one split audio chunk.
type ChunkState string

const (
	ChunkPending    ChunkState = "PENDING"
	ChunkInProgress ChunkState = "IN_PROGRESS"
	ChunkCompleted  ChunkState = "COMPLETED"
	ChunkFailed     ChunkState = "FAILED"
)

// ArtifactState represents the state of a generated output file.
type ArtifactState string

const (
	ArtifactPending ArtifactState = "PENDING"
	ArtifactReady   ArtifactState = "READY"
	ArtifactInvalid ArtifactState = "INVALID"
)

// validTaskStates is the set of all valid task states.
var validTaskStates = map[TaskState]bool{
	TaskCreated:           true,
	TaskValidatingAudio:   true,
	TaskSplitting:         true,
	TaskTranscribing:      true,
	TaskMerging:           true,
	TaskNotesGenerating:   true,
	TaskMinutesGenerating: true,
	TaskValidatingOutputs: true,
	TaskCompleted:         true,
	TaskFailed:            true,
}

// validChunkStates is the set of all valid chunk states.
var validChunkStates = map[ChunkState]bool{
	ChunkPending:    true,
	ChunkInProgress: true,
	ChunkCompleted:  true,
	ChunkFailed:     true,
}

// validArtifactStates is the set of all valid artifact states.
var validArtifactStates = map[ArtifactState]bool{
	ArtifactPending: true,
	ArtifactReady:   true,
	ArtifactInvalid: true,
}

// ResumeStage determines the earliest incomplete stage for a task based on
// reconciled manifest and filesystem state. Returns the stage to execute and
// whether the task is already complete.
func ResumeStage(m *Manifest) (TaskState, bool) {
	// Source validity: if chunks exist, source was valid at split time.
	// We check chunk presence instead.

	// Chunks missing or invalid: need splitting.
	if m.Chunks.Total == 0 || !chunksExist(m) {
		return TaskSplitting, false
	}

	// Any chunk transcript missing or invalid: need transcription.
	for i := range m.Chunks.Items {
		if m.Chunks.Items[i].State != ChunkCompleted {
			return TaskTranscribing, false
		}
	}

	// transcript.md missing or invalid: need merging.
	t := m.Artifacts.Transcript
	if t.State != ArtifactReady || !isCompleteFile(t.Path) {
		return TaskMerging, false
	}

	// Notes input required but missing or invalid.
	if m.OutputMode == "notes" || m.OutputMode == "minutes" {
		n := m.Artifacts.NotesInput
		if n.State != ArtifactReady || !isCompleteFile(n.Path) {
			return TaskNotesGenerating, false
		}
	}

	// Minutes required but missing or invalid.
	if m.OutputMode == "minutes" {
		min := m.Artifacts.Minutes
		if min.State != ArtifactReady || !isCompleteFile(min.Path) {
			return TaskMinutesGenerating, false
		}
	}

	// A successful final validation is the only route to COMPLETED. A task that
	// was already completed remains a no-op unless reconciliation found an
	// earlier missing artifact above.
	if m.State == TaskValidatingOutputs || m.State == TaskCompleted {
		return TaskCompleted, true
	}
	return TaskValidatingOutputs, false
}

// chunksExist checks that chunk audio files exist on disk.
func chunksExist(m *Manifest) bool {
	for i := range m.Chunks.Items {
		if !isCompleteFile(m.Chunks.Items[i].AudioPath) {
			return false
		}
	}
	return true
}
