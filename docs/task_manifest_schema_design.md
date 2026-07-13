# Task Manifest and Schema Design

## 1. Purpose and scope

The Task manifest is the durable execution record for one audio-processing
Task. Its future responsibilities are to provide:

- execution truth for the workflow that ran;
- references to validated generated artifacts;
- processing lifecycle state; and
- enough stable information to validate and resume supported work.

This document describes a conceptual schema and ownership boundaries. It does
not add a machine-readable schema, change an existing manifest, migrate stored
Tasks, or modify runtime behavior. The generic Task model and metadata authority
rules are defined in [Task Metadata and Tagging Model](task_metadata_tagging_model.md).

## 2. Schema version strategy

Every future persisted Task manifest should contain a top-level
`schema_version`. The version identifies the contract used to interpret the
document; it is not the application version or backend version.

Before implementation, the project should define:

- a version format with unambiguous comparison rules;
- which changes are additive and backward-compatible;
- which changes require an explicit migration;
- the oldest versions supported for reading and resume; and
- behavior for an unknown or newer version.

Readers should never guess how to interpret an unsupported version. Old Tasks
should remain readable whenever safely possible, even if a newer feature is not
available for them. Migration must be explicit, tested, and non-destructive:
the original record should remain recoverable until the upgraded record has
been validated. Opening or listing a Task must not silently rewrite its
manifest.

## 3. Conceptual Task data sections

The fields below describe logical Task data and ownership. They are not a
committed JSON layout, field type definition, or migration instruction.

### 3.1 Execution metadata

System-owned execution metadata includes:

- `task_id`: stable Task identity;
- `created_at`: Task creation timestamp;
- `updated_at`: timestamp of the latest system-owned lifecycle change;
- backend and provider identifiers used by processing;
- `output_mode`: selected processing workflow;
- `status`: current processing lifecycle state; and
- a concise, non-sensitive error summary when execution fails.

The application workflow controls these values. They describe processing
history and must not be casually editable by users. Status, error state,
`updated_at`, and other evolving fields may change only through defined system
transitions.

### 3.2 Input metadata

Input metadata includes:

- a source-audio reference and explicit ownership classification;
- duration, when known;
- media format; and
- safe source metadata needed for processing or display, such as size or
  technical media properties.

A reference to user-owned audio outside the Task folder does not make that file
Task-owned. The manifest must not embed raw audio, create audio copies, or imply
permission to delete the source. Any future copied-audio behavior requires an
explicit ownership policy and separate implementation review.

### 3.3 Artifact metadata

Artifact metadata includes references for:

- transcript output;
- notes-preparation output;
- minutes or summary output;
- other generated files approved by the schema; and
- an independent lifecycle status for each artifact.

The artifact lifecycle should support states such as:

- `pending`;
- `processing`;
- `completed`;
- `failed`; and
- `unavailable`.

Exact state names and transitions belong to schema implementation review, but
artifact status must remain distinguishable from the overall Task status. One
Task may partially succeed: for example, its transcript may be completed while
its minutes or summary fails. The manifest must preserve that outcome rather
than reducing all artifacts to a single success/failure flag or claiming that
completed output is unavailable because a later artifact failed.

Artifact references should be relative to, or otherwise validated against, the
Task boundary and configured output root. The schema must distinguish final
user-facing outputs from reproducible intermediates and scratch data. A path
alone must not establish ownership or validity.

### 3.4 User metadata

User-owned organization data includes:

- title;
- optional description;
- free-form tags;
- future favorite or pin state; and
- future user notes or annotations.

These fields may change independently of execution history. They logically
belong to the Task model but should live in a separate future metadata layer,
not as frequently rewritten execution-manifest fields. The execution manifest
may eventually carry a stable reference to that layer, but this design does not
choose a storage format or add such a reference.

When a value may come from several sources, its source should be identifiable.
For example, a future title source may be `user`, `filename`, `generated`, or
`fallback`. User edits have highest authority; automatic inference must not
silently overwrite them. AI-generated metadata must remain optional and be
identifiable as generated.

## 4. Ownership and update rules

The future schema should enforce these authority boundaries:

- The system owns Task identity, input references, processing lifecycle,
  backend/provider facts, output mode, execution errors, and artifact
  references.
- Users own titles, descriptions, tags, favorite or pin state, and
  annotations.
- System-owned fields change only through validated workflow transitions or
  explicit compatibility migration.
- User-owned fields may change without mutating the record of how processing
  executed.
