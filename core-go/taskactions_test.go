package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yudi/meet-asr/core-go/tasklibrary"
)

// interruptedTaskManifest builds a Task that is mid-transcription and whose
// transcript is not ready yet, which is the state a crash or force-quit leaves
// behind.
func interruptedTaskManifest(t *testing.T) (string, *Manifest) {
	t.Helper()
	taskDir := t.TempDir()
	source := filepath.Join(taskDir, "audio.wav")
	if err := os.WriteFile(source, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	m := CreateManifest(taskDir, source, "transcript_only", 5, 10, string(ASRProviderMimo), string(ASRModelMimoV25))
	m.State = TaskTranscribing
	m.Chunks.Items = []ChunkItem{{
		ID:             "part_001",
		AudioPath:      filepath.Join(taskDir, "chunks", "part_001.mp3"),
		TranscriptPath: filepath.Join(taskDir, "transcripts", "part_001.txt"),
		State:          ChunkPending,
	}}
	m.Chunks.Total = 1
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}
	return taskDir, m
}

func TestDescribeTaskActionsReportsInterruptedAndResumable(t *testing.T) {
	taskDir, m := interruptedTaskManifest(t)
	summary := tasklibrary.TaskSummary{TaskFolder: filepath.Base(taskDir), Status: "processing"}
	describeTaskActions(taskDir, m, &summary)

	if summary.Actions == nil {
		t.Fatal("actions missing")
	}
	if summary.Actions.Locked {
		t.Fatal("a free task lock must not report locked")
	}
	if !summary.Actions.CanResume || !summary.Actions.CanResumeTranscribe {
		t.Fatalf("actions = %#v", summary.Actions)
	}
	if summary.Actions.NextStage != TaskSplitting && summary.Actions.NextStage != TaskTranscribing {
		t.Fatalf("next stage = %q", summary.Actions.NextStage)
	}
	if summary.Actions.PendingChunks != 1 {
		t.Fatalf("pending chunks = %d", summary.Actions.PendingChunks)
	}
	// An in-flight stage with no owner is interrupted, not processing.
	if summary.Status != "interrupted" {
		t.Fatalf("status = %q", summary.Status)
	}
}

