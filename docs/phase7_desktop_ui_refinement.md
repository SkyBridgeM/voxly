# Phase 7 Desktop UI Refinement

## 1. Objective and baseline

Phase 7 refines the existing Tauri desktop experience after the Phase 5
runtime foundation and Phase 6 unsigned distribution path were completed. The
goal is to make the current runtime policy and three output workflows easier to
understand without changing how the application runs.

PR #20 is an audit and copy baseline. It documents the current UI, fixes a
small set of clearly stale or misleading labels, and proposes focused follow-up
work. It is not a full visual redesign.

PR #21 is the modern desktop UI refresh. It keeps the same single-screen
workflow and frontend state while introducing a clearer card hierarchy,
selectable output-mode cards, a stronger primary action, more composed
progress/result states, and a sectioned Settings dialog. The work is
frontend-only: it does not change runtime policy, Tauri commands, backend
payloads, credential handling, ffmpeg behavior, packaging, release, or CI.

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

## 4. PR #21 modern desktop refresh

PR #21 addresses the broad visual hierarchy findings from the audit without
turning the app into a multi-screen product or introducing a design-system
dependency. The refresh includes:

- a softer desktop background and more deliberate app-header hierarchy;
- a unified workflow card with a modern file picker, selectable mode cards,
  and a visually prominent start action;
- clearer empty, processing, log, result, warning, and error presentation;
- six visible Settings sections: Local readiness, Appearance and language,
  Processing backend, Credentials, Minutes model, and Output location; and
- localized section headings, helper text, theme labels, and accessible names
  needed by the refreshed presentation.

All existing controls and diagnostic details remain available. Selection,
disabled/loading behavior, preflight readiness semantics, execution commands,
credential resolution, and output paths are unchanged.

## 5. PR #22 accessibility and interaction QA

PR #22 is a narrow frontend accessibility and interaction pass after the
modern refresh. It adds contained Settings-dialog focus management, Escape
handling, focus restoration, associated form labels, and keyboard focus
containment without changing settings values or commands. It also preserves
native output-mode radio behavior and the scoped live-region policy: progress,
final status, and results may announce concise updates while logs remain
`aria-live="off"`.

The pass raises muted-text contrast in light and dark themes and adds small
wrapping safeguards for compact headers, Settings metadata, status rows, and
result paths. These are targeted QA fixes rather than another visual redesign.

## 6. Remaining follow-up findings

These items should be handled in later focused PRs rather than expanded here:

- Full assistive-technology testing on supported macOS versions remains a
  release QA activity beyond this source-level and keyboard interaction pass.
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

## 7. Proposed Phase 7 PR sequence

1. **PR #20 — UI audit and copy baseline:** inventory the current UI and remove
   stale Python-first, backend, mode, credential, and folder labels.
2. **PR #21 — modern desktop UI refresh:** modernize the app shell, workflow
   cards, mode selection, primary action, status presentation, and Settings
   organization while remaining frontend-only.
3. **PR #22 — accessibility and interaction QA:** tighten keyboard and dialog
   focus behavior, accessible names, scoped announcements, theme contrast, and
   compact-window wrapping without changing application behavior.
4. **Results and recovery states:** test and clarify output files, task-folder
   actions, partial results, backend errors, and log disclosure without changing
   runtime error contracts.
5. **Narrow-window and localization QA:** test compact desktop dimensions,
   long translated strings, scrolling, and status density in realistic states.

Each follow-up should remain independently reviewable and preserve the current
runtime contract.

## 8. Strict non-goals

PR #20 through PR #22 do not:

- change runtime or backend behavior;
- change production packaged-Go policy or development Python default;
- remove Python or change the experimental Go path;
- change Tauri commands, typed payloads, process arguments, or state machines;
- change credential storage, secret resolution, or `.env` policy;
- change output-root containment or task-folder behavior;
- change ffmpeg discovery, installation, downloading, or bundling;
- change packaging, CI, release, signing, notarization, or updater behavior;
- add routes, dependencies, a design system, or a settings migration;
- introduce a routing rewrite or multi-screen information architecture; or
- add telemetry, live API checks, or public release artifacts.
