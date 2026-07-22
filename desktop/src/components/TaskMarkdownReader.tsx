import { useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  openTaskArtifact,
  readTaskArtifact,
  revealTaskArtifact,
  type PromptTemplateSummary,
  type TaskSummary,
  type GenerationInput,
} from "../lib/meetingAsr";
import type { TaskLibraryLabels } from "./TaskLibrary";
import { GenerationInputControl, generationInputIsValid, SaveTemporaryPromptPanel } from "./GenerationInputControl";

interface ReadableArtifact {
  key: string;
  title: string;
  reference: string;
  createdAt: string;
  provider: string;
  model: string;
  section: "generated" | "transcript";
}

function timestamp(value: string) {
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function readableArtifacts(
  task: TaskSummary,
  templates: PromptTemplateSummary[],
  locale: "zh" | "en",
  labels: TaskLibraryLabels
) {
  const generated: ReadableArtifact[] = task.generated_content.flatMap((group) => {
    const registryName = group.template_id === "temporary-prompt"
      ? (locale === "zh" ? "临时提示词" : "Temporary prompt")
      : templates.find((template) => template.built_in && template.id === group.template_id)?.display_name[locale];
    return group.versions
      .filter((version) => version.status === "completed" && Boolean(version.reference))
      .map((version) => ({
        key: `generation:${version.generation_id}`,
        title:
          registryName ||
          version.template_display_name.trim() ||
          group.display_name.trim() ||
          group.template_id ||
          labels.generatedContent,
        reference: version.reference!,
        createdAt: version.created_at,
        provider: version.provider,
        model: version.model,
        section: "generated" as const,
      }));
  });
  generated.sort((left, right) => timestamp(right.createdAt) - timestamp(left.createdAt) || left.key.localeCompare(right.key));

  const transcript = task.canonical_transcript ?? task.primary_artifacts.find((artifact) => artifact.kind === "transcript") ?? null;
  if (transcript?.status === "completed" && transcript.reference) {
    generated.push({
      key: "transcript",
      title: labels.canonicalTranscript,
      reference: transcript.reference,
      createdAt: task.updated_at,
      provider: transcript.provider,
      model: transcript.model,
      section: "transcript",
    });
  }
  return generated;
}

export function TaskMarkdownReader({
  task,
  locale,
  labels,
  templates,
  onBack,
  onOpenDetails,
  generationPending,
  generationError,
  onGenerate,
  promptOffer,
  onDismissPromptOffer,
  onTemplatesChanged,
  llmReady,
  llmProviderLabel,
  llmModelLabel,
  onOpenSettings,
}: {
  task: TaskSummary;
  locale: "zh" | "en";
  labels: TaskLibraryLabels;
  templates: PromptTemplateSummary[];
  onBack: () => void;
  onOpenDetails: () => void;
  generationPending: boolean;
  generationError: string | null;
  onGenerate: (task: TaskSummary, input: GenerationInput) => void;
  promptOffer: string | null;
  onDismissPromptOffer: () => void;
  onTemplatesChanged: () => Promise<void>;
  llmReady: boolean;
  llmProviderLabel: string;
  llmModelLabel: string;
  onOpenSettings: () => void;
}) {
  const artifacts = useMemo(
    () => readableArtifacts(task, templates, locale, labels),
    [labels, locale, task, templates]
  );
  const [selectedKey, setSelectedKey] = useState(() => artifacts[0]?.key ?? "");
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryNonce, setRetryNonce] = useState(0);
  const [actionPending, setActionPending] = useState<"open" | "reveal" | null>(null);
  const [generationOpen, setGenerationOpen] = useState(false);
  const [generationInput, setGenerationInput] = useState<GenerationInput>({ kind: "template", templateId: "structured-notes" });
  const requestIdRef = useRef(0);
  const mountedRef = useRef(true);
  const selected = artifacts.find((artifact) => artifact.key === selectedKey) ?? artifacts[0] ?? null;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      requestIdRef.current += 1;
    };
  }, []);

  useEffect(() => {
    if (!selected) return;
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setError(null);
    void readTaskArtifact(task.task_folder, selected.reference)
      .then((result) => {
        if (mountedRef.current && requestIdRef.current === requestId) setContent(result.content);
      })
      .catch(() => {
        if (mountedRef.current && requestIdRef.current === requestId) setError(labels.markdownLoadError);
      })
      .finally(() => {
        if (mountedRef.current && requestIdRef.current === requestId) setLoading(false);
      });
  }, [labels.markdownLoadError, retryNonce, selected?.key, selected?.reference, task.task_folder]);

  const performAction = async (action: "open" | "reveal") => {
    if (!selected || actionPending) return;
    setActionPending(action);
    setError(null);
    try {
      if (action === "open") await openTaskArtifact(task.task_folder, selected.reference);
      else await revealTaskArtifact(task.task_folder, selected.reference);
    } catch {
      if (mountedRef.current) setError(labels.artifactActionFailed);
    } finally {
      if (mountedRef.current) setActionPending(null);
    }
  };

  return (
    <article className="markdown-reader">
      <header className="markdown-reader-header">
        <div>
          <div className="markdown-reader-title-line">
            <button className="reader-back" onClick={onBack}>← {labels.backToTaskLibrary}</button>
            <div className="markdown-reader-title-copy">
              <h2>{task.title}</h2>
              {selected && <p>{selected.title}{selected.provider ? ` · ${selected.provider}${selected.model ? ` / ${selected.model}` : ""}` : ""}</p>}
            </div>
          </div>
        </div>
        <div className="markdown-reader-header-actions">
          {task.can_generate && <button className="btn-primary" onClick={() => setGenerationOpen(true)} disabled={generationPending}>{generationPending ? labels.generatingContent : labels.generateContent}</button>}
          <button className="btn-secondary" onClick={onOpenDetails}>{labels.taskDetails}</button>
          {selected && <button className="btn-secondary" onClick={() => void performAction("open")} disabled={actionPending !== null}>{actionPending === "open" ? labels.openingArtifact : labels.openArtifact}</button>}
          {selected && <button className="btn-secondary" onClick={() => void performAction("reveal")} disabled={actionPending !== null}>{actionPending === "reveal" ? labels.revealingArtifact : labels.revealArtifact}</button>}
        </div>
      </header>

      {generationOpen && (
        <section className="markdown-generation-panel" aria-label={labels.generateContent}>
          <div>
            <strong>{labels.generateContent}</strong>
            <span>{labels.generatedWithProvider}: {llmProviderLabel} · {llmModelLabel}</span>
          </div>
          {!llmReady && <p className="provider-not-ready">{labels.providerNotReady} <button className="link-button" onClick={onOpenSettings}>{labels.openSettings}</button></p>}
          <GenerationInputControl input={generationInput} onChange={setGenerationInput} templates={templates} locale={locale} disabled={generationPending || !llmReady} />
          <div className="markdown-generation-actions form-action-row">
            <button className="btn-secondary" onClick={() => setGenerationOpen(false)} disabled={generationPending}>{labels.cancel}</button>
            <button className="btn-primary" onClick={() => onGenerate(task, generationInput)} disabled={generationPending || !generationInputIsValid(generationInput) || !llmReady}>{generationPending ? labels.generatingContent : labels.generateContent}</button>
          </div>
          {generationError && <p className="library-artifact-error" role="status">{generationError}</p>}
        </section>
      )}

      {promptOffer && <SaveTemporaryPromptPanel prompt={promptOffer} locale={locale} onDismiss={onDismissPromptOffer} onSaved={onTemplatesChanged} />}

      {artifacts.length > 0 ? (
        <div className="markdown-reader-layout">
          <nav className="markdown-document-list" aria-label={labels.taskDocuments}>
            <span>{labels.taskDocuments}</span>
            {artifacts.map((artifact, index) => (
              <button
                key={artifact.key}
                className={artifact.key === selected?.key ? "is-active" : ""}
                onClick={() => setSelectedKey(artifact.key)}
                aria-current={artifact.key === selected?.key ? "page" : undefined}
              >
                <strong>{artifact.title}</strong>
                <small>{artifact.section === "transcript" ? labels.canonicalTranscript : index === 0 ? labels.latestVersion : labels.generatedContent}</small>
              </button>
            ))}
          </nav>
          <main className="markdown-document" aria-busy={loading}>
            {loading && <div className="markdown-reader-progress" role="status">{labels.markdownLoading}</div>}
            {error && <div className="library-inline-error" role="alert"><span>{error}</span><button onClick={() => setRetryNonce((value) => value + 1)}>{labels.retry}</button></div>}
            {content ? (
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                skipHtml
                components={{
                  a: ({ children }) => <span className="markdown-disabled-link">{children}</span>,
                  img: () => null,
                }}
              >
                {content}
              </ReactMarkdown>
            ) : !loading && !error ? <p className="markdown-empty">{labels.markdownEmpty}</p> : null}
          </main>
        </div>
      ) : (
        <div className="library-state">{labels.noReadableMarkdown}</div>
      )}
    </article>
  );
}
