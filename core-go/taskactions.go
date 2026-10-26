package main

import (
	"github.com/yudi/meet-asr/core-go/tasklibrary"
	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

// describeTaskActions annotates one discovered Task with what the desktop may do
// right now and why anything else is blocked. Everything here comes from
// durable state: the manifest, the Task lock, and provider runtime support.
func describeTaskActions(taskDir string, m *taskmanifest.TaskManifest, summary *tasklibrary.TaskSummary) {
	describeTaskActionsWithReadiness(taskDir, m, summary, taskSpeechReadiness)
}

// A library scan verifies each local engine once, rather than hashing the same
// installed model again for every interrupted Task. The cache ends with the scan.
func taskActionDescriber() tasklibrary.TaskDescriber {
	cache := make(map[ASRProviderConfig]error)
	return func(dir string, m *taskmanifest.TaskManifest, summary *tasklibrary.TaskSummary) {
		describeTaskActionsWithReadiness(dir, m, summary, func(config ASRProviderConfig) error {
			if err, exists := cache[config]; exists {
				return err
			}
			err := taskSpeechReadiness(config)
			cache[config] = err
			return err
		})
	}
}

func describeTaskActionsWithReadiness(taskDir string, m *taskmanifest.TaskManifest, summary *tasklibrary.TaskSummary, readiness func(ASRProviderConfig) error) {
	locked := !ProbeTaskLock(taskDir)
	stage, complete := ResumeStage(m)

	pendingChunks := 0
	for i := range m.Chunks.Items {
		if m.Chunks.Items[i].State != ChunkCompleted {
			pendingChunks++
		}
	}

	// Reasons are collected first; every action flag is then derived from them, so
	// a Task never advertises an action whose own precondition is already known to
	// fail.
	reasons := make([]tasklibrary.TaskBlockingReason, 0)
	needsAudioWork := !complete && (stage == TaskSplitting || stage == TaskTranscribing)
	audioBlocked := false

	if locked {
		reasons = append(reasons, tasklibrary.TaskBlockingReason{
			Code:    "task_locked",
			Message: "Another process is working on this task.",
		})
	}

	if needsAudioWork {
		if recorded, ok := recordedASRIdentity(m); ok {
			if err := ValidateASRProviderConfig(recorded); err != nil {
				audioBlocked = true
				reasons = append(reasons, tasklibrary.TaskBlockingReason{
					Code:    "speech_provider_unsupported",
					Message: "The speech provider recorded on this task is not supported.",
				})
			} else if err := readiness(recorded); err != nil {
				audioBlocked = true
				reasons = append(reasons, tasklibrary.TaskBlockingReason{
					Code:    "speech_provider_unavailable",
					Message: "The local speech model or runtime required by this task is unavailable.",
				})
			}
		}
		// File-verified state only: a manifest that still claims a stage is
		// resumable must not win over missing audio on disk.
		if !isCompleteFile(m.Source.Path) && !chunksPresent(m) {
			audioBlocked = true
			reasons = append(reasons, tasklibrary.TaskBlockingReason{
				Code:    "source_audio_missing",
				Message: "The task audio is missing, so it cannot be split again.",
			})
		}
	}

	actions := tasklibrary.TaskActions{
		Locked:              locked,
		CanResumeTranscribe: needsAudioWork && !locked && !audioBlocked,
		CanResumeGeneration: !complete && !locked && stage == TaskContentGenerating,
		CanRetryGeneration:  retryableGenerationExists(m),
		PendingChunks:       pendingChunks,
		BlockingReasons:     reasons,
	}
	// Every stage the runtime can continue counts, not only transcription and
	// generation: merging and local output validation need no provider and no
	// audio, so they stay resumable while speech work is blocked.
	actions.CanResume = !complete && !locked && !(needsAudioWork && audioBlocked)
	if !complete {
		actions.NextStage = stage
	}

	summary.Actions = &actions

	// A durable in-flight stage with a free Task lock means nobody owns the work
	// any more: it was interrupted (crash, force-quit, hard stop) rather than
	// running now. A task that holds its lock keeps reporting processing.
	if summary.Status == "processing" && !locked {
		summary.Status = "interrupted"
	}
}

// chunksPresent reports whether every recorded chunk still has its audio on
// disk. It checks existence only: full hashing belongs to reconciliation, not to
// a read-model pass over the whole library.
func chunksPresent(m *taskmanifest.TaskManifest) bool {
	if len(m.Chunks.Items) == 0 {
		return false
	}
	for i := range m.Chunks.Items {
		if !isCompleteFile(m.Chunks.Items[i].AudioPath) {
			return false
		}
	}
	return true
}

// retryableGenerationExists reports whether any generation of this Task may run
// again under its own identity.
func retryableGenerationExists(m *taskmanifest.TaskManifest) bool {
	for i := range m.Generations {
		if generationRetryable(&m.Generations[i]) {
			return true
		}
	}
	return false
}

// Capabilities describe an engine, but do not establish that local model files
// and the bundled runtime are actually ready to execute it.
func taskSpeechReadiness(config ASRProviderConfig) error {
	if _, err := ResolveASRCapabilities(config); err != nil {
		return err
	}
	if config.ProviderID == ASRProviderLocalSenseVoice {
		_, err := localASRStatus("")
		return err
	}
	return nil
}
