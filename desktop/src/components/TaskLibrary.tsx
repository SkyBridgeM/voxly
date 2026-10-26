import { CacheCleanupPanel } from "./CacheCleanupPanel";
import type { CacheLabels } from "./CacheCleanupPanel";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  listTasks,
  listProviders,
  generateTaskContent,
  resumeTask,
  retryTaskGeneration,
  cancelTask,
  openTaskArtifact,
  revealTaskArtifact,
  updateTaskMetadata,
  parseBackendError,
  type ArtifactSummary,
  type DiscoveryResult,
  type ProviderCatalog,
  type TaskBlockingReason,
  type TaskSummary,
  type PromptTemplateSummary,
  type ProviderSelection,
  type TaskMetadataUpdate,
} from "../lib/meetingAsr";

export interface TaskLibraryLabels extends CacheLabels {
  taskLibraryTitle: string;
  taskLibraryDescription: string;
  newTask: string;
  refresh: string;
  refreshing: string;
  taskLibraryLoading: string;
  taskLibraryEmpty: string;
  taskLibraryEmptyDescription: string;
  taskLibraryLoadError: string;
  retry: string;
  partialErrors: string;
  discoveryIssues: string;
  updatedAt: string;
  createdAt: string;
  outputMode: string;
  modeTranscript: string;
  modeNotes: string;
  modeMinutes: string;
  outputs: string;
  tags: string;
  available: string;
  availablePartial: string;
  unavailable: string;
  artifactError: string;
  generationIncomplete: string;
  generationRefused: string;
  generationUnverified: string;
  generationEmpty: string;
  generationCancelled: string;
  retryable: string;
  notRetryable: string;
  invalidDate: string;
  statusRunning: string;
  statusCompleted: string;
  statusCompletedWithErrors: string;
  statusFailed: string;
  statusInterrupted: string;
  statusCancelled: string;
  statusPending: string;
  statusProcessing: string;
  statusUnavailable: string;
  statusUnknown: string;
  artifactTranscript: string;
  artifactTimedTranscript: string;
  artifactVADSegments: string;
  artifactNotes: string;
  artifactMinutes: string;
  openArtifact: string;
  revealArtifact: string;
  openingArtifact: string;
  revealingArtifact: string;
  artifactActionFailed: string;
  refreshLibraryHint: string;
  canonicalTranscript: string;
  generatedContent: string;
  generationHistory: string;
  generateContent: string;
  generateAgain: string;
  generatingContent: string;
  generationFailed: string;
  templateLabel: string;
  latestVersion: string;
  legacyArtifact: string;
  speechRecognitionProvider: string;
  generatedWithProvider: string;
  providerUnknown: string;
  providerNotReady: string;
  openSettings: string;
  editMetadata: string;
  taskTitle: string;
  taskDescription: string;
  descriptionPlaceholder: string;
  addTag: string;
  tagPlaceholder: string;
  removeTag: string;
  save: string;
  cancel: string;
  metadataSaveFailed: string;
  sortBy: string;
  sortUpdated: string;
  sortCreated: string;
  sortTitle: string;
  filterByTag: string;
  allTasks: string;
  taskDetails: string;
  openTaskDetails: string;
  backToTaskLibrary: string;
  transcriptSummary: string;
  generatedArtifactCount: string;
  saveTitle: string;
  cancelTitleEdit: string;
  taskInformation: string;
  existingTags: string;
  builtInTemplates: string;
  myTemplates: string;
  resumeTask: string;
  resumeCancelled: string;
  resumingTask: string;
  stopTask: string;
  stoppingTask: string;
  pendingChunksHint: string;
  blockedTaskLocked: string;
  blockedSpeechProviderUnsupported: string;
  blockedSpeechProviderUnavailable: string;
  blockedSourceAudioMissing: string;
  retryGeneration: string;
  retryingGeneration: string;
  retryKeepsIdentity: string;
  newGenerationKeepsHistory: string;
  uploadsAudioHint: string;
  uploadsTextHint: string;
}

interface TaskLibraryProps {
  locale: "zh" | "en";
  labels: TaskLibraryLabels;
  refreshToken: number;
  onReturnToWorkflow: () => void;
  deepseekApiKey: string;
  templates: PromptTemplateSummary[];
  llmReady: boolean;
  llmProviderLabel: string;
  llmModelLabel: string;
  llmConfig: ProviderSelection;
  onOpenSettings: () => void;
}

