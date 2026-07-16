import { useEffect } from "react";
import type { ProgressEvent, LogEntry } from "../lib/meetingAsr";

interface LogPanelProps {
  isRunning: boolean;
  progress: ProgressEvent | null;
  logs: LogEntry[];
  showLogs: boolean;
  onToggleLogs: () => void;
  onSetShowLogs: (show: boolean) => void;
  statusOk: boolean | null;
  labels: {
    splitting: string;
    transcribing: string;
    merging: string;
    preparingNotes: string;
    generatingMinutes: string;
    generatingContent: string;
    done: string;
    showLogs: string;
    hideLogs: string;
  };
}

export function LogPanel({
  isRunning,
  progress,
  logs,
  showLogs,
  onToggleLogs,
  onSetShowLogs,
  statusOk,
  labels,
}: LogPanelProps) {
  const hasStderr = logs.some((l) => l.stream === "stderr");

  useEffect(() => {
    if (hasStderr && !showLogs) {
      onSetShowLogs(true);
    }
  }, [hasStderr, showLogs, onSetShowLogs]);

  useEffect(() => {
    if (statusOk === false && !showLogs && logs.length > 0) {
      onSetShowLogs(true);
    }
  }, [statusOk, showLogs, logs.length, onSetShowLogs]);

  const getProgressPercent = () => {
    if (!progress || progress.total === 0) return 0;
    return Math.round((progress.current / progress.total) * 100);
  };

  const getProgressLabel = () => {
    if (!progress) return "";
    switch (progress.status) {
      case "splitting":
        return labels.splitting;
      case "transcribing":
        if (progress.total > 0) {
          return `${labels.transcribing} ${progress.current}/${progress.total}`;
        }
        return labels.transcribing;
      case "merging":
        return labels.merging;
      case "notes":
        return labels.preparingNotes;
      case "minutes":
        return labels.generatingMinutes;
      case "generating":
        return labels.generatingContent;
      case "done":
        return labels.done;
      default:
        return progress.status;
    }
  };

  return (
    <>
      {isRunning && (
        <div className="section progress-section">
          <div
            className="progress-meta"
            role="status"
            aria-live="polite"
            aria-atomic="true"
          >
            <span className="progress-status">
              <span className="status-pulse" aria-hidden="true"></span>
              {getProgressLabel()}
            </span>
            <span className="progress-percent">{getProgressPercent()}%</span>
          </div>
          <div className="progress-bar-container">
            <div
              className="progress-bar"
              style={{ width: `${getProgressPercent()}%` }}
            ></div>
          </div>
        </div>
      )}

      {logs.length > 0 && (
        <div className="section logs-section" aria-live="off">
          <div className="logs-header">
            <span className="logs-count">{logs.length}</span>
            <button className="btn-link" onClick={onToggleLogs}>
              {showLogs ? labels.hideLogs : labels.showLogs}
            </button>
          </div>
          {showLogs && (
            <div className="log-container">
              {logs.map((entry, i) => (
                <p key={i} className={`log-line ${entry.stream}`}>
                  {entry.line}
                </p>
              ))}
            </div>
          )}
        </div>
      )}
    </>
  );
}
