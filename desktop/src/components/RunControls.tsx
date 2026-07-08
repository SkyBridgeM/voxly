interface RunControlsProps {
  outputMode: "transcript" | "minutes";
  onModeChange: (mode: "transcript" | "minutes") => void;
  onStartRun: () => void;
  isRunning: boolean;
  audioPath: string | null;
  labels: {
    modeTranscript: string;
    modeTranscriptDesc: string;
    modeMinutes: string;
    modeMinutesDesc: string;
    start: string;
    processing: string;
    llmWarning: string;
    transcriptWarning: string;
  };
}

export function RunControls({
  outputMode,
  onModeChange,
  onStartRun,
  isRunning,
  audioPath,
  labels,
}: RunControlsProps) {
  return (
    <>
      <div className="mode-options">
        <div className="mode-option">
          <input
            type="radio"
            id="mode-transcript"
            name="mode"
            value="transcript"
            checked={outputMode === "transcript"}
            onChange={() => onModeChange("transcript")}
            disabled={isRunning}
          />
          <label htmlFor="mode-transcript">
            {labels.modeTranscript}
            <span>{labels.modeTranscriptDesc}</span>
          </label>
        </div>
        <div className="mode-option">
          <input
            type="radio"
            id="mode-minutes"
            name="mode"
            value="minutes"
            checked={outputMode === "minutes"}
            onChange={() => onModeChange("minutes")}
            disabled={isRunning}
          />
          <label htmlFor="mode-minutes">
            {labels.modeMinutes}
            <span>{labels.modeMinutesDesc}</span>
          </label>
        </div>
      </div>

      {outputMode === "transcript" && (
        <div className="warning-banner">{labels.transcriptWarning}</div>
      )}
      {outputMode === "minutes" && (
        <div className="warning-banner">{labels.llmWarning}</div>
      )}

      <div className="section run-section">
        <button
          className="btn-primary"
          onClick={onStartRun}
          disabled={!audioPath || isRunning}
        >
          {isRunning ? (
            <>
              <span className="spinner"></span>
              {labels.processing}
            </>
          ) : (
            labels.start
          )}
        </button>
      </div>
    </>
  );
}
