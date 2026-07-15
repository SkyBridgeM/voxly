import { useCallback, useEffect, useRef, useState } from "react";
import {
  listTasks,
  openTaskArtifact,
  revealTaskArtifact,
  type ArtifactSummary,
  type DiscoveryResult,
  type TaskSummary,
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
}

interface TaskLibraryProps {
  locale: "zh" | "en";
  labels: TaskLibraryLabels;
  refreshToken: number;
  onReturnToWorkflow: () => void;
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
  if (mode === "transcript") return labels.modeTranscript;
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

function TaskCard({
  task,
  locale,
  labels,
  pendingActions,
  actionErrors,
  onArtifactAction,
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
}) {
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
        <h4>{labels.outputs}</h4>
        <ul>
          {task.primary_artifacts.map((artifact, index) => (
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
    </article>
  );
}

export function TaskLibrary({
  locale,
  labels,
  refreshToken,
  onReturnToWorkflow,
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
              key={task.task_id}
              task={task}
              locale={locale}
              labels={labels}
              pendingActions={pendingActions}
              actionErrors={actionErrors}
              onArtifactAction={handleArtifactAction}
            />
          ))}
        </div>
      )}
    </section>
  );
}
