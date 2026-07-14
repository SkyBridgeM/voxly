# Task Library and Storage Presentation Model

## Purpose

This document defines how a future Task Library should present processed audio Tasks without exposing the internal task folder as the primary product experience. It is a design contract only. It does not implement a library UI, index, search, cleanup, database, or storage migration.

`Task` remains the core product object: one input source, one processing workflow, and one related set of outputs. The model is intentionally generic and applies to meetings, interviews, lectures, podcasts, and other recordings.

## User-facing Task model

A Task Library entry should contain enough information for a person to recognize a Task and reach its useful outputs:

- **Title:** user-provided title first, then sanitized source filename, then a date/time fallback. AI-generated titles are not a default.
- **Status:** a user-readable projection of the Task lifecycle.
- **Created time:** when processing was first recorded.
- **Updated time:** when execution state, outputs, or user metadata last changed.
- **Tags:** free-form user-owned organization metadata.
- **Primary outputs:** available transcript, notes preparation, and summary/minutes deliverables.

Descriptions, favorite/pin state, and user annotations may be added by the future user-metadata layer. They are organization data and must not rewrite execution history unnecessarily.

## User presentation and internal storage

The Task Library must distinguish product presentation from filesystem implementation.

### User view

The default Task view should emphasize:

- recognizable title and timestamps;
- lifecycle status and a concise, non-sensitive error summary when action is needed;
- tags and other user-owned organization metadata;
- final meaningful outputs;
- an explicit retry or recovery state when a requested output failed and recovery is supported.

Filesystem paths may remain available under secondary details or an “open task folder” action, but they should not dominate the Task card or result view.

### Internal storage

The task folder may contain:

- `TaskManifest`, which owns execution truth and resume state;
- final output artifacts;
- execution metadata and integrity information;
- temporary chunks, retry scratch files, logs, and other intermediate artifacts.

These files support processing, recovery, diagnostics, and future maintenance. Their presence does not make them user-facing deliverables. The library must not treat every file in a task folder as an output.

## Task lifecycle presentation

The future library should present the runtime lifecycle without inventing a second status system:

| Runtime status | User meaning | Presentation guidance |
| --- | --- | --- |
| `created` | Task exists but processing has not materially started | Show as queued or ready to process. |
| `processing` | At least one required processing stage is active or pending | Show progress/status; do not imply completion. |
| `completed` | All requested primary outputs completed successfully | Emphasize the primary output and completion time. |
| `completed_with_errors` | Useful output exists, but a requested downstream output failed | Preserve completed outputs and show the failed output as recoverable or blocked. |
| `failed` | No sufficient requested deliverable completed, or a non-recoverable execution failure blocks the Task | Show a concise failure state and safe next action when one exists. |

Artifact states remain independent from Task status. A completed transcript can coexist with failed minutes, and the library must not hide or downgrade the completed transcript.

## Artifact presentation hierarchy

### Primary deliverables

Primary outputs are user-meaningful results:

1. Summary/minutes, when requested and completed.
2. Notes preparation, when requested and no completed summary/minutes is available.
3. Transcript, when it is the requested result or the most complete available output.

All completed primary outputs should remain accessible, even when one is visually emphasized. Failed requested outputs should show their status without replacing completed outputs with a generic Task failure screen.

### Secondary and internal details

The following belong in secondary or advanced details:

- source reference and task folder location;
- raw generated files that are not final deliverables;
- chunk and retry state;
- debug information and transient logs;
- intermediate JSON or prompt-composition files;
- integrity hashes and execution diagnostics.

Debug details must remain non-sensitive. API keys, credentials, secrets, raw audio content, and environment values must never be copied into presentation metadata.

## Storage layout principles

### TaskManifest ownership

`TaskManifest` is the authoritative record for:

- Task identity and execution lifecycle;
- input reference and ownership classification;
- requested output mode;
- artifact state, location, integrity, and structured errors;
- checkpoint and resume information.

The library may read this information, but it must not casually mutate execution history. User-editable title, description, tags, favorite/pin state, and annotations belong to a separate metadata responsibility when implemented.

### Artifact locations

- Final and intermediate artifacts remain inside validated task boundaries unless an explicit future export operation creates a user-selected copy.
- Stored artifact references must remain stable enough for resume and result presentation.
- The library must use manifest-declared artifacts and validated metadata rather than infer primary outputs by scanning arbitrary filenames.
- Folder structure remains filesystem organization; metadata supplies meaning and discovery.

### Future index

A future index may accelerate recent-task lists, filtering, and search, but it is a derived discovery layer rather than execution truth. It should be rebuildable from Task manifests plus user metadata where practical. Index corruption must not alter TaskManifest or artifact files.

No index format, database, migration, or synchronization mechanism is selected in this document.

## Cleanup integration boundary

Cleanup and deletion must follow the separate storage cleanup lifecycle design in [storage_cleanup_lifecycle.md](storage_cleanup_lifecycle.md). The Task Library may eventually expose those capabilities, but it must not implement deletion by treating the task folder as an undifferentiated directory.

### User delete

- Destructive Task deletion requires explicit confirmation and a validated task boundary.
- The UI should show what will be removed and whether source audio is external, copied, or otherwise retained.
- A failed or partial deletion must produce a clear result and preserve recoverable metadata.

### Automatic cleanup

- Automatic cleanup should target reproducible intermediates, transient logs, retry scratch data, and failed-run scratch output.
- Final transcript, notes, summary/minutes, minimal Task metadata, and user metadata are preserved by default.
- Automatic cleanup must use dry-run planning and must not silently become Task deletion.

### Audio retention policy

- Externally referenced user-owned source audio is never deleted.
- A future copied-audio policy must explicitly distinguish a managed task copy from the original source.
- The product must not silently create or delete extra audio copies.
- Retention settings, if added, must state which copy is managed and what recovery impact cleanup has.

## Future implementation boundaries

A future implementation should keep these responsibilities separate:

1. Read-only Task discovery from existing manifests and user metadata.
2. A derived index for efficient listing and filtering.
3. Task Library presentation of status and primary outputs.
4. User metadata editing.
5. Explicit cleanup/archive integration following dry-run and confirmation rules.

Each stage should preserve old Task readability, tolerate partial tasks, and avoid rewriting execution manifests for presentation-only changes.

## Non-goals

This design does not add or authorize:

- Task Library UI implementation;
- database or index implementation;
- search or filtering implementation;
- cleanup, deletion, archive, or migration behavior;
- storage layout or manifest schema changes;
- runtime, backend, Tauri, Python, Go, or provider changes;
- cloud sync, accounts, or collaboration;
- CI, packaging, or release changes.
