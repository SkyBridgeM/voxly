# Task Metadata and Tagging Model

## 1. Purpose and product object

Meeting ASR should organize work around a generic **Task**, not around a
meeting-management model. A Task represents:

- one audio-processing job;
- one input source;
- one selected processing workflow; and
- one related group of generated outputs.

The same model should support meetings, interviews, lectures, podcasts,
personal recordings, and other authorized audio. A task type may eventually be
expressed through user metadata or tags, but it is not a fixed product
taxonomy.

This design deliberately does not introduce attendees, calendars, a meetings
table, organizations, teams, or collaboration concepts. It is a planning
document only and does not define a committed storage schema or implement
metadata behavior.

## 2. Conceptual Task metadata

The future metadata model should cover the following groups. Fields marked as
optional may be absent without making a Task invalid.

### Identity

- `task_id`: stable, unique identifier for the Task.
- `created_at`: timestamp for initial Task creation.
- `updated_at`: timestamp for the most recent meaningful metadata or lifecycle
  change.

### User metadata

- `title`: user-facing name, with a safe generated default when the user has
  not supplied one.
- `description` (optional): user-authored context about the recording or
  desired outcome.
- `tags`: a user-defined collection used for organization and discovery.

### Input

- source-audio reference, preserving the distinction between a user-owned
  external source and any explicitly created task-owned copy;
- duration, when known; and
- safe technical metadata such as supported format, size, or media properties
  needed to describe and process the source.

The metadata should reference audio ownership and location without embedding
raw audio content.

### Processing

- backend/provider used for the run;
- selected output mode;
- current processing status; and
- a concise, non-sensitive error summary when processing fails.

Provider and backend information describes what happened for reproducibility;
this plan does not implement a provider-abstraction layer.

### Outputs

- transcript reference;
- notes-preparation reference;
- minutes or summary reference; and
- references to future user-requested exports.

Output references should describe final user-facing deliverables without
turning internal scratch files into primary Task metadata.

### Lifecycle

The conceptual lifecycle includes:

- `created`;
- `processing`;
- `completed`;
- `failed`;
- `cleaned`; and
- `archived` (future).

These names express product states for future schema design. They do not change
the current runtime state machine. Future schema work must define transitions,
partial-output behavior, compatibility, and whether lifecycle and processing
status are represented separately before implementation.

## 3. User-defined tags

Tags should be free-form user metadata rather than a fixed enum. Users should
be able to organize Tasks according to their own context, language, and
workflow. Example tags include:

- `工作`
- `学习`
- `访谈`
- `播客`
- `灵感`

Future product work may add tag filtering, recent-tag suggestions, optional tag
colors, or automatic tag suggestions. Automatic tagging would require a
separate privacy, consent, and provider review and must never silently replace
or rewrite user-defined tags.

Before implementation, a schema PR should define normalization, whitespace,
case handling, duplicate behavior, length/count limits, and safe display of
arbitrary user text. This planning PR does not add tag controls or tag UI.

## 4. Relationship with storage and discovery

The conceptual relationship is:

```text
Task
 ├── manifest or equivalent task metadata
 ├── output artifacts
 ├── user-defined tags
 └── future index
```

Task folders remain the filesystem organization and containment boundary for
task-owned data. Tags do not replace folders, rename folders, or imply that a
Task's files can be moved without validation.

The manifest or equivalent task metadata is the durable per-Task record. A
future index may provide discovery across Tasks by storing or deriving only the
minimum metadata needed for search, filters, and lifecycle views. The index
must be rebuildable or safely reconcilable with validated Task metadata so it
does not become an independent source of truth for file ownership.

In short, folders organize files; metadata describes a Task; and an index
enables discovery. This document does not change the current folder layout,
manifest, or storage behavior.

## 5. Future Task Library direction

A future **Task Library** could provide:

- search across safe Task metadata;
- filtering by user-defined tags;
- recently created or updated Tasks;
- favorite or pin behavior; and
- archive views.

The Task Library should continue to emphasize final outputs and useful user
metadata rather than internal task artifacts. Meeting-specific language should
appear only when it accurately describes a selected output or user-authored
metadata, not as the organizing model for every recording.

No history view, Task Library, search, filtering, favorites, pinning, or archive
UI is implemented by this PR.

## 6. Privacy and sensitive-data rules

Task metadata, manifests, and any future index must not contain:

- API keys or access tokens;
- secrets or sensitive credentials;
- raw audio content; or
- credential values copied from environment, secure storage, settings, logs,
  or provider requests.

Titles, descriptions, tags, source filenames, error summaries, and output
references may themselves reveal sensitive context. Future implementations
must minimize indexed data, avoid transcript excerpts by default, keep local
metadata within the established Task/output-root boundaries, and define
redaction and support-export behavior explicitly. Automatic metadata generation
must not introduce an undisclosed provider data flow.

## 7. Future implementation sequence

Implementation should proceed through independently reviewed changes:

1. **PR 1 — Task metadata schema:** define field types, validation, lifecycle
   transitions, compatibility, ownership semantics, and privacy constraints.
2. **PR 2 — manifest extension:** add versioned Task metadata to the manifest
   with migration and backward-compatibility tests.
3. **PR 3 — Task index and search backend:** introduce a local discovery index,
   reconciliation/rebuild behavior, and bounded search APIs without changing
   filesystem ownership rules.
4. **PR 4 — Task Library UI:** add the generic Task browsing experience with
   search and recent Tasks.
5. **PR 5 — tag filtering UI:** add tag editing, filtering, recent tags, and
   accessible free-form tag interaction after backend validation exists.
6. **PR 6 — cleanup and archive integration:** connect Task lifecycle metadata
   to the separately designed cleanup policy and any reviewed archive behavior.

Each PR must define observable behavior and tests before changing persisted
data. Manifest and index migrations must not be inferred from this planning
document.

## 8. Non-goals for this PR

This PR does not:

- change code, runtime behavior, or backend behavior;
- migrate storage or manifests;
- implement or extend a manifest schema;
- add Task, tag, history, search, or archive UI;
- add a database, cloud sync, account system, collaboration, organizations, or
  teams;
- add attendees, calendars, or a meetings table;
- remove Python or change the Python or Go backend;
- implement provider abstraction;
- change Tauri commands, CI, packaging, release, credentials, or ffmpeg
  behavior;
- add dependencies; or
- create generated artifacts.