func TestDescribeTaskActionsReportsLockAndProviderSupport(t *testing.T) {
	taskDir, m := interruptedTaskManifest(t)
	m.Chunks.ASRProvider, m.Chunks.ASRModel = string(ASRProviderLocalSenseVoice), string(ASRModelSenseVoiceSmallINT8)

	unlocked := tasklibrary.TaskSummary{TaskFolder: filepath.Base(taskDir), Status: "processing"}
	describeTaskActions(taskDir, m, &unlocked)
	if !hasBlockingReason(unlocked.Actions, "speech_provider_unavailable") {
		t.Fatalf("blocking reasons = %#v", unlocked.Actions)
	}
	// The pending work needs speech recognition, so the action is unavailable and
	// the flag must say so instead of leaving a doomed button enabled.
	if unlocked.Actions.CanResume || unlocked.Actions.CanResumeTranscribe {
		t.Fatalf("unavailable runtime must block the audio actions: %#v", unlocked.Actions)
	}

	// While another process owns the Task, conflicting actions are blocked and
	// the Task keeps reporting that it is being processed.
	lock, err := AcquireTaskLock(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	locked := tasklibrary.TaskSummary{TaskFolder: filepath.Base(taskDir), Status: "processing"}
	describeTaskActions(taskDir, m, &locked)
	if !locked.Actions.Locked || locked.Actions.CanResume || locked.Actions.CanResumeTranscribe {
		t.Fatalf("locked actions = %#v", locked.Actions)
	}
	if !hasBlockingReason(locked.Actions, "task_locked") {
		t.Fatalf("locked blocking reasons = %#v", locked.Actions)
	}
	if locked.Status != "processing" {
		t.Fatalf("locked status = %q", locked.Status)
	}
}

func TestDescribeTaskActionsBlocksMissingAudioWork(t *testing.T) {
	taskDir, m := interruptedTaskManifest(t)
	// Neither the source audio nor the chunk audio is on disk, so a resume would
	// have nothing to work with even though the manifest still records a stage.
	if err := os.Remove(m.Source.Path); err != nil {
		t.Fatal(err)
	}
	summary := tasklibrary.TaskSummary{TaskFolder: filepath.Base(taskDir), Status: "processing"}
	describeTaskActions(taskDir, m, &summary)
	if !hasBlockingReason(summary.Actions, "source_audio_missing") {
		t.Fatalf("blocking reasons = %#v", summary.Actions)
	}
	if summary.Actions.CanResume || summary.Actions.CanResumeTranscribe {
		t.Fatalf("missing audio must block the audio actions: %#v", summary.Actions)
	}
}

func TestCancelledTaskStatusStaysResumable(t *testing.T) {
	taskDir, m := interruptedTaskManifest(t)
	markTaskCancelled(taskDir, m)

	if got := taskStatus(m); got != "cancelled" {
		t.Fatalf("status = %q", got)
	}
	stage, complete := ResumeStage(m)
	if complete {
		t.Fatal("a cancelled task with unfinished work must not be complete")
	}
	if stage != TaskSplitting && stage != TaskTranscribing {
		t.Fatalf("resume stage = %q", stage)
	}
	stored, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "cancelled" || stored.LastError == nil || stored.LastError.Stage != cancellationErrorCode {
		t.Fatalf("stored manifest = %#v", stored.LastError)
	}
}

func hasBlockingReason(actions *tasklibrary.TaskActions, code string) bool {
	if actions == nil {
		return false
	}
	for _, reason := range actions.BlockingReasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func TestMergingStageStaysResumableWithoutSpeech(t *testing.T) {
	taskDir, m := interruptedTaskManifest(t)
	// Every chunk is transcribed and its audio is still on disk, but the merged
	// transcript was never written: the pending work is local merging only.
	chunkPath := m.Chunks.Items[0].AudioPath
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunkPath, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Chunks.Items[0].State = ChunkCompleted
	m.Chunks.Items[0].TranscriptSHA256 = "hash"
	// Speech recognition is unavailable and the source audio is gone, neither of
	// which the merging stage needs.
	m.Chunks.ASRProvider, m.Chunks.ASRModel = string(ASRProviderLocalSenseVoice), string(ASRModelSenseVoiceSmallINT8)
	if err := os.Remove(m.Source.Path); err != nil {
		t.Fatal(err)
	}

	stage, complete := ResumeStage(m)
	if complete || stage != TaskMerging {
		t.Fatalf("stage = %q complete = %v", stage, complete)
	}

	summary := tasklibrary.TaskSummary{TaskFolder: filepath.Base(taskDir), Status: "processing"}
	describeTaskActions(taskDir, m, &summary)
	if summary.Actions == nil {
		t.Fatal("actions missing")
	}
	if !summary.Actions.CanResume {
		t.Fatalf("merging must stay resumable: %#v", summary.Actions)
	}
	if summary.Actions.CanResumeTranscribe || summary.Actions.CanResumeGeneration {
		t.Fatalf("no speech or generation work is pending: %#v", summary.Actions)
	}
	if len(summary.Actions.BlockingReasons) != 0 {
		t.Fatalf("local merging must not report speech blockers: %#v", summary.Actions.BlockingReasons)
	}
	if summary.Status != "interrupted" {
		t.Fatalf("status = %q", summary.Status)
	}
}

func TestCompletedTranscriptDoesNotRequireLocalSpeechReadiness(t *testing.T) {
	dir, m := completedV2Manifest(t)
	m.Chunks.ASRProvider, m.Chunks.ASRModel = string(ASRProviderLocalSenseVoice), string(ASRModelSenseVoiceSmallINT8)
	m.Chunks.Items = []ChunkItem{{ID: "part_001", State: ChunkCompleted}}
	calls := 0
	summary := tasklibrary.TaskSummary{Status: "completed"}
	describeTaskActionsWithReadiness(dir, m, &summary, func(ASRProviderConfig) error { calls++; return os.ErrNotExist })
	if calls != 0 || len(summary.Actions.BlockingReasons) != 0 || summary.Actions.CanResume {
		t.Fatalf("completed transcript depended on audio readiness: calls=%d actions=%+v", calls, summary.Actions)
	}
}
