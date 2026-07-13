# Task Manifest Runtime Integration Strategy

## 1. Purpose and boundaries

This document describes how the isolated Go `taskmanifest` package could be
integrated with the existing runtime through staged, reversible changes. It is
design guidance only. It does not switch the runtime manifest, add an adapter,
write a shadow file, migrate a Task, or change storage behavior.

The target contract is documented in
[Task Manifest and Schema Design](task_manifest_schema_design.md). The current
runtime manifest remains authoritative until a separately reviewed adoption PR
meets every phase gate below.

## 2. Current runtime manifest flow

The Go runtime currently owns a separate `Manifest` type in
`core-go/manifest.go`. Its `task_manifest.json` is both the processing
checkpoint and the source used by resume.

### New Task execution

The current workflow:

1. creates a validated Task directory and copies the source audio into its
   `audio/` subdirectory;
2. constructs the current `Manifest` with `CreateManifest`;
3. calls `SetTaskState` at pipeline boundaries, which persists through
   `SaveManifest`;
4. saves chunk planning and per-chunk completion as work progresses;
5. marks transcript, notes-input, and minutes artifacts ready only after the
   corresponding files are written and hashed;
6. records a runtime error with `RecordTaskError`, which moves the current Task
   state to `FAILED`; and
7. validates required outputs before `MarkTaskCompleted` records final
   completion.

`SaveManifest` updates the current schema version and timestamp, writes a
temporary owner-only file, syncs it, and atomically replaces the current
`task_manifest.json`.

### Resume execution

Resume currently:

1. canonicalizes the Task directory and takes the exclusive Task lock;
2. loads `task_manifest.json`, or loads and migrates the older flat
   `manifest.json` when necessary;
3. migrates an older current-manifest schema explicitly;
4. validates schema, Task state, chunk and artifact integrity metadata, and
   path containment;
5. reconciles chunk and artifact state with files inside the Task boundary;
6. persists the reconciled current manifest; and
7. selects the earliest incomplete stage and resumes it idempotently.

This behavior, including its filename, layout, locking, reconciliation, state
machine, and legacy migration, must remain unchanged during the design and
shadow phases.

## 3. New `taskmanifest` integration point

PR #29 added `core-go/taskmanifest` as an isolated package. It supplies the new
logical structs, validation, version handling, fixture-backed reader/writer,
unknown-field preservation, and sensitive-field rejection. Nothing in the
existing workflow imports it today.

The first integration point should be a pure adapter at the existing current
manifest persistence checkpoints. The runtime should continue mutating and
saving its current `Manifest`; after a successful current save, the adapter may
derive a `taskmanifest.TaskManifest` in memory for shadow validation. It must
not become a second state machine or receive independent business decisions.
Its only responsibility is translating an already-decided runtime checkpoint
into the new representation. It does not own workflow decisions, retry logic,
cleanup logic, or runtime behavior.

During Phases 1 and 2:

- the current manifest is the sole execution and resume authority;
- shadow conversion cannot change runtime success or failure;
- the new reader is not used by resume;
- no current manifest or legacy migration code is replaced; and
- no physical shadow-file location is assumed by this document.

If a later implementation needs persistent shadow output, its filename,
location, permissions, atomicity, cleanup, and support policy require explicit
review. It must never overwrite the current `task_manifest.json` during shadow
operation.

## 4. Adapter responsibilities

The adapter should be deterministic, side-effect-free during conversion, and
covered by table-driven tests for every current Task and artifact state.

### Runtime state to `TaskManifest`

The adapter should:

- preserve `task_id`, creation/update timestamps, output mode, input ownership,
  and safe source metadata;
- identify the runtime backend as Go and preserve the actual processing
  provider information available from the current manifest;
- map `CREATED` to `created`, active pipeline stages to `processing`,
  `COMPLETED` to `completed`, and `FAILED` to `failed`;
- convert the current last error into a stable, non-sensitive execution error
  without copying credentials, request bodies, or sensitive logs; and
- convert current absolute Task-local paths into validated Task-relative
  artifact/input references where the new contract requires portability.

The adapter must reject paths outside the canonical Task boundary rather than
normalizing an unsafe path into apparent validity.

### Artifact updates

The adapter should map each requested output independently:

- current `PENDING` becomes `pending` or `unavailable` according to whether the
  output is requested and reachable in the selected workflow;
- current `READY` becomes `completed` only after existing path, containment,
  file, and integrity validation passes; and
- current `INVALID` becomes `failed` with a stable adapter-defined structured
  error, without inventing provider details that the current manifest does not
  contain.

Transcript, notes, and minutes states must be derived separately. A completed
transcript remains completed when later minutes generation fails. Task-level
status must not erase artifact-level partial success.

### Lifecycle updates

The adapter should run from one centralized manifest-persistence boundary, not
from scattered provider calls. Each new representation must correspond to a
successfully persisted current checkpoint. It must preserve monotonic execution
history, distinguish aggregate Task failure from artifact failure, and avoid
changing user-owned metadata.

User title, description, tags, favorites, and annotations remain outside the
execution adapter. No AI-generated metadata should be introduced while mapping
runtime state.

