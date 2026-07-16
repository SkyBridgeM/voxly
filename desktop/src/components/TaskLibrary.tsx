import { useCallback, useEffect, useRef, useState } from "react";
import {
  listTasks,
  generateTaskContent,
  openTaskArtifact,
  revealTaskArtifact,
  type ArtifactSummary,
  type DiscoveryResult,
  type TaskSummary,
  type PromptTemplateSummary,
} from "../lib/meetingAsr";

export interface TaskLibraryLabels {
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
  unavailable: string;
  artifactError: string;
  retryable: string;
  notRetryable: string;
  invalidDate: string;
  statusRunning: string;
  statusCompleted: string;
  statusCompletedWithErrors: string;
  statusFailed: string;
  statusInterrupted: string;
  statusPending: string;
  statusProcessing: string;
  statusUnavailable: string;
  statusUnknown: string;
  artifactTranscript: string;
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
}

interface TaskLibraryProps {
  locale: "zh" | "en";
  labels: TaskLibraryLabels;
  refreshToken: number;
  onReturnToWorkflow: () => void;
  deepseekApiKey: string;
  deepseekModel: string;
  templates: PromptTemplateSummary[];
  llmReady: boolean;
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
    unavailable: labels.statusUnavailable,
  };
  return known[status] ?? `${labels.statusUnknown}: ${status || "—"}`;
}

function statusTone(status: string) {
  if (status === "completed") return "success";
  if (status === "completed_with_errors" || status === "interrupted") return "warning";
  if (status === "failed") return "error";
  if (status === "running" || status === "processing") return "active";
  return "neutral";
}

function artifactKindLabel(kind: string, labels: TaskLibraryLabels) {
  if (kind === "transcript") return labels.artifactTranscript;
  if (kind === "notes") return labels.artifactNotes;
  if (kind === "minutes" || kind === "summary") return labels.artifactMinutes;
  return kind || labels.statusUnknown;
}

function outputModeLabel(mode: string, labels: TaskLibraryLabels) {
  if (mode === "transcript" || mode === "transcript_only") return labels.modeTranscript;
  if (mode === "transcript_and_generate") return labels.generatedContent;
  if (mode === "notes") return labels.modeNotes;
  if (mode === "minutes") return labels.modeMinutes;
  return mode || labels.statusUnknown;
}

function ArtifactItem({
  artifact,
  labels,
  actionKey,
  pendingAction,
  actionError,
  onAction,
}: {
  artifact: ArtifactSummary;
  labels: TaskLibraryLabels;
  actionKey: string;
  pendingAction: "open" | "reveal" | null;
  actionError: string | null;
  onAction: (actionKey: string, action: "open" | "reveal", reference: string) => void;
}) {
  const available = artifact.status === "completed" && Boolean(artifact.reference);
  return (
    <li className="library-artifact">
      <div className="library-artifact-main">
        <strong>{artifactKindLabel(artifact.kind, labels)}</strong>
        <span className={`library-status ${statusTone(artifact.status)}`}>
          {statusLabel(artifact.status, labels)}
        </span>
      </div>
      <div className="library-artifact-meta">
        <span>{available ? labels.available : labels.unavailable}</span>
        {artifact.has_error && <span>{labels.artifactError}</span>}
        {artifact.retryable !== null && (
          <span>{artifact.retryable ? labels.retryable : labels.notRetryable}</span>
        )}
      </div>
      {available && artifact.reference && (
        <div className="library-artifact-actions">
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
        </div>
      )}
      {actionError && (
        <p className="library-artifact-error" role="status">
          {actionError} {labels.refreshLibraryHint}
        </p>
      )}
    </li>
  );
}

