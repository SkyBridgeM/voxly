package main

import "path/filepath"

// TaskState represents the current pipeline stage for a task.
type TaskState = string

const (
	TaskCreated           TaskState = "CREATED"
	TaskValidatingAudio   TaskState = "VALIDATING_AUDIO"
	TaskSplitting         TaskState = "SPLITTING"
	TaskTranscribing      TaskState = "TRANSCRIBING"
	TaskMerging           TaskState = "MERGING"
	TaskNotesGenerating   TaskState = "NOTES_GENERATING"
	TaskMinutesGenerating TaskState = "MINUTES_GENERATING"
	TaskContentGenerating TaskState = "CONTENT_GENERATING"
	TaskValidatingOutputs TaskState = "VALIDATING_OUTPUTS"
	TaskCompleted         TaskState = "COMPLETED"
	TaskCancelled         TaskState = "CANCELLED"
	TaskFailed            TaskState = "FAILED"
)

// cancellationErrorCode is the durable error code for a requested stop, so a
// cancel stays distinguishable from a provider or workflow failure.
const cancellationErrorCode = "cancelled"

// ChunkState represents the state of one split audio chunk.
type ChunkState = string

const (
	ChunkPending    ChunkState = "PENDING"
	ChunkInProgress ChunkState = "IN_PROGRESS"
	ChunkCompleted  ChunkState = "COMPLETED"
	ChunkFailed     ChunkState = "FAILED"
)

// ArtifactState represents the state of a generated output file.
type ArtifactState = string

const (
	ArtifactPending    ArtifactState = "pending"
	ArtifactProcessing ArtifactState = "processing"
	ArtifactReady      ArtifactState = "completed"
	ArtifactFailed     ArtifactState = "failed"
	ArtifactInvalid    ArtifactState = "unavailable"
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
	TaskContentGenerating: true,
	TaskValidatingOutputs: true,
	TaskCompleted:         true,
	TaskCancelled:         true,
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
	ArtifactPending:    true,
	ArtifactProcessing: true,
	ArtifactReady:      true,
	ArtifactFailed:     true,
	ArtifactInvalid:    true,
}

// ResumeStage determines the earliest incomplete stage for a task based on
// reconciled manifest and filesystem state. Returns the stage to execute and
// whether the task is already complete.
func ResumeStage(m *Manifest) (TaskState, bool) {
	if m.Chunks.VAD != nil && m.Chunks.VAD.Ready {
		if validVADPlan(m) != nil {
			return TaskSplitting, false
		}
		if len(m.Chunks.Items) == 0 {
			if !vadArtifactsValid(filepath.Dir(m.Artifacts.Transcript.Path), m) || m.Artifacts.Transcript.State != ArtifactReady {
				return TaskMerging, false
			}
		} else if m.Artifacts.Transcript.State == ArtifactReady && !vadArtifactsValid(filepath.Dir(m.Artifacts.Transcript.Path), m) {
			if !chunksExist(m) {
				return TaskSplitting, false
			}
			for i := range m.Chunks.Items {
				if !completedTranscriptValid(&m.Chunks.Items[i]) {
					return TaskTranscribing, false
				}
			}
			return TaskMerging, false
		}
	}

	// A verified completed output is authoritative. Resume must not rebuild its
	// upstream intermediates merely because chunks were later removed.
	t := m.Artifacts.Transcript
	if t.State != ArtifactReady || !isCompleteFile(t.Path) {
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

		return TaskMerging, false
	}

	if m.OutputMode == "transcript_and_generate" {
		if len(m.Generations) == 0 {
			return TaskContentGenerating, false
		}
		last := m.Generations[len(m.Generations)-1]
		if last.Status == "processing" || (last.Status == "failed" && last.Error != nil && last.Error.Retryable != nil && *last.Error.Retryable) {
			return TaskContentGenerating, false
		}
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
		if m.Chunks.VAD != nil && completedTranscriptValid(&m.Chunks.Items[i]) {
			continue
		}
		if !isCompleteFile(m.Chunks.Items[i].AudioPath) {
			return false
		}
	}
	return true
}
