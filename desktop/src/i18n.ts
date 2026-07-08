export type Locale = "zh" | "en";

export interface Translations {
  appTitle: string;
  appSubtitle: string;
  inputTitle: string;
  chooseFile: string;
  noFileSelected: string;
  modeTranscript: string;
  modeTranscriptDesc: string;
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
  folder: string;
  transcript: string;
  notesInput: string;
  meetingMinutes: string;
  completedSuccessfully: string;
  exitedWithCode: string;
  terminatedBySignal: string;
  transcriptWarning: string;
  llmWarning: string;
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
}

export const translations: Record<Locale, Translations> = {
  zh: {
    appTitle: "meeting-asr Desktop Prototype",
    appSubtitle: "实验性 Tauri 桌面原型 — 调用本地 Python CLI",
    inputTitle: "输入",
    chooseFile: "选择音频文件",
    noFileSelected: "未选择文件",
    modeTranscript: "转写文本",
    modeTranscriptDesc: "仅转写",
    modeMinutes: "LLM 总结",
    modeMinutesDesc: "AI 生成纪要",
    start: "开始",
    processing: "处理中...",
    openOutputFolder: "打开输出目录",
    splitting: "分割音频...",
    transcribing: "转写中...",
    merging: "合并转写...",
    preparingNotes: "准备笔记...",
    generatingMinutes: "生成会议纪要...",
    done: "完成",
    resultsTitle: "结果",
    folder: "文件夹",
    transcript: "转写",
    notesInput: "笔记输入",
    meetingMinutes: "会议纪要",
    completedSuccessfully: "完成成功",
    exitedWithCode: "退出码",
    terminatedBySignal: "被信号终止",
    transcriptWarning:
      "transcript 模式会将音频发送至 MiMo ASR 进行转写。请勿上传未授权的敏感会议内容。\n\nTranscript mode sends audio to MiMo ASR for transcription.",
    llmWarning:
      "minutes 模式会将转写文本发送至 DeepSeek 生成会议总结。请勿上传未授权的敏感会议内容。\n\nMinutes mode sends transcript text to DeepSeek for summary generation.",
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
  },
  en: {
    appTitle: "meeting-asr Desktop Prototype",
    appSubtitle: "Experimental Tauri prototype — calls the local Python CLI",
    inputTitle: "Input",
    chooseFile: "Choose Audio File",
    noFileSelected: "No file selected",
    modeTranscript: "Transcript",
    modeTranscriptDesc: "Text only",
    modeMinutes: "LLM Summary",
    modeMinutesDesc: "AI minutes",
    start: "Start",
    processing: "Processing...",
    openOutputFolder: "Open Output Folder",
    splitting: "Splitting audio...",
    transcribing: "Transcribing...",
    merging: "Merging transcript...",
    preparingNotes: "Preparing notes...",
    generatingMinutes: "Generating minutes...",
    done: "Done",
    resultsTitle: "Results",
    folder: "Folder",
    transcript: "Transcript",
    notesInput: "Notes input",
    meetingMinutes: "Meeting minutes",
    completedSuccessfully: "Completed successfully",
    exitedWithCode: "Exited with code",
    terminatedBySignal: "Terminated by signal",
    transcriptWarning:
      "Transcript mode sends audio to MiMo ASR for transcription. Do not upload unauthorized sensitive meeting content.",
    llmWarning:
      "Minutes mode sends transcript text to DeepSeek for summary generation. Do not upload unauthorized sensitive meeting content.",
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
