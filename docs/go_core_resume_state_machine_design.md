# Go Core Resume State Machine Design

## Purpose

This document defines the resume architecture for the experimental Go backend of `meeting-asr`.

The goal is to recover interrupted long-running transcription tasks without repeating valid completed work. The design applies only to the Go backend task workflow and `task_manifest.json`.

Do not use this document to redesign the desktop app, packaging, cloud synchronization, or provider architecture.

## Scope and Constraints

The Go backend currently supports:

- audio splitting through `ffmpeg`;
- MiMo ASR transcription;
- `transcript.md` generation;
- `notes_input.md` generation;
- DeepSeek `meeting_minutes.md` generation;
- output modes: `transcript`, `notes`, and `minutes`;
- task-local `task_manifest.json`;
- stable stdout progress lines.

The resume design must:

1. Recover interrupted tasks.
2. Avoid repeating completed ASR or LLM work.
3. Treat `task_manifest.json` as the durable task checkpoint.
4. Resume at chunk level for ASR.
5. Recover safely from failures at every pipeline stage.
6. Keep all task data local and owner-only.

## Terminology

- **Task state**: the current pipeline stage for one task.
- **Chunk state**: the state of one split audio chunk.
- **Artifact**: a generated output, such as `transcript.md`.
- **Reconciliation**: validating manifest records against files before resuming.
- **Final file**: the validated file at its normal task path.
- **Temporary file**: an incomplete file that must never be treated as final output.

## Task-Level State Machine

### States

```text
CREATED
VALIDATING_AUDIO
SPLITTING
TRANSCRIBING
MERGING
NOTES_GENERATING
MINUTES_GENERATING
VALIDATING_OUTPUTS
COMPLETED
FAILED
```

### Normal transitions

```text
CREATED
  -> VALIDATING_AUDIO
  -> SPLITTING
  -> TRANSCRIBING
  -> MERGING
  -> NOTES_GENERATING       (notes and minutes modes only)
  -> MINUTES_GENERATING     (minutes mode only)
  -> VALIDATING_OUTPUTS
  -> COMPLETED
```

For `transcript` mode:

```text
MERGING -> VALIDATING_OUTPUTS -> COMPLETED
```

For `notes` mode:

```text
MERGING -> NOTES_GENERATING -> VALIDATING_OUTPUTS -> COMPLETED
```

For `minutes` mode:

```text
MERGING -> NOTES_GENERATING -> MINUTES_GENERATING -> VALIDATING_OUTPUTS -> COMPLETED
```

### Failure transitions

Any active state may transition to `FAILED`:

```text
VALIDATING_AUDIO  -> FAILED
SPLITTING         -> FAILED
TRANSCRIBING      -> FAILED
MERGING           -> FAILED
NOTES_GENERATING  -> FAILED
MINUTES_GENERATING -> FAILED
VALIDATING_OUTPUTS -> FAILED
```

`FAILED` is resumable. A resume does not blindly continue from the recorded state. It first reconciles the task and then moves to the earliest incomplete valid stage.

### Completed-task behavior

`COMPLETED` is idempotent:

- If all required artifacts are still valid, `resume` performs validation only and exits successfully without API calls.
- If a required artifact was deleted or corrupted, `resume` invalidates that artifact and its downstream artifacts, then resumes at the earliest affected stage.

### State transition rule

Only move a task forward after the output of the current stage has:

1. been written to a temporary file when applicable;
2. been flushed and closed;
3. passed stage-specific validation;
4. been atomically promoted to its final path;
5. been recorded in an atomically updated manifest.

## Chunk-Level State Machine

```text
PENDING -> IN_PROGRESS -> COMPLETED
                     -> FAILED
```

### Chunk rules

- `PENDING`: no valid final transcript exists.
- `IN_PROGRESS`: an API request is running. This state is never trusted after process restart.
- `COMPLETED`: a non-empty, validated final transcript file exists.
- `FAILED`: the latest attempt failed; retryability is recorded in the chunk error.

After a crash or forced termination:

- Treat `IN_PROGRESS` as `PENDING` unless its final transcript validates.
- Treat a missing, empty, malformed, or temporary transcript as `PENDING`.
- Do not make a new MiMo request if a valid final transcript already exists.

## Manifest Design

Use `task_manifest.json` as the task-local durable checkpoint.

### Minimal schema

