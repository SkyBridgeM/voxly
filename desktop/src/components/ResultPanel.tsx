interface ResultPanelProps {
  taskFolder: string | null;
  transcriptPath: string | null;
  notesInputPath: string | null;
  minutesPath: string | null;
  generatedContentPath: string | null;
  onOpenFolder: () => void;
  labels: {
    resultsTitle: string;
    resultsSummary: string;
    primaryOutput: string;
    generatedFiles: string;
    taskFolder: string;
    advancedDetails: string;
    transcript: string;
    notesInput: string;
    meetingMinutes: string;
    generatedContent: string;
    openTaskFolder: string;
  };
}

export function ResultPanel({
  taskFolder,
  transcriptPath,
  notesInputPath,
  minutesPath,
  generatedContentPath,
  onOpenFolder,
  labels,
}: ResultPanelProps) {
  if (!taskFolder) return null;

  const outputs = [
    transcriptPath && {
      kind: "T",
      label: labels.transcript,
      path: transcriptPath,
    },
    notesInputPath && {
      kind: "N",
      label: labels.notesInput,
      path: notesInputPath,
    },
    minutesPath && {
      kind: "M",
      label: labels.meetingMinutes,
      path: minutesPath,
    },
    generatedContentPath && { kind: "G", label: labels.generatedContent, path: generatedContentPath },
  ].filter((output): output is { kind: string; label: string; path: string } =>
    Boolean(output),
  );
  const primaryOutput = generatedContentPath
    ? outputs.find((output) => output.path === generatedContentPath)
    : minutesPath
    ? outputs.find((output) => output.path === minutesPath)
    : notesInputPath
      ? outputs.find((output) => output.path === notesInputPath)
      : outputs.find((output) => output.path === transcriptPath);
  const secondaryOutputs = outputs.filter(
    (output) => output.path !== primaryOutput?.path,
  );

  return (
    <div
      className="section results-section"
      role="status"
      aria-live="polite"
      aria-atomic="true"
    >
      <div className="results-header">
        <span className="result-success-mark" aria-hidden="true">
          ✓
        </span>
        <div>
          <strong>{labels.resultsTitle}</strong>
          <span>{labels.resultsSummary}</span>
        </div>
      </div>
      {primaryOutput && (
        <div className="primary-result">
          <span className="result-file-mark primary" aria-hidden="true">
            {primaryOutput.kind}
          </span>
          <div className="result-copy">
            <span className="result-eyebrow">{labels.primaryOutput}</span>
            <strong>{primaryOutput.label}</strong>
            <code>{primaryOutput.path}</code>
          </div>
        </div>
      )}
      {secondaryOutputs.length > 0 && (
        <div className="generated-results">
          <h3>{labels.generatedFiles}</h3>
          <div className="results-container">
            {secondaryOutputs.map((output) => (
              <div className="result-item" key={output.path}>
                <span className="result-file-mark" aria-hidden="true">
                  {output.kind}
                </span>
                <div className="result-copy">
                  <span className="result-label">{output.label}</span>
                  <code title={output.path}>{output.path}</code>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      <details className="result-details">
        <summary>{labels.advancedDetails}</summary>
        <div className="task-folder-detail">
          <span className="result-label">{labels.taskFolder}</span>
          <code>{taskFolder}</code>
        </div>
      </details>
      <div className="result-actions">
        <button className="btn-secondary" onClick={onOpenFolder}>
          {labels.openTaskFolder}
        </button>
      </div>
    </div>
  );
}
