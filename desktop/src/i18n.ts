export type Locale = "zh" | "en";

export interface Translations {
  cacheScope: string;
  cacheChunksOnly: string;
  cacheTextOnly: string;
  cacheTextOnlyDescription: string;
  cacheTextOnlyConfirm: string;
  cacheTextOnlyCleaned: string;
  cacheNoAudio: string;

  settingsNavigation: string;
  settingsCategories: Record<"general" | "processing" | "connections" | "templates" | "storage" | "about", string>;
  localVadEnabled: string;
  localVadHelp: string;
  artifactTimedTranscript: string;
  artifactVADSegments: string;
  localAsrLanguage: string;
  localAsrChinese: string;
  localAsrAuto: string;
  localAsrLanguageHelp: string;
  privacyLoading: string;
  privacyLocalTranscript: string;
  privacyCloudTranscript: string;
  privacyLocalCloudGeneration: string;
  privacyCloudGeneration: string;
  privacyLocalGeneration: string;
  privacyCloudLocalGeneration: string;
  localModelStates: Record<string, string>;
  appTitle: string;
  appSubtitle: string;
  appKicker: string;
  aboutTitle: string;
  aboutProductNames: string;
  aboutSubtitles: string;
  aboutVersion: string;
  aboutDescription: string;
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
  modeGenerate: string;
  modeGenerateDesc: string;
  templateLabel: string;
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
  currentOutputFolder: string;
  changeOutputFolder: string;
  openOutputFolderAction: string;
  resetOutputFolder: string;
  outputFolderChanged: string;
  outputFolderReset: string;
  outputFolderMoveWarning: string;
  outputFolderChangeBlocked: string;
  outputFolderUnavailable: string;
  outputFolderNotWritable: string;
  chooseOutputFolder: string;
  theme: string;
  themeSystem: string;
  themeLight: string;
  themeDark: string;
  workspaceNavigation: string;
  newTask: string;
  taskLibraryTitle: string;
  taskLibraryDescription: string;
  refreshing: string;
  taskLibraryLoading: string;
  taskLibraryEmpty: string;
  taskLibraryEmptyDescription: string;
  taskLibraryLoadError: string;
  retry: string;
  partialErrors: string;
  discoveryIssues: string;
  updatedAt: string;
  createdAt: string;
  outputMode: string;
  outputs: string;
  tags: string;
  editMetadata: string;
  taskTitle: string;
  taskDescription: string;
  descriptionPlaceholder: string;
  addTag: string;
  tagPlaceholder: string;
  removeTag: string;
  metadataSaveFailed: string;
  sortBy: string;
  sortUpdated: string;
  sortCreated: string;
  sortTitle: string;
  filterByTag: string;
  allTasks: string;
  taskDetails: string;
  storageTitle: string;
  storageDescription: string;
  storageTotal: string;
  storageSource: string;
  storageChunks: string;
  storageTranscripts: string;
  storageGenerated: string;
  storageModels: string;
  storageTemporary: string;
  storageOther: string;
  storageRefresh: string;
  storageEstimate: string;
  storageIssues: string;
  storageAutoChunks: string;
  storageAutoChunksHelp: string;
  storageAutoTemporary: string;
  storageAutoTemporaryHelp: string;
  storageCleanTemporary: string;
  storageTemporaryResult: string;
  storageBatchPreview: string;
  storageBatchConfirm: string;
  storageBatchHelp: string;
  storageSelectAll: string;
  storageTask: string;
  storageSize: string;
  storageCleanedCount: string;
  storageSkippedCount: string;
  storageFailedCount: string;
  storageNoTasks: string;
  storageSaved: string;
  cacheCleanup: string;
  cachePreview: string;
  cacheDescription: string;
  cacheConfirm: string;
  cacheWorking: string;
  cacheReclaimable: string;
  cacheCleaned: string;
  cacheNoFiles: string;
  cacheNotCompleted: string;
  cacheBusy: string;
  cacheSourceUnavailable: string;
  cacheTranscriptUnverified: string;
  cacheUnsafe: string;
  cacheChanged: string;
  cacheFailed: string;
  openTaskDetails: string;
  backToTaskLibrary: string;
  transcriptSummary: string;
  generatedArtifactCount: string;
  saveTitle: string;
  cancelTitleEdit: string;
  taskInformation: string;
  existingTags: string;
  available: string;
  availablePartial: string;
  unavailable: string;
  artifactError: string;
  generationIncomplete: string;
  generationRefused: string;
  generationUnverified: string;
  generationEmpty: string;
  generationCancelled: string;
  retryable: string;
  notRetryable: string;
  invalidDate: string;
  statusRunning: string;
  statusCompleted: string;
  statusCompletedWithErrors: string;
  statusFailed: string;
  statusInterrupted: string;
  statusCancelled: string;
  statusPending: string;
  statusProcessing: string;
  statusUnavailable: string;
  statusUnknown: string;
  artifactTranscript: string;
  artifactNotes: string;
  artifactMinutes: string;
  openArtifact: string;
  revealArtifact: string;
  openingArtifact: string;
  revealingArtifact: string;
  artifactActionFailed: string;
  refreshLibraryHint: string;
  canonicalTranscript: string;
  generatedContent: string;
  generationHistory: string;
  generateContent: string;
  generateAgain: string;
  generatingContent: string;
  generationFailed: string;
  latestVersion: string;
  legacyArtifact: string;
  settingsProviders: string;
	localModelTitle: string; localModelDescription: string; localChineseFirst: string; localModelStatus: string;
	localNotInstalled: string; localRuntimeMissing: string; localModelInvalid: string; localUnsupported: string;
	localDownload: string; localImport: string; localCancel: string; localDelete: string; localSource: string; localSize: string;
	localAdvanced: string; localEndpoint: string; localTestEndpoint: string; localEndpointReachable: string;
	localOperationFailed: string; localConfirmDelete: string; localPrivacyTitle: string; localPrivacyBody: string;
	localEndpointScope: string;
  settingsProvidersDesc: string;
  settingsTemplates: string;
  settingsTemplatesDesc: string;
  builtInTemplates: string;
  myTemplates: string;
  newTemplate: string;
  editTemplate: string;
  deleteTemplate: string;
  templateName: string;
  templateDescription: string;
  templateInstructions: string;
  technicalDetails: string;
  revision: string;
  saveCreatesRevision: string;
  confirmDeleteTemplate: string;
  templateSaveFailed: string;
  noCustomTemplates: string;
  speechRecognition: string;
  generatedContentProvider: string;
  provider: string;
  model: string;
  speechRecognitionProvider: string;
  generatedWithProvider: string;
  providerUnknown: string;
  providerNotReady: string;
  openSettings: string;
  resumeTask: string;
  resumeCancelled: string;
  resumingTask: string;
  stopTask: string;
  stoppingTask: string;
  pendingChunksHint: string;
  blockedTaskLocked: string;
  blockedSpeechProviderUnsupported: string;
  blockedSpeechProviderUnavailable: string;
  blockedSourceAudioMissing: string;
  retryGeneration: string;
  retryingGeneration: string;
  retryKeepsIdentity: string;
  newGenerationKeepsHistory: string;
  uploadsAudioHint: string;
  uploadsTextHint: string;
}

