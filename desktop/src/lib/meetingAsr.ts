import { invoke } from "@tauri-apps/api/core";
import { listen, UnlistenFn } from "@tauri-apps/api/event";

export type BackendKind = "python" | "go";
export type RuntimeMode = "development" | "production";
export type OutputMode = "transcript" | "notes" | "minutes";
export type CredentialKey = "MIMO_API_KEY" | "DEEPSEEK_API_KEY";
export type CredentialSource = "session" | "secure_store" | "env" | "missing";

export interface CredentialStatus {
  key: CredentialKey;
  configured: boolean;
  source: CredentialSource;
}

export interface RunResult {
  task_folder: string;
  transcript_path: string;
  notes_input_path: string | null;
  minutes_path: string | null;
}

export interface RuntimeInfo {
  runtime_mode: RuntimeMode;
  backend: BackendKind;
}

export interface PreflightCheck {
  ready: boolean;
  message: string;
}

export interface OutputRootPreflight {
  path: string;
  configured: boolean;
  ready: boolean;
  message: string;
}

export interface CredentialPreflight {
  status: CredentialStatus;
  message: string;
}

export interface RuntimePreflight {
  runtime_mode: RuntimeMode;
  backend: BackendKind;
  ready: boolean;
  ready_for_minutes: boolean;
  sidecar: PreflightCheck;
  ffmpeg: PreflightCheck;
  output_root: OutputRootPreflight;
  mimo_api_key: CredentialPreflight;
  deepseek_api_key: CredentialPreflight;
}

export interface ArtifactSummary {
  kind: string;
  status: string;
  reference: string | null;
  has_error: boolean;
  retryable: boolean | null;
}

export interface TaskSummary {
  task_id: string;
  title: string;
  title_source: string;
  status: string;
  created_at: string;
  updated_at: string;
  tags: string[];
  output_mode: string;
  primary_artifacts: ArtifactSummary[];
  has_partial_errors: boolean;
  task_folder: string;
}

export interface DiscoveryIssue {
  task_folder: string;
  code: string;
  message: string;
}

export interface DiscoveryResult {
  tasks: TaskSummary[];
  issues: DiscoveryIssue[];
}

export interface BackendErrorInfo {
  category: string;
  message: string;
  code?: number;
}

export interface ProgressEvent {
  status: string;
  current: number;
  total: number;
}

export interface LogEntry {
  stream: string;
  line: string;
}

export interface RunCallbacks {
  onLog: (entry: LogEntry) => void;
  onProgress: (event: ProgressEvent) => void;
  onDone: () => void;
}

/** Returns runtime/backend policy metadata only; no credentials are included. */
export async function getRuntimeInfo(): Promise<RuntimeInfo> {
  return invoke<RuntimeInfo>("get_runtime_info");
}

/** Returns presentation-safe Task Library data from the configured output root. */
export async function listTasks(): Promise<DiscoveryResult> {
  return invoke<DiscoveryResult>("list_tasks");
}

/**
 * Returns local runtime readiness metadata only. Credential values are sent
 * only for the current invocation and are never returned by Rust.
 */
export async function getRuntimePreflight(options: {
  mimoApiKey?: string;
  deepseekApiKey?: string;
  backend?: BackendKind;
} = {}): Promise<RuntimePreflight> {
  return invoke<RuntimePreflight>("get_runtime_preflight", {
    mimoApiKey: options.mimoApiKey || undefined,
    deepseekApiKey: options.deepseekApiKey || undefined,
    backend: options.backend,
  });
}

export interface RunOptions {
  mimoApiKey?: string;
  deepseekApiKey?: string;
  deepseekModel?: string;
  backend?: BackendKind;
}

export function parseBackendError(raw: string): BackendErrorInfo {
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed.category === "string") {
      return {
        category: parsed.category,
        message: parsed.message || raw,
        code: parsed.code,
      };
    }
  } catch {
    // not JSON, return as generic error
  }
  return {
    category: "process_failed",
    message: raw,
  };
}

export async function runMeetingAsr(
  audioPath: string,
  outputMode: OutputMode,
  options: RunOptions,
  callbacks: RunCallbacks
): Promise<RunResult> {
  const unlisteners: UnlistenFn[] = [];

  const unlistenLog = await listen<LogEntry>("log-line", (event) => {
    callbacks.onLog(event.payload);
  });
  unlisteners.push(unlistenLog);

  const unlistenProgress = await listen<ProgressEvent>("progress", (event) => {
    callbacks.onProgress(event.payload);
  });
  unlisteners.push(unlistenProgress);

  const unlistenDone = await listen<{ code: number | null }>(
    "process-done",
    () => {
      callbacks.onDone();
    }
  );
  unlisteners.push(unlistenDone);

  try {
    const result = await invoke<RunResult>("run_meeting_asr", {
      audioPath,
      outputMode,
      backend: options.backend || "python",
      mimoApiKey: options.mimoApiKey || undefined,
      deepseekApiKey: options.deepseekApiKey || undefined,
      deepseekModel: options.deepseekModel || undefined,
    });
    return result;
  } finally {
    for (const fn of unlisteners) {
      fn();
    }
  }
}

export async function resumeMeetingAsr(
  taskDir: string,
  options: {
    outputMode?: OutputMode;
    backend?: BackendKind;
    mimoApiKey?: string;
    deepseekApiKey?: string;
    deepseekModel?: string;
  },
  callbacks: RunCallbacks
): Promise<RunResult> {
  const unlisteners: UnlistenFn[] = [];

  const unlistenLog = await listen<LogEntry>("log-line", (event) => {
    callbacks.onLog(event.payload);
  });
  unlisteners.push(unlistenLog);

  const unlistenProgress = await listen<ProgressEvent>("progress", (event) => {
    callbacks.onProgress(event.payload);
  });
  unlisteners.push(unlistenProgress);

  const unlistenDone = await listen<{ code: number | null }>(
    "process-done",
    () => {
      callbacks.onDone();
    }
  );
  unlisteners.push(unlistenDone);

  try {
    const result = await invoke<RunResult>("resume_meeting_asr", {
      taskDir,
      outputMode: options.outputMode || undefined,
      backend: options.backend || "python",
      mimoApiKey: options.mimoApiKey || undefined,
      deepseekApiKey: options.deepseekApiKey || undefined,
      deepseekModel: options.deepseekModel || undefined,
    });
    return result;
  } finally {
    for (const fn of unlisteners) {
      fn();
    }
  }
}

export async function openTaskFolder(taskFolder: string): Promise<void> {
  await invoke("open_task_folder", { taskFolder });
}

export async function saveEnvKey(key: string, value: string): Promise<void> {
  await invoke("save_env_key", { key, value });
}

/**
 * Returns safe credential metadata only; Rust never returns the stored value.
 */
export async function getCredentialStatus(
  key: CredentialKey
): Promise<CredentialStatus> {
  return invoke<CredentialStatus>("get_credential_status", { key });
}

/**
 * Saves a credential to the OS secure store and returns safe status metadata.
 */
export async function saveCredential(
  key: CredentialKey,
  value: string
): Promise<CredentialStatus> {
  return invoke<CredentialStatus>("save_credential", { key, value });
}

/**
 * Deletes a credential from the OS secure store and returns safe status metadata.
 */
export async function deleteCredential(
  key: CredentialKey
): Promise<CredentialStatus> {
  return invoke<CredentialStatus>("delete_credential", { key });
}
