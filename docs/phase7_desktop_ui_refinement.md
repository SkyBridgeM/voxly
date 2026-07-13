# Phase 7 Desktop UI Refinement

## 1. Objective and baseline

Phase 7 refines the existing Tauri desktop experience after the Phase 5
runtime foundation and Phase 6 unsigned distribution path were completed. The
goal is to make the current runtime policy and three output workflows easier to
understand without changing how the application runs.

PR #20 is an audit and copy baseline. It documents the current UI, fixes a
small set of clearly stale or misleading labels, and proposes focused follow-up
work. It is not a full visual redesign.

## 2. Current UI inventory

The desktop frontend is a single React screen without routing. `App.tsx` owns
runtime state and composes four small presentation components.

| Area | Current implementation | User purpose |
| --- | --- | --- |
| App shell | Header, subtitle, settings button, scrollable main content | Identifies the app and opens configuration |
| Audio input | `FilePicker` | Selects one local MP3, WAV, or M4A file |
| Output mode and run action | `RunControls` | Chooses transcript, notes preparation, or meeting minutes and starts a run |
| Progress and logs | `LogPanel` | Shows progress and optionally exposes redacted child-process logs |
| Results | `ResultPanel` | Lists the task folder and generated files, then opens the task folder |
| Settings modal | Inline in `App.tsx` | Shows readiness, language, backend policy, theme, session credentials, development `.env` compatibility, and DeepSeek model |
| Error modal | Inline in `App.tsx` | Displays a structured backend error message |
| Localization | `i18n.ts` | Supplies Chinese and English labels for most visible text |
| Tauri bridge | `lib/meetingAsr.ts` | Provides typed runtime metadata, preflight, run, credential, and folder commands |

The current layout is compact and functional. Phase 7 should preserve the
single-screen workflow until a concrete navigation need is demonstrated.

## 3. Runtime and workflow assumptions found

### Correct current behavior

- Development defaults to Python and may opt into experimental Go.
- Production uses the packaged Go backend and does not offer Python selection.
- Preflight is local-only and reports runtime mode, backend, Go executable,
  ffmpeg, output location, and credential source metadata without secret values.
- Transcript and notes readiness use the MiMo requirement; minutes readiness
  additionally requires DeepSeek.
- Session API-key values are not persisted by the frontend. Development alone
  retains the explicit project-root `.env` compatibility action.
- Results are contained in a task folder under the configured output root.

### Outdated or confusing items

The audit found these user-facing issues:

1. The header still described an early Python-via-`uv` Tauri prototype, which
   was false for the production packaged-Go policy.
2. A Go preflight result always used the production Go label, including when
   the app was in development with the experimental Go backend.
3. Credential fields told production users to leave the value blank for
   `.env`, while production intentionally never reads the project-root `.env`.
4. The three output choices mixed implementation terms (`LLM Summary`,
   `Notes input`) with user outcomes, making transcript, notes preparation, and
   meeting minutes harder to compare.
5. The results action said “Open Output Folder” even though it opens the
   specific task folder.
6. “Backend”, “sidecar”, and “output root” are accurate engineering terms but
   need clearer user-facing labels around the retained diagnostic details.

PR #20 corrects those labels in Chinese and English. The backend label helper
uses the existing typed `RuntimeMode` and `BackendKind`; it does not alter
backend selection or execution.

## 4. Follow-up audit findings

These items should be handled in later focused PRs rather than expanded here:

- The settings modal mixes readiness, appearance, backend policy, credentials,
  and model selection in one long list without grouping or section hierarchy.
- Theme labels are still embedded directly in `App.tsx` rather than fully
  represented in the translation interface.
- The settings icon and modal close controls need an accessibility pass for
  explicit accessible names, keyboard behavior, and focus restoration.
- Raw preflight detail messages come from the backend and may not match the
  selected UI language; stable status labels and diagnostic detail should be
  visually distinguished before considering localization.
- The DeepSeek model control is always visible even though it affects only the
  minutes workflow; future helper text can clarify this without hiding useful
  configuration unexpectedly.
- Run completion, result files, errors, and logs compete for attention after a
  run. Their hierarchy should be tested with success, partial-output, and
  failure states before changing layout.
- Output-location status is visible but configuration and reset behavior are
  not explained in the main workflow.
- Secure-store status exists, but a credential-management UI remains a separate
  product decision and must not be implied by status-only metadata.

## 5. Proposed Phase 7 PR sequence

1. **PR #20 — UI audit and copy baseline:** inventory the current UI and remove
   stale Python-first, backend, mode, credential, and folder labels.
2. **Settings information hierarchy:** group readiness, appearance, processing,
   and credentials without changing settings storage or commands.
3. **Run workflow clarity:** refine mode guidance, readiness callouts, start and
   disabled states, and the distinction between local checks and live API use.
4. **Results and recovery states:** clarify output files, task-folder actions,
   partial results, backend errors, and log disclosure without changing runtime
   error contracts.
5. **Accessibility and localization:** complete translation coverage, keyboard
   and focus behavior, accessible names, contrast review, and narrow-window QA.
6. **Visual refinement:** make measured spacing and hierarchy improvements only
   after the preceding behavior and copy are stable.

Each follow-up should remain independently reviewable and preserve the current
runtime contract.

## 6. Strict non-goals

PR #20 does not:

- change runtime or backend behavior;
- change production packaged-Go policy or development Python default;
- remove Python or change the experimental Go path;
- change Tauri commands, typed payloads, process arguments, or state machines;
- change credential storage, secret resolution, or `.env` policy;
- change output-root containment or task-folder behavior;
- change ffmpeg discovery, installation, downloading, or bundling;
- change packaging, CI, release, signing, notarization, or updater behavior;
- add routes, dependencies, a design system, or a settings migration;
- perform a full layout redesign; or
- add telemetry, live API checks, or public release artifacts.
