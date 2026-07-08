import { useState, useRef, useEffect, useCallback } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen, UnlistenFn } from "@tauri-apps/api/event";
import { open } from "@tauri-apps/plugin-dialog";
import { openPath } from "@tauri-apps/plugin-opener";
import "./App.css";

interface LogEntry {
  stream: "stdout" | "stderr";
  line: string;
}

function App() {
  const [audioPath, setAudioPath] = useState<string | null>(null);
  const [outputMode, setOutputMode] = useState<"transcript" | "minutes">(
    "transcript"
  );
  const [isRunning, setIsRunning] = useState(false);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [taskFolder, setTaskFolder] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [statusOk, setStatusOk] = useState<boolean | null>(null);

  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  const pickFile = async () => {
    const selected = await open({
      multiple: false,
      filters: [
        {
          name: "Audio",
          extensions: ["mp3", "wav"],
        },
      ],
    });
    if (selected && typeof selected === "string") {
      setAudioPath(selected);
    }
  };

  const startRun = useCallback(async () => {
    if (!audioPath || isRunning) return;

    setIsRunning(true);
    setLogs([]);
    setTaskFolder(null);
    setStatus(null);
    setStatusOk(null);

    const unlisteners: UnlistenFn[] = [];

    const unlistenLog = await listen<LogEntry>("log-line", (event) => {
      const entry = event.payload;
      setLogs((prev) => [...prev, entry]);

      const trimmed = entry.line.trim();
      if (entry.stream === "stdout" && trimmed.startsWith("Task folder:")) {
        const folder = trimmed.slice("Task folder:".length).trim();
        if (folder) {
          setTaskFolder(folder);
        }
      }
    });
    unlisteners.push(unlistenLog);

    const unlistenDone = await listen<{ code: number | null }>(
      "process-done",
      (event) => {
        const { code } = event.payload;
        if (code === 0) {
          setStatus("Completed successfully.");
          setStatusOk(true);
        } else if (code !== null) {
          setStatus(`Exited with code ${code}.`);
          setStatusOk(false);
        } else {
          setStatus("Terminated by signal.");
          setStatusOk(false);
        }
      }
    );
    unlisteners.push(unlistenDone);

    try {
      const result = await invoke<string>("run_meeting_asr", {
        audioPath,
        outputMode,
      });
      if (result === "success") {
        if (!status) {
          setStatus("Completed successfully.");
          setStatusOk(true);
        }
      } else if (!status) {
        setStatus(result);
        setStatusOk(false);
      }
    } catch (e) {
      setStatus(String(e));
      setStatusOk(false);
    } finally {
      setIsRunning(false);
      for (const fn of unlisteners) {
        fn();
      }
    }
  }, [audioPath, isRunning, outputMode, status]);

  const openOutputDir = useCallback(async () => {
    if (!taskFolder) return;
    const projectRoot = (await import.meta.env.DEV)
      ? "../.."
      : ".";
    const fullPath = `${projectRoot}/${taskFolder}`;
    await openPath(fullPath);
  }, [taskFolder]);

  return (
    <div className="app">
      <h1>meeting-asr Desktop Prototype</h1>
      <p className="subtitle">
        Tauri thin-shell experiment — Python CLI handles all ASR/LLM logic.
      </p>

      <div className="section">
        <label>Audio File</label>
        <div className="file-row">
          <button
            className="btn-secondary"
            onClick={pickFile}
            disabled={isRunning}
          >
            Choose File…
          </button>
          <span className="file-path">
            {audioPath || "No file selected"}
          </span>
        </div>
      </div>

      <div className="section">
        <label>Output Mode</label>
        <div className="mode-options">
          <label>
            <input
              type="radio"
              name="mode"
              value="transcript"
              checked={outputMode === "transcript"}
              onChange={() => setOutputMode("transcript")}
              disabled={isRunning}
            />
            transcript
          </label>
          <label>
            <input
              type="radio"
              name="mode"
              value="minutes"
              checked={outputMode === "minutes"}
              onChange={() => setOutputMode("minutes")}
              disabled={isRunning}
            />
            minutes
          </label>
        </div>
      </div>

      <div className="section">
        <button
          className="btn-primary"
          onClick={startRun}
          disabled={!audioPath || isRunning}
        >
          {isRunning ? "Running…" : "Start"}
        </button>
      </div>

      <div className="section">
        <label>Logs</label>
        <div className="log-container">
          {logs.length === 0 && !isRunning && (
            <p className="log-placeholder">Logs will appear here…</p>
          )}
          {logs.map((entry, i) => (
            <p key={i} className={`log-line ${entry.stream}`}>
              {entry.line}
            </p>
          ))}
          {isRunning && logs.length === 0 && (
            <p className="log-placeholder">Waiting for output…</p>
          )}
          <div ref={logEndRef} />
        </div>
      </div>

      {taskFolder && (
        <div className="section">
          <label>Task Folder</label>
          <div className="task-folder">
            <code>{taskFolder}</code>
            <button className="btn-secondary" onClick={openOutputDir}>
              Open Output Directory
            </button>
          </div>
        </div>
      )}

      {status && (
        <div className="section">
          <p
            className={`status-text ${
              statusOk === true ? "success" : statusOk === false ? "error" : ""
            }`}
          >
            {status}
          </p>
        </div>
      )}
    </div>
  );
}

export default App;
