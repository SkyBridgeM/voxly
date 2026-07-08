interface ResultPanelProps {
  taskFolder: string | null;
  transcriptPath: string | null;
  notesInputPath: string | null;
  minutesPath: string | null;
  onOpenFolder: () => void;
  labels: {
    resultsTitle: string;
    folder: string;
    transcript: string;
    notesInput: string;
    meetingMinutes: string;
    openOutputFolder: string;
  };
}

export function ResultPanel({
  taskFolder,
  transcriptPath,
  notesInputPath,
  minutesPath,
  onOpenFolder,
  labels,
}: ResultPanelProps) {
  if (!taskFolder) return null;

  const getFileName = (path: string) => {
    return path.split("/").pop() || path.split("\\").pop() || path;
  };

  return (
    <div className="section results-section">
      <div className="results-header">{labels.resultsTitle}</div>
      <div className="results-container">
        <div className="result-item">
          <span className="result-label">{labels.folder}:</span>
          <code>{taskFolder}</code>
        </div>
        {transcriptPath && (
          <div className="result-item">
            <span className="result-label">{labels.transcript}:</span>
            <code>{getFileName(transcriptPath)}</code>
          </div>
        )}
        {notesInputPath && (
          <div className="result-item">
            <span className="result-label">{labels.notesInput}:</span>
            <code>{getFileName(notesInputPath)}</code>
          </div>
        )}
        {minutesPath && (
          <div className="result-item">
            <span className="result-label">{labels.meetingMinutes}:</span>
            <code>{getFileName(minutesPath)}</code>
          </div>
        )}
      </div>
      <div className="result-actions">
        <button className="btn-secondary" onClick={onOpenFolder}>
          {labels.openOutputFolder}
        </button>
      </div>
    </div>
  );
}
