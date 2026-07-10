import type { OutputMode } from "../lib/meetingAsr";

interface RunControlsProps {
  outputMode: OutputMode;
  onModeChange: (mode: OutputMode) => void;
  onStartRun: () => void;
  isRunning: boolean;
  audioPath: string | null;
  outputDir: string;
  onPickOutputDir: () => void;
  labels: {
    modeTranscript: string;
    modeTranscriptDesc: string;
    modeNotes: string;
    modeNotesDesc: string;
    modeMinutes: string;
    modeMinutesDesc: string;
    start: string;
    processing: string;
    llmWarning: string;
    transcriptWarning: string;
    notesWarning: string;
    outputDir: string;
    pickOutputDir: string;
  };
}

export function RunControls({
  outputMode,
  onModeChange,
  onStartRun,
  isRunning,
  audioPath,
  outputDir,
  onPickOutputDir,
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
            id="mode-notes"
            name="mode"
            value="notes"
            checked={outputMode === "notes"}
            onChange={() => onModeChange("notes")}
            disabled={isRunning}
          />
          <label htmlFor="mode-notes">
            {labels.modeNotes}
            <span>{labels.modeNotesDesc}</span>
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
      {outputMode === "notes" && (
        <div className="warning-banner">{labels.notesWarning}</div>
      )}
      {outputMode === "minutes" && (
        <div className="warning-banner">{labels.llmWarning}</div>
      )}

      <div className="output-dir-row">
        <label className="output-dir-label">{labels.outputDir}:</label>
        <code className="output-dir-value">
          {outputDir || '(project-root/output)'}
        </code>
        <button
          className="btn-secondary btn-small"
          onClick={onPickOutputDir}
          disabled={isRunning}
        >
          {labels.pickOutputDir}
        </button>
      </div>

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