function formatDate(value: string, locale: "zh" | "en", fallback: string) {
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function statusLabel(status: string, labels: TaskLibraryLabels) {
  const known: Record<string, string> = {
    running: labels.statusRunning,
    processing: labels.statusProcessing,
    pending: labels.statusPending,
    completed: labels.statusCompleted,
    completed_with_errors: labels.statusCompletedWithErrors,
    failed: labels.statusFailed,
    interrupted: labels.statusInterrupted,
    cancelled: labels.statusCancelled,
    unavailable: labels.statusUnavailable,
  };
  return known[status] ?? `${labels.statusUnknown}: ${status || "—"}`;
}

function statusTone(status: string) {
  if (status === "completed") return "success";
  if (status === "completed_with_errors" || status === "interrupted" || status === "cancelled") return "warning";
  if (status === "failed") return "error";
  if (status === "running" || status === "processing") return "active";
  return "neutral";
}

// generationErrorNote explains why a generation did not produce a complete
// result. Unknown codes fall back to the generic error label.
function generationErrorNote(code: string, labels: TaskLibraryLabels) {
  const known: Record<string, string> = {
    output_incomplete: labels.generationIncomplete,
    output_refused: labels.generationRefused,
    output_unverified: labels.generationUnverified,
    output_empty: labels.generationEmpty,
    cancelled: labels.generationCancelled,
  };
  return known[code] ?? null;
}

function artifactKindLabel(kind: string, labels: TaskLibraryLabels) {
  if (kind === "transcript") return labels.artifactTranscript;
  if (kind === "timed_transcript") return labels.artifactTimedTranscript;
  if (kind === "vad_segments") return labels.artifactVADSegments;
  if (kind === "notes") return labels.artifactNotes;
  if (kind === "minutes" || kind === "summary") return labels.artifactMinutes;
  return kind || labels.statusUnknown;
}

// blockingReasonLabel localizes a runtime blocking reason. Unknown codes fall
// back to the runtime message, which is already safe to display.
function blockingReasonLabel(reason: TaskBlockingReason, labels: TaskLibraryLabels) {
  const known: Record<string, string> = {
    task_locked: labels.blockedTaskLocked,
    speech_provider_unsupported: labels.blockedSpeechProviderUnsupported,
    speech_provider_unavailable: labels.blockedSpeechProviderUnavailable,
    source_audio_missing: labels.blockedSourceAudioMissing,
  };
  return known[reason.code] ?? reason.message;
}

// backendErrorMessage turns a rejected IPC call into one displayable line: the
// runtime reports errors as a JSON string, so anything else falls back to empty.
function backendErrorMessage(error: unknown) {
  return parseBackendError(String(error ?? "")).message.trim();
}

/** backendErrorCategory exposes the error class so a requested stop is not shown
 * as a provider or workflow failure. */
function backendErrorCategory(error: unknown) {
  return parseBackendError(String(error ?? "")).category;
}

/** isRequestedStop reports whether the runtime stopped because a stop was asked
 * for; the Rust layer reports that as the "interrupted" category. */
function isRequestedStop(error: unknown) {
  return backendErrorCategory(error) === "interrupted";
}

// providerDescriptor resolves the catalog entry a recorded provider id refers to.
function providerDescriptor(kind: "asr" | "llm", providerId: string | undefined, catalog: ProviderCatalog | null) {
  if (!catalog || !providerId) return null;
  return (kind === "asr" ? catalog.asr : catalog.llm).find((provider) => provider.id === providerId) ?? null;
}

function ArtifactItem({
  artifact,
  labels,
  actionKey,
  pendingAction,
  actionError,
  metadata = [],
  onAction,
  onRetry,
  retryDisabled = false,
  retryPending = false,
  retryError = null,
  retryHint = null,
}: {
  artifact: ArtifactSummary;
  labels: TaskLibraryLabels;
  actionKey: string;
  pendingAction: "open" | "reveal" | null;
  actionError: string | null;
  metadata?: readonly string[];
  onAction: (actionKey: string, action: "open" | "reveal", reference: string) => void;
  onRetry?: () => void;
  retryDisabled?: boolean;
  retryPending?: boolean;
  retryError?: string | null;
  retryHint?: string | null;
}) {
  const available = (artifact.status === "completed" || artifact.partial === true) && Boolean(artifact.reference);
  const errorNote = artifact.error_code ? generationErrorNote(artifact.error_code, labels) : null;
  return (
    <li className="library-artifact">
      <div className="library-artifact-main">
        <strong>{artifactKindLabel(artifact.kind, labels)}</strong>
        <span className={`library-status ${statusTone(artifact.status)}`}>
          {statusLabel(artifact.status, labels)}
        </span>
      </div>
      <div className="library-artifact-meta">
        <span>{available ? (artifact.partial ? labels.availablePartial : labels.available) : labels.unavailable}</span>
        {errorNote ? <span>{errorNote}</span> : artifact.has_error ? <span>{labels.artifactError}</span> : null}
        {artifact.retryable !== null && (
          <span>{artifact.retryable ? labels.retryable : labels.notRetryable}</span>
        )}
        {onRetry && artifact.retryable === true && <span>{labels.retryKeepsIdentity}</span>}
        {artifact.kind !== "timed_transcript" && artifact.kind !== "vad_segments" && <span>
          {artifact.provider
            ? `${artifact.provider === "mimo" || artifact.provider === "local-sensevoice" ? labels.speechRecognitionProvider : labels.generatedWithProvider}: ${artifact.provider === "mimo" ? "Mimo" : artifact.provider === "local-sensevoice" ? "SenseVoice" : artifact.provider === "deepseek" ? "DeepSeek" : labels.providerUnknown} · ${artifact.model || labels.providerUnknown}`
            : labels.providerUnknown}
        </span>}
        {metadata.map((item) => <span key={item}>{item}</span>)}
      </div>
      {retryHint && <p className="provider-not-ready">{retryHint}</p>}
      {((available && artifact.reference) || (onRetry && artifact.retryable === true)) && (
        <div className="library-artifact-actions">
          {available && artifact.reference && (
            <>
              <button
                className="btn-secondary"
                onClick={() => onAction(actionKey, "open", artifact.reference!)}
                disabled={pendingAction !== null}
              >
                {pendingAction === "open" ? labels.openingArtifact : labels.openArtifact}
              </button>
              <button
                className="btn-secondary"
                onClick={() => onAction(actionKey, "reveal", artifact.reference!)}
                disabled={pendingAction !== null}
              >
                {pendingAction === "reveal" ? labels.revealingArtifact : labels.revealArtifact}
              </button>
            </>
          )}
          {onRetry && artifact.retryable === true && (
            <button
              className="btn-secondary"
              onClick={onRetry}
              disabled={retryDisabled || retryPending}
            >
              {retryPending ? labels.retryingGeneration : labels.retryGeneration}
            </button>
          )}
        </div>
      )}
      {retryError && (
        <p className="library-artifact-error" role="status">
          {retryError}
        </p>
      )}
      {actionError && (
        <p className="library-artifact-error" role="status">
          {actionError} {labels.refreshLibraryHint}
        </p>
      )}
    </li>
  );
}

function GenerationGroupView({ group, task, locale, labels, templates, pendingActions, actionErrors, onArtifactAction, retryPending, retryErrors, retryDisabled, providerCatalog, onRetryGeneration }: { group: TaskSummary["generated_content"][number]; task: TaskSummary; locale: "zh" | "en"; labels: TaskLibraryLabels; templates: PromptTemplateSummary[]; pendingActions: ReadonlyMap<string, "open" | "reveal">; actionErrors: ReadonlyMap<string, string>; onArtifactAction: (taskFolder: string, actionKey: string, action: "open" | "reveal", reference: string) => void; retryPending: ReadonlySet<string>; retryErrors: ReadonlyMap<string, string>; retryDisabled: boolean; providerCatalog: ProviderCatalog | null; onRetryGeneration: (generationId: string) => void }) {
  const snapshotName = group.display_name.trim();
  const registryName = templates.find((template) => template.built_in && template.id === group.template_id)?.display_name[locale];
  const visibleVersions = group.latest ? [group.latest] : group.versions;
  const versionName = (version: TaskSummary["generated_content"][number]["versions"][number]) => registryName || version.template_display_name.trim() || snapshotName || group.template_id || labels.generatedContent;
  const versionMetadata = (version: TaskSummary["generated_content"][number]["versions"][number]) => [`${labels.createdAt}: ${formatDate(version.created_at, locale, labels.invalidDate)}`];
  // versionRetryHint states where a retry of this exact version sends text.
  const versionRetryHint = (version: TaskSummary["generated_content"][number]["versions"][number]) => version.retryable === true && providerDescriptor("llm", version.provider, providerCatalog)?.privacy.sends_text_off_device === true ? labels.uploadsTextHint : null;
  return <section className="generation-group"><ul>{visibleVersions.map((version) => <ArtifactItem key={version.generation_id} artifact={{kind: version.legacy ? `${versionName(version)} · ${labels.legacyArtifact}` : versionName(version), status: version.status, reference: version.reference, has_error: version.has_error, error_code: version.error_code, partial: version.partial_output, retryable: version.retryable, provider: version.provider, model: version.model}} labels={labels} metadata={versionMetadata(version)} actionKey={`${task.task_folder}:generation:${version.generation_id}`} pendingAction={pendingActions.get(`${task.task_folder}:generation:${version.generation_id}`) ?? null} actionError={actionErrors.get(`${task.task_folder}:generation:${version.generation_id}`) ?? null} onAction={(actionKey, action, reference) => onArtifactAction(task.task_folder, actionKey, action, reference)} onRetry={() => onRetryGeneration(version.generation_id)} retryDisabled={retryDisabled} retryPending={retryPending.has(version.generation_id)} retryError={retryErrors.get(version.generation_id) ?? null} retryHint={versionRetryHint(version)} />)}</ul>{group.latest && group.versions.length > 1 && <details><summary>{labels.generationHistory} ({group.versions.length})</summary><ul>{group.versions.map((version) => <ArtifactItem key={version.generation_id} artifact={{kind: version.legacy ? `${versionName(version)} · ${labels.legacyArtifact}` : versionName(version), status: version.status, reference: version.reference, has_error: version.has_error, error_code: version.error_code, partial: version.partial_output, retryable: version.retryable, provider: version.provider, model: version.model}} labels={labels} metadata={versionMetadata(version)} actionKey={`${task.task_folder}:history:${version.generation_id}`} pendingAction={pendingActions.get(`${task.task_folder}:history:${version.generation_id}`) ?? null} actionError={actionErrors.get(`${task.task_folder}:history:${version.generation_id}`) ?? null} onAction={(actionKey, action, reference) => onArtifactAction(task.task_folder, actionKey, action, reference)} onRetry={() => onRetryGeneration(version.generation_id)} retryDisabled={retryDisabled} retryPending={retryPending.has(version.generation_id)} retryError={retryErrors.get(version.generation_id) ?? null} retryHint={versionRetryHint(version)} />)}</ul></details>}</section>;
}

function TaskCard({
  task,
  locale,
  labels,
  metadataPending,
  metadataError,
  onUpdateMetadata,
  onOpenDetails,
}: {
  task: TaskSummary;
  locale: "zh" | "en";
  labels: TaskLibraryLabels;
  metadataPending: boolean;
  metadataError: string | null;
  onUpdateMetadata: (task: TaskSummary, metadata: TaskMetadataUpdate) => Promise<boolean>;
  onOpenDetails: () => void;
}) {
  const [editingTitle, setEditingTitle] = useState(false);
  const [titleDraft, setTitleDraft] = useState(task.title_source === "user" ? task.title : "");
  const transcript = task.canonical_transcript ?? task.primary_artifacts.find((artifact) => artifact.kind === "transcript") ?? null;
  const generatedCount = task.generated_content.reduce((count, group) => count + group.versions.length, 0);
  const visibleTags = task.tags.slice(0, 2);

  const beginTitleEditing = () => {
    setTitleDraft(task.title_source === "user" ? task.title : "");
    setEditingTitle(true);
  };

  const cancelTitleEditing = () => {
    setTitleDraft(task.title_source === "user" ? task.title : "");
    setEditingTitle(false);
  };

  const saveTitle = async () => {
    const saved = await onUpdateMetadata(task, {
      title: titleDraft,
      description: task.description,
      tags: task.tags,
    });
    if (saved) setEditingTitle(false);
  };

  return (
    <article className="library-task-card" aria-labelledby={`task-${task.task_id}`}>
      <div className="library-task-header">
        <div className="library-task-identity">
          {editingTitle ? (
            <form className="library-title-form" onSubmit={(event) => { event.preventDefault(); void saveTitle(); }}>
              <input
                id={`task-${task.task_id}`}
                autoFocus
                value={titleDraft}
                onChange={(event) => setTitleDraft(event.target.value)}
                onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); cancelTitleEditing(); } }}
                maxLength={200}
                aria-label={labels.taskTitle}
                placeholder={task.title}
                disabled={metadataPending}
              />
              <button type="submit" className="library-title-icon save" disabled={metadataPending} aria-label={labels.saveTitle} title={labels.saveTitle}>✓</button>
              <button type="button" className="library-title-icon cancel" onClick={cancelTitleEditing} disabled={metadataPending} aria-label={labels.cancelTitleEdit} title={labels.cancelTitleEdit}>×</button>
            </form>
          ) : (
            <h3 id={`task-${task.task_id}`}>
              <button className="library-title-button" onClick={beginTitleEditing} disabled={metadataPending} aria-label={`${labels.editMetadata}: ${task.title}`}>
                {task.title}<span className="library-title-edit-mark" aria-hidden="true">✎</span>
              </button>
            </h3>
          )}
          <div className="library-task-time">
            <span>
              {labels.updatedAt}: {formatDate(task.updated_at, locale, labels.invalidDate)}
            </span>
          </div>
        </div>
        <span className={`library-status ${statusTone(task.status)}`}>
          {statusLabel(task.status, labels)}
        </span>
      </div>

      {metadataError && <p className="library-artifact-error" role="status">{metadataError}</p>}
      {task.tags.length > 0 && (
        <div className="library-tags library-tag-summary" aria-label={labels.tags}>
          {visibleTags.map((tag, index) => <span key={`${tag}-${index}`}>{tag}</span>)}
          {task.tags.length > visibleTags.length && <span>+{task.tags.length - visibleTags.length}</span>}
        </div>
      )}
      <div className="library-card-summary">
        <span>{labels.transcriptSummary}: {transcript ? statusLabel(transcript.status, labels) : labels.unavailable}</span>
        <span>{labels.generatedArtifactCount}: {generatedCount}</span>
        {task.has_partial_errors && <span className="library-partial-error">{labels.partialErrors}</span>}
      </div>
      <button className="btn-secondary library-open-details" onClick={onOpenDetails}>{labels.openTaskDetails}</button>
    </article>
  );
}