- Automatically inferred values have lower authority than user-authored values.
- AI-generated user metadata is optional, records its origin, and cannot
  silently replace a user edit.

Separating ownership prevents ordinary organization changes from rewriting
execution history or weakening resume and support evidence.

## 5. Lifecycle and resume rules

The manifest should describe processing lifecycle accurately enough to
distinguish created, processing, completed, failed, and future cleaned or
archived states. Before implementation, each state and transition must define:

- required and optional fields;
- which artifacts may exist;
- whether resume is supported;
- behavior for partial outputs and interrupted writes; and
- validation and error handling for inconsistent state.

Resume must use validated execution state and artifact references rather than
assuming that files exist because paths are present. A completed state must not
be recorded before required artifacts and the updated manifest are durably and
consistently written. The implementation design must separately define atomic
write or recovery behavior.

## 6. Compatibility and migration rules

Future implementation must provide:

- schema-version-aware readers and writers;
- fixtures for every supported version;
- an explicit migration strategy for incompatible changes;
- backward compatibility for reading old Tasks where safe;
- stable handling of unknown fields in compatible versions; and
- non-destructive migration with validation and recoverable failure behavior.

Migration must preserve Task identity, execution history, artifact references,
metadata ownership, and source authority. It must not invent successful states,
erase errors, reclassify external audio as Task-owned, or overwrite user edits.
Index rebuilds and user-metadata upgrades should remain separate from execution
manifest migration unless a reviewed compatibility plan requires coordination.

## 7. Security and privacy rules

The Task manifest and user metadata layer must never contain:

- API keys or access tokens;
- credentials or secure-store values;
- secrets copied from environment, settings, provider requests, or logs; or
- raw audio content.

Error summaries must be non-sensitive and must not include headers, request
bodies, credential values, or unnecessary transcript excerpts. Paths and
filenames may reveal sensitive context, so implementations should store only
what is required, use validated Task-relative references where possible, and
redact data in support exports and diagnostics.

## 8. Storage, manifest, metadata, and index relationship

The layers have distinct responsibilities:

```text
Task folder
  stores validated Task-owned files

Execution manifest
  describes processing truth, lifecycle, and artifact references

User metadata layer
  describes user organization, labels, tags, descriptions, and annotations

Future index
  enables discovery and search from rebuildable metadata
```

The Task folder is the filesystem containment boundary. The manifest describes
execution but does not replace path validation. The user metadata layer must
not rewrite execution truth when organization fields change. A future index is
a discovery layer, not an authority for execution history, artifact ownership,
or deletion, and should be rebuildable from validated Task records.

These are logical responsibilities, not a prescribed physical file layout.
Because user metadata changes more frequently than execution history, a future
implementation may store it in a separate physical layer, but this document
does not decide whether that means another file, store, or representation. The
implementation phase owns the final physical-layout decision after atomicity,
backup, portability, performance, and failure-recovery requirements are known.

Schema compatibility and migration strategy must describe logical records and
ownership rather than depend on the current folder layout. A future migration
must be able to evolve the physical representation without changing execution
truth, user authority, or validated Task boundaries merely because current
files happen to be colocated.

No storage layer, metadata file, database, or index is created by this design
PR.

## 9. Future implementation order

Implementation should proceed through separately reviewed changes:

1. **Schema fixtures:** agree on versioned valid, partial, failed, legacy, and
   invalid examples plus compatibility expectations.
2. **Go manifest writer:** implement version-aware, validated, safe writes and
   corresponding readers without changing user metadata ownership.
3. **Migration support:** add explicit, non-destructive migrations and
   backward-compatibility tests for supported old Tasks.
4. **Task index:** add a rebuildable local discovery/search layer after the
   manifest and user metadata contracts are stable.
5. **History UI:** expose generic Tasks, lifecycle, final outputs, and user
   organization data after backend contracts exist.

Each implementation step must define observable behavior, security boundaries,
failure recovery, and tests. This sequence is planning guidance and does not
authorize an implementation PR automatically.

## 10. Non-goals for this PR

This PR does not:

- add code or change runtime behavior;
- implement a schema or alter manifest writing;
- migrate manifests, Tasks, or storage;
- add a database, metadata store, or index;
- add or change desktop UI;
- implement backend behavior or cleanup;
- remove Python or change the Python or Go backend;
- change Tauri commands, CI, packaging, release, credentials, or ffmpeg
  behavior;
- add dependencies; or
- create generated artifacts.
