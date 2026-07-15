import { useState, useEffect, useCallback, useRef } from "react";
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
const MAX_RETAINED_LOGS = 1000;
const DEVELOPMENT_BACKEND_OPTIONS = ["go", "python"] as const satisfies readonly BackendKind[];

function resolveInitialBackend(
  runtimeMode: RuntimeMode,
  savedBackend: string | null
): BackendKind {
  if (runtimeMode === "production") return "go";
  return savedBackend === "python" || savedBackend === "go"
    ? savedBackend
    : "go";
}

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
    resolveInitialBackend(buildRuntimeMode, localStorage.getItem("backend"))
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
  const settingsButtonRef = useRef<HTMLButtonElement>(null);
  const settingsDialogRef = useRef<HTMLDivElement>(null);
  const settingsCloseButtonRef = useRef<HTMLButtonElement>(null);
  const pendingLogsRef = useRef<LogEntry[]>([]);
  const logFrameRef = useRef<number | null>(null);

  const [mimoApiKey, setMimoApiKey] = useState("");
  const [deepseekApiKey, setDeepseekApiKey] = useState("");
  const [deepseekModel, setDeepseekModel] = useState("deepseek-v4-flash");
  const [preflight, setPreflight] = useState<RuntimePreflight | null>(null);
  const [preflightLoading, setPreflightLoading] = useState(false);
  const [preflightError, setPreflightError] = useState<string | null>(null);

  const t = translations[locale];

  const flushPendingLogs = useCallback(() => {
    if (logFrameRef.current !== null) {
      cancelAnimationFrame(logFrameRef.current);
    }
    logFrameRef.current = null;
    const pending = pendingLogsRef.current.splice(0);
    if (pending.length === 0) return;
    setLogs((previous) =>
      [...previous, ...pending].slice(-MAX_RETAINED_LOGS)
    );
  }, []);

  const queueLog = useCallback(
    (entry: LogEntry) => {
      pendingLogsRef.current.push(entry);
      if (logFrameRef.current === null) {
        logFrameRef.current = requestAnimationFrame(flushPendingLogs);
      }
    },
    [flushPendingLogs]
  );

  useEffect(
    () => () => {
      if (logFrameRef.current !== null) {
        cancelAnimationFrame(logFrameRef.current);
      }
      pendingLogsRef.current = [];
    },
    []
  );

  const refreshPreflight = useCallback(async (
    selectedBackend: BackendKind = backend
  ) => {
    setPreflightLoading(true);
    setPreflightError(null);
    try {
      const result = await getRuntimePreflight({
        mimoApiKey,
        deepseekApiKey,
        backend: selectedBackend,
      });
      setPreflight(result);
    } catch {
      setPreflight(null);
      setPreflightError(t.preflightUnavailable);
    } finally {
      setPreflightLoading(false);
    }
  }, [backend, mimoApiKey, deepseekApiKey, t.preflightUnavailable]);

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
        setBackend(
          resolveInitialBackend(
            info.runtime_mode,
            localStorage.getItem("backend")
          )
        );
      })
      .catch(() => {
        // Standalone frontend builds do not have a Tauri command bridge.
        // The build-time default remains safe, and Rust enforces the policy.
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!showSettings) return;

    const dialog = settingsDialogRef.current;
    settingsCloseButtonRef.current?.focus();

    const handleSettingsKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setShowSettings(false);
        return;
      }

      if (event.key !== "Tab" || !dialog) return;

      const focusableElements = Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])'
        )
      ).filter(
        (element) =>
          !element.hasAttribute("hidden") &&
          element.getAttribute("aria-hidden") !== "true"
      );

      if (focusableElements.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }

      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];
      const activeElement = document.activeElement;

      if (
        event.shiftKey &&
        (activeElement === firstElement || !dialog.contains(activeElement))
      ) {
        event.preventDefault();
        lastElement.focus();
      } else if (
        !event.shiftKey &&
        (activeElement === lastElement || !dialog.contains(activeElement))
      ) {
        event.preventDefault();
        firstElement.focus();
      }
    };

    document.addEventListener("keydown", handleSettingsKeyDown);
    return () => {
      document.removeEventListener("keydown", handleSettingsKeyDown);
      settingsButtonRef.current?.focus();
    };
  }, [showSettings]);

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
    pendingLogsRef.current = [];
    if (logFrameRef.current !== null) {
      cancelAnimationFrame(logFrameRef.current);
      logFrameRef.current = null;
    }
    setLogs([]);
    setErrorMessage("");

    try {
      const result = await runMeetingAsr(
        audioPath,
        outputMode,
        { mimoApiKey, deepseekApiKey, deepseekModel, backend },
        {
          onLog: queueLog,
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
      flushPendingLogs();
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
    queueLog,
    flushPendingLogs,
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

  const outputModeLabel = (mode: OutputMode) => {
    switch (mode) {
      case "notes":
        return t.modeNotes;
      case "minutes":
        return t.modeMinutes;
      case "transcript":
      default:
        return t.modeTranscript;
    }
  };

  const backendLabel = (mode: RuntimeMode, kind: BackendKind) => {
    if (kind === "python") return t.backendPython;
    return mode === "production"
      ? t.backendGoProduction
      : t.backendGoExperimental;
  };

  const credentialPlaceholder =
    runtimeMode === "production"
      ? t.productionCredentialPlaceholder
      : t.developmentCredentialPlaceholder;

  const preflightReadyForMode =
    outputMode === "minutes" ? preflight?.ready_for_minutes : preflight?.ready;

  const hasActivity = isRunning || logs.length > 0 || taskFolder || status;

  return (
    <div className="app-shell">
      <div className="app">
        <header className="app-header">
          <div className="brand-lockup">
            <div className="brand-mark" aria-hidden="true">
              <span></span>
              <span></span>
              <span></span>
            </div>
            <div className="brand-copy">
              <span className="app-kicker">{t.appKicker}</span>
              <h1>{t.appTitle}</h1>
              <p className="subtitle">{t.appSubtitle}</p>
            </div>
          </div>
          <div className="header-actions">
            <span className="runtime-pill">
              <span className="runtime-dot" aria-hidden="true"></span>
              {runtimeMode === "production" ? t.production : t.development}
            </span>
            <button
              ref={settingsButtonRef}
              className="btn-settings"
              onClick={handleOpenSettings}
              aria-haspopup="dialog"
              aria-expanded={showSettings}
              aria-controls="settings-dialog"
            >
              {t.settingsTitle}
            </button>
          </div>
        </header>

        <main className="main-content">
          <section className="workflow-card">
            <div className="card-heading">
              <div>
                <span className="section-kicker">{t.workflowEyebrow}</span>
                <h2>{t.workflowTitle}</h2>
                <p>{t.workflowDescription}</p>
              </div>
              <span className="workflow-number" aria-hidden="true">
                01
              </span>
            </div>

            <div className="workflow-body">
              <div className="workflow-block">
                <div className="workflow-block-heading">
                  <span>01</span>
                  <h3>{t.inputTitle}</h3>
                </div>
                <FilePicker
                  audioPath={audioPath}
                  onFileSelected={setAudioPath}
                  disabled={isRunning}
                  labels={{
                    chooseFile: t.chooseFile,
                    noFileSelected: t.noFileSelected,
                    audioFile: t.audioFile,
                  }}
                />
              </div>

              <div className="workflow-divider"></div>

              <div className="workflow-block">
                <div className="workflow-block-heading">
                  <span>02</span>
                  <h3>{t.outputModeTitle}</h3>
                </div>
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
            </div>
          </section>

          <section className="activity-region">
            {!hasActivity && (
              <div className={`empty-state ${audioPath ? "has-file" : ""}`}>
                <div className="empty-visual" aria-hidden="true">
                  <span></span>
                  <span></span>
                  <span></span>
                  <span></span>
                  <span></span>
                </div>
                <div>
                  <strong>{audioPath ? t.readyToRun : t.chooseFileToBegin}</strong>
                  <p>{audioPath ? t.readyToRunDesc : t.chooseFileToBeginDesc}</p>
                </div>
              </div>
            )}

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
                resultsSummary: t.resultsSummary,
                primaryOutput: t.primaryOutput,
                generatedFiles: t.generatedFiles,
                taskFolder: t.taskFolder,
                advancedDetails: t.advancedDetails,
                transcript: t.transcript,
                notesInput: t.notesInput,
                meetingMinutes: t.meetingMinutes,
                openTaskFolder: t.openTaskFolder,
              }}
            />

            {status && !isRunning && (
              <div
                className={`section status-section ${
                  statusOk === true
                    ? "success"
                    : statusOk === false
                      ? "error"
                      : ""
                }`}
                role="status"
                aria-live="polite"
                aria-atomic="true"
              >
                <span className="status-mark" aria-hidden="true">
                  {statusOk === true ? "✓" : statusOk === false ? "!" : "·"}
                </span>
                <span>{status}</span>
              </div>
            )}
          </section>
        </main>
      </div>

      {showSettings && (
        <div className="modal-overlay" onClick={handleCloseSettings}>
          <div
            id="settings-dialog"
            ref={settingsDialogRef}
            className="modal settings-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="settings-title"
            aria-describedby="settings-description"
            tabIndex={-1}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="modal-header">
              <div>
                <span className="modal-eyebrow">{t.appKicker}</span>
                <h2 id="settings-title">{t.settingsTitle}</h2>
                <p id="settings-description">{t.settingsIntro}</p>
              </div>
              <button
                ref={settingsCloseButtonRef}
                className="btn-close"
                onClick={handleCloseSettings}
                aria-label={t.close}
              >
                ×
              </button>
            </div>

            <div className="modal-body settings-body">
              <section className="settings-section">
                <div className="settings-section-header settings-section-header-action">
                  <div className="settings-section-title">
                    <span>01</span>
                    <div>
                      <h3>{t.settingsReadiness}</h3>
                      <p>{t.settingsReadinessDesc}</p>
                    </div>
                  </div>
                  <button
                    className="btn-secondary preflight-refresh"
                    onClick={() => void refreshPreflight()}
                    disabled={preflightLoading}
                  >
                    {preflightLoading ? t.checking : t.refresh}
                  </button>
                </div>

                {preflightError && (
                  <div className="preflight-error">{preflightError}</div>
                )}
                {!preflight && !preflightError && (
                  <div className="preflight-empty">{t.preflightPrompt}</div>
                )}
                {preflight && (
                  <div className="preflight-panel">
                    <div
                      className={`preflight-summary ${
                        preflightReadyForMode ? "ready" : "not-ready"
                      }`}
                    >
                      <span aria-hidden="true">
                        {preflightReadyForMode ? "✓" : "!"}
                      </span>
                      <strong>
                        {preflightReadyForMode ? t.ready : t.notReady}
                      </strong>
                      <span>{outputModeLabel(outputMode)}</span>
                    </div>
                    <div className="preflight-grid">
                      <div className="preflight-item">
                        <span className="preflight-label">{t.runtimeMode}</span>
                        <span className="preflight-value">
                          {preflight.runtime_mode === "production"
                            ? t.production
                            : t.development}
                        </span>
                      </div>
                      <div className="preflight-item">
                        <span className="preflight-label">{t.backend}</span>
                        <span className="preflight-value">
                          {backendLabel(
                            preflight.runtime_mode,
                            preflight.backend
                          )}
                        </span>
                      </div>
                      <div className="preflight-item preflight-item-diagnostic">
                        <div className="preflight-item-row">
                          <span className="preflight-label">{t.sidecar}</span>
                          <span
                            className={
                              preflight.sidecar.ready
                                ? "preflight-value ready"
                                : "preflight-value not-ready"
                            }
                          >
                            {preflight.sidecar.ready ? t.ready : t.notReady}
                          </span>
                        </div>
                        <span className="preflight-detail">
                          {preflight.sidecar.message}
                        </span>
                      </div>
                      <div className="preflight-item preflight-item-diagnostic">
                        <div className="preflight-item-row">
                          <span className="preflight-label">ffmpeg</span>
                          <span
                            className={
                              preflight.ffmpeg.ready
                                ? "preflight-value ready"
                                : "preflight-value not-ready"
                            }
                          >
                            {preflight.ffmpeg.ready ? t.ready : t.notReady}
                          </span>
                        </div>
                        <span className="preflight-detail">
                          {preflight.ffmpeg.message}
                        </span>
                      </div>
                    </div>
                  </div>
                )}
              </section>

              <section className="settings-section">
                <div className="settings-section-header">
                  <div className="settings-section-title">
                    <span>02</span>
                    <div>
                      <h3>{t.settingsAppearance}</h3>
                      <p>{t.settingsAppearanceDesc}</p>
                    </div>
                  </div>
                </div>
                <div className="settings-form-grid">
                  <div className="form-group">
                    <label htmlFor="settings-language">{t.language}</label>
                    <select
                      id="settings-language"
                      value={locale}
                      onChange={(e) =>
                        setLocaleState(e.target.value as Locale)
                      }
                    >
                      <option value="zh">中文</option>
                      <option value="en">English</option>
                    </select>
                  </div>
                  <div className="form-group">
                    <label htmlFor="settings-theme">{t.theme}</label>
                    <select
                      id="settings-theme"
                      value={theme}
                      onChange={(e) => setTheme(e.target.value as Theme)}
                    >
                      <option value="system">{t.themeSystem}</option>
                      <option value="light">{t.themeLight}</option>
                      <option value="dark">{t.themeDark}</option>
                    </select>
                  </div>
                </div>
              </section>

              <section className="settings-section">
                <div className="settings-section-header">
                  <div className="settings-section-title">
                    <span>03</span>
                    <div>
                      <h3>{t.settingsBackend}</h3>
                      <p>{t.settingsBackendDesc}</p>
                    </div>
                  </div>
                </div>
                {runtimeMode === "production" ? (
                  <div className="readonly-field">
                    <span className="readonly-dot" aria-hidden="true"></span>
                    {t.backendGoProduction}
                  </div>
                ) : (
                  <div className="form-group form-group-compact">
                    <label htmlFor="settings-backend">{t.backend}</label>
                    <select
                      id="settings-backend"
                      value={backend}
                      onChange={(e) => {
                        const selectedBackend =
                          e.target.value === "go" ? "go" : "python";
                        setBackend(selectedBackend);
                        void refreshPreflight(selectedBackend);
                      }}
                    >
                      {DEVELOPMENT_BACKEND_OPTIONS.map((option) => (
                        <option key={option} value={option}>
                          {option === "go"
                            ? t.backendGoExperimental
                            : t.backendPython}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </section>

              <section className="settings-section">
                <div className="settings-section-header">
                  <div className="settings-section-title">
                    <span>04</span>
                    <div>
                      <h3>{t.settingsCredentials}</h3>
                      <p>{t.settingsCredentialsDesc}</p>
                    </div>
                  </div>
                </div>

                <div className="settings-note">
                  <span className="warning-icon" aria-hidden="true">
                    i
                  </span>
                  <span>
                    {runtimeMode === "production"
                      ? t.productionCredentialsNote
                      : t.keysNotPersisted}
                  </span>
                </div>

                {preflight && (
                  <div className="credential-status-grid">
                    <div className="credential-status-item">
                      <span>{t.mimoApiKey}</span>
                      <strong
                        className={
                          preflight.mimo_api_key.status.configured
                            ? "ready"
                            : "not-ready"
                        }
                      >
                        {credentialStatusText(preflight.mimo_api_key)}
                      </strong>
                    </div>
                    <div className="credential-status-item">
                      <span>{t.deepseekApiKey}</span>
                      <strong
                        className={
                          preflight.deepseek_api_key.status.configured
                            ? "ready"
                            : "not-ready"
                        }
                      >
                        {credentialStatusText(preflight.deepseek_api_key)}
                      </strong>
                    </div>
                  </div>
                )}

                <div className="credential-fields">
                  <div className="form-group">
                    <label htmlFor="settings-mimo-key">{t.mimoApiKey}</label>
                    <div className="key-input-row">
                      <input
                        id="settings-mimo-key"
                        type="password"
                        value={mimoApiKey}
                        onChange={(e) => setMimoApiKey(e.target.value)}
                        placeholder={credentialPlaceholder}
                      />
                      {runtimeMode === "development" && (
                        <button
                          className="btn-secondary btn-save-key"
                          onClick={() =>
                            handleSaveKey("MIMO_API_KEY", mimoApiKey)
                          }
                          disabled={!mimoApiKey.trim()}
                          title={t.saveToEnv}
                        >
                          {t.saveToEnv}
                        </button>
                      )}
                    </div>
                    {saveStatus["MIMO_API_KEY"] && (
                      <span className="save-status">
                        {saveStatus["MIMO_API_KEY"]}
                      </span>
                    )}
                  </div>
                  <div className="form-group">
                    <label htmlFor="settings-deepseek-key">
                      {t.deepseekApiKey}
                    </label>
                    <div className="key-input-row">
                      <input
                        id="settings-deepseek-key"
                        type="password"
                        value={deepseekApiKey}
                        onChange={(e) => setDeepseekApiKey(e.target.value)}
                        placeholder={credentialPlaceholder}
                      />
                      {runtimeMode === "development" && (
                        <button
                          className="btn-secondary btn-save-key"
                          onClick={() =>
                            handleSaveKey(
                              "DEEPSEEK_API_KEY",
                              deepseekApiKey
                            )
                          }
                          disabled={!deepseekApiKey.trim()}
                          title={t.saveToEnv}
                        >
                          {t.saveToEnv}
                        </button>
                      )}
                    </div>
                    {saveStatus["DEEPSEEK_API_KEY"] && (
                      <span className="save-status">
                        {saveStatus["DEEPSEEK_API_KEY"]}
                      </span>
                    )}
                  </div>
                </div>
              </section>

              <section className="settings-section">
                <div className="settings-section-header">
                  <div className="settings-section-title">
                    <span>05</span>
                    <div>
                      <h3>{t.settingsMinutesModel}</h3>
                      <p>{t.settingsMinutesModelDesc}</p>
                    </div>
                  </div>
                </div>
                <div className="form-group form-group-compact">
                  <label htmlFor="settings-deepseek-model">
                    {t.deepseekModel}
                  </label>
                  <select
                    id="settings-deepseek-model"
                    value={deepseekModel}
                    onChange={(e) => setDeepseekModel(e.target.value)}
                  >
                    <option value="deepseek-v4-flash">deepseek-v4-flash</option>
                    <option value="deepseek-v4-pro">deepseek-v4-pro</option>
                  </select>
                </div>
              </section>

              <section className="settings-section">
                <div className="settings-section-header">
                  <div className="settings-section-title">
                    <span>06</span>
                    <div>
                      <h3>{t.settingsOutputLocation}</h3>
                      <p>{t.settingsOutputLocationDesc}</p>
                    </div>
                  </div>
                </div>
                {preflight ? (
                  <div className="output-location-card">
                    <div className="output-location-meta">
                      <span className="preflight-label">{t.outputRoot}</span>
                      <span
                        className={
                          preflight.output_root.ready
                            ? "preflight-value ready"
                            : "preflight-value not-ready"
                        }
                      >
                        {preflight.output_root.ready ? t.ready : t.notReady}
                      </span>
                    </div>
                    <code>{preflight.output_root.path}</code>
                    <p>{preflight.output_root.message}</p>
                  </div>
                ) : (
                  <div className="preflight-empty">{t.preflightPrompt}</div>
                )}
              </section>
            </div>

            <div className="modal-footer">
              <button className="btn-secondary" onClick={handleCloseSettings}>
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
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="error-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="modal-header">
              <div>
                <span className="modal-eyebrow">Meeting ASR</span>
                <h2 id="error-title">{t.errorTitle}</h2>
              </div>
              <button
                className="btn-close"
                onClick={() => setShowError(false)}
                aria-label={t.close}
              >
                ×
              </button>
            </div>
            <div className="modal-body">
              <div className="error-state-mark" aria-hidden="true">
                !
              </div>
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
