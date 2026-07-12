import { useState, useEffect, useCallback } from "react";
import { translations, getLocale, setLocale, type Locale } from "./i18n";
import {
  runMeetingAsr,
  openTaskFolder,
  saveEnvKey,
  getRuntimeInfo,
  getRuntimePreflight,
  parseBackendError,
  type BackendKind,
  type RuntimeMode,
  type RuntimePreflight,
  type LogEntry,
  type OutputMode,
  type ProgressEvent,
} from "./lib/meetingAsr";
import { FilePicker } from "./components/FilePicker";
import { RunControls } from "./components/RunControls";
import { LogPanel } from "./components/LogPanel";
import { ResultPanel } from "./components/ResultPanel";
import "./App.css";

type Theme = "light" | "dark" | "system";

function App() {
  const buildRuntimeMode: RuntimeMode = import.meta.env.DEV ? "development" : "production";
  const [locale, setLocaleState] = useState<Locale>(getLocale());
  const [theme, setTheme] = useState<Theme>(
    (localStorage.getItem("theme") as Theme) || "system"
  );
  const [audioPath, setAudioPath] = useState<string | null>(null);
  const [outputMode, setOutputMode] = useState<OutputMode>("transcript");
  const [runtimeMode, setRuntimeMode] = useState<RuntimeMode>(buildRuntimeMode);
  const [backend, setBackend] = useState<BackendKind>(() =>
    buildRuntimeMode === "production"
      ? "go"
      : localStorage.getItem("backend") === "go"
        ? "go"
        : "python"
  );
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
  const [preflight, setPreflight] = useState<RuntimePreflight | null>(null);
  const [preflightLoading, setPreflightLoading] = useState(false);
  const [preflightError, setPreflightError] = useState<string | null>(null);

  const t = translations[locale];

  const refreshPreflight = useCallback(async () => {
    setPreflightLoading(true);
    setPreflightError(null);
    try {
      const result = await getRuntimePreflight({ mimoApiKey, deepseekApiKey });
      setPreflight(result);
    } catch {
      setPreflight(null);
      setPreflightError(t.preflightUnavailable);
    } finally {
      setPreflightLoading(false);
    }
  }, [mimoApiKey, deepseekApiKey, t.preflightUnavailable]);

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
    if (runtimeMode === "development") {
      localStorage.setItem("backend", backend);
    }
  }, [backend, runtimeMode]);

  useEffect(() => {
    let active = true;
    getRuntimeInfo()
      .then((info) => {
        if (!active) return;
        setRuntimeMode(info.runtime_mode);
        setBackend(info.backend);
      })
      .catch(() => {
        // Standalone frontend builds do not have a Tauri command bridge.
        // The build-time default remains safe, and Rust enforces the policy.
      });
    return () => {
      active = false;
    };
  }, []);

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
        { mimoApiKey, deepseekApiKey, deepseekModel, backend },
        {
          onLog: (entry) => setLogs((prev) => [...prev, entry]),
          onProgress: (event) => setProgress(event),
          onDone: () => {},
        }
      );

      if (result.task_folder) setTaskFolder(result.task_folder);
      if (result.transcript_path) setTranscriptPath(result.transcript_path);
      if (result.notes_input_path) setNotesInputPath(result.notes_input_path);
      if (result.minutes_path) setMinutesPath(result.minutes_path);

      setStatus(t.completedSuccessfully);
      setStatusOk(true);
    } catch (e) {
      const error = parseBackendError(String(e));
      setErrorMessage(error.message);
      setShowError(true);
      setStatus(error.message);
      setStatusOk(false);
    } finally {
      setIsRunning(false);
      setProgress(null);
    }
  }, [
    audioPath,
    isRunning,
    outputMode,
    mimoApiKey,
    deepseekApiKey,
    deepseekModel,
    backend,
    t,
  ]);

  const handleOpenFolder = useCallback(async () => {
    if (!taskFolder) return;
    try {
      await openTaskFolder(taskFolder);
    } catch (e) {
      setErrorMessage(String(e));
      setShowError(true);
    }
  }, [taskFolder]);

  const handleCloseSettings = () => {
    setShowSettings(false);
  };

  const handleOpenSettings = () => {
    setShowSettings(true);
    void refreshPreflight();
  };

  const handleSaveKey = async (key: string, value: string) => {
    if (!value.trim()) return;
    try {
      await saveEnvKey(key, value.trim());
      setSaveStatus((prev) => ({ ...prev, [key]: t.savedSuccessfully }));
      await refreshPreflight();
    } catch (e) {
      setSaveStatus((prev) => ({ ...prev, [key]: `${t.saveFailed}: ${String(e)}` }));
    }
  };

  const [saveStatus, setSaveStatus] = useState<Record<string, string>>({});

  const credentialStatusText = (credential: RuntimePreflight["mimo_api_key"]) => {
    if (!credential.status.configured) return t.missing;
    switch (credential.status.source) {
      case "session":
        return t.session;
      case "secure_store":
        return t.secureStore;
      case "env":
        return t.developmentEnv;
      default:
        return t.missing;
    }
  };

  const preflightReadyForMode =
    outputMode === "minutes" ? preflight?.ready_for_minutes : preflight?.ready;

  return (
    <div className="app">
      <header className="app-header">
        <h1>{t.appTitle}</h1>
        <p className="subtitle">{t.appSubtitle}</p>
        <div className="header-actions">
          <button
            className="btn-icon"
            onClick={handleOpenSettings}
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
                {runtimeMode === "production" ? t.productionCredentialsNote : t.keysNotPersisted}
              </div>
              <div className="preflight-panel">
                <div className="preflight-header">
                  <div>
                    <h3>{t.readinessTitle}</h3>
                    <span>{t.readinessLocalOnly}</span>
                  </div>
                  <button
                    className="btn-secondary preflight-refresh"
                    onClick={() => void refreshPreflight()}
                    disabled={preflightLoading}
                  >
                    {preflightLoading ? t.checking : t.refresh}
                  </button>
                </div>
                {preflightError && <div className="preflight-error">{preflightError}</div>}
                {preflight && (
                  <>
                    <div
                      className={`preflight-summary ${
                        preflightReadyForMode ? "ready" : "not-ready"
                      }`}
                    >
                      {preflightReadyForMode ? "✓" : "!"} {preflightReadyForMode ? t.ready : t.notReady}
                      {outputMode === "minutes" ? ` · ${t.modeMinutes}` : ` · ${t.modeTranscript}`}
                    </div>
                    <div className="preflight-grid">
                      <div className="preflight-item">
                        <span className="preflight-label">{t.runtimeMode}</span>
                        <span className="preflight-value">
                          {preflight.runtime_mode === "production" ? t.production : t.development}
                        </span>
                      </div>
                      <div className="preflight-item">
                        <span className="preflight-label">{t.backend}</span>
                        <span className="preflight-value">
                          {preflight.backend === "go" ? t.backendGoProduction : t.backendPython}
                        </span>
                      </div>
                      <div className="preflight-item">
                        <span className="preflight-label">{t.sidecar}</span>
                        <span className={preflight.sidecar.ready ? "preflight-value ready" : "preflight-value not-ready"}>
                          {preflight.sidecar.ready ? t.ready : t.notReady}
                        </span>
                        <span className="preflight-detail">{preflight.sidecar.message}</span>
                      </div>
                      <div className="preflight-item">
                        <span className="preflight-label">ffmpeg</span>
                        <span className={preflight.ffmpeg.ready ? "preflight-value ready" : "preflight-value not-ready"}>
                          {preflight.ffmpeg.ready ? t.ready : t.notReady}
                        </span>
                        <span className="preflight-detail">{preflight.ffmpeg.message}</span>
                      </div>
                      <div className="preflight-item preflight-item-wide">
                        <span className="preflight-label">{t.outputRoot}</span>
                        <code className="preflight-path">{preflight.output_root.path}</code>
                        <span className={preflight.output_root.ready ? "preflight-value ready" : "preflight-value not-ready"}>
                          {preflight.output_root.ready ? t.ready : t.notReady}
                        </span>
                        <span className="preflight-detail">{preflight.output_root.message}</span>
                      </div>
                      <div className="preflight-item">
                        <span className="preflight-label">{t.mimoApiKey}</span>
                        <span className={preflight.mimo_api_key.status.configured ? "preflight-value ready" : "preflight-value not-ready"}>
                          {credentialStatusText(preflight.mimo_api_key)}
                        </span>
                      </div>
                      <div className="preflight-item">
                        <span className="preflight-label">{t.deepseekApiKey}</span>
                        <span className={preflight.deepseek_api_key.status.configured ? "preflight-value ready" : "preflight-value not-ready"}>
                          {credentialStatusText(preflight.deepseek_api_key)}
                        </span>
                      </div>
                    </div>
                  </>
                )}
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
              {runtimeMode === "production" ? (
                <div className="form-group">
                  <label>{t.backend}</label>
                  <div>{t.backendGoProduction}</div>
                </div>
              ) : (
                <div className="form-group">
                  <label>{t.backend}</label>
                  <select
                    value={backend}
                    onChange={(e) => setBackend(e.target.value === "go" ? "go" : "python")}
                  >
                    <option value="python">{t.backendPython}</option>
                    <option value="go">{t.backendGoExperimental}</option>
                  </select>
                </div>
              )}
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
              <div className="form-group">
                <label>{t.mimoApiKey}</label>
                <div className="key-input-row">
                  <input
                    type="password"
                    value={mimoApiKey}
                    onChange={(e) => setMimoApiKey(e.target.value)}
                    placeholder={
                      locale === "zh" ? "留空则使用 .env 配置" : "Leave empty to use .env"
                    }
                  />
                  {runtimeMode === "development" && (
                    <button
                      className="btn-secondary btn-save-key"
                      onClick={() => handleSaveKey("MIMO_API_KEY", mimoApiKey)}
                      disabled={!mimoApiKey.trim()}
                      title={t.saveToEnv}
                    >
                      {t.saveToEnv}
                    </button>
                  )}
                </div>
                {saveStatus["MIMO_API_KEY"] && (
                  <span className="save-status">{saveStatus["MIMO_API_KEY"]}</span>
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
                      locale === "zh" ? "留空则使用 .env 配置" : "Leave empty to use .env"
                    }
                  />
                  {runtimeMode === "development" && (
                    <button
                      className="btn-secondary btn-save-key"
                      onClick={() => handleSaveKey("DEEPSEEK_API_KEY", deepseekApiKey)}
                      disabled={!deepseekApiKey.trim()}
                      title={t.saveToEnv}
                    >
                      {t.saveToEnv}
                    </button>
                  )}
                </div>
                {saveStatus["DEEPSEEK_API_KEY"] && (
                  <span className="save-status">{saveStatus["DEEPSEEK_API_KEY"]}</span>
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
