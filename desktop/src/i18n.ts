export type Locale = "zh" | "en";

export interface Translations {
  appTitle: string;
  appSubtitle: string;
  appKicker: string;
  workflowEyebrow: string;
  workflowTitle: string;
  workflowDescription: string;
  inputTitle: string;
  outputModeTitle: string;
  chooseFile: string;
  noFileSelected: string;
  audioFile: string;
  modeTranscript: string;
  modeTranscriptDesc: string;
  modeNotes: string;
  modeNotesDesc: string;
  modeMinutes: string;
  modeMinutesDesc: string;
  start: string;
  processing: string;
  openOutputFolder: string;
  splitting: string;
  transcribing: string;
  merging: string;
  preparingNotes: string;
  generatingMinutes: string;
  done: string;
  resultsTitle: string;
  resultsSummary: string;
  primaryOutput: string;
  generatedFiles: string;
  taskFolder: string;
  openTaskFolder: string;
  advancedDetails: string;
  folder: string;
  transcript: string;
  notesInput: string;
  meetingMinutes: string;
  completedSuccessfully: string;
  exitedWithCode: string;
  terminatedBySignal: string;
  transcriptWarning: string;
  notesWarning: string;
  llmWarning: string;
  backend: string;
  backendPython: string;
  backendGoExperimental: string;
  backendGoProduction: string;
  settingsTitle: string;
  language: string;
  mimoApiKey: string;
  deepseekApiKey: string;
  deepseekModel: string;
  save: string;
  cancel: string;
  lightMode: string;
  darkMode: string;
  errorTitle: string;
  close: string;
  showLogs: string;
  hideLogs: string;
  keysNotPersisted: string;
  saveToEnv: string;
  productionCredentialsNote: string;
  developmentCredentialPlaceholder: string;
  productionCredentialPlaceholder: string;
  savedSuccessfully: string;
  saveFailed: string;
  readinessTitle: string;
  readinessLocalOnly: string;
  refresh: string;
  checking: string;
  ready: string;
  notReady: string;
  runtimeMode: string;
  development: string;
  production: string;
  sidecar: string;
  outputRoot: string;
  missing: string;
  session: string;
  secureStore: string;
  developmentEnv: string;
  preflightUnavailable: string;
  preflightPrompt: string;
  chooseFileToBegin: string;
  chooseFileToBeginDesc: string;
  readyToRun: string;
  readyToRunDesc: string;
  settingsIntro: string;
  settingsReadiness: string;
  settingsReadinessDesc: string;
  settingsAppearance: string;
  settingsAppearanceDesc: string;
  settingsBackend: string;
  settingsBackendDesc: string;
  settingsCredentials: string;
  settingsCredentialsDesc: string;
  settingsMinutesModel: string;
  settingsMinutesModelDesc: string;
  settingsOutputLocation: string;
  settingsOutputLocationDesc: string;
  theme: string;
  themeSystem: string;
  themeLight: string;
  themeDark: string;
}

