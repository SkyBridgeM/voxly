# Task Manifest Fixtures and Validation Cases

## 1. Purpose

This document and the adjacent JSON examples make the Task manifest design
concrete enough for implementation review. They are proposed fixtures only:
they are not consumed by runtime tests, do not define an active schema, and do
not change manifest writing or reading.

The examples implement the responsibilities described in
[Task Manifest and Schema Design](task_manifest_schema_design.md): execution
truth, artifact-specific lifecycle, non-sensitive errors, and resume-relevant
references. They intentionally keep frequently edited user metadata outside
the execution manifest.

Proposed fixtures:

- [`task_manifest_completed.json`](fixtures/task_manifest/task_manifest_completed.json)
- [`task_manifest_partial_success.json`](fixtures/task_manifest/task_manifest_partial_success.json)
- [`task_manifest_failed.json`](fixtures/task_manifest/task_manifest_failed.json)

All paths and identifiers are synthetic. The fixtures contain no credentials,
real recordings, raw audio, transcript content, or user-specific home paths.

## 2. Proposed common shape

The fixtures use the following logical groups:

- `schema_version`: manifest contract version;
- `task_id`, `created_at`, and `updated_at`: system-owned identity and
  timestamps;
- `execution`: backend/provider, output mode, Task status, and optional
  non-sensitive error summary;
- `input`: source reference, ownership, duration, format, and safe media
  metadata; and
- `artifacts`: independent transcript, notes, and minutes records with status
  and optional path or error summary.

This shape is implementation guidance, not a committed JSON Schema. Field
types, enum values, timestamp precision, path representation, and atomic-write
behavior must be finalized in the schema implementation PR.

## 3. Required fields

For the proposed first version, a readable execution manifest requires:

- `schema_version`;
- `task_id`;
- `created_at` and `updated_at`;
- `execution.backend`;
- `execution.provider`;
- `execution.output_mode`;
- `execution.status`;
- `input.source_reference`;
- `input.source_ownership`;
- `input.format`; and
- an `artifacts` object whose expected entries are consistent with the output
  mode and execution state.

Each present artifact requires a `status`. A `completed` artifact also requires
a validated path. A `failed` artifact requires a non-sensitive structured
`error`. An `unavailable`, `pending`, or `processing` artifact must not pretend
to have a completed path.

The schema implementation must decide whether all known artifact keys are
always required or whether unrequested artifacts may be omitted. The fixtures
use explicit entries so partial and failed outcomes remain unambiguous.

## 4. Optional fields

The proposed optional data includes:

- input duration when it cannot be determined safely;
- additional safe input media metadata;
- `execution.error_summary` when the Task-level workflow has an error;
- artifact paths for states other than `completed` only if a later schema
  explicitly defines safe partial-output semantics; and
- future extension fields permitted by a compatible schema version.

User-owned title, description, tags, favorites, and annotations are not
execution-manifest fields in these fixtures. A future physical-layout decision
may define a separate user metadata record or a stable reference without
rewriting execution history.

## 5. Missing and invalid field behavior

Future readers and validators should apply these expectations:

- Missing `schema_version`, `task_id`, timestamps, execution identity/status,
  or input reference makes the manifest invalid rather than silently inventing
  execution truth.
- Missing optional input data is represented as unknown; it must not cause a
  fabricated value.
- A missing required artifact entry, missing completed path, or missing failed
  artifact error is a validation error.
- An artifact path outside the validated Task/output-root boundary is invalid
  even when the JSON is otherwise well formed.
- Unknown fields in a compatible version may be preserved or ignored according
  to the eventual schema policy; they must not alter known execution facts.
- Inconsistent Task and artifact states must be rejected or surfaced as
  invalid. A reader must not upgrade partial success to full success.
- Validation errors should be non-destructive: reading an invalid manifest
  must not rewrite, migrate, or delete it automatically.

## 6. Validation cases

### Completed Task

`task_manifest_completed.json` represents a minutes workflow in which the
transcript, notes preparation, and minutes artifacts all completed. The Task is
`completed`, every artifact has a path, and no error summary is present.

Expected validation result: valid completed Task; all three outputs may be
presented as completed, with minutes as the primary user-facing output.

### Partial success

`task_manifest_partial_success.json` represents a Task whose transcript and
notes preparation completed while minutes generation failed. It proposes the
Task status `completed_with_errors` so aggregate state remains distinguishable
from each artifact state.

Expected validation result: valid partial outcome; completed transcript and
notes remain usable, failed minutes retains a non-sensitive error, and the Task
must not be reported as either full success or total failure. The final enum
name remains subject to schema implementation review.

### Failed Task

`task_manifest_failed.json` represents processing failure before a transcript
was produced. The transcript is `failed`; downstream notes and minutes are
`unavailable`; Task-level and artifact-level errors contain only stable code
and safe summary text.

Expected validation result: valid failed Task with diagnostic information and
no completed artifact paths. Resume eligibility must be decided by future
runtime rules rather than inferred from this fixture alone.

## 7. Artifact error model

A failed artifact should carry a structured error with:

- `code`: a stable machine-readable identifier;
- `message`: a concise, non-sensitive user- or developer-readable summary; and
- `retryable`: a boolean indicating whether retry may be appropriate under the
  same logical Task workflow.

Task failure and artifact failure are different. A Task-level error summarizes
the aggregate execution outcome, while an artifact error explains why one
output failed. In partial success, completed artifacts must remain valid and
recoverable even when another artifact fails. A future retry should be able to
target or resume the failed work without discarding completed outputs, subject
to separately implemented resume rules.

The `retryable` value is guidance, not automatic authorization to retry. It
must not bypass credential, provider, input, lifecycle, or user-confirmation
requirements.

## 8. Fixture portability

Fixtures must be deterministic and environment independent so they can support
future local and CI validation. They must:

- contain no absolute local paths, home directories, usernames, or machine
  identifiers;
- use relative or clearly synthetic example paths;
- avoid relying on files, providers, credentials, locale, timezone, or state
  unique to the author's environment; and
- remain valid inputs for future schema validation in CI without network or
  live API access.

The current fixtures use UTC timestamps, synthetic identifiers and provider
names, relative example input references, and Task-relative artifact paths.
They do not require the referenced example audio files to exist for JSON/schema
validation. Runtime fixture integration and filesystem behavior remain outside
this PR.

## 9. `schema_version` and compatibility

The examples use proposed version `1.0`. This value does not activate a schema
or promise that the implementation must use this exact version format.

Future validation should:

- require an explicit version for newly written manifests;
- select a version-aware reader rather than guessing from present fields;
- read supported older versions without rewriting them merely for inspection;
- define additive fields and unknown-field handling for compatible versions;
- reject an unsupported newer major version with a clear, non-destructive
  error; and
- migrate only through an explicit, tested, non-destructive operation that
  preserves Task identity, execution history, artifact states, source
  ownership, and user authority.

Backward compatibility tests should include the oldest supported fixture,
unknown compatible fields, a missing version, an unsupported newer version,
and migration failure recovery. No backward-compatibility reader or migration
is implemented in this PR.

## 10. Non-goals

This PR does not:

- implement or activate a manifest schema;
- add or change Go, Python, backend, or runtime code;
- change manifest readers or writers;
- migrate storage or existing Tasks;
- add database or index behavior;
- add or change desktop UI;
- implement cleanup or resume behavior;
- add CI or runtime test integration; or
- create generated runtime artifacts.