export const translations: Record<Locale, Translations> = {
  zh: {
    localVadEnabled: "按讲话停顿分段（VAD）",
    localVadHelp: "本地检测讲话区间，输出带时间的转写。长段仍会切分；时间与文字在清理音频缓存后保留。保存后用于新任务。",
    artifactTimedTranscript: "带时间转写",
    artifactVADSegments: "语音分段数据",
    localAsrLanguage: "识别语言",
    localAsrChinese: "中文（默认）",
    localAsrAuto: "自动判断",
    localAsrLanguageHelp: "中文访谈建议选中文；多语种录音可选自动判断，短句可能误判。保存后用于后续本地转写。",
    privacyLoading: "正在读取语音识别配置…",
    privacyLocalTranscript: "音频和转写文本均保存在本机，无需 API Key。",
    privacyCloudTranscript: "音频将发送到所选云端语音识别服务；不会调用内容生成模型。",
    privacyLocalCloudGeneration: "音频在本机转写；转写文本将发送到所选云端服务生成内容。",
    privacyCloudGeneration: "音频将发送到云端转写，转写文本将发送到云端生成内容。",
    privacyLocalGeneration: "转写和内容生成均在本机完成，音频与文本不会上传。",
    privacyCloudLocalGeneration: "音频将发送到云端转写，后续内容生成在本机完成。",
    localModelStates: {
      ready: "已就绪，可离线转写", not_installed: "尚未安装模型",
      runtime_missing: "模型已安装，但本地识别组件缺失或版本不兼容",
      model_invalid: "模型校验未通过，请重新安装", unsupported_platform: "当前系统暂不支持此模型",
      downloading: "正在下载模型…", verifying: "正在校验模型…", installing: "正在安装模型…",
      storage_unavailable: "无法访问模型存储位置，请检查目录权限", error: "模型状态检查失败，请重试",
    },
    appTitle: "声笺",
    appSubtitle: "本地语音工作台",
    appKicker: "Voxly",
    aboutTitle: "关于声笺",
    aboutProductNames: "Voxly · 声笺",
    aboutSubtitles: "Local Voice Workspace · 本地语音工作台",
    aboutVersion: "版本 0.1.0-alpha",
    aboutDescription: "将语音转写为可整理、可生成、可追溯的内容。",
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
    modeGenerate: "转写 + 生成内容",
    modeGenerateDesc: "先完成转写，再用内置模板生成内容",
    templateLabel: "内容模板",
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
    transcriptWarning: "音频会发送至 MiMo ASR 进行转写。不会调用内容生成模型。",
    notesWarning:
      "notes 模式会将音频发送至 MiMo ASR 转写，并在本地生成 notes_input.md；不会将转写文本发送至 DeepSeek。\n\nNotes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning: "音频将发送至 MiMo ASR，完成后的转写文本将发送至 DeepSeek，并按所选模板生成内容。",
    backend: "处理后端",
    backendGoExperimental: "Go（开发模式）",
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
    chooseFileToBeginDesc: "选择本机音频文件，直接转写，或按模板生成摘要、纪要等内容。",
    readyToRun: "已准备好开始",
    readyToRunDesc: "音频已选择。确认输出类型后即可开始处理。",
    settingsNavigation: "设置分类",
    settingsCategories: { general: "通用", processing: "识别与生成", connections: "模型与连接", templates: "内容模板", storage: "存储", about: "关于与诊断" },
    settingsIntro: "按分类调整工作台，管理识别、模型与本机文件。",
    settingsReadiness: "本机就绪状态",
    settingsReadinessDesc: "检查运行模式、后端和系统依赖，不发起在线请求。",
    settingsAppearance: "外观与语言",
    settingsAppearanceDesc: "调整界面语言和主题偏好。",
    settingsBackend: "处理后端",
    settingsBackendDesc: "桌面版使用 Go 后端；正式安装包已内置运行时。",
    settingsCredentials: "凭据",
    settingsCredentialsDesc: "会话 Key 不会显示在状态信息、日志或输出文件中。",
    settingsMinutesModel: "会议纪要模型",
    settingsMinutesModelDesc: "DeepSeek 模型仅用于会议纪要输出。",
    settingsOutputLocation: "输出位置",
    settingsOutputLocationDesc: "所有任务文件都会保存在已配置输出位置之内。",
    currentOutputFolder: "当前文件夹",
    changeOutputFolder: "更改文件夹…",
    openOutputFolderAction: "打开文件夹",
    resetOutputFolder: "恢复默认",
    outputFolderChanged: "输出文件夹已更改。",
    outputFolderReset: "输出位置已恢复为默认设置。",
    outputFolderMoveWarning: "更改输出文件夹不会移动现有任务。",
    outputFolderChangeBlocked: "处理任务时无法更改输出文件夹。",
    outputFolderUnavailable: "输出文件夹不可用。",
    outputFolderNotWritable: "输出文件夹不可写。",
    chooseOutputFolder: "选择输出文件夹",
    theme: "主题",
    themeSystem: "跟随系统",
    themeLight: "浅色",
    themeDark: "深色",
    workspaceNavigation: "工作区导航",
    newTask: "新建任务",
    taskLibraryTitle: "任务库",
    taskLibraryDescription: "查看最近处理的任务、状态和主要输出。",
    refreshing: "刷新中…",
    taskLibraryLoading: "正在载入任务库…",
    taskLibraryEmpty: "还没有任务",
    taskLibraryEmptyDescription: "完成音频处理后，任务会显示在这里。",
    taskLibraryLoadError: "暂时无法载入任务库。",
    retry: "重试",
    partialErrors: "部分输出有问题",
    discoveryIssues: "部分任务无法读取",
    updatedAt: "更新",
    createdAt: "创建",
    outputMode: "输出模式",
    outputs: "主要输出",
    tags: "标签",
    editMetadata: "编辑任务信息",
    taskTitle: "任务名称",
    taskDescription: "描述",
    descriptionPlaceholder: "添加任务背景、重点或备注（可选）",
    addTag: "添加",
    tagPlaceholder: "输入标签",
    removeTag: "移除标签",
    metadataSaveFailed: "无法保存任务信息，请重试。",
    sortBy: "排序",
    sortUpdated: "最近更新",
    sortCreated: "最近创建",
    sortTitle: "名称",
    filterByTag: "标签筛选",
    allTasks: "全部任务",
    taskDetails: "任务详情",
    storageTitle: "存储管理",
    storageDescription: "查看任务与模型占用，集中回收可再生成的处理缓存。",
    storageTotal: "合计",
    storageSource: "原音频",
    storageChunks: "音频切片缓存",
    storageTranscripts: "转写文件",
    storageGenerated: "生成内容",
    storageModels: "模型",
    storageTemporary: "下载与解压临时文件",
    storageOther: "其他及任务信息",
    storageRefresh: "刷新存储统计",
    storageEstimate: "按当前输出目录和应用数据目录中的文件大小估算；实际可用空间受文件系统影响。",
    storageIssues: "未跟随链接、无法读取或格式异常的项目",
    storageAutoChunks: "任务成功完成后自动清理音频切片",
    storageAutoChunksHelp: "默认关闭。只用于此后新完成或继续完成的任务；失败或中断任务保留切片，不会追溯清理旧任务。",
    storageAutoTemporary: "启动时回收过期临时文件",
    storageAutoTemporaryHelp: "仅处理模型管理器生成、超过 7 天且未被占用的下载和解压残留；保留已安装模型与未知文件。",
    storageCleanTemporary: "检查并清理过期临时文件",
    storageTemporaryResult: "本次临时文件回收",
    storageBatchPreview: "预览批量缓存清理",
    storageBatchConfirm: "确认清理选中任务",
    storageBatchHelp: "逐任务检查并选择，保留原音频和文本。任务或文件发生变化时跳过该任务，其余任务继续执行。",
    storageSelectAll: "全选可清理任务",
    storageTask: "任务",
    storageSize: "任务占用",
    storageCleanedCount: "已清理",
    storageSkippedCount: "已跳过",
    storageFailedCount: "未完成",
    storageNoTasks: "当前输出位置没有可检查的任务。",
    storageSaved: "清理偏好已保存",
    cacheScope: "清理范围",
    cacheChunksOnly: "仅清理音频切片",
    cacheTextOnly: "仅保留文本（删除任务内音频）",
    cacheTextOnlyDescription: "删除本任务的录音副本和音频切片，保留全文、时间转写、分段数据和所有生成版本。不删除导入的外部原文件。此操作无法撤销；删除后无法用本任务音频回听或重新识别，请先检查转写结果。",
    cacheTextOnlyConfirm: "确认删除任务内音频，仅保留文本",
    cacheTextOnlyCleaned: "任务内音频已删除，文本和时间结果已保留",
    cacheNoAudio: "任务内已没有可清理的音频",
    cacheCleanup: "音频与缓存",
    cachePreview: "检查可清理空间",
    cacheDescription: "仅回收已完成任务的音频切片。保留原音频、转写和所有生成版本；再次转写时会重新生成切片。",
    cacheConfirm: "确认清理切片",
    cacheWorking: "正在检查或清理…",
    cacheReclaimable: "预计回收",
    cacheCleaned: "处理缓存已清理",
    cacheNoFiles: "没有可清理的音频切片",
    cacheNotCompleted: "任务尚未完成，保留切片以便继续处理。",
    cacheBusy: "任务正在使用中，请稍后重试。",
    cacheSourceUnavailable: "原音频缺失或不符合校验要求，暂不能清理。",
    cacheTranscriptUnverified: "完整转写未通过校验，暂不能清理。",
    cacheUnsafe: "缓存目录包含异常文件，暂不能清理。",
    cacheChanged: "任务或缓存已变化，请重新检查后再清理。",
    cacheFailed: "操作未完成，请重新检查后再试。",
    openTaskDetails: "打开任务详情",
    backToTaskLibrary: "返回任务库",
    transcriptSummary: "转写",
    generatedArtifactCount: "生成内容",
    saveTitle: "保存任务名称",
    cancelTitleEdit: "取消编辑任务名称",
    taskInformation: "任务信息",
    existingTags: "已有标签",
    available: "可用",
    availablePartial: "可查看（正文不完整）",
    unavailable: "不可用",
    artifactError: "生成出错",
    generationIncomplete: "输出被截断，正文不完整",
    generationRefused: "内容服务拒绝了本次生成",
    generationUnverified: "无法确认输出是否完整",
    generationEmpty: "内容服务未返回正文",
    generationCancelled: "本次生成已被取消",
    retryable: "可重试",
    notRetryable: "不可重试",
    invalidDate: "时间未知",
    statusRunning: "运行中",
    statusCompleted: "已完成",
    statusCompletedWithErrors: "完成但有错误",
    statusFailed: "失败",
    statusInterrupted: "已中断",
    statusCancelled: "已取消",
    statusPending: "等待中",
    statusProcessing: "处理中",
    statusUnavailable: "不可用",
    statusUnknown: "未知状态",
    artifactTranscript: "转写文本",
    artifactNotes: "笔记准备",
    artifactMinutes: "会议纪要",
    openArtifact: "打开",
    revealArtifact: "在访达中显示",
    openingArtifact: "正在打开…",
    revealingArtifact: "正在定位…",
    artifactActionFailed: "无法访问此文件。",
    refreshLibraryHint: "请刷新任务库后重试。",
    canonicalTranscript: "规范转写",
    generatedContent: "生成内容",
    generationHistory: "历史版本",
    generateContent: "生成内容",
    generateAgain: "再次生成",
    generatingContent: "生成中…",
    generationFailed: "生成失败；已有转写和历史版本保持不变。",
    latestVersion: "最新成功版本",
    legacyArtifact: "旧版输出",
    settingsProviders: "服务商与模型",
	localModelTitle: "本地语音识别", localModelDescription: "管理声笺应用数据中的官方 SenseVoice 模型。模型安装不会写入任务目录。", localChineseFirst: "中文为主", localModelStatus: "状态",
	localNotInstalled: "未安装", localRuntimeMissing: "模型已安装，但本地识别组件缺失或版本不兼容", localModelInvalid: "模型无效，请重新安装", localUnsupported: "当前平台暂不支持",
	localDownload: "下载模型", localImport: "导入官方模型", localCancel: "取消", localDelete: "删除模型", localSource: "官方来源", localSize: "下载大小",
	localAdvanced: "高级下载设置", localEndpoint: "Hugging Face Endpoint", localTestEndpoint: "测试连接", localEndpointReachable: "连接可用", localOperationFailed: "本地模型操作失败，现有模型未被修改。", localConfirmDelete: "删除已安装的 SenseVoice 模型？任务和历史记录不会被删除。", localPrivacyTitle: "隐私：", localPrivacyBody: "选择本地语音识别时，原始音频不会发送到云端。若使用云端内容生成，转写文本仍会发送给所选服务。",
	localEndpointScope: "仅适用于未来由 Hugging Face 托管且目录声明兼容的模型。当前 SenseVoice 固定从 k2-fsa GitHub Release 下载；此设置和连接测试不会改变或测试 SenseVoice 下载源。",
    settingsProvidersDesc: "选择语音识别和生成内容使用的内置服务商与模型。密钥仍单独安全保存。",
    settingsTemplates: "内容模板",
    settingsTemplatesDesc: "管理本机自定义提示词模板；内置模板保持只读。",
    builtInTemplates: "内置模板",
    myTemplates: "我的模板",
    newTemplate: "新建模板",
    editTemplate: "编辑",
    deleteTemplate: "删除",
    templateName: "名称",
    templateDescription: "描述",
    templateInstructions: "提示词指令",
    technicalDetails: "技术详情",
    revision: "修订",
    saveCreatesRevision: "保存更改不会影响之前生成的内容。",
    confirmDeleteTemplate: "删除此模板？历史生成内容会保留，但该模板将不再可选。",
    templateSaveFailed: "无法保存模板，请刷新后重试。",
    noCustomTemplates: "还没有自定义模板。",
    speechRecognition: "语音识别",
    generatedContentProvider: "生成内容",
    provider: "服务商",
    model: "模型",
    speechRecognitionProvider: "语音识别",
    generatedWithProvider: "生成服务",
    providerUnknown: "旧版 / 未知",
    providerNotReady: "当前服务配置尚未就绪。",
    openSettings: "打开设置",
    resumeTask: "继续处理",
    resumeCancelled: "本次继续处理已取消",
    resumingTask: "正在继续处理…",
    stopTask: "停止任务",
    stoppingTask: "正在停止…",
    pendingChunksHint: "待处理的音频分片",
    blockedTaskLocked: "另一个进程正在处理此任务，暂时无法继续。",
    blockedSpeechProviderUnsupported: "此任务记录的语音识别服务已不受支持，无法继续。",
    blockedSpeechProviderUnavailable: "此任务记录的语音识别运行时尚不可用，无法继续。",
    blockedSourceAudioMissing: "此任务的原始音频已丢失，无法继续分割。",
    retryGeneration: "重试此版本",
    retryingGeneration: "正在重试…",
    retryKeepsIdentity: "重试沿用原版本与服务商",
    newGenerationKeepsHistory: "将生成新版本，已有版本保持不变。",
    uploadsAudioHint: "继续处理会把音频发送到云端语音识别服务。",
    uploadsTextHint: "此操作会把转写文本发送到云端内容服务。",
  },
  en: {
    localVadEnabled: "Segment by speech pauses (VAD)",
    localVadHelp: "Detect speech locally and save timed transcripts. Long speech is still split. Text and timing survive cache cleanup. Save to apply to new tasks.",
    artifactTimedTranscript: "Timed transcript",
    artifactVADSegments: "Speech segment data",
    localAsrLanguage: "Recognition language",
    localAsrChinese: "Chinese (default)",
    localAsrAuto: "Detect automatically",
    localAsrLanguageHelp: "Use Chinese for Chinese interviews. Auto detection supports multilingual recordings but may misidentify short phrases. Save to apply to subsequent local runs.",
    privacyLoading: "Loading speech recognition settings…",
    privacyLocalTranscript: "Audio and transcripts stay on this device. No API key is needed.",
    privacyCloudTranscript: "Audio is sent to the selected cloud speech service. No content-generation model is called.",
    privacyLocalCloudGeneration: "Audio is transcribed on this device; the transcript is sent to the selected cloud service for content generation.",
    privacyCloudGeneration: "Audio is sent to the cloud for transcription, then the transcript is sent to the cloud for content generation.",
    privacyLocalGeneration: "Transcription and content generation run on this device. Audio and text are not uploaded.",
    privacyCloudLocalGeneration: "Audio is sent to the cloud for transcription; content generation runs on this device.",
    localModelStates: {
      ready: "Ready for offline transcription", not_installed: "Model not installed",
      runtime_missing: "Model installed; local recognition component missing or incompatible",
      model_invalid: "Model verification failed; reinstall required", unsupported_platform: "Model unsupported on this system",
      downloading: "Downloading model…", verifying: "Verifying model…", installing: "Installing model…",
      storage_unavailable: "Model storage unavailable; check directory permissions", error: "Could not check model status. Try again.",
    },
    appTitle: "Voxly",
    appSubtitle: "Local Voice Workspace",
    appKicker: "声笺",
    aboutTitle: "About Voxly",
    aboutProductNames: "Voxly · 声笺",
    aboutSubtitles: "Local Voice Workspace · 本地语音工作台",
    aboutVersion: "Version 0.1.0-alpha",
    aboutDescription: "Turn voice into structured, traceable content.",
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
    modeGenerate: "Transcript + generated content",
    modeGenerateDesc: "Complete the transcript, then generate from a built-in template",
    templateLabel: "Content template",
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
    transcriptWarning: "Audio is sent to MiMo ASR for transcription. No content-generation model is called.",
    notesWarning:
      "Notes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning: "Audio is sent to MiMo ASR. The completed transcript is then sent to DeepSeek using the selected template.",
    backend: "Processing backend",
    backendGoExperimental: "Go (development)",
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
    chooseFileToBeginDesc: "Choose an audio file to transcribe, or generate a summary, meeting minutes, or other content using a template.",
    readyToRun: "Ready to begin",
    readyToRunDesc: "Your audio is selected. Confirm the output type, then start processing.",
    settingsNavigation: "Settings categories",
    settingsCategories: { general: "General", processing: "Transcription", connections: "Models & connections", templates: "Templates", storage: "Storage", about: "About & diagnostics" },
    settingsIntro: "Manage your workspace, transcription, models, and local files.",
    settingsReadiness: "Local readiness",
    settingsReadinessDesc: "Checks runtime policy, backend, and system dependencies without making online requests.",
    settingsAppearance: "Appearance and language",
    settingsAppearanceDesc: "Choose the interface language and theme preference.",
    settingsBackend: "Processing backend",
    settingsBackendDesc: "The desktop uses Go. Release builds include the runtime.",
    settingsCredentials: "Credentials",
    settingsCredentialsDesc: "Session keys are never shown in status metadata, logs, or output files.",
    settingsMinutesModel: "Minutes model",
    settingsMinutesModelDesc: "The DeepSeek model is used only for meeting-minutes output.",
    settingsOutputLocation: "Output location",
    settingsOutputLocationDesc: "Every task folder and generated file stays inside the configured output location.",
    currentOutputFolder: "Current folder",
    changeOutputFolder: "Change Folder…",
    openOutputFolderAction: "Open Folder",
    resetOutputFolder: "Reset to Default",
    outputFolderChanged: "Output folder changed successfully.",
    outputFolderReset: "Output location reset successfully.",
    outputFolderMoveWarning: "Changing the output folder does not move existing tasks.",
    outputFolderChangeBlocked: "Cannot change output folder while processing.",
    outputFolderUnavailable: "Output folder unavailable.",
    outputFolderNotWritable: "Output folder is not writable.",
    chooseOutputFolder: "Choose output folder",
    theme: "Theme",
    themeSystem: "System",
    themeLight: "Light",
    themeDark: "Dark",
    workspaceNavigation: "Workspace navigation",
    newTask: "New task",
    taskLibraryTitle: "Task Library",
    taskLibraryDescription: "Review recent processed tasks, their status, and primary outputs.",
    refreshing: "Refreshing…",
    taskLibraryLoading: "Loading Task Library…",
    taskLibraryEmpty: "No tasks yet",
    taskLibraryEmptyDescription: "Completed audio tasks will appear here.",
    taskLibraryLoadError: "Task Library could not be loaded.",
    retry: "Retry",
    partialErrors: "Some outputs have errors",
    discoveryIssues: "Some tasks could not be read",
    updatedAt: "Updated",
    createdAt: "Created",
    outputMode: "Output mode",
    outputs: "Primary outputs",
    tags: "Tags",
    editMetadata: "Edit task details",
    taskTitle: "Task title",
    taskDescription: "Description",
    descriptionPlaceholder: "Add optional context, highlights, or notes",
    addTag: "Add",
    tagPlaceholder: "Enter a tag",
    removeTag: "Remove tag",
    metadataSaveFailed: "Task details could not be saved. Try again.",
    sortBy: "Sort by",
    sortUpdated: "Recently updated",
    sortCreated: "Recently created",
    sortTitle: "Title",
    filterByTag: "Filter by tag",
    allTasks: "All tasks",
    taskDetails: "Task details",
    storageTitle: "Storage management",
    storageDescription: "Review task and model storage, and reclaim regenerable processing cache.",
    storageTotal: "Total",
    storageSource: "Original audio",
    storageChunks: "Audio chunk cache",
    storageTranscripts: "Transcripts",
    storageGenerated: "Generated content",
    storageModels: "Models",
    storageTemporary: "Download and extraction temporary files",
    storageOther: "Other files and task metadata",
    storageRefresh: "Refresh storage usage",
    storageEstimate: "Estimated from file sizes in the current output and app data folders; actual free space depends on the filesystem.",
    storageIssues: "Unreadable, linked, overlapping or invalid entries",
    storageAutoChunks: "Automatically clean audio chunks after successful task completion",
    storageAutoChunksHelp: "Off by default. Applies to subsequent successful task completion or resume; failed and interrupted tasks keep chunks. Existing tasks are not swept.",
    storageAutoTemporary: "Reclaim expired temporary files on startup",
    storageAutoTemporaryHelp: "Only manager-owned download and extraction leftovers older than 7 days and not in use. Installed models and unknown files are kept.",
    storageCleanTemporary: "Check and clean expired temporary files",
    storageTemporaryResult: "Temporary entries reclaimed",
    storageBatchPreview: "Preview batch cache cleanup",
    storageBatchConfirm: "Confirm cleanup for selected tasks",
    storageBatchHelp: "Review and select tasks. Original audio and text are kept. Changed tasks are skipped while others continue.",
    storageSelectAll: "Select all eligible tasks",
    storageTask: "Task",
    storageSize: "Task size",
    storageCleanedCount: "Cleaned",
    storageSkippedCount: "Skipped",
    storageFailedCount: "Failed",
    storageNoTasks: "No tasks to inspect in the current output folder.",
    storageSaved: "Cleanup preferences saved",
    cacheScope: "Cleanup scope",
    cacheChunksOnly: "Audio chunks only",
    cacheTextOnly: "Keep text only (remove task audio)",
    cacheTextOnlyDescription: "Remove this task’s recording copy and audio chunks. Keep transcripts, timing data and all generated versions. The external imported file is untouched. This cannot be undone; check the transcript first, because this task will no longer contain audio for playback or retranscription.",
    cacheTextOnlyConfirm: "Confirm: remove task audio and keep text",
    cacheTextOnlyCleaned: "Task audio removed; text and timing results retained",
    cacheNoAudio: "No task audio remains to clean",
    cacheCleanup: "Audio & cache",
    cachePreview: "Check reclaimable space",
    cacheDescription: "Remove audio chunks from completed tasks. Original audio, transcripts and all generated versions are kept; retranscription recreates chunks.",
    cacheConfirm: "Confirm chunk cleanup",
    cacheWorking: "Checking or cleaning…",
    cacheReclaimable: "Estimated reclaimable space",
    cacheCleaned: "Processing cache cleaned",
    cacheNoFiles: "No audio chunks to clean",
    cacheNotCompleted: "Keep chunks until the task completes so processing can continue.",
    cacheBusy: "The task is in use. Try again later.",
    cacheSourceUnavailable: "Original audio is missing or cannot be verified. Cleanup is unavailable.",
    cacheTranscriptUnverified: "The complete transcript could not be verified. Cleanup is unavailable.",
    cacheUnsafe: "The cache contains unexpected files. Cleanup is unavailable.",
    cacheChanged: "The task or cache changed. Check again before cleaning.",
    cacheFailed: "The operation did not complete. Check again and retry.",
    openTaskDetails: "Open task details",
    backToTaskLibrary: "Back to Task Library",
    transcriptSummary: "Transcript",
    generatedArtifactCount: "Generated artifacts",
    saveTitle: "Save task title",
    cancelTitleEdit: "Cancel title editing",
    taskInformation: "Task information",
    existingTags: "Existing tags",
    available: "Available",
    availablePartial: "Viewable (incomplete text)",
    unavailable: "Unavailable",
    artifactError: "Generation error",
    generationIncomplete: "The output was truncated, so the text is incomplete",
    generationRefused: "The content service declined this request",
    generationUnverified: "The output could not be verified as complete",
    generationEmpty: "The content service returned no text",
    generationCancelled: "This generation was cancelled",
    retryable: "Retryable",
    notRetryable: "Not retryable",
    invalidDate: "Unknown time",
    statusRunning: "Running",
    statusCompleted: "Completed",
    statusCompletedWithErrors: "Completed with errors",
    statusFailed: "Failed",
    statusInterrupted: "Interrupted",
    statusCancelled: "Cancelled",
    statusPending: "Pending",
    statusProcessing: "Processing",
    statusUnavailable: "Unavailable",
    statusUnknown: "Unknown status",
    artifactTranscript: "Transcript",
    artifactNotes: "Notes preparation",
    artifactMinutes: "Meeting minutes",
    openArtifact: "Open",
    revealArtifact: "Show in Finder",
    openingArtifact: "Opening…",
    revealingArtifact: "Revealing…",
    artifactActionFailed: "This file could not be accessed.",
    refreshLibraryHint: "Refresh the Task Library and try again.",
    canonicalTranscript: "Canonical transcript",
    generatedContent: "Generated content",
    generationHistory: "Version history",
    generateContent: "Generate content",
    generateAgain: "Generate again",
    generatingContent: "Generating…",
    generationFailed: "Generation failed. The transcript and earlier versions are unchanged.",
    latestVersion: "Latest successful version",
    legacyArtifact: "Legacy output",
    settingsProviders: "Providers and models",
	localModelTitle: "Local speech recognition", localModelDescription: "Manage the official SenseVoice model in Voxly app data. Model installation never writes into Task folders.", localChineseFirst: "Chinese-first", localModelStatus: "Status",
	localNotInstalled: "Not installed", localRuntimeMissing: "Model installed; local recognition component missing or incompatible", localModelInvalid: "Model invalid; reinstall required", localUnsupported: "Unsupported on this platform",
	localDownload: "Download model", localImport: "Import official model", localCancel: "Cancel", localDelete: "Delete model", localSource: "Official source", localSize: "Download size",
	localAdvanced: "Advanced download settings", localEndpoint: "Hugging Face Endpoint", localTestEndpoint: "Test connection", localEndpointReachable: "Endpoint is reachable", localOperationFailed: "The local model operation failed. Existing model data was not changed.", localConfirmDelete: "Delete the installed SenseVoice model? Tasks and history will remain unchanged.", localPrivacyTitle: "Privacy:", localPrivacyBody: "With local speech recognition, original audio is not sent to the cloud. If cloud content generation is selected, transcript text is still sent to that service.",
	localEndpointScope: "Applies only to future Hugging Face-hosted models explicitly marked compatible by the catalog. The current SenseVoice model is pinned to the k2-fsa GitHub Release; this setting and connection test neither change nor test its download source.",
    settingsProvidersDesc: "Choose the built-in providers and models used for speech recognition and generated content. Credentials remain stored separately.",
    settingsTemplates: "Content templates",
    settingsTemplatesDesc: "Manage local custom prompt templates. Built-in templates remain read-only.",
    builtInTemplates: "Built-in templates",
    myTemplates: "My templates",
    newTemplate: "New template",
    editTemplate: "Edit",
    deleteTemplate: "Delete",
    templateName: "Name",
    templateDescription: "Description",
    templateInstructions: "Prompt instructions",
    technicalDetails: "Technical details",
    revision: "Revision",
    saveCreatesRevision: "Saving changes will not affect content generated earlier.",
    confirmDeleteTemplate: "Delete this template? Historical outputs remain, but the template will no longer be selectable.",
    templateSaveFailed: "The template could not be saved. Refresh and try again.",
    noCustomTemplates: "No custom templates yet.",
    speechRecognition: "Speech recognition",
    generatedContentProvider: "Generated content",
    provider: "Provider",
    model: "Model",
    speechRecognitionProvider: "Speech recognition",
    generatedWithProvider: "Generated with",
    providerUnknown: "Legacy / unknown",
    providerNotReady: "The current provider configuration is not ready.",
    openSettings: "Open Settings",
    resumeTask: "Continue processing",
    resumeCancelled: "This continue request was cancelled",
    resumingTask: "Continuing…",
    stopTask: "Stop task",
    stoppingTask: "Stopping…",
    pendingChunksHint: "Pending audio chunks",
    blockedTaskLocked: "Another process is working on this task, so it cannot continue right now.",
    blockedSpeechProviderUnsupported: "The speech provider recorded on this task is no longer supported.",
    blockedSpeechProviderUnavailable: "The speech runtime recorded on this task is not available yet.",
    blockedSourceAudioMissing: "The task audio is missing, so it cannot be split again.",
    retryGeneration: "Retry this version",
    retryingGeneration: "Retrying…",
    retryKeepsIdentity: "Retry keeps the same version and provider",
    newGenerationKeepsHistory: "Generates a new version and leaves existing versions unchanged.",
    uploadsAudioHint: "Continuing sends the audio to a cloud speech-recognition service.",
    uploadsTextHint: "This action sends transcript text to a cloud content service.",
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
