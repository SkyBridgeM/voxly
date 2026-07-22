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
  resumeMeetingAsr,
  scanRecordings,
  recoverRecording,
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
  type RecordingTask,
} from "./lib/meetingAsr";
import { FilePicker } from "./components/FilePicker";
import { RunControls } from "./components/RunControls";
import { LogPanel } from "./components/LogPanel";
import { ResultPanel } from "./components/ResultPanel";
import { TaskLibrary } from "./components/TaskLibrary";
import { TemplateManager } from "./components/TemplateManager";
import { LocalModelManager } from "./components/LocalModelManager";
import { RecordingWorkspace } from "./components/RecordingWorkspace";
import voxlyAppIcon from "./assets/voxly-app-icon.png";
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
  const [sidebarCollapsed, setSidebarCollapsed] = useState(
    () => localStorage.getItem("sidebarCollapsed") === "true"
  );
  const [audioPath, setAudioPath] = useState<string | null>(null);
  const [inputChoice, setInputChoice] = useState<"home" | "record" | "import">("home");
  const [recordingTask, setRecordingTask] = useState<RecordingTask | null>(null);
  const [recoverableRecordings, setRecoverableRecordings] = useState<RecordingTask[]>([]);
  const [recordingRecoveryPending, setRecordingRecoveryPending] = useState<string | null>(null);
  const [outputMode, setOutputMode] = useState<OutputMode>("transcript_only");
  const [templates, setTemplates] = useState<PromptTemplateSummary[]>([]);
  const [templateIssues, setTemplateIssues] = useState<string[]>([]);
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
  const [activeArea, setActiveArea] = useState<"workflow" | "library" | "models" | "templates">("workflow");
  const [libraryOpened, setLibraryOpened] = useState(false);
  const [libraryRefreshToken, setLibraryRefreshToken] = useState(0);
  const workflowTabRef = useRef<HTMLButtonElement>(null);
  const libraryTabRef = useRef<HTMLButtonElement>(null);
  const modelsTabRef = useRef<HTMLButtonElement>(null);
  const templatesTabRef = useRef<HTMLButtonElement>(null);
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
    localStorage.setItem("sidebarCollapsed", String(sidebarCollapsed));
  }, [sidebarCollapsed]);

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
    void scanRecordings().then(setRecoverableRecordings).catch(() => {});
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

  const refreshTemplates = useCallback(async () => {
    const catalog = await listPromptTemplates();
    setTemplates(catalog.templates);
    setTemplateIssues(catalog.issues.map((issue) => issue.message));
    setTemplateId((current) => catalog.templates.some((template) => template.id === current) ? current : catalog.templates[0]?.id ?? "");
  }, []);

  useEffect(() => { void refreshTemplates().catch(() => { setTemplates([]); setTemplateIssues([t.templateSaveFailed]); }); }, [refreshTemplates]);

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
      const callbacks = { onLog: queueLog, onProgress: (event: ProgressEvent) => setProgress(event), onDone: () => {} };
      const result = recordingTask
        ? await resumeMeetingAsr(recordingTask.task_folder, {
            outputMode, backend: "go", mimoApiKey, deepseekApiKey, templateId,
          }, callbacks)
        : await runMeetingAsr(
            audioPath,
            outputMode,
            {
              mimoApiKey, deepseekApiKey, backend, templateId,
              asrConfig: providerSettings.asr,
              llmConfig: outputMode === "transcript_and_generate" ? providerSettings.llm : undefined,
            }, callbacks
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
    recordingTask,
  ]);

  const handleRecordingSaved = useCallback((task: RecordingTask) => {
    setRecordingTask(task);
    setAudioPath(task.audio_path);
    setTaskFolder(task.task_folder);
    setStatus(locale === "zh" ? "录音已保存。确认处理方式，或稍后从任务库继续。" : "Recording saved. Confirm how to process it, or continue later from the Task Library.");
    setStatusOk(true);
    setLibraryRefreshToken((token) => token + 1);
    setRecoverableRecordings((items) => [task, ...items.filter((item) => item.task_id !== task.task_id)]);
  }, [locale]);

  const handleRecordingError = useCallback((message: string) => {
    const parsed = parseBackendError(message).message;
    setErrorMessage(parsed);
    setShowError(true);
  }, []);

  const handleRecoverRecording = useCallback(async (task: RecordingTask) => {
    setRecordingRecoveryPending(task.task_id);
    try {
      const recovered = await recoverRecording(task.task_folder);
      setRecoverableRecordings((items) => items.filter((item) => item.task_id !== task.task_id));
      setInputChoice("record");
      handleRecordingSaved(recovered);
    } catch (error) {
      handleRecordingError(String(error));
    } finally {
      setRecordingRecoveryPending(null);
    }
  }, [handleRecordingError, handleRecordingSaved]);

  const showWorkflow = useCallback(() => {
    setActiveArea("workflow");
    requestAnimationFrame(() => workflowTabRef.current?.focus());
  }, []);

  const showLibrary = () => {
    setLibraryOpened(true);
    setActiveArea("library");
    void refreshPreflight();
  };

  const showModels = () => {
    setActiveArea("models");
    void refreshPreflight();
  };

  const showTemplates = () => {
    setActiveArea("templates");
    void refreshTemplates();
  };

  const handleWorkspaceTabKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (!["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) return;
    event.preventDefault();
    const areas = ["workflow", "library", "models", "templates"] as const;
    const refs = [workflowTabRef, libraryTabRef, modelsTabRef, templatesTabRef];
    const current = areas.indexOf(activeArea);
    const forward = event.key === "ArrowRight" || event.key === "ArrowDown";
    const next = (current + (forward ? 1 : areas.length - 1)) % areas.length;
    if (areas[next] === "workflow") showWorkflow();
    if (areas[next] === "library") showLibrary();
    if (areas[next] === "models") showModels();
    if (areas[next] === "templates") showTemplates();
    requestAnimationFrame(() => refs[next].current?.focus());
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

	const asrPreflightMatchesSelection = preflight?.asr_provider.provider_id === providerSettings.asr.provider_id && preflight.asr_provider.model_id === providerSettings.asr.model_id;
	const llmPreflightMatchesSelection = preflight?.llm_provider.provider_id === providerSettings.llm.provider_id && preflight.llm_provider.model_id === providerSettings.llm.model_id;
	const preflightReadyForMode = Boolean(asrPreflightMatchesSelection && (outputMode === "transcript_and_generate" ? llmPreflightMatchesSelection && preflight?.ready_for_minutes : preflight?.ready));

  const asrDescriptor = providerCatalog?.asr.find((provider) => provider.id === providerSettings.asr.provider_id);
  const llmDescriptor = providerCatalog?.llm.find((provider) => provider.id === providerSettings.llm.provider_id);
  const asrProviderLabel = asrDescriptor?.display_name[locale] ?? "Mimo";
  const llmProviderLabel = llmDescriptor?.display_name[locale] ?? "DeepSeek";
  const asrModelLabel = asrDescriptor?.models.find((model) => model.id === providerSettings.asr.model_id)?.display_name[locale] ?? providerSettings.asr.model_id;
  const llmModelLabel = llmDescriptor?.models.find((model) => model.id === providerSettings.llm.model_id)?.display_name[locale] ?? providerSettings.llm.model_id;
	const privacySummary = (() => {
		const audioCloud = asrDescriptor?.privacy.sends_audio_off_device ?? true;
		const textCloud = llmDescriptor?.privacy.sends_text_off_device ?? true;
		if (!audioCloud && !textCloud) return locale === "zh" ? "音频与转写文本均留在本机。" : "Audio and transcript text stay on this device.";
		if (!audioCloud && textCloud) return locale === "zh" ? "音频留在本机；生成内容时转写文本会发送到云端。" : "Audio stays local; transcript text is sent to the cloud for content generation.";
		if (audioCloud && !textCloud) return locale === "zh" ? "音频会发送到云端；内容生成在本机完成。" : "Audio is sent to the cloud; content generation stays local.";
		return locale === "zh" ? "音频和生成内容所需的转写文本会发送到所选云端服务。" : "Audio and transcript text used for generation are sent to the selected cloud services.";
	})();

  const hasActivity = isRunning || logs.length > 0 || taskFolder || status;

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        {locale === "zh" ? "跳到主要内容" : "Skip to main content"}
      </a>
      <div className={`app ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}>
        <aside className="app-sidebar">
          <div className="brand-lockup">
            <img
              className="brand-mark"
              src={voxlyAppIcon}
              alt=""
              aria-hidden="true"
            />
            <div className="brand-copy">
              <span className="app-kicker">{t.appKicker}</span>
              <h1>{t.appTitle}</h1>
              <p className="subtitle">{t.appSubtitle}</p>
            </div>
          </div>

          <button
            type="button"
            className="sidebar-toggle"
            onClick={() => setSidebarCollapsed((collapsed) => !collapsed)}
            aria-label={sidebarCollapsed
              ? (locale === "zh" ? "展开侧边栏" : "Expand sidebar")
              : (locale === "zh" ? "折叠侧边栏" : "Collapse sidebar")}
            title={sidebarCollapsed
              ? (locale === "zh" ? "展开侧边栏" : "Expand sidebar")
              : (locale === "zh" ? "折叠侧边栏" : "Collapse sidebar")}
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d={sidebarCollapsed ? "m9 6 6 6-6 6" : "m15 6-6 6 6 6"} />
            </svg>
          </button>

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
              onClick={showWorkflow}
            >
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg>
              <span>{t.newTask}</span>
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
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 6.5h16v13H4zM7 3.5h10M8 10h8M8 14h5" /></svg>
              <span>{t.taskLibraryTitle}</span>
            </button>
            <button
              ref={modelsTabRef}
              id="models-tab"
              role="tab"
              aria-selected={activeArea === "models"}
              aria-controls="models-panel"
              tabIndex={activeArea === "models" ? 0 : -1}
              onClick={showModels}
            >
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 7c0-2 3.1-3.5 7-3.5S19 5 19 7s-3.1 3.5-7 3.5S5 9 5 7Zm0 0v5c0 2 3.1 3.5 7 3.5s7-1.5 7-3.5V7M5 12v5c0 2 3.1 3.5 7 3.5s7-1.5 7-3.5v-5" /></svg>
              <span>{locale === "zh" ? "模型" : "Models"}</span>
            </button>
            <button
              ref={templatesTabRef}
              id="templates-tab"
              role="tab"
              aria-selected={activeArea === "templates"}
              aria-controls="templates-panel"
              tabIndex={activeArea === "templates" ? 0 : -1}
              onClick={showTemplates}
            >
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3.5h9l3 3V20.5H6zM15 3.5v4h3M9 12h6M9 16h6" /></svg>
              <span>{locale === "zh" ? "模板" : "Templates"}</span>
            </button>
          </nav>

          <div className="sidebar-footer">
            <span className="runtime-pill">
              <span className="runtime-dot" aria-hidden="true"></span>
              <span className="runtime-label">{runtimeMode === "production" ? t.production : t.development}</span>
            </span>
            <button
              ref={settingsButtonRef}
              className="btn-settings"
              onClick={handleOpenSettings}
              aria-haspopup="dialog"
              aria-expanded={showSettings}
              aria-controls="settings-dialog"
            >
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7ZM19 12l2-1-2-3-2 .5a8 8 0 0 0-2-1.2L14.5 5h-5L9 7.3a8 8 0 0 0-2 1.2L5 8l-2 3 2 1a8 8 0 0 0 0 2l-2 1 2 3 2-.5a8 8 0 0 0 2 1.2l.5 2.3h5l.5-2.3a8 8 0 0 0 2-1.2l2 .5 2-3-2-1a8 8 0 0 0 0-2Z" /></svg>
              <span>{t.settingsTitle}</span>
            </button>
          </div>
        </aside>

        <main id="main-content" className="main-content" tabIndex={-1}>
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
                {inputChoice === "home" && (
                  <>
                  <div className="input-choice-grid">
                    <button className="input-choice-card primary" onClick={() => setInputChoice("record")} disabled={isRunning}>
                      <span className="input-choice-icon record-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><rect x="9" y="3" width="6" height="11" rx="3" /><path d="M6.5 11.5a5.5 5.5 0 0 0 11 0M12 17v4M9 21h6" /></svg></span>
                      <strong>{locale === "zh" ? "开始录音" : "Start recording"}</strong>
                      <small>{locale === "zh" ? "使用麦克风采集语音，停止后再确认处理。" : "Capture voice with your microphone, then choose how to process it."}</small>
                    </button>
                    <button className="input-choice-card" onClick={() => setInputChoice("import")} disabled={isRunning}>
                      <span className="input-choice-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M12 16V4m0 0L7.5 8.5M12 4l4.5 4.5M5 14v5h14v-5" /></svg></span>
                      <strong>{locale === "zh" ? "导入音频" : "Import audio"}</strong>
                      <small>{locale === "zh" ? "处理已有的 MP3、WAV 或 M4A 文件。" : "Process an existing MP3, WAV, or M4A file."}</small>
                    </button>
                  </div>
                  {recoverableRecordings.length > 0 && <div className="recovery-list">
                    <strong>{locale === "zh" ? "已保存与中断的录音" : "Saved and interrupted recordings"}</strong>
                    {recoverableRecordings.slice(0, 5).map((task) => <div key={task.task_id}><span>{new Date(task.task_id.slice(0, 8).replace(/(\d{4})(\d{2})(\d{2})/, "$1-$2-$3")).toLocaleDateString()} · {task.status === "interrupted" ? (locale === "zh" ? "待恢复" : "Interrupted") : `${Math.round(task.duration_ms / 1000)}s`}</span>{task.status === "interrupted" ? <button className="btn-secondary" onClick={() => void handleRecoverRecording(task)} disabled={recordingRecoveryPending !== null || task.checkpoint_count === 0}>{recordingRecoveryPending === task.task_id ? (locale === "zh" ? "恢复中…" : "Recovering…") : (locale === "zh" ? "恢复录音" : "Recover")}</button> : <button className="btn-secondary" onClick={() => { setInputChoice("record"); handleRecordingSaved(task); }}>{locale === "zh" ? "继续处理" : "Continue"}</button>}</div>)}
                  </div>}
                  </>
                )}
                {inputChoice === "record" && (
                  <>
                    <button className="link-button input-back" onClick={() => setInputChoice("home")} disabled={isRunning}>{locale === "zh" ? "← 返回输入方式" : "← Back to input methods"}</button>
                    <RecordingWorkspace locale={locale} disabled={isRunning} onSaved={handleRecordingSaved} onError={handleRecordingError} />
                    {recordingTask?.audio_path && <div className="recording-saved-banner"><strong>{locale === "zh" ? "录音已保存" : "Recording saved"}</strong><span>{Math.round(recordingTask.duration_ms / 1000)}s · {recordingTask.checkpoint_count} {locale === "zh" ? "个安全分段" : "checkpoints"}</span></div>}
                  </>
                )}
                {inputChoice === "import" && (
                  <>
                    <button className="link-button input-back" onClick={() => setInputChoice("home")} disabled={isRunning}>{locale === "zh" ? "← 返回输入方式" : "← Back to input methods"}</button>
                    <FilePicker
                      audioPath={audioPath}
                      onFileSelected={(path) => { setRecordingTask(null); setAudioPath(path); }}
                      disabled={isRunning}
                      labels={{ chooseFile: t.chooseFile, noFileSelected: t.noFileSelected, audioFile: t.audioFile }}
                    />
                  </>
                )}
              </div>

              {audioPath && <>
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
                    builtInTemplates: t.builtInTemplates,
                    myTemplates: t.myTemplates,
                  }}
                />
				<p className="privacy-summary">{privacySummary}</p>
              </div>
              </>}
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

          <div
            id="models-panel"
            role="tabpanel"
            aria-labelledby="models-tab"
            hidden={activeArea !== "models"}
          >
            <section className="management-page">
              <header className="management-page-heading">
                <span className="section-kicker">{locale === "zh" ? "本地运行时" : "Local runtime"}</span>
                <h2>{locale === "zh" ? "模型" : "Models"}</h2>
                <p>{locale === "zh" ? "管理 Voxly 使用的本地处理模型与运行资产。" : "Manage local processing models and runtime assets used by Voxly."}</p>
              </header>
              <div className="management-surface"><LocalModelManager locale={locale} disabled={isRunning} onStatusChanged={() => void refreshPreflight()} labels={{ title: t.localModelTitle, description: t.localModelDescription, chineseFirst: t.localChineseFirst, status: t.localModelStatus, notInstalled: t.localNotInstalled, runtimeMissing: t.localRuntimeMissing, invalid: t.localModelInvalid, unsupported: t.localUnsupported, download: t.localDownload, import: t.localImport, cancel: t.localCancel, delete: t.localDelete, source: t.localSource, size: t.localSize, advanced: t.localAdvanced, endpoint: t.localEndpoint, save: t.save, test: t.localTestEndpoint, endpointReachable: t.localEndpointReachable, operationFailed: t.localOperationFailed, confirmDelete: t.localConfirmDelete, privacyTitle: t.localPrivacyTitle, privacyBody: t.localPrivacyBody, technicalDetails: t.technicalDetails, endpointScope: t.localEndpointScope }} /></div>
            </section>
          </div>

          <div
            id="templates-panel"
            role="tabpanel"
            aria-labelledby="templates-tab"
            hidden={activeArea !== "templates"}
          >
            <section className="management-page">
              <header className="management-page-heading">
                <span className="section-kicker">{locale === "zh" ? "内容生成" : "Content generation"}</span>
                <h2>{t.settingsTemplates}</h2>
                <p>{t.settingsTemplatesDesc}</p>
              </header>
              {templateIssues.map((issue) => <p className="template-manager-error" key={issue}>{issue}</p>)}
              <div className="management-surface"><TemplateManager templates={templates} locale={locale} disabled={isRunning} onChanged={refreshTemplates} labels={{ builtInTemplates: t.builtInTemplates, myTemplates: t.myTemplates, newTemplate: t.newTemplate, editTemplate: t.editTemplate, deleteTemplate: t.deleteTemplate, templateName: t.templateName, templateDescription: t.templateDescription, templateInstructions: t.templateInstructions, technicalDetails: t.technicalDetails, revision: t.revision, saveCreatesRevision: t.saveCreatesRevision, confirmDeleteTemplate: t.confirmDeleteTemplate, templateSaveFailed: t.templateSaveFailed, noCustomTemplates: t.noCustomTemplates, save: t.save, cancel: t.cancel }} /></div>
            </section>
          </div>
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

              <section className="settings-section about-section">
                <div className="settings-section-header">
                  <div className="settings-section-title">
                    <span aria-hidden="true">V</span>
                    <div>
                      <h3>{t.aboutTitle}</h3>
                      <p>{t.aboutDescription}</p>
                    </div>
                  </div>
                </div>
                <div className="about-card">
                  <strong>{t.aboutProductNames}</strong>
                  <span>{t.aboutSubtitles}</span>
                  <span>{t.aboutVersion}</span>
                </div>
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
                    <p className={asrPreflightMatchesSelection && preflight?.asr_provider.ready ? "provider-readiness ready" : "provider-readiness not-ready"}>{asrPreflightMatchesSelection && preflight?.asr_provider.ready ? t.ready : t.notReady}</p>
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