export const translations: Record<Locale, Translations> = {
  zh: {
    appTitle: "Meeting ASR Desktop",
    appSubtitle: "用于转写、笔记准备和 AI 会议纪要的实验性桌面工作流",
    appKicker: "本地会议工作台",
    workflowEyebrow: "新建任务",
    workflowTitle: "处理会议录音",
    workflowDescription: "选择一段音频和需要的输出，其余文件会整理到同一个任务文件夹。",
    inputTitle: "音频来源",
    outputModeTitle: "输出类型",
    chooseFile: "选择音频文件",
    noFileSelected: "未选择文件",
    audioFile: "音频文件",
    modeTranscript: "转写",
    modeTranscriptDesc: "音频 → 转写文本",
    modeNotes: "笔记准备",
    modeNotesDesc: "转写 + notes_input.md",
    modeMinutes: "会议纪要",
    modeMinutesDesc: "转写 + AI 纪要",
    start: "开始",
    processing: "处理中...",
    openOutputFolder: "打开任务文件夹",
    splitting: "分割音频...",
    transcribing: "转写中...",
    merging: "合并转写...",
    preparingNotes: "准备笔记...",
    generatingMinutes: "生成会议纪要...",
    done: "完成",
    resultsTitle: "结果",
    resultsSummary: "你的文件已准备就绪",
    primaryOutput: "主要输出",
    generatedFiles: "同时生成的文件",
    taskFolder: "任务文件夹",
    openTaskFolder: "打开任务文件夹",
    advancedDetails: "任务文件夹详情",
    folder: "任务文件夹",
    transcript: "转写文本",
    notesInput: "笔记准备文件",
    meetingMinutes: "会议纪要",
    completedSuccessfully: "处理完成",
    exitedWithCode: "退出码",
    terminatedBySignal: "被信号终止",
    transcriptWarning:
      "transcript 模式会将音频发送至 MiMo ASR 进行转写。请勿上传未授权的敏感会议内容。\n\nTranscript mode sends audio to MiMo ASR for transcription.",
    notesWarning:
      "notes 模式会将音频发送至 MiMo ASR 转写，并在本地生成 notes_input.md；不会将转写文本发送至 DeepSeek。\n\nNotes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning:
      "minutes 模式会将转写文本发送至 DeepSeek 生成会议总结。请勿上传未授权的敏感会议内容。\n\nMinutes mode sends transcript text to DeepSeek for summary generation.",
    backend: "处理后端",
    backendPython: "Python（开发模式默认）",
    backendGoExperimental: "Go（开发模式实验性）",
    backendGoProduction: "Go（生产模式内置）",
    settingsTitle: "设置",
    language: "语言",
    mimoApiKey: "MIMO API Key",
    deepseekApiKey: "DeepSeek API Key",
    deepseekModel: "DeepSeek 模型",
    save: "保存",
    cancel: "取消",
    lightMode: "浅色模式",
    darkMode: "深色模式",
    errorTitle: "错误",
    close: "关闭",
    showLogs: "显示日志",
    hideLogs: "隐藏日志",
    keysNotPersisted: "此处输入的 Key 仅在本次会话有效。开发模式可保存到项目根 .env，也可使用已有安全存储凭据。",
    saveToEnv: "保存到 .env",
    productionCredentialsNote: "生产模式不读取项目根 .env；请使用本次会话 Key 或安全存储。",
    developmentCredentialPlaceholder: "可选会话 Key；留空则使用安全存储或开发 .env",
    productionCredentialPlaceholder: "可选会话 Key；留空则使用安全存储",
    savedSuccessfully: "已保存",
    saveFailed: "保存失败",
    readinessTitle: "本机运行就绪状态",
    readinessLocalOnly: "仅检查本机配置，不调用 MiMo 或 DeepSeek",
    refresh: "刷新",
    checking: "检查中…",
    ready: "就绪",
    notReady: "未就绪",
    runtimeMode: "运行模式",
    development: "开发",
    production: "生产",
    sidecar: "Go 后端可执行文件",
    outputRoot: "输出位置",
    missing: "未配置",
    session: "本次会话",
    secureStore: "安全存储",
    developmentEnv: "开发 .env",
    preflightUnavailable: "运行就绪检查暂不可用。",
    preflightPrompt: "刷新后查看本机运行环境与配置状态。",
    chooseFileToBegin: "从一段会议录音开始",
    chooseFileToBeginDesc: "选择本机音频文件，再选择转写、笔记准备或会议纪要。",
    readyToRun: "已准备好开始",
    readyToRunDesc: "音频已选择。确认输出类型后即可开始处理。",
    settingsIntro: "检查本机运行状态，并调整界面、处理后端与会话配置。",
    settingsReadiness: "本机就绪状态",
    settingsReadinessDesc: "检查运行模式、后端和系统依赖，不发起在线请求。",
    settingsAppearance: "外观与语言",
    settingsAppearanceDesc: "调整界面语言和主题偏好。",
    settingsBackend: "处理后端",
    settingsBackendDesc: "生产模式固定使用内置 Go；开发模式可选择 Python 或实验性 Go。",
    settingsCredentials: "凭据",
    settingsCredentialsDesc: "会话 Key 不会显示在状态信息、日志或输出文件中。",
    settingsMinutesModel: "会议纪要模型",
    settingsMinutesModelDesc: "DeepSeek 模型仅用于会议纪要输出。",
    settingsOutputLocation: "输出位置",
    settingsOutputLocationDesc: "所有任务文件都会保存在已配置输出位置之内。",
    theme: "主题",
    themeSystem: "跟随系统",
    themeLight: "浅色",
    themeDark: "深色",
  },
  en: {
    appTitle: "Meeting ASR Desktop",
    appSubtitle: "Experimental desktop workflow for transcripts, notes preparation, and AI meeting minutes",
    appKicker: "Local meeting workspace",
    workflowEyebrow: "New task",
    workflowTitle: "Process a meeting recording",
    workflowDescription: "Choose an audio file and the output you need. Generated files stay together in one task folder.",
    inputTitle: "Audio source",
    outputModeTitle: "Output type",
    chooseFile: "Choose Audio File",
    noFileSelected: "No file selected",
    audioFile: "Audio file",
    modeTranscript: "Transcript",
    modeTranscriptDesc: "Audio → transcript",
    modeNotes: "Notes preparation",
    modeNotesDesc: "Transcript + notes_input.md",
    modeMinutes: "Meeting minutes",
    modeMinutesDesc: "Transcript + AI minutes",
    start: "Start",
    processing: "Processing...",
    openOutputFolder: "Open Task Folder",
    splitting: "Splitting audio...",
    transcribing: "Transcribing...",
    merging: "Merging transcript...",
    preparingNotes: "Preparing notes...",
    generatingMinutes: "Generating minutes...",
    done: "Done",
    resultsTitle: "Results",
    resultsSummary: "Your files are ready",
    primaryOutput: "Primary output",
    generatedFiles: "Also generated",
    taskFolder: "Task folder",
    openTaskFolder: "Open Task Folder",
    advancedDetails: "Task folder details",
    folder: "Task folder",
    transcript: "Transcript",
    notesInput: "Notes preparation",
    meetingMinutes: "Meeting minutes",
    completedSuccessfully: "Completed successfully",
    exitedWithCode: "Exited with code",
    terminatedBySignal: "Terminated by signal",
    transcriptWarning:
      "Transcript mode sends audio to MiMo ASR for transcription. Do not upload unauthorized sensitive meeting content.",
    notesWarning:
      "Notes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning:
      "Minutes mode sends transcript text to DeepSeek for summary generation. Do not upload unauthorized sensitive meeting content.",
    backend: "Processing backend",
    backendPython: "Python (development default)",
    backendGoExperimental: "Go (experimental in development)",
    backendGoProduction: "Go (packaged for production)",
    settingsTitle: "Settings",
    language: "Language",
    mimoApiKey: "MIMO API Key",
    deepseekApiKey: "DeepSeek API Key",
    deepseekModel: "DeepSeek Model",
    save: "Save",
    cancel: "Cancel",
    lightMode: "Light Mode",
    darkMode: "Dark Mode",
    errorTitle: "Error",
    close: "Close",
    showLogs: "Show Logs",
    hideLogs: "Hide Logs",
    keysNotPersisted: "Keys entered here are session-only. Development can save to the project-root .env or use an existing secure-store credential.",
    saveToEnv: "Save to .env",
    productionCredentialsNote: "Production does not read the project-root .env; use session keys or secure storage.",
    developmentCredentialPlaceholder: "Optional session key; leave blank for secure store or development .env",
    productionCredentialPlaceholder: "Optional session key; leave blank for secure storage",
    savedSuccessfully: "Saved",
    saveFailed: "Save failed",
    readinessTitle: "Local runtime readiness",
    readinessLocalOnly: "Checks local configuration only; does not call MiMo or DeepSeek",
    refresh: "Refresh",
    checking: "Checking…",
    ready: "Ready",
    notReady: "Not ready",
    runtimeMode: "Runtime mode",
    development: "Development",
    production: "Production",
    sidecar: "Go backend executable",
    outputRoot: "Output location",
    missing: "Missing",
    session: "Session",
    secureStore: "Secure store",
    developmentEnv: "Development .env",
    preflightUnavailable: "Runtime readiness check is unavailable.",
    preflightPrompt: "Refresh to inspect this Mac's runtime and configuration status.",
    chooseFileToBegin: "Start with a meeting recording",
    chooseFileToBeginDesc: "Choose a local audio file, then select transcript, notes preparation, or meeting minutes.",
    readyToRun: "Ready to begin",
    readyToRunDesc: "Your audio is selected. Confirm the output type, then start processing.",
    settingsIntro: "Inspect local readiness and adjust appearance, processing, and session configuration.",
    settingsReadiness: "Local readiness",
    settingsReadinessDesc: "Checks runtime policy, backend, and system dependencies without making online requests.",
    settingsAppearance: "Appearance and language",
    settingsAppearanceDesc: "Choose the interface language and theme preference.",
    settingsBackend: "Processing backend",
    settingsBackendDesc: "Production is fixed to packaged Go; development can use Python or experimental Go.",
    settingsCredentials: "Credentials",
    settingsCredentialsDesc: "Session keys are never shown in status metadata, logs, or output files.",
    settingsMinutesModel: "Minutes model",
    settingsMinutesModelDesc: "The DeepSeek model is used only for meeting-minutes output.",
    settingsOutputLocation: "Output location",
    settingsOutputLocationDesc: "Every task folder and generated file stays inside the configured output location.",
    theme: "Theme",
    themeSystem: "System",
    themeLight: "Light",
    themeDark: "Dark",
  },
};

export function getLocale(): Locale {
  const saved = localStorage.getItem("locale");
  if (saved === "zh" || saved === "en") return saved;
  const lang = navigator.language;
  return lang.startsWith("zh") ? "zh" : "en";
}

export function setLocale(locale: Locale) {
  localStorage.setItem("locale", locale);
}