function GenerationGroupView({ group, task, locale, labels, templates, pendingActions, actionErrors, onArtifactAction }: { group: TaskSummary["generated_content"][number]; task: TaskSummary; locale: "zh" | "en"; labels: TaskLibraryLabels; templates: PromptTemplateSummary[]; pendingActions: ReadonlyMap<string, "open" | "reveal">; actionErrors: ReadonlyMap<string, string>; onArtifactAction: (taskFolder: string, actionKey: string, action: "open" | "reveal", reference: string) => void }) {
  const name = templates.find((template) => template.id === group.template_id)?.display_name[locale] ?? group.display_name;
  const visibleVersions = group.latest ? [group.latest] : group.versions;
  return <section className="generation-group"><div className="generation-group-heading"><strong>{name}</strong>{group.latest && <span>{labels.latestVersion}</span>}</div><ul>{visibleVersions.map((version) => <ArtifactItem key={version.generation_id} artifact={{kind: version.legacy ? `${name} · ${labels.legacyArtifact}` : name, status: version.status, reference: version.reference, has_error: version.has_error, retryable: version.retryable}} labels={labels} actionKey={`${task.task_folder}:generation:${version.generation_id}`} pendingAction={pendingActions.get(`${task.task_folder}:generation:${version.generation_id}`) ?? null} actionError={actionErrors.get(`${task.task_folder}:generation:${version.generation_id}`) ?? null} onAction={(actionKey, action, reference) => onArtifactAction(task.task_folder, actionKey, action, reference)} />)}</ul>{group.latest && group.versions.length > 1 && <details><summary>{labels.generationHistory} ({group.versions.length})</summary><ul>{group.versions.map((version) => <ArtifactItem key={version.generation_id} artifact={{kind: version.legacy ? `${name} · ${labels.legacyArtifact}` : name, status: version.status, reference: version.reference, has_error: version.has_error, retryable: version.retryable}} labels={labels} actionKey={`${task.task_folder}:history:${version.generation_id}`} pendingAction={pendingActions.get(`${task.task_folder}:history:${version.generation_id}`) ?? null} actionError={actionErrors.get(`${task.task_folder}:history:${version.generation_id}`) ?? null} onAction={(actionKey, action, reference) => onArtifactAction(task.task_folder, actionKey, action, reference)} />)}</ul></details>}</section>;
}

function TaskCard({
  task,
  locale,
  labels,
  pendingActions,
  actionErrors,
  onArtifactAction,
  templates,
  generationPending,
  generationError,
  onGenerate,
  llmReady,
}: {
  task: TaskSummary;
  locale: "zh" | "en";
  labels: TaskLibraryLabels;
  pendingActions: ReadonlyMap<string, "open" | "reveal">;
  actionErrors: ReadonlyMap<string, string>;
  onArtifactAction: (
    taskFolder: string,
    actionKey: string,
    action: "open" | "reveal",
    reference: string
  ) => void;
  templates: PromptTemplateSummary[];
  generationPending: boolean;
  generationError: string | null;
  onGenerate: (task: TaskSummary, templateId: string) => void;
  llmReady: boolean;
}) {
  const [selectedTemplate, setSelectedTemplate] = useState("structured-notes");
  const transcript = task.canonical_transcript ?? task.primary_artifacts.find((artifact) => artifact.kind === "transcript") ?? null;
  return (
    <article className="library-task-card" aria-labelledby={`task-${task.task_id}`}>
      <div className="library-task-header">
        <div>
          <h3 id={`task-${task.task_id}`}>{task.title}</h3>
          <div className="library-task-time">
            <span>
              {labels.updatedAt}: {formatDate(task.updated_at, locale, labels.invalidDate)}
            </span>
            {task.created_at && (
              <span>
                {labels.createdAt}: {formatDate(task.created_at, locale, labels.invalidDate)}
              </span>
            )}
          </div>
        </div>
        <span className={`library-status ${statusTone(task.status)}`}>
          {statusLabel(task.status, labels)}
        </span>
      </div>

      <div className="library-task-meta">
        <span>{labels.outputMode}: {outputModeLabel(task.output_mode, labels)}</span>
        {task.has_partial_errors && (
          <span className="library-partial-error">{labels.partialErrors}</span>
        )}
      </div>

      {task.tags.length > 0 && (
        <div className="library-tags" aria-label={labels.tags}>
          {task.tags.map((tag) => <span key={tag}>{tag}</span>)}
        </div>
      )}

      <div className="library-artifacts">
        <h4>{labels.canonicalTranscript}</h4>
        <ul>
          {transcript && [transcript].map((artifact, index) => (
            <ArtifactItem
              key={`${artifact.kind}-${index}`}
              artifact={artifact}
              labels={labels}
              actionKey={`${task.task_folder}:${artifact.kind}:${index}`}
              pendingAction={pendingActions.get(`${task.task_folder}:${artifact.kind}:${index}`) ?? null}
              actionError={actionErrors.get(`${task.task_folder}:${artifact.kind}:${index}`) ?? null}
              onAction={(actionKey, action, reference) =>
                onArtifactAction(task.task_folder, actionKey, action, reference)
              }
            />
          ))}
        </ul>
      </div>

      {task.generated_content.length > 0 && <div className="library-generated"><h4>{labels.generatedContent}</h4>{task.generated_content.map((group) => <GenerationGroupView key={group.template_id} group={group} task={task} locale={locale} labels={labels} templates={templates} pendingActions={pendingActions} actionErrors={actionErrors} onArtifactAction={onArtifactAction} />)}</div>}

      {task.can_generate && <div className="task-generation-action"><label htmlFor={`template-${task.task_id}`}>{labels.templateLabel}</label><select id={`template-${task.task_id}`} value={selectedTemplate} onChange={(event) => setSelectedTemplate(event.target.value)} disabled={generationPending || !llmReady}>{templates.map((template) => <option key={template.id} value={template.id}>{template.display_name[locale]}</option>)}</select><button className="btn-secondary" disabled={generationPending || templates.length === 0 || !llmReady} onClick={() => onGenerate(task, selectedTemplate)}>{generationPending ? labels.generatingContent : task.generated_content.some((group) => group.template_id === selectedTemplate) ? labels.generateAgain : labels.generateContent}</button>{generationError && <p className="library-artifact-error" role="status">{generationError}</p>}</div>}
    </article>
  );
}