```json
{
  "schema_version": 1,
  "task_id": "20260710_120000_team_planning",
  "output_mode": "minutes",

  "source": {
    "path": "audio/meeting.m4a",
    "original_name": "meeting.m4a",
    "size_bytes": 12345678,
    "sha256": "optional-content-hash"
  },

  "state": "TRANSCRIBING",
  "attempt": 2,

  "chunks": {
    "format": "wav",
    "total": 10,
    "items": [
      {
        "id": "part_001",
        "audio_path": "chunks/part_001.wav",
        "transcript_path": "transcripts/part_001.txt",
        "state": "COMPLETED",
        "attempts": 1,
        "size_bytes": 1048576,
        "transcript_sha256": "optional-hash",
        "last_error": null
      }
    ]
  },

  "artifacts": {
    "transcript": {
      "path": "transcript.md",
      "state": "READY",
      "sha256": "optional-hash"
    },
    "notes_input": {
      "path": "notes_input.md",
      "state": "PENDING"
    },
    "minutes": {
      "path": "meeting_minutes.md",
      "state": "PENDING"
    }
  },

  "last_error": {
    "stage": "TRANSCRIBING",
    "message": "MiMo API timeout for part_005",
    "retryable": true,
    "at": "2026-07-10T10:20:00Z"
  },

  "created_at": "2026-07-10T10:00:00Z",
  "updated_at": "2026-07-10T10:20:00Z",
  "completed_at": null
}
```

### Required fields

- `schema_version`
- `task_id`
- `output_mode`
- `source.path` and `source.size_bytes`
- `state`
- `chunks` after splitting succeeds
- `artifacts`
- `created_at` and `updated_at`

### Derived fields

These values should be recomputed or validated during resume rather than accepted blindly:

- total chunk count;
- completed chunk count;
- chunk completion state;
- artifact readiness;
- artifact hashes;
- effective resume stage.

### Fields that must not be blindly trusted

Never trust these values without filesystem validation:

- task `state`;
- chunk `state`;
- artifact `state`;
- completed/total counters;
- artifact paths;
- `last_error`;
- any `IN_PROGRESS` record.

All manifest paths must be normalized and verified to remain within the task directory.

### Manifest authority rule

The manifest is authoritative for:

- task identity;
- output mode;
- expected chunk plan;
- stage history;
- artifact intent;
- safe task-local paths.

The filesystem is authoritative for whether a claimed output actually exists and is usable.

## Resume Algorithm

When the user runs:

```text
meeting-asr-go resume <task-folder>
```

perform the following steps.

### 1. Acquire a task lock

Create an exclusive task-local lock, for example `.resume.lock`.

The lock prevents two processes from resuming the same task and issuing duplicate API calls for the same chunk. Store process ID and start time. Stale-lock handling must be explicit and safe.

### 2. Load and validate the manifest

1. Read `task_manifest.json`.
2. Validate JSON, schema version, required fields, output mode, and task-local paths.
3. Verify source audio exists inside the task directory.
4. Verify source size and optional hash.
5. Load credentials only for stages that need them.

If the manifest is unreadable, schema-invalid, or unsafe, do not overwrite it and do not silently fabricate a replacement.

### 3. Reconcile manifest and filesystem

For every expected chunk:

1. Verify chunk audio exists, is non-empty, and has the expected deterministic filename.
2. Verify completed transcript files exist, are non-empty, and are valid text.
3. Reclassify missing, empty, malformed, or temporary transcripts as `PENDING`.
4. Reclassify `IN_PROGRESS` as `PENDING` unless a valid final transcript exists.
5. Set `completed_chunks` from actual valid transcript files, including zero.

For artifacts:

1. Validate `transcript.md` against current valid chunk transcripts.
2. Validate `notes_input.md` against the current transcript and selected prompt identity.
3. Validate minutes only when `output_mode=minutes`.
4. If an upstream artifact changes, invalidate every downstream artifact.

Known temporary files may be cleaned up or quarantined. Never treat them as final output.

### 4. Choose the earliest incomplete stage

Choose exactly one stage in this order:

1. Source invalid: fail safely.
2. Chunks missing or invalid: `SPLITTING`.
3. Any chunk transcript missing or invalid: `TRANSCRIBING`.
4. `transcript.md` missing or inconsistent: `MERGING`.
5. Required notes input missing or inconsistent: `NOTES_GENERATING`.
6. Required minutes missing or inconsistent: `MINUTES_GENERATING`.
7. Required outputs present: `VALIDATING_OUTPUTS`.
8. All validations pass: `COMPLETED`.

