# Task Cleanup and Storage Lifecycle Plan

## 1. Purpose

This document defines a safe future lifecycle for Meeting ASR task data. It is
a planning document only: it does not add deletion behavior, cleanup commands,
desktop controls, storage migrations, or manifest changes.

Cleanup should reduce user-visible clutter without treating a task folder as
the primary product experience. Final user-facing outputs must remain safe,
while enough non-sensitive metadata is retained for resume, debugging, future
history views, and best-effort support. Cleanup must never accidentally delete
user-owned source audio.

## 2. Cleanup levels

Future cleanup behavior should expose distinct operations rather than one
ambiguous action:

1. **Clean intermediate files:** remove reproducible processing data that is no
   longer required for the task's completed outputs, resume policy, or audit
   record.
2. **Clean debug details:** remove transient or verbose diagnostic files while
   preserving a minimal non-sensitive run summary.
3. **Delete task:** remove the validated task folder and all data explicitly
   owned by that task. This is destructive and requires stronger confirmation
   than intermediate cleanup.
4. **Archive or export:** a future, separately designed operation may package
   selected final outputs and metadata before cleanup or deletion. This plan
   does not define an archive format or implement export behavior.

Each level must have an explicit, reviewable file policy. A lower cleanup level
must not silently escalate into task deletion.

## 3. Mandatory safety rules

Any future implementation must enforce all of the following rules in the
backend before exposing cleanup in the desktop UI:

- Cleanup may operate only inside the configured and canonicalized output
  root.
- Cleanup may operate only on a validated task folder contained by that root.
  A directory name or user-provided path alone is not sufficient proof.
- Path containment must be checked again immediately before execution. Cleanup
  must not follow symlinks or other path indirections outside the output root
  or validated task boundary.
- Original source audio must never be deleted by default.
- Every destructive operation must first produce a dry-run inventory describing
  the candidate files, cleanup level, estimated file count, and estimated
  bytes. Execution must be based on a validated dry-run result or equivalent
  immutable plan, not on an unchecked second path scan.
- Destructive execution requires explicit user confirmation. Task deletion
  must use wording that clearly distinguishes it from cleaning intermediates.
- Once implemented, the cleanup outcome must be recorded in the manifest or
  equivalent task metadata, including the requested level, completion state,
  timestamp, and any partial failures. This requirement does not change the
  current manifest schema in this PR.
- Partial failures must be reported without claiming full success. The task
  must remain inspectable, retryable where safe, and must not be left with
  falsely updated metadata.
- Credentials, application configuration, source files outside the task, and
  any file outside the validated task boundary must never be cleanup targets.
- Cleanup must reject unknown or malformed task ownership metadata rather than
  guessing that a directory is safe to delete.

The backend remains the security boundary. UI confirmation alone must never be
treated as path validation or deletion authorization.

## 4. Files eligible for cleanup

Subject to the selected cleanup level, validated ownership, resume policy, and
a successful dry run, future cleanup may consider:

- temporary audio chunks created during processing;
- retry scratch files and abandoned temporary writes;
- transient child-process logs;
- verbose debug logs beyond the retained minimal summary;
- generated intermediate JSON that is safely reproducible and not required by
  resume, history, support, or final-output validation; and
- failed-run scratch output that is not a valid final deliverable.

Eligibility is not automatic permission to delete. The implementation must
maintain an allowlist for each cleanup level and must treat unknown files as
preserved until their ownership and lifecycle are defined.

## 5. Data preserved by default

Intermediate and debug cleanup must preserve, when produced:

- the final transcript;
- the final notes-preparation file;
- the final meeting minutes;
- the manifest or equivalent task metadata;
- a minimal non-sensitive run summary;
- the task identifier and relevant timestamps; and
- enough metadata to render the task accurately in a future history UI.

Preserved metadata must not contain API keys, credential values, unnecessary
audio content, or sensitive transcript excerpts. A file required for a
supported resume path must also remain preserved unless the user chooses a
separately defined operation that explicitly makes the task non-resumable.

Only the future **Delete task** operation may remove these default-preserved
task-owned files, and only after its dedicated dry run and confirmation.

## 6. Original audio ownership policy

The original recording and task-generated audio data require different rules:

- When the task references source audio outside the task folder, cleanup and
  task deletion must not modify, move, or delete that source file.
- If a future setting copies source audio into a task folder, task metadata and
  the cleanup preview must clearly distinguish the task-owned copy from the
  user-owned source. The UI must state whether the copy would be retained or
  deleted for the selected operation.
- The application must never create an additional audio copy silently and must
  never silently delete either the external source or a task-owned copy.
- A path located inside a task folder is not by itself proof that the
  application owns the audio. Ownership must be explicit and validated.

## 7. Desktop UI model

The primary result experience should continue to emphasize the transcript,
notes preparation, or meeting minutes. Cleanup is a secondary lifecycle action
and must not be mixed into the primary completion presentation introduced in
PR #23.

Future desktop behavior should:

- place cleanup under a secondary action area such as **More actions** or task
  management;
- identify the cleanup level and explain what will be preserved;
- show the dry-run file count and estimated reclaimed space before execution;
- require explicit confirmation for destructive cleanup and stronger,
  unambiguous confirmation for deleting a task;
- show a clear success, failure, or partial-failure result; and
- keep final deliverables visible and usable after intermediate cleanup.

No cleanup button or other UI behavior is added by this planning PR.

## 8. Future implementation sequence

The lifecycle should be implemented as separately reviewable changes:

1. **PR 1 — cleanup lifecycle plan:** establish policy, boundaries, and staged
   implementation requirements in documentation.
2. **PR 2 — backend dry-run cleanup command:** return a validated, non-mutating
   cleanup inventory with file count, estimated bytes, preservation decisions,
   and stable error handling.
3. **PR 3 — backend cleanup execution with tests:** execute an explicitly
   confirmed dry-run plan, enforce containment and symlink protections, record
   outcomes, and test success and partial-failure cases.
4. **PR 4 — desktop cleanup UI:** expose the reviewed backend behavior behind
   previews and confirmations without changing the primary result hierarchy.
5. **PR 5 — cleanup state in task metadata and results:** surface retained
   lifecycle state in the manifest or equivalent metadata and in secondary
   result details.
6. **PR 6 — optional task history/archive integration:** connect lifecycle
   state to a future history view and separately reviewed archive/export
   behavior.

Each implementation PR must define observable behavior, failure handling,
tests, compatibility expectations, and migration impact before changing data.

## 9. Non-goals for this PR

This PR does not:

- implement deletion or cleanup;
- add a cleanup button or change UI behavior;
- migrate storage or change the manifest schema;
- change runtime or backend behavior;
- add or change Tauri commands;
- alter output-root or task-folder behavior;
- remove Python or change either the Python or Go backend;
- change credential or ffmpeg handling;
- change CI, packaging, release, signing, notarization, or updater behavior;
- add dependencies; or
- create generated artifacts.