export function TaskLibrary({
  locale,
  labels,
  refreshToken,
  onReturnToWorkflow,
  deepseekApiKey,
  deepseekModel,
  templates,
  llmReady,
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
  const [generationPending, setGenerationPending] = useState(new Set<string>());
  const [generationErrors, setGenerationErrors] = useState(new Map<string, string>());

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
    };
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
    if (generationPendingRef.current.has(task.task_folder)) return;
    generationPendingRef.current.add(task.task_folder);
    setGenerationPending((previous) => new Set(previous).add(task.task_folder));
    setGenerationErrors((previous) => { const next = new Map(previous); next.delete(task.task_folder); return next; });
    try {
      await generateTaskContent({ taskFolder: task.task_folder, templateId, deepseekApiKey, deepseekModel });
      await refresh();
    } catch {
      if (mountedRef.current) setGenerationErrors((previous) => new Map(previous).set(task.task_folder, labels.generationFailed));
    } finally {
      generationPendingRef.current.delete(task.task_folder);
      if (mountedRef.current) setGenerationPending((previous) => { const next = new Set(previous); next.delete(task.task_folder); return next; });
    }
  }, [deepseekApiKey, deepseekModel, labels.generationFailed, refresh]);

  return (
    <section className="task-library" aria-labelledby="task-library-title">
      <div className="library-heading">
        <div>
          <span className="section-kicker">{labels.taskLibraryTitle}</span>
          <h2 id="task-library-title">{labels.taskLibraryTitle}</h2>
          <p>{labels.taskLibraryDescription}</p>
        </div>
        <button
          className="btn-secondary"
          onClick={() => void refresh()}
          disabled={loading}
          aria-label={labels.refresh}
        >
          {loading && result ? labels.refreshing : labels.refresh}
        </button>
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
          {result.tasks.map((task) => (
            <TaskCard
              key={`${task.task_id}:${task.task_folder}`}
              task={task}
              locale={locale}
              labels={labels}
              pendingActions={pendingActions}
              actionErrors={actionErrors}
              onArtifactAction={handleArtifactAction}
              templates={templates}
              generationPending={generationPending.has(task.task_folder)}
              generationError={generationErrors.get(task.task_folder) ?? null}
              onGenerate={handleGenerate}
              llmReady={llmReady}
            />
          ))}
        </div>
      )}
    </section>
  );
}