function TaskDetails({ task, locale, labels, templates, pendingActions, actionErrors, onArtifactAction, generationPending, generationError, onGenerate, llmReady, llmProviderLabel, llmModelLabel, llmConfig, onOpenSettings, metadataPending, metadataError, onUpdateMetadata, tagSuggestions, onBack, libraryRunKind, stopping, resumeError, retryPending, retryErrors, providerCatalog, onResume, onRetryGeneration, onStop, onCacheCleaned }: { task: TaskSummary; locale: "zh" | "en"; labels: TaskLibraryLabels; templates: PromptTemplateSummary[]; pendingActions: ReadonlyMap<string, "open" | "reveal">; actionErrors: ReadonlyMap<string, string>; onArtifactAction: (taskFolder: string, actionKey: string, action: "open" | "reveal", reference: string) => void; generationPending: boolean; generationError: string | null; onGenerate: (task: TaskSummary, templateId: string) => void; llmReady: boolean; llmProviderLabel: string; llmModelLabel: string; llmConfig: ProviderSelection; onOpenSettings: () => void; metadataPending: boolean; metadataError: string | null; onUpdateMetadata: (task: TaskSummary, metadata: TaskMetadataUpdate) => Promise<boolean>; tagSuggestions: readonly string[]; onBack: () => void; libraryRunKind: string | null; stopping: boolean; resumeError: string | null; retryPending: ReadonlySet<string>; retryErrors: ReadonlyMap<string, string>; providerCatalog: ProviderCatalog | null; onResume: (task: TaskSummary) => void; onRetryGeneration: (task: TaskSummary, generationId: string) => void; onStop: (task: TaskSummary) => void; onCacheCleaned: () => Promise<void> }) {
  const [cacheBusy, setCacheBusy] = useState(false);
  const [description, setDescription] = useState(task.description);
  const [tags, setTags] = useState<string[]>(task.tags);
  const [editingTitle, setEditingTitle] = useState(false);
  const [titleDraft, setTitleDraft] = useState(task.title_source === "user" ? task.title : "");
  const [tagInput, setTagInput] = useState("");
  const [tagPanelOpen, setTagPanelOpen] = useState(false);
  const [generationOpen, setGenerationOpen] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState("structured-notes");
  const transcript = task.canonical_transcript ?? task.primary_artifacts.find((artifact) => artifact.kind === "transcript") ?? null;
  const suggestions = tagSuggestions.filter((tag) => !tags.includes(tag)).filter((tag) => !tagInput.trim() || tag.toLocaleLowerCase().includes(tagInput.trim().toLocaleLowerCase())).slice(0, 8);
  const actions = task.actions ?? null;
  const blockingReasons = actions ? actions.blocking_reasons : [];
  const pendingChunks = actions ? actions.pending_chunks : 0;
  const resumeOffered = actions?.can_resume === true;
  const resumeProvider = actions?.can_resume_transcribe === true ? providerDescriptor("asr", task.canonical_transcript?.provider, providerCatalog) : null;
  const resuming = libraryRunKind === "resume";
  const runPending = libraryRunKind !== null;
  const conflictingPending = runPending || generationPending || cacheBusy;
  const generationSendsText = providerDescriptor("llm", llmConfig.provider_id, providerCatalog)?.privacy.sends_text_off_device === true;
  const showResumeSection = Boolean(actions && (resumeOffered || blockingReasons.length > 0 || pendingChunks > 0)) || runPending;
  const saveMetadata = async () => { await onUpdateMetadata(task, { title: task.title_source === "user" ? task.title : "", description, tags }); };
  const beginTitleEditing = () => { setTitleDraft(task.title_source === "user" ? task.title : ""); setEditingTitle(true); };
  const cancelTitleEditing = () => { setTitleDraft(task.title_source === "user" ? task.title : ""); setEditingTitle(false); };
  const saveTitle = async () => {
    const saved = await onUpdateMetadata(task, { title: titleDraft, description: task.description, tags: task.tags });
    if (saved) setEditingTitle(false);
  };
  const persistTags = async (nextTags: string[]) => {
    const saved = await onUpdateMetadata(task, { title: task.title_source === "user" ? task.title : "", description: task.description, tags: nextTags });
    if (saved) setTags(nextTags);
    return saved;
  };
  const addTag = async () => {
    const next = tagInput.trim();
    if (!next || tags.includes(next) || tags.length >= 20) return;
    const nextTags = [...tags, next];
    if (await persistTags(nextTags)) { setTagInput(""); setTagPanelOpen(false); }
  };
  return <article className="library-details-view">
    <button className="btn-secondary library-back" onClick={onBack} disabled={cacheBusy}>← {labels.backToTaskLibrary}</button>
    <header className="library-details-heading"><div className="library-details-title"><span className="section-kicker">{labels.taskDetails}</span>{editingTitle ? <form className="library-title-form" onSubmit={(event) => { event.preventDefault(); void saveTitle(); }}><input id="task-details-title" autoFocus value={titleDraft} onChange={(event) => setTitleDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); cancelTitleEditing(); } }} maxLength={200} aria-label={labels.taskTitle} placeholder={task.title} disabled={metadataPending || cacheBusy} /><button type="submit" className="library-title-icon save" disabled={metadataPending || cacheBusy} aria-label={labels.saveTitle} title={labels.saveTitle}>✓</button><button type="button" className="library-title-icon cancel" onClick={cancelTitleEditing} disabled={metadataPending || cacheBusy} aria-label={labels.cancelTitleEdit} title={labels.cancelTitleEdit}>×</button></form> : <h2 id="task-details-title"><button className="library-title-button" onClick={beginTitleEditing} disabled={metadataPending || cacheBusy} aria-label={`${labels.editMetadata}: ${task.title}`}>{task.title}<span className="library-title-edit-mark" aria-hidden="true">✎</span></button></h2>}</div><span className={`library-status ${statusTone(task.status)}`}>{statusLabel(task.status, labels)}</span></header>
    {metadataError && <p className="library-artifact-error" role="status">{metadataError}</p>}
    {showResumeSection && <section className="library-details-section">
      <div className="library-section-heading"><h3>{labels.resumeTask}</h3><div className="library-generation-buttons">{resumeOffered && <button className="btn-primary" onClick={() => onResume(task)} disabled={conflictingPending || stopping}>{resuming ? labels.resumingTask : labels.resumeTask}</button>}{runPending && <button className="btn-secondary" onClick={() => onStop(task)} disabled={stopping}>{stopping ? labels.stoppingTask : labels.stopTask}</button>}</div></div>
      {pendingChunks > 0 && <p>{labels.pendingChunksHint}: {pendingChunks}</p>}
      {resumeProvider?.privacy.sends_audio_off_device === true && <p className="provider-not-ready">{labels.uploadsAudioHint}</p>}
      {blockingReasons.map((reason) => <p key={reason.code} className="provider-not-ready">{blockingReasonLabel(reason, labels)}</p>)}
      {resumeError && <p className="library-artifact-error" role="status">{resumeError}</p>}
    </section>}
    <CacheCleanupPanel key={task.task_folder} taskFolder={task.task_folder} labels={labels} locale={locale} disabled={runPending || generationPending || metadataPending || actions?.locked === true} onBusyChange={setCacheBusy} onCleaned={onCacheCleaned} />
    <section className="library-quick-tags" aria-label={labels.tags}><div className="library-tags">{tags.map((tag) => <span key={tag}>{tag}<button type="button" onClick={() => void persistTags(tags.filter((item) => item !== tag))} aria-label={`${labels.removeTag}: ${tag}`} disabled={metadataPending || cacheBusy}>×</button></span>)}</div>{!tagPanelOpen && <button type="button" className="btn-secondary" onClick={() => setTagPanelOpen(true)} disabled={metadataPending || cacheBusy || tags.length >= 20}>+ {labels.addTag}</button>}{tagPanelOpen && <form className="library-tag-panel" onSubmit={(event) => { event.preventDefault(); void addTag(); }}>{suggestions.length > 0 && <div className="library-tag-suggestions"><span>{labels.existingTags}</span><div>{suggestions.map((tag) => <button type="button" key={tag} onClick={() => setTagInput(tag)} disabled={metadataPending || cacheBusy}>+ {tag}</button>)}</div></div>}<input autoFocus value={tagInput} onChange={(event) => setTagInput(event.target.value)} maxLength={50} placeholder={labels.tagPlaceholder} aria-label={labels.addTag} disabled={metadataPending || cacheBusy || tags.length >= 20} /><div className="library-tag-panel-actions form-action-row"><button type="button" className="btn-secondary" onClick={() => { setTagInput(""); setTagPanelOpen(false); }} disabled={metadataPending || cacheBusy}>{labels.cancel}</button><button type="submit" className="btn-primary" disabled={metadataPending || cacheBusy || !tagInput.trim() || tags.includes(tagInput.trim()) || tags.length >= 20}>{labels.addTag}</button></div></form>}</section>
    <section className="library-details-section library-artifacts"><h3>{labels.canonicalTranscript}</h3><ul>{transcript ? <ArtifactItem artifact={transcript} labels={labels} actionKey={`${task.task_folder}:transcript`} pendingAction={pendingActions.get(`${task.task_folder}:transcript`) ?? null} actionError={actionErrors.get(`${task.task_folder}:transcript`) ?? null} onAction={(key, action, reference) => onArtifactAction(task.task_folder, key, action, reference)} /> : <ArtifactItem artifact={{ kind: "transcript", status: "unavailable", reference: null, has_error: false, retryable: null, provider: "", model: "" }} labels={labels} actionKey={`${task.task_folder}:transcript`} pendingAction={null} actionError={null} onAction={() => undefined} />}</ul></section>
    {task.primary_artifacts.filter((artifact) => artifact.kind === "timed_transcript" || artifact.kind === "vad_segments").map((artifact) => <section className="library-details-section library-artifacts" key={artifact.kind}><ul><ArtifactItem artifact={artifact} labels={labels} actionKey={`${task.task_folder}:${artifact.kind}`} pendingAction={pendingActions.get(`${task.task_folder}:${artifact.kind}`) ?? null} actionError={actionErrors.get(`${task.task_folder}:${artifact.kind}`) ?? null} onAction={(key, action, reference) => onArtifactAction(task.task_folder, key, action, reference)} /></ul></section>)}
    <section className="library-details-section"><div className="library-section-heading"><h3>{labels.generatedContent}</h3>{task.can_generate && !generationOpen && <button className="btn-secondary" onClick={() => setGenerationOpen(true)}>{labels.generateContent}</button>}</div>{generationOpen && <div className="task-generation-action"><p>{labels.generatedWithProvider}: {llmProviderLabel} · {llmModelLabel}</p><p>{labels.newGenerationKeepsHistory}</p>{generationSendsText && <p className="provider-not-ready">{labels.uploadsTextHint}</p>}{!llmReady && <p className="provider-not-ready">{labels.providerNotReady} <button className="link-button" onClick={onOpenSettings}>{labels.openSettings}</button></p>}<label htmlFor={`template-${task.task_id}`}>{labels.templateLabel}</label><select id={`template-${task.task_id}`} value={selectedTemplate} onChange={(event) => setSelectedTemplate(event.target.value)} disabled={generationPending || runPending || !llmReady}><optgroup label={labels.builtInTemplates}>{templates.filter((template) => template.built_in).map((template) => <option key={template.id} value={template.id}>{template.display_name[locale]}</option>)}</optgroup>{templates.some((template) => !template.built_in) && <optgroup label={labels.myTemplates}>{templates.filter((template) => !template.built_in).map((template) => <option key={template.id} value={template.id}>{template.display_name[locale]}</option>)}</optgroup>}</select><div className="library-generation-buttons form-action-row">{generationPending && <button className="btn-secondary" onClick={() => onStop(task)} disabled={stopping}>{stopping ? labels.stoppingTask : labels.stopTask}</button>}<button className="btn-secondary" onClick={() => setGenerationOpen(false)} disabled={generationPending}>{labels.cancel}</button><button className="btn-primary" disabled={generationPending || runPending || templates.length === 0 || !llmReady} onClick={() => onGenerate(task, selectedTemplate)}>{generationPending ? labels.generatingContent : labels.generateContent}</button></div>{generationError && <p className="library-artifact-error" role="status">{generationError}</p>}</div>}<div className="library-generated">{task.generated_content.map((group) => <GenerationGroupView key={group.template_id} group={group} task={task} locale={locale} labels={labels} templates={templates} pendingActions={pendingActions} actionErrors={actionErrors} onArtifactAction={onArtifactAction} retryPending={retryPending} retryErrors={retryErrors} retryDisabled={conflictingPending} providerCatalog={providerCatalog} onRetryGeneration={(generationId) => onRetryGeneration(task, generationId)} />)}</div></section>
    <details className="library-task-information"><summary>{labels.taskInformation}</summary><div className="library-full-time"><span>{labels.createdAt}: {formatDate(task.created_at, locale, labels.invalidDate)}</span><span>{labels.updatedAt}: {formatDate(task.updated_at, locale, labels.invalidDate)}</span></div><form className="library-details-form" onSubmit={(event) => { event.preventDefault(); void saveMetadata(); }}><label><span>{labels.taskDescription}</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} maxLength={4000} placeholder={labels.descriptionPlaceholder} disabled={metadataPending || cacheBusy} /></label><div className="library-metadata-actions form-action-row"><button type="submit" className="btn-primary" disabled={metadataPending || cacheBusy}>{labels.save}</button></div></form></details>
  </article>;
}

