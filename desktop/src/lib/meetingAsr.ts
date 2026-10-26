import { invoke } from "@tauri-apps/api/core";
import { listen, UnlistenFn } from "@tauri-apps/api/event";

export type BackendKind = "python" | "go";
export type RuntimeMode = "development" | "production";
export type OutputMode = "transcript_only" | "transcript_and_generate";
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
  generated_content_path: string | null;
}

export interface RetryGenerationResult {
  generation_id: string;
  status: string;
}

export interface CancelTaskResult {
  state: string;
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

export interface OutputRootInfo {
  output_root: string;
  configured: boolean;
  runtime_mode: RuntimeMode;
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
  asr_provider: ProviderReadiness;
  llm_provider: ProviderReadiness;
}

export interface ProviderSelection {
  provider_id: string;
  model_id: string;
}

export interface ProviderSettingsInfo {
  local_asr_language: "zh" | "auto";
  local_vad_enabled: boolean;
  asr: ProviderSelection;
  llm: ProviderSelection;
}

export interface ProviderReadiness {
  provider_id: string;
  model_id: string;
  ready: boolean;
  blocking_reasons: string[];
  warnings: string[];
}

export interface ProviderModelDescriptor {
  id: string;
  display_name: Record<"zh" | "en", string>;
}

export interface ProviderDescriptor {
  kind: "asr" | "llm";
  id: string;
  display_name: Record<"zh" | "en", string>;
  models: ProviderModelDescriptor[];
  default_model: string;
  credential_key: CredentialKey | "";
  allow_custom_model: boolean;
  built_in: boolean;
	execution_location: "local" | "cloud";
	runtime_kind: "remote_http" | "bundled_sidecar" | "external_local_service";
	requires_credentials: boolean;
	requires_external_service: boolean;
	model_source: "static_catalog" | "installed_models" | "external_discovery";
	supports_model_installation: boolean;
	supports_manual_import: boolean;
	privacy: { sends_audio_off_device: boolean; sends_text_off_device: boolean };
}

export interface ProviderCatalog {
  asr: ProviderDescriptor[];
  llm: ProviderDescriptor[];
}

export interface ManagedModelCatalog {
	schema_version: string;
	catalog_revision: string;
	models: ManagedModel[];
}

export interface ManagedModel {
	model_id: string;
	display_mode: string;
	display_name: Record<"zh" | "en", string>;
	engine: string;
	engine_runtime: string;
	runtime_version: string | null;
	model_version: string;
	platforms: string[];
	source: { kind: string; repository: string; revision: string; url: string };
	archive: { filename: string; type: string; size_bytes: number; sha256: string; root_directory: string };
}

export interface LocalModelStatus { model_id: string; model_version: string; state: string; message: string; installed_size_bytes: number; runtime_id?: string; runtime_version?: string }
export interface ModelProgressEvent { state: string; completed: number; total: number }
export interface ModelDownloadSettings { huggingface_endpoint: string }

export interface ArtifactSummary {
  kind: string;
  status: string;
  reference: string | null;
  has_error: boolean;
  retryable: boolean | null;
  provider: string;
  model: string;
  /** Safe generation error code, when the runtime reported one. */
  error_code?: string;
  /** True when a failed generation kept a usable but incomplete body. */
  partial?: boolean;
}

export interface GenerationSummary {
  generation_id: string;
  template_revision: number;
  template_display_name: string;
  status: string;
  created_at: string;
  reference: string | null;
  has_error: boolean;
  error_code: string;
  partial_output: boolean;
  retryable: boolean | null;
  legacy: boolean;
  provider: string;
  model: string;
}

export interface GenerationGroup {
  template_id: string;
  display_name: string;
  latest: GenerationSummary | null;
  versions: GenerationSummary[];
}

export interface CachePreview {
  scope?: string;
  can_clean: boolean;
  reason?: string;
  files: { reference: string; size_bytes: number }[];
  total_bytes: number;
  token?: string;
  cache_status: string;
  removed_bytes: number;
}

export const previewTaskCache = (taskFolder: string, textOnly = false) =>
  invoke<CachePreview>("preview_task_cache", { taskFolder, textOnly });

export const cleanTaskCache = (taskFolder: string, expectedToken: string, textOnly = false) =>
  invoke<CachePreview>("clean_task_cache", { taskFolder, expectedToken, textOnly });

export interface StorageSettings {
  auto_clean_chunks: boolean;
  cleanup_temporary_on_startup: boolean;
}
export interface TemporaryCleanup {
  removed_bytes: number;
  removed_count: number;
  skipped_count: number;
}
export interface StorageSummary {
  output_root: string;
  categories: Record<string, number>;
  total_bytes: number;
  tasks: { folder: string; title: string; status: string; total_bytes: number; chunk_bytes: number }[];
  issue_count: number;
}
export interface BatchCacheResult {
  output_root: string;
  tasks: { folder: string; title: string; preview: CachePreview }[];
  removed_bytes: number;
  cleaned_count: number;
  skipped_count: number;
  failed_count: number;
}
export const getStorageSettings = () => invoke<StorageSettings>("get_storage_settings");
export const setStorageSettings = (settings: StorageSettings) => invoke<StorageSettings>("set_storage_settings", {
  autoCleanChunks: settings.auto_clean_chunks,
  cleanupTemporaryOnStartup: settings.cleanup_temporary_on_startup,
});
export const getStorageSummary = () => invoke<StorageSummary>("get_storage_summary");
export const previewCacheBatch = () => invoke<BatchCacheResult>("preview_cache_batch");
export const cleanCacheBatch = (request: { output_root: string; tasks: {folder: string; token: string}[] }) =>
  invoke<BatchCacheResult>("clean_cache_batch", { request });
export const runStorageMaintenance = (startup = false) => invoke<TemporaryCleanup>("run_storage_maintenance", { startup });

export interface TaskActions {
  locked: boolean;
  /** Omitted by the runtime when the Task has no unfinished stage. */
  next_stage?: string;
  can_resume: boolean;
  can_resume_transcribe: boolean;
  can_resume_generation: boolean;
  can_retry_generation: boolean;
  pending_chunks: number;
  blocking_reasons: TaskBlockingReason[];
}

/** One reason an action is unavailable. The code is stable so the UI can localize it. */
export interface TaskBlockingReason {
  code: string;
  message: string;
}

export interface TaskSummary {
  task_id: string;
  title: string;
  title_source: string;
  description: string;
  status: string;
  created_at: string;
  updated_at: string;
  tags: string[];
  output_mode: string;
  primary_artifacts: ArtifactSummary[];
  has_partial_errors: boolean;
  task_folder: string;
  canonical_transcript: ArtifactSummary | null;
  generated_content: GenerationGroup[];
  can_generate: boolean;
  /** Runtime-derived actions, absent on manifests written before the field existed. */
  actions?: TaskActions | null;
}

export interface TaskMetadata {
  task_id: string;
  title: string;
  description: string;
  tags: string[];
  created_at: string;
  updated_at: string;
}

export interface TaskMetadataUpdate {
  title: string;
  description: string;
  tags: string[];
}

export interface PromptTemplateSummary {
  id: string;
  revision: number;
  display_name: Record<"zh" | "en", string>;
  description: Record<"zh" | "en", string>;
  instructions: string;
  content_type: string;
  extension: string;
  built_in: boolean;
  ownership: "builtin" | "custom";
  status: "active" | "deleted";
  can_edit: boolean;
  can_delete: boolean;
  created_at: string;
  updated_at: string;
}

export interface PromptTemplateCatalog {
  templates: PromptTemplateSummary[];
  issues: Array<{ code: string; message: string }>;
}

export interface CustomTemplateInput {
  display_name: string;
  description: string;
  instructions: string;
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

export async function getTaskMetadata(taskId: string): Promise<TaskMetadata> {
  return invoke<TaskMetadata>("get_task_metadata", { taskId });
}

export async function updateTaskMetadata(
  taskId: string,
  metadata: TaskMetadataUpdate
): Promise<TaskMetadata> {
  return invoke<TaskMetadata>("update_task_metadata", { taskId, metadata });
}

export async function listPromptTemplates(): Promise<PromptTemplateCatalog> {
  return invoke<PromptTemplateCatalog>("list_prompt_templates");
}

export async function createCustomTemplate(input: CustomTemplateInput): Promise<PromptTemplateSummary> {
  return invoke("create_custom_template", { input });
}

export async function updateCustomTemplate(input: CustomTemplateInput & { template_id: string; expected_current_revision: number }): Promise<PromptTemplateSummary> {
  return invoke("update_custom_template", { input });
}

export async function deleteCustomTemplate(templateId: string, expectedCurrentRevision: number): Promise<void> {
  await invoke("delete_custom_template", { input: { template_id: templateId, expected_current_revision: expectedCurrentRevision } });
}

export async function generateTaskContent(options: { taskFolder: string; templateId: string; deepseekApiKey?: string; llmConfig: ProviderSelection }): Promise<{ generation_id: string }> {
  return invoke("generate_task_content", { taskFolder: options.taskFolder, templateId: options.templateId, deepseekApiKey: options.deepseekApiKey || undefined, llmConfig: options.llmConfig });
}

export async function listProviders(): Promise<ProviderCatalog> {
  return invoke<ProviderCatalog>("list_providers");
}

export const getLocalModelCatalog = () => invoke<ManagedModelCatalog>("get_local_model_catalog");
export const getLocalModelStatus = () => invoke<LocalModelStatus>("get_local_model_status");
export const startModelDownload = () => invoke<LocalModelStatus>("start_model_download");
export const cancelModelDownload = () => invoke<void>("cancel_model_download");
export const importLocalModel = (sourceArchive: string) => invoke<LocalModelStatus>("import_local_model", { sourceArchive });
export const deleteLocalModel = () => invoke<LocalModelStatus>("delete_local_model");
export const getModelDownloadSettings = () => invoke<ModelDownloadSettings>("get_model_download_settings");
export const updateModelDownloadSettings = (endpoint: string) => invoke<ModelDownloadSettings>("update_model_download_settings", { endpoint });
export const testHuggingFaceEndpoint = (endpoint: string) => invoke<{ endpoint: string; reachable: boolean }>("test_huggingface_endpoint", { endpoint });
export const listenModelProgress = (handler: (event: ModelProgressEvent) => void) => listen<ModelProgressEvent>("model-progress", ({ payload }) => handler(payload));

export async function getProviderSettings(): Promise<ProviderSettingsInfo> {
  return invoke<ProviderSettingsInfo>("get_provider_settings");
}

export async function setProviderSettings(asr: ProviderSelection, llm: ProviderSelection, localAsrLanguage: "zh" | "auto", localVadEnabled: boolean): Promise<ProviderSettingsInfo> {
  return invoke<ProviderSettingsInfo>("set_provider_settings", { asr, llm, localAsrLanguage, localVadEnabled });
}

export async function getOutputRoot(): Promise<OutputRootInfo> {
  return invoke<OutputRootInfo>("get_output_root");
}

export async function setOutputRoot(outputRoot: string): Promise<OutputRootInfo> {
  return invoke<OutputRootInfo>("set_output_root", { outputRoot });
}

export async function resetOutputRoot(): Promise<OutputRootInfo> {
  return invoke<OutputRootInfo>("reset_output_root");
}

export async function openOutputRoot(): Promise<void> {
  return invoke("open_output_root");
}

export async function openTaskArtifact(
  taskFolder: string,
  artifactReference: string
): Promise<void> {
  return invoke("open_task_artifact", { taskFolder, artifactReference });
}

export async function revealTaskArtifact(
  taskFolder: string,
  artifactReference: string
): Promise<void> {
  return invoke("reveal_task_artifact", { taskFolder, artifactReference });
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
  backend?: BackendKind;
  templateId?: string;
  asrConfig: ProviderSelection;
  llmConfig?: ProviderSelection;
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
      backend: options.backend || "go",
      mimoApiKey: options.mimoApiKey || undefined,
      deepseekApiKey: options.deepseekApiKey || undefined,
      templateId: options.templateId || undefined,
      asrConfig: options.asrConfig,
      llmConfig: options.llmConfig,
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
    });
    return result;
  } finally {
    for (const fn of unlisteners) {
      fn();
    }
  }
}

