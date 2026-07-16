import {
  useState,
  useEffect,
  useCallback,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { open } from "@tauri-apps/plugin-dialog";
import { translations, getLocale, setLocale, type Locale } from "./i18n";
import {
  runMeetingAsr,
  openTaskFolder,
  saveEnvKey,
  getRuntimeInfo,
  getRuntimePreflight,
  setOutputRoot,
  resetOutputRoot,
  openOutputRoot,
  parseBackendError,
  listPromptTemplates,
  listProviders,
  getProviderSettings,
  setProviderSettings,
  type BackendKind,
  type RuntimeMode,
  type RuntimePreflight,
  type LogEntry,
  type OutputMode,
  type OutputRootInfo,
  type ProgressEvent,
  type PromptTemplateSummary,
  type ProviderCatalog,
  type ProviderSettingsInfo,
} from "./lib/meetingAsr";
import { FilePicker } from "./components/FilePicker";
import { RunControls } from "./components/RunControls";
import { LogPanel } from "./components/LogPanel";
import { ResultPanel } from "./components/ResultPanel";
import { TaskLibrary } from "./components/TaskLibrary";
import "./App.css";

type Theme = "light" | "dark" | "system";
const MAX_RETAINED_LOGS = 1000;
const DEVELOPMENT_BACKEND_OPTIONS = ["go", "python"] as const satisfies readonly BackendKind[];
const DEFAULT_PROVIDER_SETTINGS: ProviderSettingsInfo = {
  asr: { provider_id: "mimo", model_id: "mimo-v2.5-asr" },
  llm: { provider_id: "deepseek", model_id: "deepseek-v4-flash" },
};

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
  const [outputMode, setOutputMode] = useState<OutputMode>("transcript_only");
  const [templates, setTemplates] = useState<PromptTemplateSummary[]>([]);
  const [templateId, setTemplateId] = useState("structured-notes");
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
  const [generatedContentPath, setGeneratedContentPath] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [statusOk, setStatusOk] = useState<boolean | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [showError, setShowError] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [activeArea, setActiveArea] = useState<"workflow" | "library">("workflow");
  const [libraryOpened, setLibraryOpened] = useState(false);
  const [libraryRefreshToken, setLibraryRefreshToken] = useState(0);
  const workflowTabRef = useRef<HTMLButtonElement>(null);
  const libraryTabRef = useRef<HTMLButtonElement>(null);
  const settingsButtonRef = useRef<HTMLButtonElement>(null);
  const settingsDialogRef = useRef<HTMLDivElement>(null);
  const settingsCloseButtonRef = useRef<HTMLButtonElement>(null);
  const changeOutputFolderButtonRef = useRef<HTMLButtonElement>(null);
  const outputMutationInFlightRef = useRef(false);
  const outputOpenInFlightRef = useRef(false);
  const pendingLogsRef = useRef<LogEntry[]>([]);
  const logFrameRef = useRef<number | null>(null);

  const [mimoApiKey, setMimoApiKey] = useState("");
  const [deepseekApiKey, setDeepseekApiKey] = useState("");
  const [providerCatalog, setProviderCatalog] = useState<ProviderCatalog | null>(null);
  const [providerSettings, setProviderSettingsState] = useState<ProviderSettingsInfo>(DEFAULT_PROVIDER_SETTINGS);
  const [providerSavePending, setProviderSavePending] = useState(false);
  const [providerSaveMessage, setProviderSaveMessage] = useState<string | null>(null);
  const [preflight, setPreflight] = useState<RuntimePreflight | null>(null);
  const [preflightLoading, setPreflightLoading] = useState(false);
  const [preflightError, setPreflightError] = useState<string | null>(null);
  const [outputRootInfo, setOutputRootInfo] = useState<OutputRootInfo | null>(null);
  const [outputPickerPending, setOutputPickerPending] = useState(false);
  const [outputMutationPending, setOutputMutationPending] = useState(false);
  const [outputOpenPending, setOutputOpenPending] = useState(false);
  const [outputLocationFeedback, setOutputLocationFeedback] = useState<{
    tone: "success" | "error";
    message: string;
  } | null>(null);

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
      setOutputRootInfo({
        output_root: result.output_root.path,
        configured: result.output_root.configured,
        runtime_mode: result.runtime_mode,
      });
    } catch {
      setPreflight(null);
      setPreflightError(t.preflightUnavailable);
    } finally {
      setPreflightLoading(false);
    }
  }, [backend, mimoApiKey, deepseekApiKey, t.preflightUnavailable]);

  useEffect(() => {
    const timer = window.setTimeout(() => { void refreshPreflight(); }, 150);
    return () => window.clearTimeout(timer);
  }, [refreshPreflight]);

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
    let active = true;
    Promise.all([listProviders(), getProviderSettings()])
      .then(([catalog, settings]) => {
        if (!active) return;
        setProviderCatalog(catalog);
        setProviderSettingsState(settings);
      })
      .catch(() => {
        if (active) setProviderCatalog(null);
      });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    let active = true;
    listPromptTemplates().then((next) => {
      if (!active) return;
      setTemplates(next);
      if (next.length > 0 && !next.some((template) => template.id === templateId)) setTemplateId(next[0].id);
    }).catch(() => { if (active) setTemplates([]); });
    return () => { active = false; };
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
    setGeneratedContentPath(null);
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
        {
          mimoApiKey,
          deepseekApiKey,
          backend,
          templateId,
          asrConfig: providerSettings.asr,
          llmConfig: outputMode === "transcript_and_generate" ? providerSettings.llm : undefined,
        },
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
      if (result.generated_content_path) setGeneratedContentPath(result.generated_content_path);

      setStatus(t.completedSuccessfully);
      setStatusOk(true);
      if (libraryOpened) setLibraryRefreshToken((token) => token + 1);
    } catch (e) {
      const error = parseBackendError(String(e));
      setErrorMessage(error.message);
      setShowError(true);
      setStatus(error.message);
      setStatusOk(false);
      if (libraryOpened) setLibraryRefreshToken((token) => token + 1);
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
    providerSettings,
    backend,
    templateId,
    t,
    queueLog,
    flushPendingLogs,
    libraryOpened,
  ]);

  const showWorkflow = useCallback(() => {
    setActiveArea("workflow");
    requestAnimationFrame(() => workflowTabRef.current?.focus());
  }, []);

  const showLibrary = () => {
    setLibraryOpened(true);
    setActiveArea("library");
    void refreshPreflight();
  };

  const handleWorkspaceTabKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    if (activeArea === "workflow") {
      showLibrary();
      requestAnimationFrame(() => libraryTabRef.current?.focus());
    } else {
      showWorkflow();
    }
  };

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
    setProviderSaveMessage(null);
    void getProviderSettings().then(setProviderSettingsState).catch(() => {});
  };

  const handleOpenSettings = () => {
    setShowSettings(true);
    setOutputLocationFeedback(null);
    void refreshPreflight();
  };

  const outputLocationError = useCallback((error: unknown) => {
    const message = parseBackendError(String(error)).message;
    if (message.includes("while processing")) return t.outputFolderChangeBlocked;
    if (message.includes("not writable")) return t.outputFolderNotWritable;
    if (
      message.includes("unavailable") ||
      message.includes("not a directory") ||
      message.includes("absolute path") ||
      message.includes("output folder") ||
      message.includes("desktop settings")
    ) {
      return t.outputFolderUnavailable;
    }
    return t.outputFolderUnavailable;
  }, [
    t.outputFolderChangeBlocked,
    t.outputFolderNotWritable,
    t.outputFolderUnavailable,
  ]);

  const refreshAfterOutputRootChange = useCallback(async () => {
    if (libraryOpened) setLibraryRefreshToken((token) => token + 1);
    await refreshPreflight();
  }, [libraryOpened, refreshPreflight]);

  const handleChangeOutputFolder = useCallback(async () => {
    if (isRunning || outputMutationInFlightRef.current) return;
    outputMutationInFlightRef.current = true;
    setOutputPickerPending(true);
    try {
      const selected = await open({
        directory: true,
        multiple: false,
        title: t.chooseOutputFolder,
        defaultPath: outputRootInfo?.output_root,
      });
      if (typeof selected !== "string") return;

      setOutputMutationPending(true);
      setOutputLocationFeedback(null);
      try {
        const next = await setOutputRoot(selected);
        setOutputRootInfo(next);
        setOutputLocationFeedback({
          tone: "success",
          message: t.outputFolderChanged,
        });
        await refreshAfterOutputRootChange();
      } catch (error) {
        setOutputLocationFeedback({
          tone: "error",
          message: outputLocationError(error),
        });
      } finally {
        setOutputMutationPending(false);
      }
    } finally {
      outputMutationInFlightRef.current = false;
      setOutputPickerPending(false);
      requestAnimationFrame(() => changeOutputFolderButtonRef.current?.focus());
    }
  }, [
    isRunning,
    outputRootInfo?.output_root,
    t.chooseOutputFolder,
    t.outputFolderChanged,
    outputLocationError,
    refreshAfterOutputRootChange,
  ]);

  const handleResetOutputFolder = useCallback(async () => {
    if (isRunning || outputMutationInFlightRef.current) return;
    outputMutationInFlightRef.current = true;
    setOutputMutationPending(true);
    setOutputLocationFeedback(null);
    try {
      const next = await resetOutputRoot();
      setOutputRootInfo(next);
      setOutputLocationFeedback({
        tone: "success",
        message: t.outputFolderReset,
      });
      await refreshAfterOutputRootChange();
    } catch (error) {
      setOutputLocationFeedback({
        tone: "error",
        message: outputLocationError(error),
      });
    } finally {
      outputMutationInFlightRef.current = false;
      setOutputMutationPending(false);
    }
  }, [
    isRunning,
    t.outputFolderReset,
    outputLocationError,
    refreshAfterOutputRootChange,
  ]);

  const handleOpenOutputFolder = useCallback(async () => {
    if (outputOpenInFlightRef.current) return;
    outputOpenInFlightRef.current = true;
    setOutputOpenPending(true);
    setOutputLocationFeedback(null);
    try {
      await openOutputRoot();
    } catch (error) {
      setOutputLocationFeedback({
        tone: "error",
        message: outputLocationError(error),
      });
    } finally {
      outputOpenInFlightRef.current = false;
      setOutputOpenPending(false);
    }
  }, [outputLocationError]);

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

  const handleSaveProviderSettings = useCallback(async () => {
    if (isRunning || providerSavePending) return;
    setProviderSavePending(true);
    setProviderSaveMessage(null);
    try {
      const saved = await setProviderSettings(providerSettings.asr, providerSettings.llm);
      setProviderSettingsState(saved);
      setProviderSaveMessage(t.savedSuccessfully);
      await refreshPreflight();
    } catch (error) {
      setProviderSaveMessage(`${t.saveFailed}: ${parseBackendError(String(error)).message}`);
    } finally {
      setProviderSavePending(false);
    }
  }, [isRunning, providerSavePending, providerSettings, refreshPreflight, t.saveFailed, t.savedSuccessfully]);

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
      case "transcript_and_generate":
        return t.modeGenerate;
      case "transcript_only":
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
    outputMode === "transcript_and_generate" ? preflight?.ready_for_minutes : preflight?.ready;

  const asrDescriptor = providerCatalog?.asr.find((provider) => provider.id === providerSettings.asr.provider_id);
  const llmDescriptor = providerCatalog?.llm.find((provider) => provider.id === providerSettings.llm.provider_id);
  const asrProviderLabel = asrDescriptor?.display_name[locale] ?? "Mimo";
  const llmProviderLabel = llmDescriptor?.display_name[locale] ?? "DeepSeek";
  const asrModelLabel = asrDescriptor?.models.find((model) => model.id === providerSettings.asr.model_id)?.display_name[locale] ?? providerSettings.asr.model_id;
  const llmModelLabel = llmDescriptor?.models.find((model) => model.id === providerSettings.llm.model_id)?.display_name[locale] ?? providerSettings.llm.model_id;

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

        <nav
          className="workspace-tabs"
          role="tablist"
          aria-label={t.workspaceNavigation}
          onKeyDown={handleWorkspaceTabKeyDown}
        >
          <button
            ref={workflowTabRef}
            id="workflow-tab"
            role="tab"
            aria-selected={activeArea === "workflow"}
            aria-controls="workflow-panel"
            tabIndex={activeArea === "workflow" ? 0 : -1}
            onClick={() => setActiveArea("workflow")}
          >
            {t.newTask}
          </button>
          <button
            ref={libraryTabRef}
            id="library-tab"
            role="tab"
            aria-selected={activeArea === "library"}
            aria-controls="library-panel"
            tabIndex={activeArea === "library" ? 0 : -1}
            onClick={showLibrary}
          >
            {t.taskLibraryTitle}
          </button>
        </nav>

        <main className="main-content">
          <div
            id="workflow-panel"
            role="tabpanel"
            aria-labelledby="workflow-tab"
            hidden={activeArea !== "workflow"}
          >
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
                  templates={templates}
                  templateId={templateId}
                  onTemplateChange={setTemplateId}
                  locale={locale}
                  onStartRun={startRun}
                  isRunning={isRunning}
                  audioPath={audioPath}
                  ready={Boolean(preflightReadyForMode)}
                  asrSummary={`${asrProviderLabel} · ${asrModelLabel}`}
                  llmSummary={`${llmProviderLabel} · ${llmModelLabel}`}
                  onOpenSettings={handleOpenSettings}
                  labels={{
                    modeTranscript: t.modeTranscript,
                    modeTranscriptDesc: t.modeTranscriptDesc,
                    modeGenerate: t.modeGenerate,
                    modeGenerateDesc: t.modeGenerateDesc,
                    templateLabel: t.templateLabel,
                    start: t.start,
                    processing: t.processing,
                    llmWarning: t.llmWarning,
                    transcriptWarning: t.transcriptWarning,
                    speechRecognitionProvider: t.speechRecognitionProvider,
                    generatedWithProvider: t.generatedWithProvider,
                    providerNotReady: t.providerNotReady,
                    openSettings: t.openSettings,
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
                generatingContent: t.generatingContent,
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
              generatedContentPath={generatedContentPath}
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
                generatedContent: t.generatedContent,
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
          </div>

          {libraryOpened && (
            <div
              id="library-panel"
              role="tabpanel"
              aria-labelledby="library-tab"
              hidden={activeArea !== "library"}
            >
              <TaskLibrary
                locale={locale}
                labels={t}
                refreshToken={libraryRefreshToken}
                onReturnToWorkflow={showWorkflow}
                deepseekApiKey={deepseekApiKey}
                templates={templates}
                llmReady={Boolean(preflight?.ready_for_minutes)}
                llmProviderLabel={llmProviderLabel}
                llmModelLabel={llmModelLabel}
                llmConfig={providerSettings.llm}
                onOpenSettings={handleOpenSettings}
              />
            </div>
          )}
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
                      <h3>{t.settingsProviders}</h3>
                      <p>{t.settingsProvidersDesc}</p>
                    </div>
                  </div>
                </div>
                <div className="provider-settings-grid">
                  <div className="provider-settings-card">
                    <h4>{t.speechRecognition}</h4>
                    <div className="form-group form-group-compact"><label htmlFor="settings-asr-provider">{t.provider}</label><select id="settings-asr-provider" value={providerSettings.asr.provider_id} disabled={isRunning || providerSavePending} onChange={(event) => { const provider = providerCatalog?.asr.find((item) => item.id === event.target.value); if (provider) setProviderSettingsState((current) => ({...current, asr: {provider_id: provider.id, model_id: provider.default_model}})); }}>{providerCatalog?.asr.map((provider) => <option key={provider.id} value={provider.id}>{provider.display_name[locale]}</option>) ?? <option value="mimo">Mimo</option>}</select></div>
                    <div className="form-group form-group-compact"><label htmlFor="settings-asr-model">{t.model}</label><select id="settings-asr-model" value={providerSettings.asr.model_id} disabled={isRunning || providerSavePending} onChange={(event) => setProviderSettingsState((current) => ({...current, asr: {...current.asr, model_id: event.target.value}}))}>{asrDescriptor?.models.map((model) => <option key={model.id} value={model.id}>{model.display_name[locale]}</option>) ?? <option value="mimo-v2.5-asr">Mimo v2.5 ASR</option>}</select></div>
                    <p className={preflight?.asr_provider.ready ? "provider-readiness ready" : "provider-readiness not-ready"}>{preflight?.asr_provider.ready ? t.ready : t.notReady}</p>
                  </div>
                  <div className="provider-settings-card">
                    <h4>{t.generatedContentProvider}</h4>
                    <div className="form-group form-group-compact"><label htmlFor="settings-llm-provider">{t.provider}</label><select id="settings-llm-provider" value={providerSettings.llm.provider_id} disabled={isRunning || providerSavePending} onChange={(event) => { const provider = providerCatalog?.llm.find((item) => item.id === event.target.value); if (provider) setProviderSettingsState((current) => ({...current, llm: {provider_id: provider.id, model_id: provider.default_model}})); }}>{providerCatalog?.llm.map((provider) => <option key={provider.id} value={provider.id}>{provider.display_name[locale]}</option>) ?? <option value="deepseek">DeepSeek</option>}</select></div>
                    <div className="form-group form-group-compact"><label htmlFor="settings-llm-model">{t.model}</label><select id="settings-llm-model" value={providerSettings.llm.model_id} disabled={isRunning || providerSavePending} onChange={(event) => setProviderSettingsState((current) => ({...current, llm: {...current.llm, model_id: event.target.value}}))}>{llmDescriptor?.models.map((model) => <option key={model.id} value={model.id}>{model.display_name[locale]}</option>) ?? <option value="deepseek-v4-flash">DeepSeek V4 Flash</option>}</select></div>
                    <p className={preflight?.llm_provider.ready ? "provider-readiness ready" : "provider-readiness not-ready"}>{preflight?.llm_provider.ready ? t.ready : t.notReady}</p>
                  </div>
                </div>
                <div className="provider-settings-actions"><button className="btn-secondary" onClick={() => void handleSaveProviderSettings()} disabled={isRunning || providerSavePending}>{providerSavePending ? t.checking : t.save}</button>{providerSaveMessage && <span className="save-status">{providerSaveMessage}</span>}</div>
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
                {outputRootInfo || preflight ? (
                  <div className="output-location-card">
                    <div className="output-location-meta">
                      <span className="preflight-label">{t.currentOutputFolder}</span>
                      {preflight && (
                        <span
                          className={
                            preflight.output_root.ready
                              ? "preflight-value ready"
                              : "preflight-value not-ready"
                          }
                        >
                          {preflight.output_root.ready ? t.ready : t.notReady}
                        </span>
                      )}
                    </div>
                    <code>{outputRootInfo?.output_root ?? preflight?.output_root.path}</code>
                    <p className="output-location-warning">{t.outputFolderMoveWarning}</p>
                    <div className="output-location-actions">
                      <button
                        ref={changeOutputFolderButtonRef}
                        className="btn-secondary"
                        onClick={() => void handleChangeOutputFolder()}
                        disabled={
                          isRunning || outputPickerPending || outputMutationPending
                        }
                        title={isRunning ? t.outputFolderChangeBlocked : undefined}
                      >
                        {t.changeOutputFolder}
                      </button>
                      <button
                        className="btn-secondary"
                        onClick={() => void handleOpenOutputFolder()}
                        disabled={
                          outputOpenPending ||
                          outputPickerPending ||
                          outputMutationPending
                        }
                      >
                        {t.openOutputFolderAction}
                      </button>
                      <button
                        className="btn-secondary"
                        onClick={() => void handleResetOutputFolder()}
                        disabled={
                          isRunning ||
                          outputPickerPending ||
                          outputMutationPending ||
                          !outputRootInfo?.configured
                        }
                        title={isRunning ? t.outputFolderChangeBlocked : undefined}
                      >
                        {t.resetOutputFolder}
                      </button>
                    </div>
                    {outputLocationFeedback && (
                      <p
                        className={`output-location-feedback ${outputLocationFeedback.tone}`}
                        role="status"
                        aria-live="polite"
                      >
                        {outputLocationFeedback.message}
                      </p>
                    )}
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
