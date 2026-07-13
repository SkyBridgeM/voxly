import type { OutputMode } from "../lib/meetingAsr";

interface RunControlsProps {
  outputMode: OutputMode;
  onModeChange: (mode: OutputMode) => void;
  onStartRun: () => void;
  isRunning: boolean;
  audioPath: string | null;
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
            <span className="mode-index">01</span>
            <span className="mode-copy">
              <strong>{labels.modeTranscript}</strong>
              <small>{labels.modeTranscriptDesc}</small>
            </span>
            <span className="mode-selected" aria-hidden="true">
              ✓
            </span>
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
            <span className="mode-index">02</span>
            <span className="mode-copy">
              <strong>{labels.modeNotes}</strong>
              <small>{labels.modeNotesDesc}</small>
            </span>
            <span className="mode-selected" aria-hidden="true">
              ✓
            </span>
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
            <span className="mode-index">03</span>
            <span className="mode-copy">
              <strong>{labels.modeMinutes}</strong>
              <small>{labels.modeMinutesDesc}</small>
            </span>
            <span className="mode-selected" aria-hidden="true">
              ✓
            </span>
          </label>
        </div>
      </div>

      {outputMode === "transcript" && (
        <div className="warning-banner">
          <span className="warning-icon" aria-hidden="true">
            i
          </span>
          <span>{labels.transcriptWarning}</span>
        </div>
      )}
      {outputMode === "notes" && (
        <div className="warning-banner">
          <span className="warning-icon" aria-hidden="true">
            i
          </span>
          <span>{labels.notesWarning}</span>
        </div>
      )}
      {outputMode === "minutes" && (
        <div className="warning-banner">
          <span className="warning-icon" aria-hidden="true">
            i
          </span>
          <span>{labels.llmWarning}</span>
        </div>
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
            <>
              {labels.start}
              <span className="button-arrow" aria-hidden="true">
                →
              </span>
            </>
          )}
        </button>
      </div>
    </>
  );
}