### 5. Execute idempotently

For each stage:

1. Update task state to the active stage.
2. Produce only the missing or invalid outputs.
3. Validate outputs before marking them ready.
4. Atomically update the manifest after each durable checkpoint.
5. On failure, retain valid upstream outputs and transition to `FAILED`.

## Failure Scenarios

| Case | Detection | Recovery strategy | User message |
|---|---|---|---|
| Process killed during chunk 5/10 | Chunk 5 is `IN_PROGRESS` or has no valid final transcript. | Reclassify chunk 5 as pending; retain valid transcripts for all other chunks. | “Recovered task: 4 of 10 chunks are complete. Resuming from part_005.” |
| MiMo API timeout | Timeout or retry exhaustion; record retryability. | Mark current chunk failed; preserve completed chunks; retry on a future resume. | “Transcription stopped at part_005 due to a MiMo timeout. Completed chunks were preserved; run resume to retry.” |
| Merge failed after all chunks completed | All chunk transcripts validate but `transcript.md` is missing or invalid. | Resume directly at `MERGING`; do not call MiMo. | “All chunk transcripts are complete. Rebuilding transcript.md without retranscribing audio.” |
| DeepSeek generation failed | Notes input is valid but minutes are missing or invalid. | Resume directly at `MINUTES_GENERATING`; do not redo ASR, merge, or notes generation. | “Transcription and notes input are complete. Retrying meeting-minutes generation only.” |
| Power loss during manifest write | Manifest is missing, truncated, or invalid. | Atomic writes normally leave either old or new valid manifest. If corrupt, stop safely and require explicit repair. | “Task state could not be verified after an interrupted write. No task files were deleted.” |
| User deletes one transcript chunk | Reconciliation finds a missing or empty transcript for an expected chunk. | Mark that chunk pending; invalidate transcript and downstream outputs; transcribe only that chunk. | “part_004 transcript is missing. Re-transcribing that chunk and rebuilding dependent outputs.” |

## Atomicity and File Safety

For every generated artifact:

1. Write `<final-name>.tmp` in the same directory.
2. Flush and close the temporary file.
3. Validate content.
4. Atomically rename it to the final path.
5. Write the updated manifest to a temporary manifest file.
6. Flush and close it.
7. Atomically rename it to `task_manifest.json`.

Use owner-only permissions for task folders and sensitive task files.

On platforms where replacing an existing file is not atomic, document the limitation and preserve the previous manifest whenever possible.

## Idempotency Rules

A valid resume must be safe to run repeatedly.

- Re-running resume on a complete task makes no API calls.
- Re-running after a merge failure never retranscribes valid chunks.
- Re-running after a minutes failure never rebuilds notes unless notes are invalid.
- Existing valid final transcript files always win over a stale manifest chunk state.
- A chunk is never sent to MiMo twice merely because the process died after its final transcript was written.

## Stable Backend Contract for Future UI Consumers

The backend should expose stable task-level events and state values:

- stage started;
- chunk started;
- chunk completed;
- retry scheduled;
- stage failed;
- task completed.

The manifest and stdout events must describe backend facts. A future UI may display them, but must not own reconciliation or resume decisions.

## Implementation Guidance

### Implement now

1. Explicit task and chunk state enums.
2. Schema-versioned manifest.
3. Deterministic chunk discovery and validation.
4. Filesystem reconciliation before every resume.
5. Atomic artifact and manifest writes.
6. Exclusive task locking.
7. Stage-specific failure messages.
8. Tests for every failure scenario in this document.

### Do not implement yet

- distributed locking;
- parallel chunk transcription;
- cross-task deduplication or cache sharing;
- database-backed task storage;
- automatic repair of corrupted manifests during ordinary resume;
- event sourcing or extensive attempt history;
- additional provider abstractions beyond current MiMo and DeepSeek behavior.

## Acceptance Criteria

The resume design is complete when tests demonstrate all of the following:

1. Killing a task during any chunk only retranscribes invalid or missing chunks.
2. A merge failure reruns merge only.
3. A minutes failure reruns minutes only.
4. A deleted transcript chunk invalidates dependent artifacts and retranscribes only that chunk.
5. A completed task resumes without API calls.
6. Manifest counters are reconciled from valid on-disk files, including reset to zero.
7. Corrupted manifest handling preserves task files and refuses unsafe automatic recovery.
8. Concurrent resume attempts are rejected by the task lock.