## 5. Staged migration strategy

### Phase 1 — shadow generation

Generate the proposed `TaskManifest` in memory after successful current
manifest checkpoints and validate it with the new package.

Required gates:

- adapter mappings cover every current Task/artifact state and output mode;
- conversion never changes the current save result or resume behavior;
- conversion errors are reported with non-sensitive diagnostics and do not
  mutate either representation; and
- fixture, adapter, and existing runtime regression tests pass.

Persistent shadow files are not required for this phase. If proposed later,
they need a separate storage decision.

### Phase 2 — validation comparison

Compare the current manifest and the in-memory shadow semantically at stable
checkpoints. Comparison should normalize representation differences and check
Task identity, execution state, artifact state, output references, source
ownership, and safe error state rather than comparing raw JSON.

Comparison should exclude or normalize values that are not stable semantic
facts, including independently generated timestamps, absolute machine paths,
and temporary runtime fields. Path comparison should use validated Task-relative
references. A difference in an excluded representation detail must not hide a
difference in Task identity, execution state, artifact state, output reference,
or error state.

Required gates:

- expected lossy mappings are documented and allowlisted narrowly;
- mismatches never expose paths, transcript content, credentials, or provider
  payloads in logs;
- completed, failed, interrupted, resumed, and partial-output scenarios have
  deterministic comparison tests; and
- comparison produces no runtime decisions and performs no migration.

### Phase 3 — runtime adoption

Only after shadow comparison is stable may a separate PR propose the new
manifest as execution authority. Adoption must include:

- a version-aware read strategy for current, new, and supported legacy Tasks;
- explicit rollback and recovery behavior;
- path containment, Task locking, reconciliation, and resume parity;
- atomic replacement, durable sync, and permission behavior suitable for a
  runtime checkpoint; and
- an explicit migration command or controlled transition rather than a silent
  rewrite during ordinary read/list operations.

PR #29's initial writer uses `os.WriteFile`; runtime adoption must not proceed
until the implementation provides and tests checkpoint-grade atomicity and
defines behavior for permissions on existing files.

### Phase 4 — old manifest removal

Remove the current runtime `Manifest` and compatibility adapter only after:

- all supported old Tasks can be read or explicitly migrated;
- rollback is no longer required by the supported compatibility window;
- fresh run, resume, interruption, corruption, and partial-output acceptance
  pass against the adopted format; and
- documentation and support procedures identify unsupported versions clearly.

Removal must be a dedicated PR. It must not also introduce Task Library, index,
cleanup, or unrelated storage behavior.

## 6. Failure handling and recovery

### Manifest write failure

During shadow phases, a current-manifest write failure retains its existing
runtime behavior and no shadow should be treated as newer truth. A shadow
conversion or validation failure is non-authoritative: it must leave the
successfully written current checkpoint untouched and must not change Task
status.

After eventual adoption, a manifest write failure must prevent the runtime from
claiming that the corresponding lifecycle transition was persisted. The last
valid manifest must remain recoverable. Temporary or partial JSON must never be
accepted as the latest checkpoint.

### Partial artifact update

Artifact files should be written and validated before their manifest state is
marked `completed`. If the artifact exists but the following manifest write
fails, recovery may discover it only through reviewed, Task-boundary-safe
reconciliation. It must verify the file and integrity metadata instead of
assuming that presence means completion.

Failure of a later artifact must not invalidate an earlier completed artifact.
The Task may report partial success while preserving a retryable structured
error for the failed artifact where the runtime can establish that safely.

### Recovery behavior

Until Phase 3, current resume remains the only recovery path. Shadow data must
never be selected for resume or used to overwrite the current checkpoint.

Future adopted recovery must:

- take the existing exclusive Task lock before read/reconcile/write;
- reject unsupported or malformed versions non-destructively;
- preserve the last valid record when migration or reconciliation fails;
- contain every path operation within the validated Task/output-root boundary;
- retain completed outputs during partial recovery; and
- avoid automatic retries solely because an artifact error says
  `retryable: true`.

## 7. Verification required for implementation PRs

Each phase should include focused unit tests plus the existing Go regression
suite. Before runtime adoption, coverage should include:

- every current Task and artifact state mapping;
- transcript-only, notes, and minutes workflows;
- fresh, completed, failed, interrupted, resumed, and partial-success Tasks;
- unknown compatible fields and unsupported schema versions;
- invalid or escaping paths and symlinks;
- write interruption and stale-manifest reconciliation;
- old/legacy manifest compatibility; and
- absence of API keys, credentials, transcript content, and sensitive provider
  data in manifests and diagnostics.

## 8. Non-goals

This PR does not:

- switch the runtime to `taskmanifest`;
- add an adapter, shadow generation, comparison, or migration execution;
- change the current Go workflow, state machine, manifest, resume, or legacy
  behavior;
- change storage layout or create a shadow file;
- change Tauri commands or desktop UI;
- add Task Library, history, index, database, or search behavior;
- implement cleanup or archive behavior;
- modify or remove Python;
- add provider abstraction;
- change CI, packaging, release, credentials, or ffmpeg behavior; or
- create generated artifacts.