/**
 * Continues an interrupted Task from the Task Library. The runtime decides which
 * work is left, and resume adopts the speech identity the manifest recorded, so
 * no session credential is sent here. The command is Go-only: no backend argument.
 */
export async function resumeTask(
  taskFolder: string,
  options?: { outputMode?: string; mimoApiKey?: string; deepseekApiKey?: string },
  callbacks?: RunCallbacks
): Promise<RunResult> {
  const unlisteners: UnlistenFn[] = [];

  const unlistenLog = await listen<LogEntry>("log-line", (event) => {
    callbacks?.onLog(event.payload);
  });
  unlisteners.push(unlistenLog);

  const unlistenProgress = await listen<ProgressEvent>("progress", (event) => {
    callbacks?.onProgress(event.payload);
  });
  unlisteners.push(unlistenProgress);

  const unlistenDone = await listen<{ code: number | null }>(
    "process-done",
    () => {
      callbacks?.onDone();
    }
  );
  unlisteners.push(unlistenDone);

  try {
    const result = await invoke<RunResult>("resume_task", {
      taskFolder,
      outputMode: options?.outputMode || undefined,
    });
    return result;
  } finally {
    for (const fn of unlisteners) {
      fn();
    }
  }
}

/**
 * Retries one user-selected failed generation under its original identity. It
 * never creates a new generation: the same generation id, provider and model run
 * again, and only process-done is reported.
 */
export async function retryTaskGeneration(options: { taskFolder: string; generationId: string; deepseekApiKey?: string }): Promise<RetryGenerationResult> {
  return invoke<RetryGenerationResult>("retry_task_generation", { taskFolder: options.taskFolder, generationId: options.generationId, deepseekApiKey: options.deepseekApiKey || undefined });
}

/**
 * Asks the active run to stop. The runtime stops cooperatively and forces a stop
 * only after its grace period, so a stopping run still owns its artifacts and a
 * new operation is rejected until the child exits.
 */
export async function cancelTask(): Promise<CancelTaskResult> {
  return invoke<CancelTaskResult>("cancel_task");
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
