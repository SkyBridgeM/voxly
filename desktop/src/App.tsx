import { useState, useEffect, useCallback } from "react";
import { translations, getLocale, setLocale, type Locale } from "./i18n";
import {
  runMeetingAsr,
  openTaskFolder,
  saveEnvKey,
  parseBackendError,
  type LogEntry,
  type ProgressEvent,
  type BackendErrorInfo,
  type BackendKind,
  type OutputMode,
} from "./lib/meetingAsr";
import { FilePicker } from "./components/FilePicker";
import { RunControls } from "./components/RunControls";
import { LogPanel } from "./components/LogPanel";
import { ResultPanel } from "./components/ResultPanel";
import "./App.css";

type Theme = "light" | "dark" | "system";

function App() {
  const [locale, setLocaleState] = useState<Locale>(getLocale());
  const [theme, setTheme] = useState<Theme>(
    (localStorage.getItem("theme") as Theme) || "system"
  );
  const [audioPath, setAudioPath] = useState<string | null>(null);
  const [outputMode, setOutputMode] = useState<OutputMode>("transcript");
  const [outputDir, setOutputDir] = useState<string>(
    localStorage.getItem("outputDir") || ""
  );
  const [backend, setBackend] = useState<BackendKind>(() => {
    const saved = localStorage.getItem("backend");
    return saved === "go" ? "go" : "python";
  });
  const [isRunning, setIsRunning] = useState(false);
  const [progress, setProgress] = useState<ProgressEvent | null>(null);
  const [taskFolder, setTaskFolder] = useState<string | null>(null);
  const [transcriptPath, setTranscriptPath] = useState<string | null>(null);
  const [notesInputPath, setNotesInputPath] = useState<string | null>(null);
  const [minutesPath, setMinutesPath] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [statusOk, setStatusOk] = useState<boolean | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [showError, setShowError] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [showLogs, setShowLogs] = useState(false);

  const [mimoApiKey, setMimoApiKey] = useState("");
  const [deepseekApiKey, setDeepseekApiKey] = useState("");
  const [deepseekModel, setDeepseekModel] = useState("deepseek-v4-flash");

  const t = translations[locale];

  useEffect(() => {
    const root = document.documentElement;
    if (theme === "system") {
      root.removeAttribute("data-theme");
    } else {
      root.setAttribute("data-theme", theme);
    }
    localStorage.setItem("theme", theme);
  }, [theme]);

  useEffect(() => {
    setLocale(locale);
  }, [locale]);

  useEffect(() => {
    localStorage.setItem("backend", backend);
  }, [backend]);

  useEffect(() => {
    localStorage.setItem("outputDir", outputDir);
  }, [outputDir]);

  const startRun = useCallback(async () => {
    if (!audioPath || isRunning) return;

    setIsRunning(true);
    setProgress(null);
    setTaskFolder(null);
    setTranscriptPath(null);
    setNotesInputPath(null);
    setMinutesPath(null);
    setStatus(null);
    setStatusOk(null);
    setLogs([]);
    setErrorMessage("");

    try {
      const result = await runMeetingAsr(
        audioPath,
        outputMode,
        {
          mimoApiKey,
          deepseekApiKey,
          deepseekModel,
          outputDir: outputDir.trim() || undefined,
          backend,
        },
        {
          onLog: (entry) => {
            setLogs((prev) => [...prev, entry]);
          },
          onProgress: (event) => setProgress(event),
          // process-done means subprocess ended — not success.
          onDone: () => {},
        }
      );

      setTaskFolder(result.task_folder);
      setTranscriptPath(result.transcript_path);
      if (result.notes_input_path) setNotesInputPath(result.notes_input_path);
      if (result.minutes_path) setMinutesPath(result.minutes_path);

      // Success only after validated output paths are received.
      setStatus(t.completedSuccessfully);
      setStatusOk(true);
    } catch (e) {
      const raw = String(e);
      const info: BackendErrorInfo = parseBackendError(raw);
      setErrorMessage(info.message || raw);
      setShowError(true);
      setStatus(info.message || raw);
      setStatusOk(false);
    } finally {
      setIsRunning(false);
      setProgress(null);
    }
  }, [
    audioPath,
    isRunning,
    outputMode,
    outputDir,
    backend,
    mimoApiKey,
    deepseekApiKey,
    deepseekModel,
  ]);

  const handleOpenFolder = useCallback(async () => {
    if (!taskFolder) return;
    try {
      await openTaskFolder(taskFolder, outputDir.trim() || undefined);
    } catch (e) {
      setErrorMessage(String(e));
      setShowError(true);
    }
  }, [taskFolder, outputDir]);

  const handleCloseSettings = () => {
    setShowSettings(false);
  };

  const handleSaveKey = async (key: string, value: string) => {
    if (!value.trim()) return;
    try {
      await saveEnvKey(key, value.trim());
      setSaveStatus((prev) => ({ ...prev, [key]: t.savedSuccessfully }));
    } catch (e) {
      setSaveStatus((prev) => ({
        ...prev,
        [key]: `${t.saveFailed}: ${String(e)}`,
      }));
    }
  };

  const handleOutputDirPick = async () => {
    try {
      const { open } = await import("@tauri-apps/plugin-dialog");
      const selected = await open({
        directory: true,
        title: locale === "zh" ? "选择输出目录" : "Choose Output Directory",
      });
      if (selected && typeof selected === "string") {
        setOutputDir(selected);
        localStorage.setItem("outputDir", selected);
      }
    } catch {
      // dialog not available or user cancelled
    }
  };

  const [saveStatus, setSaveStatus] = useState<Record<string, string>>({});

  return (
    <div className="app">
      <header className="app-header">
        <h1>{t.appTitle}</h1>
        <p className="subtitle">{t.appSubtitle}</p>
        <div className="header-actions">
          <button
            className="btn-icon"
            onClick={() => setShowSettings(true)}
            title={t.settingsTitle}
          >
            ⚙️
          </button>
        </div>
      </header>

      <div className="main-content">
        <div className="section input-section">
          <div className="section-header">
            <label>{t.inputTitle}</label>
          </div>

          <FilePicker
            audioPath={audioPath}
            onFileSelected={setAudioPath}
            disabled={isRunning}
            labels={{ chooseFile: t.chooseFile, noFileSelected: t.noFileSelected }}
          />

          <RunControls
            outputMode={outputMode}
            onModeChange={setOutputMode}
            onStartRun={startRun}
            isRunning={isRunning}
            audioPath={audioPath}
            outputDir={outputDir}
            onPickOutputDir={handleOutputDirPick}
            labels={{
              modeTranscript: t.modeTranscript,
              modeTranscriptDesc: t.modeTranscriptDesc,
              modeNotes: t.modeNotes,
              modeNotesDesc: t.modeNotesDesc,
              modeMinutes: t.modeMinutes,
              modeMinutesDesc: t.modeMinutesDesc,
              start: t.start,
              processing: t.processing,
              llmWarning: t.llmWarning,
              transcriptWarning: t.transcriptWarning,
              notesWarning: t.notesWarning,
              outputDir: t.outputDir,
              pickOutputDir: t.pickOutputDir,
            }}
          />
        </div>

        <LogPanel
          isRunning={isRunning}
          progress={progress}
          logs={logs}
          showLogs={showLogs}
          onToggleLogs={() => setShowLogs(!showLogs)}
          onSetShowLogs={setShowLogs}
          statusOk={statusOk}
          labels={{
            splitting: t.splitting,
            transcribing: t.transcribing,
            merging: t.merging,
            preparingNotes: t.preparingNotes,
            generatingMinutes: t.generatingMinutes,
            done: t.done,
            showLogs: t.showLogs,
            hideLogs: t.hideLogs,
          }}
        />

        <ResultPanel
          taskFolder={taskFolder}
          transcriptPath={transcriptPath}
          notesInputPath={notesInputPath}
          minutesPath={minutesPath}
          onOpenFolder={handleOpenFolder}
          labels={{
            resultsTitle: t.resultsTitle,
            folder: t.folder,
            transcript: t.transcript,
            notesInput: t.notesInput,
            meetingMinutes: t.meetingMinutes,
            openOutputFolder: t.openOutputFolder,
          }}
        />

        {status && !isRunning && (
          <div
            className={`section status-section ${
              statusOk === true ? "success" : statusOk === false ? "error" : ""
            }`}
          >
            {status}
          </div>
        )}
      </div>

      {showSettings && (
        <div className="modal-overlay" onClick={() => setShowSettings(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>{t.settingsTitle}</h2>
              <button
                className="btn-close"
                onClick={() => setShowSettings(false)}
              >
                ×
              </button>
            </div>
            <div className="modal-body">
              <div className="warning-banner settings-warning">
                {t.keysNotPersisted}
              </div>
              <div className="form-group">
                <label>{t.language}</label>
                <select
                  value={locale}
                  onChange={(e) => setLocaleState(e.target.value as Locale)}
                >
                  <option value="zh">中文</option>
                  <option value="en">English</option>
                </select>
              </div>
              <div className="form-group">
                <label>{locale === "zh" ? "主题" : "Theme"}</label>
                <select
                  value={theme}
                  onChange={(e) => setTheme(e.target.value as Theme)}
                >
                  <option value="system">
                    {locale === "zh" ? "跟随系统" : "System"}
                  </option>
                  <option value="light">
                    {locale === "zh" ? "浅色" : "Light"}
                  </option>
                  <option value="dark">
                    {locale === "zh" ? "深色" : "Dark"}
                  </option>
                </select>
              </div>
              <h3 className="settings-section-title">{t.advancedSettings}</h3>
              <div className="form-group">
                <label>{t.backend}</label>
                <select
                  value={backend}
                  onChange={(e) =>
                    setBackend(e.target.value as BackendKind)
                  }
                >
                  <option value="python">{t.backendPython}</option>
                  <option value="go">{t.backendGo}</option>
                </select>
                {backend === "go" && (
                  <span className="setting-hint">{t.goBackendHint}</span>
                )}
              </div>
              <div className="form-group">
                <label>{t.mimoApiKey}</label>
                <div className="key-input-row">
                  <input
                    type="password"
                    value={mimoApiKey}
                    onChange={(e) => setMimoApiKey(e.target.value)}
                    placeholder={
                      locale === "zh"
                        ? "留空则使用 .env 配置"
                        : "Leave empty to use .env"
                    }
                  />
                  <button
                    className="btn-secondary btn-save-key"
                    onClick={() => handleSaveKey("MIMO_API_KEY", mimoApiKey)}
                    disabled={!mimoApiKey.trim()}
                    title={t.saveToEnv}
                  >
                    {t.saveToEnv}
                  </button>
                </div>
                {saveStatus["MIMO_API_KEY"] && (
                  <span className="save-status">
                    {saveStatus["MIMO_API_KEY"]}
                  </span>
                )}
              </div>
              <div className="form-group">
                <label>{t.deepseekApiKey}</label>
                <div className="key-input-row">
                  <input
                    type="password"
                    value={deepseekApiKey}
                    onChange={(e) => setDeepseekApiKey(e.target.value)}
                    placeholder={
                      locale === "zh"
                        ? "留空则使用 .env 配置"
                        : "Leave empty to use .env"
                    }
                  />
                  <button
                    className="btn-secondary btn-save-key"
                    onClick={() =>
                      handleSaveKey("DEEPSEEK_API_KEY", deepseekApiKey)
                    }
                    disabled={!deepseekApiKey.trim()}
                    title={t.saveToEnv}
                  >
                    {t.saveToEnv}
                  </button>
                </div>
                {saveStatus["DEEPSEEK_API_KEY"] && (
                  <span className="save-status">
                    {saveStatus["DEEPSEEK_API_KEY"]}
                  </span>
                )}
              </div>
              <div className="form-group">
                <label>{t.deepseekModel}</label>
                <select
                  value={deepseekModel}
                  onChange={(e) => setDeepseekModel(e.target.value)}
                >
                  <option value="deepseek-v4-flash">deepseek-v4-flash</option>
                  <option value="deepseek-v4-pro">deepseek-v4-pro</option>
                </select>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn-primary" onClick={handleCloseSettings}>
                {t.close}
              </button>
            </div>
          </div>
        </div>
      )}

      {showError && (
        <div className="modal-overlay" onClick={() => setShowError(false)}>
          <div
            className="modal modal-error"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="modal-header">
              <h2>{t.errorTitle}</h2>
              <button
                className="btn-close"
                onClick={() => setShowError(false)}
              >
                ×
              </button>
            </div>
            <div className="modal-body">
              <p className="error-message">{errorMessage}</p>
            </div>
            <div className="modal-footer">
              <button
                className="btn-secondary"
                onClick={() => setShowError(false)}
              >
                {t.close}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;