export function TaskLibrary({
  locale,
  labels,
  refreshToken,
  onReturnToWorkflow,
  deepseekApiKey,
  templates,
  llmReady,
  llmProviderLabel,
  llmModelLabel,
  llmConfig,
  onOpenSettings,
}: TaskLibraryProps) {
  const [result, setResult] = useState<DiscoveryResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(false);
  const inFlightRef = useRef(false);
  const pendingRefreshRef = useRef(false);
  const pendingArtifactActionsRef = useRef(new Set<string>());
  const [pendingActions, setPendingActions] = useState(
    () => new Map<string, "open" | "reveal">()
  );
  const [actionErrors, setActionErrors] = useState(
    () => new Map<string, string>()
  );
  const generationPendingRef = useRef(new Set<string>());
  const metadataPendingRef = useRef(new Set<string>());
  const [generationPending, setGenerationPending] = useState(new Set<string>());
  const [generationErrors, setGenerationErrors] = useState(new Map<string, string>());
  const [metadataPending, setMetadataPending] = useState(new Set<string>());
  const [metadataErrors, setMetadataErrors] = useState(new Map<string, string>());
  const [sortMode, setSortMode] = useState<"updated" | "created" | "title">("updated");
  const [tagFilter, setTagFilter] = useState("");
  const [selectedTaskKey, setSelectedTaskKey] = useState<string | null>(null);
  const [providerCatalog, setProviderCatalog] = useState<ProviderCatalog | null>(null);
  const libraryRunRef = useRef(new Set<string>());
  const stoppingRef = useRef(new Set<string>());
  const retryPendingRef = useRef(new Set<string>());
  const [libraryRuns, setLibraryRuns] = useState(new Map<string, string>());
  const [stoppingTasks, setStoppingTasks] = useState(new Set<string>());
  const [resumeErrors, setResumeErrors] = useState(new Map<string, string>());
  const [retryPending, setRetryPending] = useState(new Set<string>());
  const [retryErrors, setRetryErrors] = useState(new Map<string, string>());

  const refresh = useCallback(async () => {
    if (inFlightRef.current) {
      pendingRefreshRef.current = true;
      return;
    }

    inFlightRef.current = true;
    if (mountedRef.current) setLoading(true);

    do {
      pendingRefreshRef.current = false;
      if (mountedRef.current) setError(null);
      try {
        const next = await listTasks();
        if (mountedRef.current) {
          setResult(next);
          setActionErrors(new Map());
        }
      } catch {
        if (mountedRef.current) setError(labels.taskLibraryLoadError);
      }
    } while (mountedRef.current && pendingRefreshRef.current);

    inFlightRef.current = false;
    if (mountedRef.current) setLoading(false);
  }, [labels.taskLibraryLoadError]);

  useEffect(() => {
    mountedRef.current = true;
    void refresh();
    return () => {
      mountedRef.current = false;
      pendingRefreshRef.current = false;
      pendingArtifactActionsRef.current.clear();
      metadataPendingRef.current.clear();
      libraryRunRef.current.clear();
      stoppingRef.current.clear();
      retryPendingRef.current.clear();
    };
  }, []);

  // The provider catalog only feeds the upload hints, so one read per mount is
  // enough and a failed read simply leaves those hints hidden.
  useEffect(() => {
    void (async () => {
      try {
        const catalog = await listProviders();
        if (mountedRef.current) setProviderCatalog(catalog);
      } catch {
        // Upload hints stay hidden; they never block an action.
      }
    })();
  }, []);

  useEffect(() => {
    if (refreshToken > 0) void refresh();
  }, [refreshToken]);

  const handleArtifactAction = useCallback(
    async (
      taskFolder: string,
      actionKey: string,
      action: "open" | "reveal",
      reference: string
    ) => {
      if (pendingArtifactActionsRef.current.has(actionKey)) return;
      pendingArtifactActionsRef.current.add(actionKey);
      setPendingActions((previous) => new Map(previous).set(actionKey, action));
      setActionErrors((previous) => {
        const next = new Map(previous);
        next.delete(actionKey);
        return next;
      });
      try {
        if (action === "open") {
          await openTaskArtifact(taskFolder, reference);
        } else {
          await revealTaskArtifact(taskFolder, reference);
        }
      } catch {
        if (mountedRef.current) {
          setActionErrors((previous) =>
            new Map(previous).set(actionKey, labels.artifactActionFailed)
          );
        }
      } finally {
        pendingArtifactActionsRef.current.delete(actionKey);
        if (mountedRef.current) {
          setPendingActions((previous) => {
            const next = new Map(previous);
            next.delete(actionKey);
            return next;
          });
        }
      }
    },
    [labels.artifactActionFailed]
  );

  const handleGenerate = useCallback(async (task: TaskSummary, templateId: string) => {
    if (generationPendingRef.current.has(task.task_folder) || libraryRunRef.current.has(task.task_folder)) return;
    generationPendingRef.current.add(task.task_folder);
    setGenerationPending((previous) => new Set(previous).add(task.task_folder));
    setGenerationErrors((previous) => { const next = new Map(previous); next.delete(task.task_folder); return next; });
    try {
      await generateTaskContent({ taskFolder: task.task_folder, templateId, deepseekApiKey, llmConfig });
    } catch (error) {
      // A stop the user asked for is reported as cancelled, not as a failure.
      const message = isRequestedStop(error) ? labels.generationCancelled : labels.generationFailed;
      if (mountedRef.current) setGenerationErrors((previous) => new Map(previous).set(task.task_folder, message));
    } finally {
      await refresh();
      generationPendingRef.current.delete(task.task_folder);
      stoppingRef.current.delete(task.task_folder);
      if (mountedRef.current) {
        setGenerationPending((previous) => { const next = new Set(previous); next.delete(task.task_folder); return next; });
        setStoppingTasks((previous) => { const next = new Set(previous); next.delete(task.task_folder); return next; });
      }
    }
  }, [deepseekApiKey, llmConfig, labels.generationCancelled, labels.generationFailed, refresh]);

  // Continues whatever work an interrupted Task still owes. The runtime decides
  // which stage runs, and the promise settles only after process-done arrives.
  const handleResume = useCallback(async (task: TaskSummary) => {
    const taskKey = task.task_folder;
    if (libraryRunRef.current.has(taskKey)) return;
    libraryRunRef.current.add(taskKey);
    setLibraryRuns((previous) => new Map(previous).set(taskKey, "resume"));
    setStoppingTasks((previous) => { const next = new Set(previous); next.delete(taskKey); return next; });
    setResumeErrors((previous) => { const next = new Map(previous); next.delete(taskKey); return next; });
    try {
      // Continue inherits the Manifest: restating the mode would make the runtime
      // treat it as a mode change and reset the requested template.
      await resumeTask(task.task_folder);
    } catch (error) {
      // A stop this component requested is not a failure: the run reports it as cancelled.
      const message = isRequestedStop(error) ? labels.resumeCancelled : backendErrorMessage(error);
      if (mountedRef.current && message) setResumeErrors((previous) => new Map(previous).set(taskKey, message));
    } finally {
      await refresh();
      libraryRunRef.current.delete(taskKey);
      stoppingRef.current.delete(taskKey);
      if (mountedRef.current) {
        setLibraryRuns((previous) => { const next = new Map(previous); next.delete(taskKey); return next; });
        setStoppingTasks((previous) => { const next = new Set(previous); next.delete(taskKey); return next; });
      }
    }
  }, [labels.resumeCancelled, refresh]);

  // Retries one failed generation under its original identity: no new version is
  // created, so completion is reported through process-done only.
  const handleRetryGeneration = useCallback(async (task: TaskSummary, generationId: string) => {
    const taskKey = task.task_folder;
    if (libraryRunRef.current.has(taskKey) || retryPendingRef.current.has(generationId)) return;
    libraryRunRef.current.add(taskKey);
    retryPendingRef.current.add(generationId);
    setLibraryRuns((previous) => new Map(previous).set(taskKey, "retry"));
    setRetryPending((previous) => new Set(previous).add(generationId));
    setStoppingTasks((previous) => { const next = new Set(previous); next.delete(taskKey); return next; });
    setRetryErrors((previous) => { const next = new Map(previous); next.delete(generationId); return next; });
    try {
      await retryTaskGeneration({ taskFolder: task.task_folder, generationId, deepseekApiKey });
    } catch (error) {
      const message = backendErrorMessage(error) || labels.generationFailed;
      if (mountedRef.current && !stoppingRef.current.has(taskKey)) setRetryErrors((previous) => new Map(previous).set(generationId, message));
    } finally {
      await refresh();
      libraryRunRef.current.delete(taskKey);
      retryPendingRef.current.delete(generationId);
      stoppingRef.current.delete(taskKey);
      if (mountedRef.current) {
        setLibraryRuns((previous) => { const next = new Map(previous); next.delete(taskKey); return next; });
        setRetryPending((previous) => { const next = new Set(previous); next.delete(generationId); return next; });
        setStoppingTasks((previous) => { const next = new Set(previous); next.delete(taskKey); return next; });
      }
    }
  }, [deepseekApiKey, labels.generationFailed, refresh]);

  // handleStop asks the active run to stop cooperatively. The conflict stays
  // disabled until the run itself settles, because the runtime keeps the child
  // alive for its grace period and rejects a new operation until then.
  const handleStop = useCallback(async (task: TaskSummary) => {
    const taskKey = task.task_folder;
    const hasRun = libraryRunRef.current.has(taskKey) || generationPendingRef.current.has(taskKey);
    if (stoppingRef.current.has(taskKey) || !hasRun) return;
    stoppingRef.current.add(taskKey);
    setStoppingTasks((previous) => new Set(previous).add(taskKey));
    let requested = false;
    try {
      const outcome = await cancelTask();
      requested = outcome.state === "stopping";
    } catch {
      // The run keeps going, so the stop affordance and the guard both reset.
    }
    if (!requested) {
      stoppingRef.current.delete(taskKey);
      if (mountedRef.current) setStoppingTasks((previous) => { const next = new Set(previous); next.delete(taskKey); return next; });
    }
  }, []);

  const handleUpdateMetadata = useCallback(async (task: TaskSummary, metadata: TaskMetadataUpdate) => {
    if (metadataPendingRef.current.has(task.task_id)) return false;
    metadataPendingRef.current.add(task.task_id);
    setMetadataPending((current) => new Set(current).add(task.task_id));
    setMetadataErrors((current) => { const next = new Map(current); next.delete(task.task_id); return next; });
    try {
      const updated = await updateTaskMetadata(task.task_id, metadata);
      if (!mountedRef.current) return false;
      setResult((current) => current ? {
        ...current,
        tasks: current.tasks.map((item) => item.task_id === task.task_id ? {
          ...item,
          title: updated.title,
          title_source: metadata.title.trim() ? "user" : "fallback",
          description: updated.description,
          tags: [...updated.tags],
          updated_at: updated.updated_at,
        } : item),
      } : current);
      void refresh();
      return true;
    } catch {
      if (mountedRef.current) setMetadataErrors((current) => new Map(current).set(task.task_id, labels.metadataSaveFailed));
      return false;
    } finally {
      metadataPendingRef.current.delete(task.task_id);
      if (mountedRef.current) setMetadataPending((current) => { const next = new Set(current); next.delete(task.task_id); return next; });
    }
  }, [labels.metadataSaveFailed, refresh]);

  const tagSuggestions = useMemo(() => {
    const tags = new Set<string>();
    result?.tasks.forEach((task) => task.tags.forEach((tag) => tags.add(tag)));
    return [...tags].sort((left, right) => left.localeCompare(right, locale === "zh" ? "zh-CN" : "en", { sensitivity: "base" }));
  }, [locale, result]);

  const effectiveTagFilter = tagSuggestions.includes(tagFilter) ? tagFilter : "";

  const sortedTasks = useMemo(() => {
    const tasks = result ? result.tasks.filter((task) => !effectiveTagFilter || task.tags.includes(effectiveTagFilter)) : [];
    const timestamp = (value: string) => { const parsed = Date.parse(value); return Number.isNaN(parsed) ? null : parsed; };
    tasks.sort((left, right) => {
      let comparison = 0;
      if (sortMode === "title") comparison = left.title.localeCompare(right.title, locale === "zh" ? "zh-CN" : "en", { sensitivity: "base" });
      else {
        const leftTime = timestamp(sortMode === "created" ? left.created_at : left.updated_at);
        const rightTime = timestamp(sortMode === "created" ? right.created_at : right.updated_at);
        if (leftTime !== null && rightTime !== null) comparison = rightTime - leftTime;
        else if (leftTime !== null) comparison = -1;
        else if (rightTime !== null) comparison = 1;
      }
      if (comparison !== 0) return comparison;
      comparison = left.task_id.localeCompare(right.task_id);
      return comparison !== 0 ? comparison : left.task_folder.localeCompare(right.task_folder);
    });
    return tasks;
  }, [effectiveTagFilter, locale, result, sortMode]);

  const selectedTask = selectedTaskKey
    ? result?.tasks.find((task) => `${task.task_id}:${task.task_folder}` === selectedTaskKey) ?? null
    : null;

  if (selectedTask) {
    return (
      <section className="task-library" aria-labelledby="task-details-title">
        <TaskDetails
          task={selectedTask}
          locale={locale}
          labels={labels}
          templates={templates}
          pendingActions={pendingActions}
          actionErrors={actionErrors}
          onArtifactAction={handleArtifactAction}
          generationPending={generationPending.has(selectedTask.task_folder)}
          generationError={generationErrors.get(selectedTask.task_folder) ?? null}
          onGenerate={handleGenerate}
          llmReady={llmReady}
          llmProviderLabel={llmProviderLabel}
          llmModelLabel={llmModelLabel}
          llmConfig={llmConfig}
          onOpenSettings={onOpenSettings}
          metadataPending={metadataPending.has(selectedTask.task_id)}
          metadataError={metadataErrors.get(selectedTask.task_id) ?? null}
          onUpdateMetadata={handleUpdateMetadata}
          tagSuggestions={tagSuggestions}
          onBack={() => setSelectedTaskKey(null)}
          libraryRunKind={libraryRuns.get(selectedTask.task_folder) ?? null}
          stopping={stoppingTasks.has(selectedTask.task_folder)}
          resumeError={resumeErrors.get(selectedTask.task_folder) ?? null}
          retryPending={retryPending}
          retryErrors={retryErrors}
          providerCatalog={providerCatalog}
          onResume={handleResume}
          onRetryGeneration={handleRetryGeneration}
          onStop={handleStop}
          onCacheCleaned={refresh}
        />
      </section>
    );
  }

  return (
    <section className="task-library" aria-labelledby="task-library-title">
      <div className="library-heading">
        <div>
          <span className="section-kicker">{labels.taskLibraryTitle}</span>
          <h2 id="task-library-title">{labels.taskLibraryTitle}</h2>
          <p>{labels.taskLibraryDescription}</p>
        </div>
        <div className="library-heading-actions">
          <label><span>{labels.filterByTag}</span><select value={effectiveTagFilter} onChange={(event) => setTagFilter(event.target.value)}><option value="">{labels.allTasks}</option>{tagSuggestions.map((tag) => <option key={tag} value={tag}>{tag}</option>)}</select></label>
          <label><span>{labels.sortBy}</span><select value={sortMode} onChange={(event) => setSortMode(event.target.value as typeof sortMode)}><option value="updated">{labels.sortUpdated}</option><option value="created">{labels.sortCreated}</option><option value="title">{labels.sortTitle}</option></select></label>
          <button className="btn-secondary" onClick={() => void refresh()} disabled={loading} aria-label={labels.refresh}>{loading && result ? labels.refreshing : labels.refresh}</button>
        </div>
      </div>

      <div aria-live="polite" aria-atomic="true">
        {loading && !result && (
          <div className="library-state">{labels.taskLibraryLoading}</div>
        )}
        {error && !result && (
          <div className="library-state library-error" role="alert">
            <strong>{error}</strong>
            <button className="btn-secondary" onClick={() => void refresh()} disabled={loading}>
              {labels.retry}
            </button>
          </div>
        )}
        {error && result && (
          <div className="library-inline-error" role="status">
            <span>{error}</span>
            <button onClick={() => void refresh()} disabled={loading}>{labels.retry}</button>
          </div>
        )}
      </div>

      {result && result.issues.length > 0 && (
        <details className="library-issues">
          <summary>{labels.discoveryIssues}: {result.issues.length}</summary>
          <ul>
            {result.issues.map((issue, index) => (
              <li key={`${issue.task_folder}-${issue.code}-${index}`}>{issue.message}</li>
            ))}
          </ul>
        </details>
      )}

      {result && result.tasks.length === 0 && !loading && (
        <div className="library-empty">
          <div className="empty-visual" aria-hidden="true"><span></span><span></span><span></span><span></span><span></span></div>
          <div>
            <h3>{labels.taskLibraryEmpty}</h3>
            <p>{labels.taskLibraryEmptyDescription}</p>
            <button className="btn-secondary" onClick={onReturnToWorkflow}>{labels.newTask}</button>
          </div>
        </div>
      )}

      {result && result.tasks.length > 0 && (
        <div className="library-task-list" aria-busy={loading}>
          {sortedTasks.map((task) => (
            <TaskCard
              key={`${task.task_id}:${task.task_folder}`}
              task={task}
              locale={locale}
              labels={labels}
              metadataPending={metadataPending.has(task.task_id)}
              metadataError={metadataErrors.get(task.task_id) ?? null}
              onUpdateMetadata={handleUpdateMetadata}
              onOpenDetails={() => setSelectedTaskKey(`${task.task_id}:${task.task_folder}`)}
            />
          ))}
        </div>
      )}
    </section>
  );
}
