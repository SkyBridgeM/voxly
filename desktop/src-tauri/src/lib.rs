use std::fs::{self, OpenOptions};
use std::io::{BufRead, BufReader, Write};
use std::path::Component;
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use tauri::{AppHandle, Emitter, Manager, RunEvent, State};

mod app_namespace;
mod credentials;
mod process_control;
use app_namespace::{migrate_app_namespace, AppNamespacePaths};
use credentials::{
    resolve_credential, status_for, CredentialError, CredentialKey, CredentialSource,
    CredentialStatus, CredentialStore, EnvironmentValues, KeyringCredentialStore,
};

// ---------------------------------------------------------------------------
// Shared event payloads
// ---------------------------------------------------------------------------

/// Exit status the Go sidecar uses for a run that stopped because a stop was
/// requested. It keeps a user cancel distinguishable from a provider failure.
const CANCELLED_EXIT_CODE: i32 = 130;

#[derive(Clone, serde::Serialize)]
struct LogEvent {
    stream: String,
    line: String,
}
#[derive(Clone, serde::Serialize)]
struct DoneEvent {
    code: Option<i32>,
}

#[derive(Clone, serde::Serialize)]
struct ProgressEvent {
    status: String,
    current: u32,
    total: u32,
}

#[derive(Clone, serde::Serialize)]
struct ModelProgressEvent {
    state: String,
    completed: u64,
    total: u64,
}

// ---------------------------------------------------------------------------
// CLI stdout status → UI progress-status mapping
// ---------------------------------------------------------------------------

fn normalize_status(cli_status: &str) -> &str {
    let t = cli_status.trim();
    if t == "Splitting audio" {
        return "splitting";
    }
    if t.starts_with("Transcribing") || t == "All chunks already transcribed" {
        return "transcribing";
    }
    if t == "Merging transcript" {
        return "merging";
    }
    if t == "Preparing notes input" {
        return "notes";
    }
    if t == "Generating meeting minutes" {
        return "minutes";
    }
    if t == "Generating content" {
        return "generating";
    }
    if t == "Done" || t == "All outputs are valid — nothing to resume" {
        return "done";
    }
    // Unknown status — keep raw text so it's visible but doesn't break parsing.
    cli_status
}

fn parse_progress(value: &str) -> Option<(u32, u32)> {
    let (current, total) = value.trim().split_once('/')?;
    let current = current.trim().parse::<u32>().ok()?;
    let total = total.trim().parse::<u32>().ok()?;
    (current > 0 && total > 0 && current <= total).then_some((current, total))
}

// ---------------------------------------------------------------------------
// Parsed stdout result
// ---------------------------------------------------------------------------

#[derive(Debug, Default, Clone, serde::Serialize)]
struct ParsedOutput {
    saw_status: bool,
    task_folder: Option<String>,
    transcript_path: Option<String>,
    notes_input_path: Option<String>,
    minutes_path: Option<String>,
    generated_content_path: Option<String>,
    generation_status: Option<String>,
}

impl ParsedOutput {
    fn parse_lines(lines: &[String]) -> Self {
        let mut out = ParsedOutput::default();
        for line in lines {
            let trimmed = line.trim();
            if let Some(v) = trimmed.strip_prefix("Status:") {
                out.saw_status |= !v.trim().is_empty();
            } else if let Some(v) = trimmed.strip_prefix("Task folder:") {
                let v = v.trim();
                if !v.is_empty() {
                    out.task_folder = Some(v.to_string());
                }
            } else if let Some(v) = trimmed.strip_prefix("Transcript saved:") {
                let v = v.trim();
                if !v.is_empty() {
                    out.transcript_path = Some(v.to_string());
                }
            } else if let Some(v) = trimmed.strip_prefix("Notes input saved:") {
                let v = v.trim();
                if !v.is_empty() {
                    out.notes_input_path = Some(v.to_string());
                }
            } else if let Some(v) = trimmed.strip_prefix("Meeting minutes saved:") {
                let v = v.trim();
                if !v.is_empty() {
                    out.minutes_path = Some(v.to_string());
                }
            } else if let Some(v) = trimmed.strip_prefix("Generated content saved:") {
                let v = v.trim();
                if !v.is_empty() {
                    out.generated_content_path = Some(v.to_string());
                }
            } else if let Some(v) = trimmed.strip_prefix("Generation status:") {
                let v = v.trim();
                if !v.is_empty() {
                    out.generation_status = Some(v.to_string());
                }
            }
        }
        out
    }

    /// Validates required outputs and, when supplied, their canonical task root.
    fn validate_required_under(
        &self,
        output_mode: &str,
        expected_task_root: Option<&Path>,
    ) -> Result<(), String> {
        if !self.saw_status {
            return Err("Missing non-empty 'Status:' in CLI output".to_string());
        }
        let task_dir = self
            .task_folder
            .as_ref()
            .ok_or_else(|| "Missing 'Task folder:' in CLI output".to_string())?;
        let tf = match expected_task_root {
            Some(root) => canonicalize_path_within(root, Path::new(task_dir), "Task folder")?,
            None => canonicalize_existing_absolute(Path::new(task_dir), "Task folder")?,
        };
        if !tf.is_dir() {
            return Err(format!("Task folder is not a directory: {}", task_dir));
        }

        // Transcript is required for all modes.
        let tp = self
            .transcript_path
            .as_ref()
            .ok_or_else(|| "Missing 'Transcript saved:' in CLI output".to_string())?;
        let transcript = canonicalize_path_within(&tf, Path::new(tp), "Transcript path")?;
        if !is_complete_file(&transcript) {
            return Err(format!("Transcript file is missing or empty: {}", tp));
        }

        // Notes input for notes/minutes modes.
        if output_mode == "notes" || output_mode == "minutes" {
            let np = self
                .notes_input_path
                .as_ref()
                .ok_or_else(|| "Missing 'Notes input saved:' in CLI output".to_string())?;
            let notes = canonicalize_path_within(&tf, Path::new(np), "Notes input path")?;
            if !is_complete_file(&notes) {
                return Err(format!("Notes input file is missing or empty: {}", np));
            }
        }

        // Minutes for minutes mode.
        if output_mode == "minutes" {
            let mp = self
                .minutes_path
                .as_ref()
                .ok_or_else(|| "Missing 'Meeting minutes saved:' in CLI output".to_string())?;
            let minutes = canonicalize_path_within(&tf, Path::new(mp), "Minutes path")?;
            if !is_complete_file(&minutes) {
                return Err(format!("Minutes file is missing or empty: {}", mp));
            }
        }
        if output_mode == "transcript_and_generate" {
            let generated = self
                .generated_content_path
                .as_ref()
                .ok_or_else(|| "Missing 'Generated content saved:' in CLI output".to_string())?;
            let generated =
                canonicalize_path_within(&tf, Path::new(generated), "Generated content path")?;
            if !is_complete_file(&generated) {
                return Err("Generated content file is missing or empty".to_string());
            }
        }

        Ok(())
    }
}

fn is_complete_file(path: &Path) -> bool {
    path.metadata()
        .map(|metadata| metadata.is_file() && metadata.len() > 0)
        .unwrap_or(false)
}

// ---------------------------------------------------------------------------
// Error categories
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(tag = "category")]
enum BackendError {
    #[serde(rename = "backend_not_found")]
    BackendNotFound { message: String },
    #[serde(rename = "dependency_missing")]
    DependencyMissing { message: String },
    #[serde(rename = "missing_api_key")]
    MissingApiKey { message: String },
    #[serde(rename = "secure_storage_unavailable")]
    SecureStorageUnavailable { message: String },
    #[serde(rename = "invalid_input")]
    InvalidInput { message: String },
    #[serde(rename = "manifest_error")]
    ManifestError { message: String },
    #[serde(rename = "task_locked")]
    TaskLocked { message: String },
    #[serde(rename = "interrupted")]
    Interrupted { message: String },
    #[serde(rename = "process_failed")]
    ProcessFailed { message: String, code: Option<i32> },
    #[serde(rename = "artifact_unavailable")]
    ArtifactUnavailable { message: String },
    #[serde(rename = "artifact_validation_failed")]
    ArtifactValidationFailed { message: String },
    #[serde(rename = "artifact_action_failed")]
    ArtifactActionFailed { message: String },
}

impl BackendError {
    fn to_json_string(&self) -> String {
        serde_json::to_string(self).unwrap_or_else(|_| {
            "{\"category\":\"process_failed\",\"message\":\"Cannot serialize error\"}".to_string()
        })
    }
}

impl From<BackendError> for String {
    fn from(e: BackendError) -> String {
        e.to_json_string()
    }
}

impl From<CredentialError> for BackendError {
    fn from(error: CredentialError) -> Self {
        let message = error.to_string();
        match error {
            CredentialError::SecureStoreUnavailable { .. } => {
                Self::SecureStorageUnavailable { message }
            }
            CredentialError::InvalidKey
            | CredentialError::InvalidValue
            | CredentialError::EnvironmentUnavailable => Self::InvalidInput { message },
        }
    }
}

// ---------------------------------------------------------------------------
// Stderr-based error classification
// ---------------------------------------------------------------------------

/// cancelled_failure reports a run that stopped because a stop was requested.
/// Every command funnels its exit status through it, so a cancel is never shown
/// as a provider or workflow failure.
fn cancelled_failure(code: Option<i32>, message: &str) -> Option<BackendError> {
    if code == Some(CANCELLED_EXIT_CODE) {
        return Some(BackendError::Interrupted {
            message: message.to_string(),
        });
    }
    None
}

/// classify_process_failure maps a finished process to a safe error, keeping a
/// requested stop distinguishable from a provider failure.
fn classify_process_failure(
    code: Option<i32>,
    stderr_lines: &[String],
    secrets: &[String],
    cancelled_message: &str,
) -> BackendError {
    if let Some(error) = cancelled_failure(code, cancelled_message) {
        return error;
    }
    match code {
        Some(c) => classify_stderr(stderr_lines, Some(c), secrets),
        None => BackendError::Interrupted {
            message: "Process was terminated by a signal.".to_string(),
        },
    }
}

fn classify_stderr(
    stderr_lines: &[String],
    exit_code: Option<i32>,
    secrets: &[String],
) -> BackendError {
    let safe_lines = redact_lines(stderr_lines, secrets);
    let combined = safe_lines.join("\n").to_lowercase();

    // Check explicit structured error patterns first.
    if combined.contains("manifest")
        && (combined.contains("missing")
            || combined.contains("corrupt")
            || combined.contains("invalid")
            || combined.contains("unsupported")
            || combined.contains("no manifest")
            || combined.contains("not found"))
    {
        return BackendError::ManifestError {
            message: "Manifest missing, corrupt, or invalid.".to_string(),
        };
    }
    if combined.contains("already being resumed") || combined.contains("acquiring task lock") {
        return BackendError::TaskLocked {
            message: "Task is locked by another resume process.".to_string(),
        };
    }
    if combined.contains("ffmpeg")
        && (combined.contains("not found") || combined.contains("was not found"))
    {
        return BackendError::DependencyMissing {
            message: "ffmpeg is not installed or not on PATH.".to_string(),
        };
    }
    if combined.contains("api_key")
        && (combined.contains("not configured")
            || combined.contains("not set")
            || combined.contains("missing"))
    {
        return BackendError::MissingApiKey {
            message: "API key is not configured — set it in .env or Settings.".to_string(),
        };
    }
    if combined.contains("invalid")
        && (combined.contains("output_mode")
            || combined.contains("output mode")
            || combined.contains("output-mode")
            || combined.contains("audio format")
            || combined.contains("unsupported"))
    {
        return BackendError::InvalidInput {
            message: "Invalid input — check output mode and audio format.".to_string(),
        };
    }

    // Fallback: generic process failure.
    BackendError::ProcessFailed {
        message: if safe_lines.is_empty() {
            format!(
                "Process exited with code {} and no stderr output.",
                exit_code.unwrap_or(-1)
            )
        } else {
            let first = safe_lines
                .iter()
                .filter(|l| !l.trim().is_empty())
                .take(3)
                .cloned()
                .collect::<Vec<_>>()
                .join("; ");
            if first.is_empty() {
                format!("Process exited with code {}.", exit_code.unwrap_or(-1))
            } else {
                format!(
                    "Process exited with code {}: {}",
                    exit_code.unwrap_or(-1),
                    first
                )
            }
        },
        code: exit_code,
    }
}

// ---------------------------------------------------------------------------
// Backend kind
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Deserialize, serde::Serialize)]
#[serde(rename_all = "lowercase")]
enum BackendKind {
    Go,
}

// ---------------------------------------------------------------------------
// Run result
// ---------------------------------------------------------------------------

#[derive(Clone, serde::Serialize)]
struct RunResult {
    task_folder: String,
    transcript_path: String,
    notes_input_path: Option<String>,
    minutes_path: Option<String>,
    generated_content_path: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct ArtifactSummary {
    kind: String,
    status: String,
    reference: Option<String>,
    has_error: bool,
    retryable: Option<bool>,
    #[serde(default)]
    provider: String,
    #[serde(default)]
    model: String,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct TaskBlockingReason {
    code: String,
    message: String,
}

/// TaskActions is what the runtime allows for one Task right now. It is derived
/// by the Go core from durable state, the Task lock, and provider support; the
/// desktop only reads it.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct TaskActions {
    #[serde(default)]
    locked: bool,
    #[serde(default)]
    next_stage: String,
    #[serde(default)]
    can_resume: bool,
    #[serde(default)]
    can_resume_transcribe: bool,
    #[serde(default)]
    can_resume_generation: bool,
    #[serde(default)]
    can_retry_generation: bool,
    #[serde(default)]
    pending_chunks: u32,
    #[serde(default)]
    blocking_reasons: Vec<TaskBlockingReason>,
}

impl TaskActions {
    fn has_reason(&self, code: &str) -> bool {
        self.blocking_reasons
            .iter()
            .any(|reason| reason.code == code)
    }
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct TaskSummary {
    task_id: String,
    title: String,
    title_source: String,
    #[serde(default)]
    description: String,
    status: String,
    created_at: String,
    updated_at: String,
    tags: Vec<String>,
    output_mode: String,
    primary_artifacts: Vec<ArtifactSummary>,
    has_partial_errors: bool,
    task_folder: String,
    #[serde(default)]
    canonical_transcript: Option<ArtifactSummary>,
    #[serde(default)]
    generated_content: Vec<GenerationGroup>,
    #[serde(default)]
    can_generate: bool,
    #[serde(default)]
    actions: Option<TaskActions>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct GenerationGroup {
    template_id: String,
    display_name: String,
    latest: Option<GenerationSummary>,
    versions: Vec<GenerationSummary>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct GenerationSummary {
    generation_id: String,
    #[serde(default)]
    template_revision: u32,
    #[serde(default)]
    template_display_name: String,
    status: String,
    created_at: String,
    reference: Option<String>,
    has_error: bool,
    #[serde(default)]
    error_code: String,
    #[serde(default)]
    partial_output: bool,
    retryable: Option<bool>,
    legacy: bool,
    #[serde(default)]
    provider: String,
    #[serde(default)]
    model: String,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct PromptTemplateSummary {
    id: String,
    revision: u32,
    display_name: std::collections::HashMap<String, String>,
    description: std::collections::HashMap<String, String>,
    instructions: String,
    content_type: String,
    extension: String,
    built_in: bool,
    ownership: String,
    status: String,
    can_edit: bool,
    can_delete: bool,
    #[serde(default)]
    created_at: String,
    #[serde(default)]
    updated_at: String,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct PromptTemplateCatalog {
    templates: Vec<PromptTemplateSummary>,
    issues: Vec<TemplateIssue>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct TemplateIssue {
    code: String,
    message: String,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct CustomTemplateInput {
    display_name: String,
    description: String,
    instructions: String,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct UpdateCustomTemplateInput {
    template_id: String,
    expected_current_revision: u32,
    display_name: String,
    description: String,
    instructions: String,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct DeleteCustomTemplateInput {
    template_id: String,
    expected_current_revision: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct ProviderModelDescriptor {
    id: String,
    display_name: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct ProviderDescriptor {
    kind: String,
    id: String,
    display_name: std::collections::HashMap<String, String>,
    models: Vec<ProviderModelDescriptor>,
    default_model: String,
    credential_key: String,
    allow_custom_model: bool,
    built_in: bool,
    execution_location: String,
    runtime_kind: String,
    requires_credentials: bool,
    requires_external_service: bool,
    model_source: String,
    supports_model_installation: bool,
    supports_manual_import: bool,
    privacy: ProviderPrivacyFacts,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct ProviderPrivacyFacts {
    sends_audio_off_device: bool,
    sends_text_off_device: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct ProviderCatalog {
    asr: Vec<ProviderDescriptor>,
    llm: Vec<ProviderDescriptor>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct DiscoveryIssue {
    task_folder: String,
    code: String,
    message: String,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct DiscoveryResult {
    tasks: Vec<TaskSummary>,
    issues: Vec<DiscoveryIssue>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct TaskMetadata {
    task_id: String,
    title: String,
    description: String,
    tags: Vec<String>,
    created_at: String,
    updated_at: String,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct TaskMetadataUpdate {
    title: String,
    description: String,
    tags: Vec<String>,
}

// ---------------------------------------------------------------------------
// Project helpers
// ---------------------------------------------------------------------------

fn project_root() -> &'static str {
    concat!(env!("CARGO_MANIFEST_DIR"), "/../..")
}

fn resolve_project_root() -> PathBuf {
    match std::env::var("MEETING_ASR_PROJECT_ROOT") {
        Ok(root) if !root.is_empty() => PathBuf::from(root),
        _ => PathBuf::from(project_root()),
    }
}

// ---------------------------------------------------------------------------
// Runtime mode and non-sensitive desktop settings
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum RuntimeMode {
    Development,
    Production,
}

impl RuntimeMode {
    fn current() -> Self {
        if cfg!(debug_assertions) {
            Self::Development
        } else {
            Self::Production
        }
    }
}

fn parse_backend_kind(value: &str) -> Result<BackendKind, BackendError> {
    match value {
        "go" => Ok(BackendKind::Go),
        other => Err(BackendError::InvalidInput {
            message: format!("Invalid backend: {other}. Only 'go' is supported."),
        }),
    }
}

fn resolve_backend_kind(
    requested: Option<&str>,
    _runtime_mode: RuntimeMode,
) -> Result<BackendKind, BackendError> {
    parse_backend_kind(requested.unwrap_or("go"))
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
struct RuntimeInfo {
    runtime_mode: RuntimeMode,
    backend: BackendKind,
}

fn runtime_info_for(runtime_mode: RuntimeMode) -> RuntimeInfo {
    RuntimeInfo {
        runtime_mode,
        backend: match runtime_mode {
            RuntimeMode::Development => BackendKind::Go,
            RuntimeMode::Production => BackendKind::Go,
        },
    }
}

#[tauri::command]
fn get_runtime_info() -> RuntimeInfo {
    runtime_info_for(RuntimeMode::current())
}

#[derive(Debug, Clone, serde::Serialize)]
struct PreflightCheck {
    ready: bool,
    message: String,
}

#[derive(Debug, Clone, serde::Serialize)]
struct OutputRootPreflight {
    path: String,
    configured: bool,
    ready: bool,
    message: String,
}

#[derive(Debug, Clone, serde::Serialize)]
struct CredentialPreflight {
    status: CredentialStatus,
    message: String,
}

#[derive(Debug, Clone, serde::Serialize)]
struct RuntimePreflight {
    runtime_mode: RuntimeMode,
    backend: BackendKind,
    /// Ready for transcript and notes mode. Minutes readiness is separate
    /// because it additionally requires DEEPSEEK_API_KEY.
    ready: bool,
    ready_for_minutes: bool,
    sidecar: PreflightCheck,
    ffmpeg: PreflightCheck,
    output_root: OutputRootPreflight,
    mimo_api_key: CredentialPreflight,
    deepseek_api_key: CredentialPreflight,
    asr_provider: ProviderReadiness,
    llm_provider: ProviderReadiness,
}

const DEFAULT_ASR_PROVIDER: &str = "mimo";
const DEFAULT_ASR_MODEL: &str = "mimo-v2.5-asr";
const LOCAL_ASR_PROVIDER: &str = "local-sensevoice";
const LOCAL_ASR_MODEL: &str = "sensevoice-small-int8";
const DEFAULT_LLM_PROVIDER: &str = "deepseek";
const DEFAULT_LLM_MODEL: &str = "deepseek-v4-flash";

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct ProviderSelection {
    provider_id: String,
    model_id: String,
}

impl ProviderSelection {
    fn default_asr() -> Self {
        Self {
            provider_id: DEFAULT_ASR_PROVIDER.to_string(),
            model_id: DEFAULT_ASR_MODEL.to_string(),
        }
    }

    fn default_llm() -> Self {
        Self {
            provider_id: DEFAULT_LLM_PROVIDER.to_string(),
            model_id: DEFAULT_LLM_MODEL.to_string(),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
struct ProviderSettingsInfo {
    local_asr_language: String,
    local_vad_enabled: bool,
    asr: ProviderSelection,
    llm: ProviderSelection,
}

#[derive(Debug, Clone, serde::Serialize)]
struct ProviderReadiness {
    provider_id: String,
    model_id: String,
    ready: bool,
    blocking_reasons: Vec<String>,
    warnings: Vec<String>,
}

fn validate_asr_selection(selection: &ProviderSelection) -> Result<(), BackendError> {
    let valid = (selection.provider_id == DEFAULT_ASR_PROVIDER
        && selection.model_id == DEFAULT_ASR_MODEL)
        || (selection.provider_id == LOCAL_ASR_PROVIDER && selection.model_id == LOCAL_ASR_MODEL);
    if !valid {
        return Err(BackendError::InvalidInput {
            message: "Unknown speech-recognition provider or model.".to_string(),
        });
    }
    Ok(())
}

fn validate_llm_selection(selection: &ProviderSelection) -> Result<(), BackendError> {
    if selection.provider_id != DEFAULT_LLM_PROVIDER {
        return Err(BackendError::InvalidInput {
            message: "Unknown generated-content provider.".to_string(),
        });
    }
    if selection.model_id != DEFAULT_LLM_MODEL {
        return Err(BackendError::InvalidInput {
            message: "The selected generated-content model is not supported by DeepSeek."
                .to_string(),
        });
    }
    Ok(())
}

fn provider_readiness(
    selection: &ProviderSelection,
    credential_ready: bool,
    asr: bool,
    local_model_status: Option<&LocalModelStatus>,
) -> ProviderReadiness {
    let validation = if asr {
        validate_asr_selection(selection)
    } else {
        validate_llm_selection(selection)
    };
    let mut blocking_reasons = Vec::new();
    if let Err(error) = validation {
        blocking_reasons.push(match error {
            BackendError::InvalidInput { message } => message,
            _ => "Provider configuration is invalid.".to_string(),
        });
    }
    if asr && selection.provider_id == LOCAL_ASR_PROVIDER {
        match local_model_status {
            Some(status) if status.state == "ready" => {}
            Some(status) => blocking_reasons.push(status.message.clone()),
            None => blocking_reasons.push("Local model readiness is unavailable.".to_string()),
        }
    } else if !credential_ready {
        blocking_reasons.push(
            if asr {
                "Speech-recognition credential is not configured."
            } else {
                "Generated-content credential is not configured."
            }
            .to_string(),
        );
    }
    ProviderReadiness {
        provider_id: selection.provider_id.clone(),
        model_id: selection.model_id.clone(),
        ready: blocking_reasons.is_empty(),
        blocking_reasons,
        warnings: Vec::new(),
    }
}

fn preflight_check(ready: bool, message: impl Into<String>) -> PreflightCheck {
    PreflightCheck {
        ready,
        message: message.into(),
    }
}

fn preflight_sidecar_at(executable_dir: &Path) -> PreflightCheck {
    let binary = match discover_production_go_binary(executable_dir) {
        Ok(binary) => binary,
        Err(BackendError::BackendNotFound { message }) => return preflight_check(false, message),
        Err(_) => {
            return preflight_check(false, "Packaged Go sidecar is not ready.");
        }
    };

    match validate_go_binary(&binary) {
        Ok(()) => preflight_check(true, "Packaged Go sidecar is available and version-valid."),
        Err(BackendError::BackendNotFound { message }) => preflight_check(false, message),
        Err(_) => preflight_check(false, "Packaged Go sidecar is not ready."),
    }
}

fn preflight_development_go_result(discovered: Result<PathBuf, BackendError>) -> PreflightCheck {
    let binary = match discovered {
        Ok(binary) => binary,
        Err(_) => {
            return preflight_check(
                false,
                "Development Go binary is unavailable. Build core-go/meeting-asr-go, set MEETING_ASR_GO_BINARY.",
            );
        }
    };

    match validate_go_binary(&binary) {
        Ok(()) => preflight_check(
            true,
            "Development Go binary is available and version-valid.",
        ),
        Err(_) => preflight_check(
            false,
            "Development Go binary is unavailable or invalid. Build core-go/meeting-asr-go, set MEETING_ASR_GO_BINARY.",
        ),
    }
}

fn preflight_development_go() -> PreflightCheck {
    preflight_development_go_result(discover_development_go_binary())
}

fn preflight_sidecar(runtime_mode: RuntimeMode, _backend: BackendKind) -> PreflightCheck {
    if runtime_mode == RuntimeMode::Development {
        return preflight_development_go();
    }

    match packaged_go_executable_dir() {
        Ok(executable_dir) => preflight_sidecar_at(&executable_dir),
        Err(BackendError::BackendNotFound { message }) => preflight_check(false, message),
        Err(_) => preflight_check(
            false,
            "Could not resolve the packaged Go sidecar directory.",
        ),
    }
}

fn check_ffmpeg_at(binary: &Path) -> PreflightCheck {
    match Command::new(binary)
        .arg("-version")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
    {
        Ok(status) if status.success() => preflight_check(
            true,
            if binary.is_absolute() {
                "Bundled ffmpeg is ready."
            } else {
                "ffmpeg is available on PATH."
            },
        ),
        _ => preflight_check(
            false,
            "ffmpeg is unavailable; reinstall Voxly, or install ffmpeg for development.",
        ),
    }
}

fn ffmpeg_binary_for(
    runtime_mode: RuntimeMode,
    executable_dir: Option<&Path>,
) -> Result<PathBuf, BackendError> {
    if runtime_mode == RuntimeMode::Development {
        return Ok(PathBuf::from("ffmpeg"));
    }
    let dir = match executable_dir {
        Some(dir) => dir.to_path_buf(),
        None => packaged_go_executable_dir()?,
    };
    Ok(dir.join("ffmpeg"))
}

fn output_root_preflight_for(path: &Path, configured: bool) -> OutputRootPreflight {
    let display_path = path.to_string_lossy().to_string();
    if !path.is_absolute() {
        return OutputRootPreflight {
            path: display_path,
            configured,
            ready: false,
            message: "Output root must be an absolute path.".to_string(),
        };
    }

    if path.exists() {
        if !path.is_dir() {
            return OutputRootPreflight {
                path: display_path,
                configured,
                ready: false,
                message: "Output root exists but is not a directory.".to_string(),
            };
        }

        return match path.canonicalize() {
            Ok(canonical) => OutputRootPreflight {
                path: canonical.to_string_lossy().to_string(),
                configured,
                ready: true,
                message: "Output root is available.".to_string(),
            },
            Err(_) => OutputRootPreflight {
                path: display_path,
                configured,
                ready: false,
                message: "Output root could not be canonicalized.".to_string(),
            },
        };
    }

    let parent_ready = path.parent().is_some_and(Path::is_dir);
    OutputRootPreflight {
        path: display_path,
        configured,
        ready: parent_ready,
        message: if parent_ready {
            "Output root is ready and will be created on the first run.".to_string()
        } else {
            "Output root and its parent directory are not available.".to_string()
        },
    }
}

fn configured_output_root_path(
    settings: &DesktopSettings,
) -> Result<Option<PathBuf>, BackendError> {
    match settings.output_root.as_deref() {
        None => Ok(None),
        Some(value) if value.trim().is_empty() => Err(BackendError::InvalidInput {
            message: "Configured output root cannot be empty.".to_string(),
        }),
        Some(value) => {
            let path = PathBuf::from(value);
            if !path.is_absolute() {
                return Err(BackendError::InvalidInput {
                    message: format!("Output root must be absolute: {}", path.display()),
                });
            }
            Ok(Some(path))
        }
    }
}

fn output_root_preflight(app: &AppHandle) -> Result<OutputRootPreflight, BackendError> {
    let settings_path = desktop_settings_path(app)?;
    let settings = load_desktop_settings(&settings_path)?;
    if let Some(preflight) = configured_output_root_preflight(&settings) {
        return Ok(preflight);
    }

    let runtime_mode = RuntimeMode::current();
    let documents_dir = if runtime_mode == RuntimeMode::Production {
        Some(
            app.path()
                .document_dir()
                .map_err(|error| BackendError::InvalidInput {
                    message: format!("Could not resolve the user Documents directory: {error}"),
                })?,
        )
    } else {
        None
    };
    let default_root = get_default_output_root(runtime_mode, documents_dir.as_deref())?;
    Ok(output_root_preflight_for(&default_root, false))
}

fn configured_output_root_preflight(settings: &DesktopSettings) -> Option<OutputRootPreflight> {
    settings.output_root.as_deref().map(|value| {
        let configured = PathBuf::from(value);
        if value.trim().is_empty() || !configured.is_absolute() {
            return OutputRootPreflight {
                path: value.to_string(),
                configured: true,
                ready: false,
                message: "Configured output root is invalid.".to_string(),
            };
        }
        output_root_preflight_for(&configured, true)
    })
}

fn credential_status_message(status: &CredentialStatus) -> String {
    if !status.configured {
        return format!("{} is not configured.", status.key);
    }
    match status.source {
        CredentialSource::Session => format!("{} is configured for this session.", status.key),
        CredentialSource::SecureStore => format!("{} is configured in secure storage.", status.key),
        CredentialSource::Env => {
            format!(
                "{} is configured through the development .env source.",
                status.key
            )
        }
        CredentialSource::Missing => format!("{} is not configured.", status.key),
    }
}

fn preflight_credential_status<S: CredentialStore>(
    store: &S,
    key: CredentialKey,
    session_value: Option<&str>,
    runtime_mode: RuntimeMode,
    environment: &EnvironmentValues,
) -> CredentialPreflight {
    match status_for(store, key, session_value, runtime_mode, environment) {
        Ok(status) => {
            let message = credential_status_message(&status);
            CredentialPreflight { status, message }
        }
        Err(error) => CredentialPreflight {
            status: CredentialStatus {
                key: key.env_name().to_string(),
                configured: false,
                source: CredentialSource::Missing,
            },
            message: error.to_string(),
        },
    }
}

#[cfg(test)]
fn runtime_preflight_for(
    runtime_mode: RuntimeMode,
    backend: BackendKind,
    sidecar: PreflightCheck,
    ffmpeg: PreflightCheck,
    output_root: OutputRootPreflight,
    mimo_api_key: CredentialPreflight,
    deepseek_api_key: CredentialPreflight,
) -> RuntimePreflight {
    let settings = ProviderSettingsInfo {
        local_asr_language: "zh".to_string(),
        local_vad_enabled: false,
        asr: ProviderSelection::default_asr(),
        llm: ProviderSelection::default_llm(),
    };
    runtime_preflight_for_configs(
        runtime_mode,
        backend,
        sidecar,
        ffmpeg,
        output_root,
        mimo_api_key,
        deepseek_api_key,
        settings,
        None,
    )
}

#[allow(clippy::too_many_arguments)]
fn runtime_preflight_for_configs(
    runtime_mode: RuntimeMode,
    backend: BackendKind,
    sidecar: PreflightCheck,
    ffmpeg: PreflightCheck,
    output_root: OutputRootPreflight,
    mimo_api_key: CredentialPreflight,
    deepseek_api_key: CredentialPreflight,
    settings: ProviderSettingsInfo,
    local_model_status: Option<LocalModelStatus>,
) -> RuntimePreflight {
    let asr_provider = provider_readiness(
        &settings.asr,
        mimo_api_key.status.configured,
        true,
        local_model_status.as_ref(),
    );
    let llm_provider = provider_readiness(
        &settings.llm,
        deepseek_api_key.status.configured,
        false,
        None,
    );
    let ready = sidecar.ready && ffmpeg.ready && output_root.ready && asr_provider.ready;
    RuntimePreflight {
        runtime_mode,
        backend,
        ready,
        ready_for_minutes: ready && llm_provider.ready,
        sidecar,
        ffmpeg,
        output_root,
        mimo_api_key,
        deepseek_api_key,
        asr_provider,
        llm_provider,
    }
}

fn get_runtime_preflight_blocking(
    app: AppHandle,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    backend: Option<String>,
) -> Result<RuntimePreflight, String> {
    let runtime_mode = RuntimeMode::current();
    let backend = resolve_backend_kind(backend.as_deref(), runtime_mode).map_err(String::from)?;
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    let sidecar = preflight_sidecar(runtime_mode, backend);
    let ffmpeg = match ffmpeg_binary_for(runtime_mode, None) {
        Ok(binary) => check_ffmpeg_at(&binary),
        Err(_) => preflight_check(false, "Could not resolve packaged FFmpeg."),
    };
    let output_root = output_root_preflight(&app).map_err(String::from)?;
    let store = KeyringCredentialStore;
    let mimo_api_key = preflight_credential_status(
        &store,
        CredentialKey::MimoApiKey,
        mimo_api_key.as_deref(),
        runtime_mode,
        &environment,
    );
    let deepseek_api_key = preflight_credential_status(
        &store,
        CredentialKey::DeepseekApiKey,
        deepseek_api_key.as_deref(),
        runtime_mode,
        &environment,
    );

    let settings = load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
        .map_err(String::from)?;
    let local_model_status = if settings.asr.provider_id == LOCAL_ASR_PROVIDER {
        Some(run_model_read(&app, "model-status", true).map_err(String::from)?)
    } else {
        None
    };
    Ok(runtime_preflight_for_configs(
        runtime_mode,
        backend,
        sidecar,
        ffmpeg,
        output_root,
        mimo_api_key,
        deepseek_api_key,
        ProviderSettingsInfo {
            local_asr_language: settings.local_asr_language,
            local_vad_enabled: settings.local_vad_enabled,
            asr: settings.asr,
            llm: settings.llm,
        },
        local_model_status,
    ))
}

#[tauri::command]
async fn get_runtime_preflight(
    app: AppHandle,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    backend: Option<String>,
) -> Result<RuntimePreflight, String> {
    tauri::async_runtime::spawn_blocking(move || {
        get_runtime_preflight_blocking(app, mimo_api_key, deepseek_api_key, backend)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Runtime preflight worker failed.".to_string(),
            code: None,
        })
    })?
}

const DESKTOP_SETTINGS_FILE: &str = "desktop-settings.json";

/// Deliberately non-sensitive settings. API keys, input history, transcripts,
/// and task content must never be added to this structure.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
// Reject unknown fields so unsupported or potentially sensitive values are
// never silently accepted and later persisted. Future schema changes require
// an explicit, reviewed migration rather than implicit forward compatibility.
struct DesktopSettings {
    auto_clean_chunks: bool,
    cleanup_temporary_on_startup: bool,
    local_asr_language: String,
    local_vad_enabled: bool,
    output_root: Option<String>,
    language: Option<String>,
    theme: Option<String>,
    asr: ProviderSelection,
    llm: ProviderSelection,
    development_backend: Option<BackendKind>,
    huggingface_endpoint: Option<String>,
}

impl Default for DesktopSettings {
    fn default() -> Self {
        Self {
            auto_clean_chunks: false,
            cleanup_temporary_on_startup: true,
            local_asr_language: "zh".to_string(),
            local_vad_enabled: false,
            output_root: None,
            language: None,
            theme: None,
            asr: ProviderSelection::default_asr(),
            llm: ProviderSelection::default_llm(),
            development_backend: None,
            huggingface_endpoint: None,
        }
    }
}

#[derive(Debug, Default, serde::Deserialize)]
#[serde(default, deny_unknown_fields)]
struct DesktopSettingsWire {
    auto_clean_chunks: Option<bool>,
    cleanup_temporary_on_startup: Option<bool>,
    local_asr_language: Option<String>,
    local_vad_enabled: Option<bool>,
    output_root: Option<String>,
    language: Option<String>,
    theme: Option<String>,
    asr: Option<ProviderSelection>,
    llm: Option<ProviderSelection>,
    // One-release read migration for settings written before provider ownership.
    deepseek_model: Option<String>,
    development_backend: Option<BackendKind>,
    huggingface_endpoint: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
struct OutputRootInfo {
    output_root: String,
    configured: bool,
    runtime_mode: RuntimeMode,
}

fn get_default_output_root(
    runtime_mode: RuntimeMode,
    documents_dir: Option<&Path>,
) -> Result<PathBuf, BackendError> {
    match runtime_mode {
        RuntimeMode::Development => Ok(resolve_project_root().join("output")),
        RuntimeMode::Production => {
            documents_dir
                .map(|dir| dir.join("Meeting ASR"))
                .ok_or_else(|| {
                    BackendError::InvalidInput {
                message:
                    "Could not resolve the user Documents directory for the production output root."
                        .to_string(),
            }
                })
        }
    }
}

fn desktop_settings_path(app: &AppHandle) -> Result<PathBuf, BackendError> {
    let config_dir = app
        .path()
        .app_config_dir()
        .map_err(|e| BackendError::InvalidInput {
            message: format!("Could not resolve the desktop settings directory: {e}"),
        })?;
    Ok(config_dir.join(DESKTOP_SETTINGS_FILE))
}

fn custom_templates_path(app: &AppHandle) -> Result<PathBuf, BackendError> {
    let config_dir = app
        .path()
        .app_config_dir()
        .map_err(|_| BackendError::InvalidInput {
            message: "Could not resolve custom template storage.".to_string(),
        })?;
    Ok(config_dir.join("templates").join("custom_templates.json"))
}

fn load_desktop_settings(path: &Path) -> Result<DesktopSettings, BackendError> {
    if !path.exists() {
        return Ok(DesktopSettings::default());
    }
    let content = fs::read_to_string(path).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not read desktop settings: {e}"),
    })?;
    let wire: DesktopSettingsWire =
        serde_json::from_str(&content).map_err(|e| BackendError::InvalidInput {
            message: format!("Desktop settings are invalid: {e}"),
        })?;
    let asr = wire.asr.unwrap_or_else(ProviderSelection::default_asr);
    let llm = wire.llm.unwrap_or_else(|| ProviderSelection {
        provider_id: DEFAULT_LLM_PROVIDER.to_string(),
        model_id: wire
            .deepseek_model
            .unwrap_or_else(|| DEFAULT_LLM_MODEL.to_string()),
    });
    validate_asr_selection(&asr)?;
    validate_llm_selection(&llm)?;
    let local_asr_language = wire.local_asr_language.unwrap_or_else(|| "zh".to_string());
    validate_local_asr_language(&local_asr_language)?;
    Ok(DesktopSettings {
        local_vad_enabled: wire.local_vad_enabled.unwrap_or(false),
        auto_clean_chunks: wire.auto_clean_chunks.unwrap_or(false),
        cleanup_temporary_on_startup: wire.cleanup_temporary_on_startup.unwrap_or(true),
        local_asr_language,
        output_root: wire.output_root,
        language: wire.language,
        theme: wire.theme,
        asr,
        llm,
        development_backend: wire.development_backend,
        huggingface_endpoint: wire.huggingface_endpoint,
    })
}

fn save_desktop_settings(path: &Path, settings: &DesktopSettings) -> Result<(), BackendError> {
    let parent = path.parent().ok_or_else(|| BackendError::InvalidInput {
        message: "Desktop settings path has no parent directory.".to_string(),
    })?;
    fs::create_dir_all(parent).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not create the desktop settings directory: {e}"),
    })?;
    let json = serde_json::to_string_pretty(settings).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not serialize desktop settings: {e}"),
    })?;
    let temporary = path.with_extension(format!("tmp-{}", std::process::id()));
    fs::write(&temporary, format!("{json}\n")).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not save desktop settings: {e}"),
    })?;
    fs::rename(&temporary, path).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not commit desktop settings: {e}"),
    })
}

#[tauri::command]
fn get_provider_settings(app: AppHandle) -> Result<ProviderSettingsInfo, String> {
    let settings = load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
        .map_err(String::from)?;
    Ok(ProviderSettingsInfo {
        local_asr_language: settings.local_asr_language,
        local_vad_enabled: settings.local_vad_enabled,
        asr: settings.asr,
        llm: settings.llm,
    })
}

fn validate_local_asr_language(language: &str) -> Result<(), BackendError> {
    match language {
        "zh" | "auto" => Ok(()),
        _ => Err(BackendError::InvalidInput {
            message: "Local ASR language must be zh or auto.".to_string(),
        }),
    }
}

fn set_provider_settings_at(
    path: &Path,
    active: &ActiveProcess,
    asr: ProviderSelection,
    llm: ProviderSelection,
    local_asr_language: String,
    local_vad_enabled: bool,
) -> Result<ProviderSettingsInfo, BackendError> {
    ensure_output_root_mutation_allowed(active)?;
    validate_asr_selection(&asr)?;
    validate_llm_selection(&llm)?;
    validate_local_asr_language(&local_asr_language)?;
    let mut settings = load_desktop_settings(path)?;
    settings.local_asr_language = local_asr_language.clone();
    settings.local_vad_enabled = local_vad_enabled;
    settings.asr = asr.clone();
    settings.llm = llm.clone();
    save_desktop_settings(path, &settings)?;
    Ok(ProviderSettingsInfo {
        asr,
        llm,
        local_asr_language,
        local_vad_enabled,
    })
}

#[tauri::command]
async fn set_provider_settings(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    asr: ProviderSelection,
    llm: ProviderSelection,
    local_asr_language: String,
    local_vad_enabled: bool,
) -> Result<ProviderSettingsInfo, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        let path = desktop_settings_path(&app).map_err(String::from)?;
        set_provider_settings_at(
            &path,
            &active,
            asr,
            llm,
            local_asr_language,
            local_vad_enabled,
        )
        .map_err(String::from)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Provider settings worker failed.".to_string(),
            code: None,
        })
    })?
}

fn validate_output_root(path: &Path) -> Result<PathBuf, BackendError> {
    if !path.is_absolute() {
        return Err(BackendError::InvalidInput {
            message: format!("Output root must be absolute: {}", path.display()),
        });
    }
    fs::create_dir_all(path).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not create output root '{}': {e}", path.display()),
    })?;
    let canonical = path
        .canonicalize()
        .map_err(|e| BackendError::InvalidInput {
            message: format!("Could not resolve output root '{}': {e}", path.display()),
        })?;
    if !canonical.is_dir() {
        return Err(BackendError::InvalidInput {
            message: format!("Output root is not a directory: {}", canonical.display()),
        });
    }
    Ok(canonical)
}

fn set_configured_output_root(
    settings_path: &Path,
    output_root: &Path,
) -> Result<PathBuf, BackendError> {
    let canonical = validate_selected_output_root(output_root)?;
    let mut settings = load_desktop_settings(settings_path)?;
    settings.output_root = Some(canonical.to_string_lossy().to_string());
    save_desktop_settings(settings_path, &settings)?;
    Ok(canonical)
}

fn validate_selected_output_root(path: &Path) -> Result<PathBuf, BackendError> {
    if path.as_os_str().is_empty() || !path.is_absolute() {
        return Err(BackendError::InvalidInput {
            message: "Output folder must be an absolute path.".to_string(),
        });
    }
    let canonical = path
        .canonicalize()
        .map_err(|_| BackendError::InvalidInput {
            message: "Output folder is unavailable.".to_string(),
        })?;
    if !canonical.is_dir() {
        return Err(BackendError::InvalidInput {
            message: "Output folder is not a directory.".to_string(),
        });
    }
    probe_output_root_writable(&canonical)?;
    Ok(canonical)
}

fn probe_output_root_writable(path: &Path) -> Result<(), BackendError> {
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let probe = path.join(format!(
        ".meeting-asr-write-probe-{}-{nonce}",
        std::process::id()
    ));
    let file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&probe)
        .map_err(|_| BackendError::InvalidInput {
            message: "Output folder is not writable.".to_string(),
        })?;
    drop(file);
    fs::remove_file(&probe).map_err(|_| BackendError::InvalidInput {
        message: "Output folder writability check could not be cleaned up.".to_string(),
    })?;
    Ok(())
}

fn reset_configured_output_root(settings_path: &Path) -> Result<(), BackendError> {
    let mut settings = load_desktop_settings(settings_path)?;
    settings.output_root = None;
    save_desktop_settings(settings_path, &settings)
}

fn select_output_root_path(
    settings: &DesktopSettings,
    runtime_mode: RuntimeMode,
    documents_dir: Option<&Path>,
) -> Result<(PathBuf, bool), BackendError> {
    if let Some(configured) = configured_output_root_path(settings)? {
        return Ok((configured, true));
    }
    get_default_output_root(runtime_mode, documents_dir).map(|root| (root, false))
}

fn resolve_output_root_path(app: &AppHandle) -> Result<(PathBuf, bool), BackendError> {
    let settings_path = desktop_settings_path(app)?;
    let settings = load_desktop_settings(&settings_path)?;
    let runtime_mode = RuntimeMode::current();
    let documents_dir = if runtime_mode == RuntimeMode::Production {
        Some(
            app.path()
                .document_dir()
                .map_err(|e| BackendError::InvalidInput {
                    message: format!("Could not resolve the user Documents directory: {e}"),
                })?,
        )
    } else {
        None
    };
    select_output_root_path(&settings, runtime_mode, documents_dir.as_deref())
}

fn resolve_output_root(app: &AppHandle) -> Result<(PathBuf, bool), BackendError> {
    let (path, configured) = resolve_output_root_path(app)?;
    validate_output_root(&path).map(|root| (root, configured))
}

fn validate_task_library_output_root(
    path: &Path,
    configured: bool,
) -> Result<Option<PathBuf>, BackendError> {
    let canonical = match path.canonicalize() {
        Ok(canonical) => canonical,
        Err(error) if !configured && error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(None);
        }
        Err(_) => {
            return Err(BackendError::InvalidInput {
                message: "Task Library output root is unavailable.".to_string(),
            });
        }
    };
    if !canonical.is_dir() {
        return Err(BackendError::InvalidInput {
            message: "Task Library output root is unavailable.".to_string(),
        });
    }
    Ok(Some(canonical))
}

fn resolve_task_library_output_root(app: &AppHandle) -> Result<Option<PathBuf>, BackendError> {
    let (path, configured) = resolve_output_root_path(app)?;
    validate_task_library_output_root(&path, configured)
}

#[tauri::command]
fn get_output_root(app: AppHandle) -> Result<OutputRootInfo, String> {
    let runtime_mode = RuntimeMode::current();
    let (output_root, configured) = resolve_output_root_path(&app).map_err(String::from)?;
    Ok(OutputRootInfo {
        output_root: output_root.to_string_lossy().to_string(),
        configured,
        runtime_mode,
    })
}

fn set_output_root_blocking(
    app: AppHandle,
    active: ActiveProcess,
    output_root: String,
) -> Result<OutputRootInfo, String> {
    ensure_output_root_mutation_allowed(&active).map_err(String::from)?;
    let settings_path = desktop_settings_path(&app).map_err(String::from)?;
    let output_root = set_configured_output_root(&settings_path, Path::new(&output_root))
        .map_err(String::from)?;
    Ok(OutputRootInfo {
        output_root: output_root.to_string_lossy().to_string(),
        configured: true,
        runtime_mode: RuntimeMode::current(),
    })
}

#[tauri::command]
async fn set_output_root(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    output_root: String,
) -> Result<OutputRootInfo, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || set_output_root_blocking(app, active, output_root))
        .await
        .map_err(|_| output_root_worker_error())?
}

fn reset_output_root_blocking(
    app: AppHandle,
    active: ActiveProcess,
) -> Result<OutputRootInfo, String> {
    ensure_output_root_mutation_allowed(&active).map_err(String::from)?;
    let settings_path = desktop_settings_path(&app).map_err(String::from)?;
    reset_configured_output_root(&settings_path).map_err(String::from)?;
    let runtime_mode = RuntimeMode::current();
    let (output_root, configured) = resolve_output_root_path(&app).map_err(String::from)?;
    Ok(OutputRootInfo {
        output_root: output_root.to_string_lossy().to_string(),
        configured,
        runtime_mode,
    })
}

#[tauri::command]
async fn reset_output_root(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
) -> Result<OutputRootInfo, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || reset_output_root_blocking(app, active))
        .await
        .map_err(|_| output_root_worker_error())?
}

fn output_root_worker_error() -> String {
    String::from(BackendError::ProcessFailed {
        message: "Output folder worker failed.".to_string(),
        code: None,
    })
}

fn validate_output_root_for_open(path: &Path) -> Result<PathBuf, BackendError> {
    let canonical = path
        .canonicalize()
        .map_err(|_| BackendError::InvalidInput {
            message: "Output folder is unavailable.".to_string(),
        })?;
    if !canonical.is_dir() {
        return Err(BackendError::InvalidInput {
            message: "Output folder is unavailable.".to_string(),
        });
    }
    Ok(canonical)
}

#[cfg(target_os = "macos")]
fn perform_open_output_root(path: &Path) -> Result<(), BackendError> {
    perform_open_output_root_with("open", path)
}

#[cfg(target_os = "windows")]
fn perform_open_output_root(path: &Path) -> Result<(), BackendError> {
    perform_open_output_root_with("explorer.exe", path)
}

#[cfg(target_os = "linux")]
fn perform_open_output_root(path: &Path) -> Result<(), BackendError> {
    perform_open_output_root_with("xdg-open", path)
}

fn perform_open_output_root_with(program: &str, path: &Path) -> Result<(), BackendError> {
    let status =
        Command::new(program)
            .arg(path)
            .status()
            .map_err(|_| BackendError::InvalidInput {
                message: "Could not open the output folder.".to_string(),
            })?;
    if !status.success() {
        return Err(BackendError::InvalidInput {
            message: "Could not open the output folder.".to_string(),
        });
    }
    Ok(())
}

fn open_output_root_blocking(app: AppHandle) -> Result<(), String> {
    let (path, _) = resolve_output_root_path(&app).map_err(String::from)?;
    let canonical = validate_output_root_for_open(&path).map_err(String::from)?;
    perform_open_output_root(&canonical).map_err(String::from)
}

#[tauri::command]
async fn open_output_root(app: AppHandle) -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(move || open_output_root_blocking(app))
        .await
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Output folder worker failed.".to_string(),
                code: None,
            })
        })?
}

// ---------------------------------------------------------------------------
// Secure credential commands and run-time resolution
// ---------------------------------------------------------------------------

struct RunCredentials {
    mimo: String,
    deepseek: Option<String>,
}

fn credential_environment(runtime_mode: RuntimeMode) -> Result<EnvironmentValues, BackendError> {
    if runtime_mode == RuntimeMode::Production {
        // Release runs deliberately do not inspect process environment or the
        // project-root .env. Session values and the secure store are the only
        // production credential sources.
        return Ok(EnvironmentValues::default());
    }

    let env_path = find_env_file();
    EnvironmentValues::for_development(env_path.as_deref()).map_err(BackendError::from)
}

fn missing_api_key(key: CredentialKey) -> BackendError {
    BackendError::MissingApiKey {
        message: format!(
            "{} is not configured. Configure it in desktop credentials or for this session.",
            key.env_name()
        ),
    }
}

fn resolve_run_credentials<S: CredentialStore>(
    store: &S,
    output_mode: &str,
    asr_provider: &str,
    runtime_mode: RuntimeMode,
    mimo_session: Option<&str>,
    deepseek_session: Option<&str>,
    environment: &EnvironmentValues,
) -> Result<RunCredentials, BackendError> {
    // Offline ASR must neither require nor read a cloud speech credential.
    let mimo = if asr_provider == LOCAL_ASR_PROVIDER {
        String::new()
    } else {
        let key = CredentialKey::MimoApiKey;
        resolve_credential(store, key, mimo_session, runtime_mode, environment)?
            .ok_or_else(|| missing_api_key(key))?
            .value
    };

    let should_resolve_deepseek = output_mode == "minutes"
        || output_mode == "transcript_and_generate"
        || deepseek_session.is_some_and(|value| !value.trim().is_empty());
    let deepseek = if should_resolve_deepseek {
        let key = CredentialKey::DeepseekApiKey;
        let resolved = resolve_credential(store, key, deepseek_session, runtime_mode, environment)?;
        if output_mode == "minutes" || output_mode == "transcript_and_generate" {
            Some(resolved.ok_or_else(|| missing_api_key(key))?.value)
        } else {
            resolved.map(|credential| credential.value)
        }
    } else {
        None
    };

    Ok(RunCredentials { mimo, deepseek })
}

fn validate_local_asr_start(
    _backend: BackendKind,
    status: &LocalModelStatus,
) -> Result<(), BackendError> {
    if status.model_id != LOCAL_ASR_MODEL || status.state != "ready" {
        return Err(BackendError::InvalidInput {
            message: format!("Local speech recognition is not ready: {}", status.message),
        });
    }
    Ok(())
}

fn parse_credential_key(value: &str) -> Result<CredentialKey, String> {
    CredentialKey::parse(value).map_err(|error| String::from(BackendError::from(error)))
}

#[tauri::command]
fn get_credential_status(key: String) -> Result<CredentialStatus, String> {
    let key = parse_credential_key(&key)?;
    let runtime_mode = RuntimeMode::current();
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    status_for(
        &KeyringCredentialStore,
        key,
        None,
        runtime_mode,
        &environment,
    )
    .map_err(|error| String::from(BackendError::from(error)))
}

#[tauri::command]
fn save_credential(key: String, value: String) -> Result<CredentialStatus, String> {
    let key = parse_credential_key(&key)?;
    let store = KeyringCredentialStore;
    store
        .save(key, value.trim())
        .map_err(|error| String::from(BackendError::from(error)))?;

    let runtime_mode = RuntimeMode::current();
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    status_for(&store, key, None, runtime_mode, &environment)
        .map_err(|error| String::from(BackendError::from(error)))
}

#[tauri::command]
fn delete_credential(key: String) -> Result<CredentialStatus, String> {
    let key = parse_credential_key(&key)?;
    let store = KeyringCredentialStore;
    store
        .delete(key)
        .map_err(|error| String::from(BackendError::from(error)))?;

    let runtime_mode = RuntimeMode::current();
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    status_for(&store, key, None, runtime_mode, &environment)
        .map_err(|error| String::from(BackendError::from(error)))
}

// ---------------------------------------------------------------------------
// Path validation
// ---------------------------------------------------------------------------

fn canonicalize_existing_absolute(path: &Path, label: &str) -> Result<PathBuf, String> {
    if !path.is_absolute() {
        return Err(format!("{label} must be absolute: {}", path.display()));
    }
    path.canonicalize().map_err(|e| {
        format!(
            "{label} does not exist or is inaccessible: {} ({e})",
            path.display()
        )
    })
}

/// Canonical containment prevents `..` traversal and symlink escapes.
fn canonicalize_path_within(root: &Path, path: &Path, label: &str) -> Result<PathBuf, String> {
    let canonical = canonicalize_existing_absolute(path, label)?;
    if !canonical.starts_with(root) {
        return Err(format!(
            "{label} escapes its allowed root: {}",
            path.display()
        ));
    }
    Ok(canonical)
}

fn canonicalize_abs(path: &str, label: &str) -> Result<PathBuf, BackendError> {
    canonicalize_existing_absolute(Path::new(path), label)
        .map_err(|message| BackendError::InvalidInput { message })
}

fn canonicalize_abs_file(path: &str, label: &str) -> Result<PathBuf, BackendError> {
    let p = canonicalize_abs(path, label)?;
    if !p.is_file() {
        return Err(BackendError::InvalidInput {
            message: format!("{} is not a regular file: {}", label, path),
        });
    }
    Ok(p)
}

fn canonicalize_abs_dir(path: &str, label: &str) -> Result<PathBuf, BackendError> {
    let p = canonicalize_abs(path, label)?;
    if !p.is_dir() {
        return Err(BackendError::InvalidInput {
            message: format!("{} is not a directory: {}", label, path),
        });
    }
    Ok(p)
}

// ---------------------------------------------------------------------------
// Go binary discovery
// ---------------------------------------------------------------------------

const GO_SIDECAR_NAME: &str = "meeting-asr-go";

fn go_binary_filename() -> &'static str {
    if cfg!(target_os = "windows") {
        "meeting-asr-go.exe"
    } else {
        GO_SIDECAR_NAME
    }
}

fn canonicalize_go_binary(
    candidate: &Path,
    not_found_message: String,
) -> Result<PathBuf, BackendError> {
    let canonical = candidate
        .canonicalize()
        .map_err(|error| BackendError::BackendNotFound {
            message: format!("{not_found_message} ({error})"),
        })?;
    if !canonical.is_file() {
        return Err(BackendError::BackendNotFound {
            message: format!(
                "{not_found_message}: {} is not a regular file.",
                candidate.display()
            ),
        });
    }
    Ok(canonical)
}

fn discover_development_go_binary() -> Result<PathBuf, BackendError> {
    let override_path = std::env::var("MEETING_ASR_GO_BINARY").ok();
    discover_development_go_binary_from(
        override_path.as_deref().filter(|path| !path.is_empty()),
        &resolve_project_root(),
    )
}

fn discover_development_go_binary_from(
    override_path: Option<&str>,
    project_root: &Path,
) -> Result<PathBuf, BackendError> {
    if let Some(p) = override_path {
        let path = Path::new(p);
        if !path.is_absolute() {
            return Err(BackendError::BackendNotFound {
                message: format!("MEETING_ASR_GO_BINARY must be an absolute path, got: {p}"),
            });
        }
        return canonicalize_go_binary(
            path,
            format!("Go binary override was not found at {}", path.display()),
        );
    }

    let candidate = project_root.join("core-go").join(go_binary_filename());
    canonicalize_go_binary(
        &candidate,
        format!(
            "Go development binary not found at {}. Build it with: cd core-go && go build -o meeting-asr-go .\n\
             Or set MEETING_ASR_GO_BINARY to the absolute path of the binary",
            candidate.display()
        ),
    )
}

fn discover_production_go_binary(executable_dir: &Path) -> Result<PathBuf, BackendError> {
    let candidate = executable_dir.join(go_binary_filename());
    canonicalize_go_binary(
        &candidate,
        format!(
            "Packaged Go sidecar not found at {}. Build the sidecar with ./scripts/build_desktop_sidecar.sh and rebuild the desktop app",
            candidate.display()
        ),
    )
}

fn discover_go_binary(
    runtime_mode: RuntimeMode,
    packaged_executable_dir: Option<&Path>,
) -> Result<PathBuf, BackendError> {
    match runtime_mode {
        RuntimeMode::Development => discover_development_go_binary(),
        RuntimeMode::Production => {
            let executable_dir =
                packaged_executable_dir.ok_or_else(|| BackendError::BackendNotFound {
                    message:
                        "Could not resolve the packaged application directory for the Go sidecar."
                            .to_string(),
                })?;
            discover_production_go_binary(executable_dir)
        }
    }
}

fn packaged_go_executable_dir() -> Result<PathBuf, BackendError> {
    // Tauri v2's macOS bundler places external binaries in Contents/MacOS
    // beside the application executable. PathResolver::executable_dir() is
    // unsupported on macOS, so use the same current-executable-parent
    // strategy as Tauri's shell sidecar resolver.
    let executable = std::env::current_exe().map_err(|error| BackendError::BackendNotFound {
        message: format!("Could not resolve the running application path: {error}"),
    })?;
    executable
        .parent()
        .map(Path::to_path_buf)
        .ok_or_else(|| BackendError::BackendNotFound {
            message: "Could not resolve the running application directory for the Go sidecar."
                .to_string(),
        })
}

fn validate_go_binary(binary: &Path) -> Result<(), BackendError> {
    let output = Command::new(binary).arg("version").output().map_err(|e| {
        BackendError::BackendNotFound {
            message: format!("Failed to run {}: {}", binary.display(), e),
        }
    })?;

    if !output.status.success() {
        return Err(BackendError::BackendNotFound {
            message: format!(
                "{} version check failed with exit code {:?}",
                binary.display(),
                output.status.code()
            ),
        });
    }

    let stdout = String::from_utf8_lossy(&output.stdout);
    if !stdout.contains("meeting-asr-go") {
        return Err(BackendError::BackendNotFound {
            message: format!(
                "{} output does not contain 'meeting-asr-go' — wrong binary? Output: {}",
                binary.display(),
                stdout.trim()
            ),
        });
    }

    Ok(())
}

// ---------------------------------------------------------------------------
// Env file discovery
// ---------------------------------------------------------------------------

fn find_env_file() -> Option<PathBuf> {
    let root = resolve_project_root();
    let candidate = root.join(".env");
    if candidate.is_file() {
        return Some(candidate);
    }
    None
}

// ---------------------------------------------------------------------------
// Shared process runner
// ---------------------------------------------------------------------------

/// ActiveRun is the one child process this app currently owns. Its identity binds
/// every control action to that exact run, so a stop request or a watchdog that
/// outlives its run can never act on a later one.
struct ActiveRun {
    id: u64,
    child: Child,
    control: Option<ChildStdin>,
}

/// NEXT_RUN_ID hands out process-unique run ids.
static NEXT_RUN_ID: AtomicU64 = AtomicU64::new(1);

#[derive(Clone, Default)]
struct ActiveProcess {
    run: Arc<Mutex<Option<ActiveRun>>>,
    stopping: Arc<Mutex<Option<u64>>>,
    model_operation: Arc<AtomicBool>,
}

/// CancelOutcome reports what a stop request did.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct CancelOutcome {
    /// "idle" when nothing was running, "stopping" when a stop was requested.
    state: String,
}

impl ActiveProcess {
    fn is_active(&self) -> bool {
        match self.run.lock() {
            Ok(guard) => guard.is_some(),
            Err(_) => true,
        }
    }

    /// is_run_active reports whether one specific run still owns the slot.
    fn is_run_active(&self, run_id: u64) -> bool {
        matches!(self.run.lock(), Ok(guard) if guard.as_ref().is_some_and(|run| run.id == run_id))
    }

    /// is_stopping reports whether a cooperative stop is in progress. The UI
    /// shows "stopping" instead of letting a new operation start on top of the
    /// process that is still winding down.
    fn is_stopping(&self) -> bool {
        matches!(self.stopping.lock(), Ok(guard) if guard.is_some())
    }

    /// register installs a freshly spawned child, refusing a second one.
    fn register(&self, mut child: Child, control: Option<ChildStdin>) -> Result<u64, BackendError> {
        let mut guard = self.run.lock().unwrap();
        if guard.is_some() {
            let _ = child.kill();
            // A run that is still winding down must not be overlapped by a new
            // one; the distinct message lets the UI say "still stopping".
            let message = if self.is_stopping() {
                "The previous operation is still stopping.".to_string()
            } else {
                "A meeting transcription process is already running.".to_string()
            };
            return Err(BackendError::ProcessFailed {
                message,
                code: None,
            });
        }
        let id = NEXT_RUN_ID.fetch_add(1, Ordering::AcqRel);
        *guard = Some(ActiveRun { id, child, control });
        *self.stopping.lock().unwrap() = None;
        Ok(id)
    }

    /// finish clears the run slot once its child has exited. A stale id (a run
    /// that was already replaced) is ignored instead of clearing a later run.
    fn finish(&self, run_id: u64) {
        {
            let mut guard = self.run.lock().unwrap();
            if guard.as_ref().is_some_and(|run| run.id == run_id) {
                *guard = None;
            }
        }
        let mut stopping = self.stopping.lock().unwrap();
        if *stopping == Some(run_id) {
            *stopping = None;
        }
    }

    /// request_stop asks the running process to stop and schedules a forced stop
    /// when it has not exited after the grace period.
    fn request_stop(&self) -> CancelOutcome {
        // The run identity is captured under the same lock as the control write,
        // so the watchdog below can only ever act on the run it was asked about.
        let run_id = {
            let mut guard = self.run.lock().unwrap();
            let run = match guard.as_mut() {
                Some(run) => run,
                None => {
                    return CancelOutcome {
                        state: "idle".to_string(),
                    }
                }
            };
            let mut stopping = self.stopping.lock().unwrap();
            if *stopping == Some(run.id) {
                return CancelOutcome {
                    state: "stopping".to_string(),
                };
            }
            *stopping = Some(run.id);
            // Cooperative phase: the sidecar stops at the next step boundary,
            // aborts in-flight requests, and terminates the ffmpeg it owns.
            // Without a control channel the watchdog is the only stop.
            if let Some(stdin) = run.control.as_mut() {
                let _ = stdin.write_all(b"cancel\n");
                let _ = stdin.flush();
            }
            run.id
        };

        let active = self.clone();
        thread::spawn(move || {
            let deadline = Instant::now() + process_control::GRACE_PERIOD;
            while Instant::now() < deadline {
                if !active.is_run_active(run_id) {
                    // This watchdog's run already exited; a later run must never
                    // be stopped by it.
                    return;
                }
                thread::sleep(Duration::from_millis(50));
            }
            active.terminate_run(run_id);
        });

        CancelOutcome {
            state: "stopping".to_string(),
        }
    }

    /// terminate_run force-stops one specific run. The identity check and the
    /// stop happen under the same lock, so no replacement run can be killed.
    fn terminate_run(&self, run_id: u64) -> bool {
        let mut guard = self.run.lock().unwrap();
        match guard.as_mut() {
            Some(run) if run.id == run_id => {
                let _ = process_control::force_stop(&mut run.child);
                true
            }
            _ => false,
        }
    }

    /// terminate stops the current run's process tree immediately. It is the
    /// last resort on the app-exit path.
    fn terminate(&self) {
        let run_id = match self.run.lock() {
            Ok(guard) => guard.as_ref().map(|run| run.id),
            Err(_) => None,
        };
        if let Some(run_id) = run_id {
            self.terminate_run(run_id);
        }
    }

    fn cancel_model_operation(&self) -> CancelOutcome {
        if !self.model_operation.load(Ordering::Acquire) {
            return CancelOutcome {
                state: "idle".to_string(),
            };
        }
        // Model work carries the same control channel as every other run, so it
        // stops cooperatively first and is only forced after the grace period.
        self.request_stop()
    }
}

struct ModelOperationGuard(Arc<AtomicBool>);
impl Drop for ModelOperationGuard {
    fn drop(&mut self) {
        self.0.store(false, Ordering::Release);
    }
}

fn ensure_output_root_mutation_allowed(active: &ActiveProcess) -> Result<(), BackendError> {
    validate_output_root_mutation_state(active.is_active())
}

fn validate_output_root_mutation_state(is_active: bool) -> Result<(), BackendError> {
    if is_active {
        return Err(BackendError::InvalidInput {
            message: "Cannot change output folder while processing.".to_string(),
        });
    }
    Ok(())
}

struct ProcessRunner {
    active: ActiveProcess,
    run_id: u64,
    stdout_lines: Arc<Mutex<Vec<String>>>,
    stderr_lines: Arc<Mutex<Vec<String>>>,
    stdout_handle: Option<JoinHandle<()>>,
    stderr_handle: Option<JoinHandle<()>>,
}

/// Replace credential values before they cross the Rust/frontend log or error
/// boundary. Values are passed to child processes through their environment,
/// never through argv, but a backend could still echo them on stdout/stderr.
fn redact_secrets(text: &str, secrets: &[String]) -> String {
    secrets
        .iter()
        .filter(|secret| !secret.is_empty())
        .fold(text.to_string(), |redacted, secret| {
            redacted.replace(secret, "[REDACTED]")
        })
}

fn redact_lines(lines: &[String], secrets: &[String]) -> Vec<String> {
    lines
        .iter()
        .map(|line| redact_secrets(line, secrets))
        .collect()
}

fn spawn_and_watch(
    app: &AppHandle,
    active: &ActiveProcess,
    mut cmd: Command,
    secrets: &[String],
    control_stdin: bool,
) -> Result<ProcessRunner, BackendError> {
    if control_stdin {
        // The control channel doubles as an ownership signal: when the desktop
        // disappears, the sidecar sees end-of-file and stops instead of running
        // unattended.
        cmd.stdin(Stdio::piped());
    }
    process_control::isolate_process_group(&mut cmd);
    let mut child = cmd
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| {
            if e.kind() == std::io::ErrorKind::NotFound {
                BackendError::BackendNotFound {
                    message: format!("Command not found: {:?}", cmd.get_program()),
                }
            } else {
                BackendError::ProcessFailed {
                    message: format!("Failed to start process: {}", e),
                    code: None,
                }
            }
        })?;

    let stdout_stream = child.stdout.take().unwrap();
    let stderr_stream = child.stderr.take().unwrap();
    let control = child.stdin.take();
    let run_id = active.register(child, control)?;

    let stdout_lines = Arc::new(Mutex::new(Vec::new()));
    let stderr_lines = Arc::new(Mutex::new(Vec::new()));
    let stdout_for_thread = stdout_lines.clone();
    let stderr_for_thread = stderr_lines.clone();
    let stdout_secrets = secrets.to_vec();
    let stderr_secrets = secrets.to_vec();

    let app_stdout = app.clone();
    let stdout_handle: JoinHandle<()> = thread::spawn(move || {
        let reader = BufReader::new(stdout_stream);
        for line in reader.lines() {
            match line {
                Ok(text) => {
                    let safe_text = redact_secrets(&text, &stdout_secrets);
                    let trimmed = safe_text.trim();

                    // Status: lines → progress events with normalized status.
                    if let Some(status_val) = trimmed.strip_prefix("Status: ") {
                        let raw = status_val.trim();
                        let normalized = normalize_status(raw);
                        let _ = app_stdout.emit(
                            "progress",
                            ProgressEvent {
                                status: normalized.to_string(),
                                current: 0,
                                total: 0,
                            },
                        );
                    }
                    // Progress: N/M → transcribing with numeric counts.
                    else if let Some(progress_str) = trimmed.strip_prefix("Progress: ") {
                        if let Some((current, total)) = parse_progress(progress_str) {
                            let _ = app_stdout.emit(
                                "progress",
                                ProgressEvent {
                                    status: "transcribing".to_string(),
                                    current,
                                    total,
                                },
                            );
                        }
                    }

                    let _ = app_stdout.emit(
                        "log-line",
                        LogEvent {
                            stream: "stdout".into(),
                            line: safe_text.clone(),
                        },
                    );
                    stdout_for_thread.lock().unwrap().push(safe_text);
                }
                Err(_) => break,
            }
        }
    });

    let app_stderr = app.clone();
    let stderr_handle: JoinHandle<()> = thread::spawn(move || {
        let reader = BufReader::new(stderr_stream);
        for line in reader.lines() {
            match line {
                Ok(text) => {
                    let safe_text = redact_secrets(&text, &stderr_secrets);
                    if let Some(value) = safe_text.trim().strip_prefix("Model-Progress: ") {
                        let parts = value.split_whitespace().collect::<Vec<_>>();
                        if parts.len() == 3 {
                            if let (Ok(completed), Ok(total)) = (parts[1].parse(), parts[2].parse())
                            {
                                let _ = app_stderr.emit(
                                    "model-progress",
                                    ModelProgressEvent {
                                        state: parts[0].to_string(),
                                        completed,
                                        total,
                                    },
                                );
                            }
                        }
                    }
                    let _ = app_stderr.emit(
                        "log-line",
                        LogEvent {
                            stream: "stderr".into(),
                            line: safe_text.clone(),
                        },
                    );
                    stderr_for_thread.lock().unwrap().push(safe_text);
                }
                Err(_) => break,
            }
        }
    });

    Ok(ProcessRunner {
        active: active.clone(),
        run_id,
        stdout_lines,
        stderr_lines,
        stdout_handle: Some(stdout_handle),
        stderr_handle: Some(stderr_handle),
    })
}

impl ProcessRunner {
    #[allow(clippy::type_complexity)]
    fn wait(mut self) -> Result<(Option<i32>, Vec<String>, Vec<String>), BackendError> {
        let status = loop {
            let status = {
                let mut guard = self.active.run.lock().unwrap();
                let run = guard.as_mut().ok_or_else(|| BackendError::Interrupted {
                    message: "Process was no longer available.".to_string(),
                })?;
                run.child
                    .try_wait()
                    .map_err(|e| BackendError::ProcessFailed {
                        message: format!("Process error: {}", e),
                        code: None,
                    })?
            };
            if let Some(status) = status {
                break status;
            }
            thread::sleep(Duration::from_millis(50));
        };

        let _ = self.stdout_handle.take().map(|h| h.join());
        let _ = self.stderr_handle.take().map(|h| h.join());

        let code = status.code();
        let stdout = self.stdout_lines.lock().unwrap().clone();
        let stderr = self.stderr_lines.lock().unwrap().clone();
        self.active.finish(self.run_id);

        Ok((code, stdout, stderr))
    }
}

// ---------------------------------------------------------------------------
// Command construction helpers
// ---------------------------------------------------------------------------

fn isolated_go_project_root() -> Result<PathBuf, BackendError> {
    let root = std::env::temp_dir().join(format!("meeting-asr-runtime-{}", std::process::id()));
    fs::create_dir_all(&root).map_err(|error| BackendError::InvalidInput {
        message: format!("Could not create the isolated Go runtime directory: {error}"),
    })?;
    // The Go core applies process-environment overrides while loading settings
    // only after it has found a .env file. This empty marker contains no
    // credentials; it keeps secure/session values environment-only while
    // preventing upward search from reaching the checkout's .env.
    fs::write(root.join(".env"), b"").map_err(|error| BackendError::InvalidInput {
        message: format!("Could not initialize the isolated Go runtime directory: {error}"),
    })?;
    Ok(root)
}

#[allow(clippy::too_many_arguments)]
fn build_go_command(
    audio_path: &str,
    output_mode: &str,
    output_dir: &str,
    mimo_key: Option<&str>,
    deepseek_key: Option<&str>,
    providers: &ProviderSettingsInfo,
    env_file: Option<&PathBuf>,
    runtime_mode: RuntimeMode,
    packaged_executable_dir: Option<&Path>,
    template_id: Option<&str>,
) -> Result<Command, BackendError> {
    let binary = discover_go_binary(runtime_mode, packaged_executable_dir)?;
    validate_go_binary(&binary)?;

    let mut cmd = Command::new(binary);
    cmd.arg("transcribe")
        .arg(audio_path)
        .arg("--output-mode")
        .arg(output_mode)
        .arg("--output-dir")
        .arg(output_dir);
    if let Some(template_id) = template_id {
        cmd.arg("--template-id").arg(template_id);
    }
    if providers.asr.provider_id == LOCAL_ASR_PROVIDER {
        cmd.arg(format!("--vad={}", providers.local_vad_enabled));
    }
    cmd.arg("--local-asr-language")
        .arg(&providers.local_asr_language);
    cmd.arg("--asr-provider")
        .arg(&providers.asr.provider_id)
        .arg("--asr-model")
        .arg(&providers.asr.model_id)
        .arg("--llm-provider")
        .arg(&providers.llm.provider_id)
        .arg("--llm-model")
        .arg(&providers.llm.model_id)
        // The run listens on the control channel, so the desktop can stop it and
        // an app exit closes the channel instead of leaving the work running.
        .arg("--control-stdin");

    if runtime_mode == RuntimeMode::Development {
        if let Some(env) = env_file {
            cmd.arg("--env-file").arg(env);
        } else {
            let isolated_root = isolated_go_project_root()?;
            cmd.arg("--project-root").arg(&isolated_root);
            cmd.current_dir(&isolated_root);
        }
    } else {
        // Go searches upward for .env when --env-file is absent. Point it at a
        // dedicated temp root so release runs cannot discover the checkout's
        // project-root .env. No credential is written to this directory.
        let isolated_root = isolated_go_project_root()?;
        cmd.arg("--project-root").arg(&isolated_root);
        cmd.current_dir(&isolated_root);
    }

    cmd.env_remove("MIMO_API_KEY");
    cmd.env_remove("DEEPSEEK_API_KEY");
    if let Some(key) = mimo_key {
        if !key.is_empty() {
            cmd.env("MIMO_API_KEY", key);
        }
    }
    if let Some(key) = deepseek_key {
        if !key.is_empty() {
            cmd.env("DEEPSEEK_API_KEY", key);
        }
    }
    Ok(cmd)
}

// ---------------------------------------------------------------------------
// Shared exec-and-check helper
// ---------------------------------------------------------------------------

fn run_and_check(
    app: &AppHandle,
    active: &ActiveProcess,
    cmd: Command,
    output_mode: &str,
    expected_task_root: &Path,
    secrets: &[String],
) -> Result<RunResult, String> {
    let runner = spawn_and_watch(app, active, cmd, secrets, true).map_err(String::from)?;
    let (code, stdout_lines, stderr_lines) = runner.wait()?;

    // Always emit process-done so the UI stops its progress view.
    let _ = app.emit("process-done", DoneEvent { code });

    match code {
        None => Err(BackendError::Interrupted {
            message: "Process was terminated by a signal.".to_string(),
        }
        .into()),
        Some(0) => {
            // Exit 0: validate output contract.
            let output = ParsedOutput::parse_lines(&stdout_lines);
            if let Err(msg) = output.validate_required_under(output_mode, Some(expected_task_root))
            {
                return Err(BackendError::ProcessFailed {
                    message: msg,
                    code: Some(0),
                }
                .into());
            }
            Ok(RunResult {
                task_folder: output.task_folder.unwrap(),
                transcript_path: output.transcript_path.unwrap(),
                notes_input_path: output.notes_input_path,
                minutes_path: output.minutes_path,
                generated_content_path: output.generated_content_path,
            })
        }
        Some(c) => Err(classify_process_failure(
            Some(c),
            &stderr_lines,
            secrets,
            "The task was cancelled before it finished.",
        )
        .into()),
    }
}

// ---------------------------------------------------------------------------
// Main command
// ---------------------------------------------------------------------------

#[allow(clippy::too_many_arguments)]
fn run_meeting_asr_blocking(
    app: AppHandle,
    active: ActiveProcess,
    audio_path: String,
    output_mode: String,
    backend: Option<String>,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    template_id: Option<String>,
    asr_config: ProviderSelection,
    llm_config: Option<ProviderSelection>,
) -> Result<RunResult, String> {
    let runtime_mode = RuntimeMode::current();

    // Validate output mode.
    if output_mode != "transcript_only" && output_mode != "transcript_and_generate" {
        return Err(BackendError::InvalidInput {
            message: format!(
                "Invalid output_mode: {}. Must be 'transcript_only' or 'transcript_and_generate'.",
                output_mode
            ),
        }
        .into());
    }

    // Validate and canonicalize audio path.
    let ext = Path::new(&audio_path)
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_lowercase();
    if ext != "mp3" && ext != "wav" && ext != "m4a" {
        return Err(BackendError::InvalidInput {
            message: "Only .mp3, .wav, and .m4a audio files are supported.".into(),
        }
        .into());
    }
    let abs_audio = canonicalize_abs_file(&audio_path, "audio_path").map_err(String::from)?;
    let abs_audio_str = abs_audio.to_string_lossy().to_string();

    // Resolve backend.
    let backend_kind =
        resolve_backend_kind(backend.as_deref(), runtime_mode).map_err(String::from)?;
    let packaged_executable_dir =
        if backend_kind == BackendKind::Go && runtime_mode == RuntimeMode::Production {
            Some(packaged_go_executable_dir().map_err(String::from)?)
        } else {
            None
        };
    if backend_kind == BackendKind::Go && runtime_mode == RuntimeMode::Production {
        discover_go_binary(runtime_mode, packaged_executable_dir.as_deref())
            .map_err(String::from)?;
    }

    // Resolve the runtime-aware output root, creating it when needed.
    let (abs_out, _) = resolve_output_root(&app).map_err(String::from)?;
    let out_dir = abs_out.to_string_lossy().to_string();
    let desktop_settings =
        load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
            .map_err(String::from)?;
    validate_asr_selection(&asr_config).map_err(String::from)?;
    if asr_config != desktop_settings.asr {
        return Err(String::from(BackendError::InvalidInput {
            message: "Speech-recognition settings changed. Refresh and try again.".to_string(),
        }));
    }
    if asr_config.provider_id == LOCAL_ASR_PROVIDER {
        let status = run_model_read(&app, "model-status", true).map_err(String::from)?;
        validate_local_asr_start(backend_kind, &status).map_err(String::from)?;
    }
    let effective_llm = llm_config.unwrap_or_else(|| desktop_settings.llm.clone());
    if output_mode == "transcript_and_generate" {
        validate_llm_selection(&effective_llm).map_err(String::from)?;
        if effective_llm != desktop_settings.llm {
            return Err(String::from(BackendError::InvalidInput {
                message: "Generated-content settings changed. Refresh and try again.".to_string(),
            }));
        }
    }
    let providers = ProviderSettingsInfo {
        local_asr_language: desktop_settings.local_asr_language.clone(),
        local_vad_enabled: desktop_settings.local_vad_enabled,
        asr: asr_config,
        llm: effective_llm,
    };

    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    let credentials = resolve_run_credentials(
        &KeyringCredentialStore,
        &output_mode,
        &providers.asr.provider_id,
        runtime_mode,
        mimo_api_key.as_deref(),
        deepseek_api_key.as_deref(),
        &environment,
    )
    .map_err(String::from)?;
    let env_file = (runtime_mode == RuntimeMode::Development)
        .then(find_env_file)
        .flatten();
    let mut cmd = match backend_kind {
        BackendKind::Go => build_go_command(
            &abs_audio_str,
            &output_mode,
            &out_dir,
            Some(&credentials.mimo),
            credentials.deepseek.as_deref(),
            &providers,
            env_file.as_ref(),
            runtime_mode,
            packaged_executable_dir.as_deref(),
            template_id.as_deref(),
        )
        .map_err(String::from)?,
    };
    if backend_kind == BackendKind::Go {
        cmd.arg(format!(
            "--auto-clean-chunks={}",
            desktop_settings.auto_clean_chunks
        ));
        cmd.arg("--app-data-root")
            .arg(model_app_data_root(&app).map_err(String::from)?);
        cmd.arg("--templates-file")
            .arg(custom_templates_path(&app).map_err(String::from)?);
    }

    let secrets = std::iter::once(credentials.mimo.clone())
        .chain(credentials.deepseek.iter().cloned())
        .collect::<Vec<_>>();
    run_and_check(&app, &active, cmd, &output_mode, &abs_out, &secrets)
}

#[allow(clippy::too_many_arguments)]
#[tauri::command]
async fn run_meeting_asr(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    audio_path: String,
    output_mode: String,
    backend: Option<String>,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    template_id: Option<String>,
    asr_config: ProviderSelection,
    llm_config: Option<ProviderSelection>,
) -> Result<RunResult, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        run_meeting_asr_blocking(
            app,
            active,
            audio_path,
            output_mode,
            backend,
            mimo_api_key,
            deepseek_api_key,
            template_id,
            asr_config,
            llm_config,
        )
    })
    .await
    .map_err(|error| {
        String::from(BackendError::ProcessFailed {
            message: format!("Run worker failed: {error}"),
            code: None,
        })
    })?
}

// ---------------------------------------------------------------------------
// Resume command (CLI-only path, not exposed in normal UI)
// ---------------------------------------------------------------------------

/// Finds the Go manifest file in a task directory.
/// Tries `task_manifest.json` first, then legacy `manifest.json`.
fn find_go_manifest(task_dir: &Path) -> Option<PathBuf> {
    let current = task_dir.join("task_manifest.json");
    if current.is_file() {
        return Some(current);
    }
    let legacy = task_dir.join("manifest.json");
    if legacy.is_file() {
        return Some(legacy);
    }
    None
}

/// Reads the output_mode from a Go manifest JSON file.
/// Returns None if the file is unreadable, invalid JSON, missing the field, or the value is not a valid mode.
fn read_manifest_output_mode(manifest_path: &Path) -> Option<String> {
    let data = fs::read_to_string(manifest_path).ok()?;
    let v: serde_json::Value = serde_json::from_str(&data).ok()?;
    let mode = v.get("output_mode").and_then(|m| m.as_str())?;
    match mode {
        "transcript" | "notes" | "minutes" | "transcript_only" | "transcript_and_generate" => {
            Some(mode.to_string())
        }
        _ => None,
    }
}

/// Validates an output_mode string and returns it.
fn validate_output_mode(mode: &str) -> Result<String, BackendError> {
    match mode {
        "transcript" | "notes" | "minutes" | "transcript_only" | "transcript_and_generate" => {
            Ok(mode.to_string())
        }
        other => Err(BackendError::InvalidInput {
            message: format!(
                "Invalid output_mode: {}. Must be 'transcript', 'notes', or 'minutes'.",
                other
            ),
        }),
    }
}

#[allow(clippy::too_many_arguments)]
fn resume_meeting_asr_blocking(
    app: AppHandle,
    active: ActiveProcess,
    task_dir: String,
    output_mode: Option<String>,
    backend: Option<String>,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
) -> Result<RunResult, String> {
    let runtime_mode = RuntimeMode::current();
    let backend_kind =
        resolve_backend_kind(backend.as_deref(), runtime_mode).map_err(String::from)?;

    let packaged_executable_dir =
        if backend_kind == BackendKind::Go && runtime_mode == RuntimeMode::Production {
            Some(packaged_go_executable_dir().map_err(String::from)?)
        } else {
            None
        };
    if backend_kind == BackendKind::Go && runtime_mode == RuntimeMode::Production {
        discover_go_binary(runtime_mode, packaged_executable_dir.as_deref())
            .map_err(String::from)?;
    }

    let (output_root, _) = resolve_output_root(&app).map_err(String::from)?;

    // Validate and canonicalize task directory. The task must remain under the
    // configured output root, including for resume calls that bypass the UI.
    let abs_task_dir = canonicalize_abs_dir(&task_dir, "task_dir").map_err(String::from)?;
    let abs_task_dir = canonicalize_path_within(&output_root, &abs_task_dir, "Task folder")
        .map_err(|message| BackendError::InvalidInput { message })
        .map_err(String::from)?;

    // If caller explicitly supplies output_mode, validate it.
    if let Some(ref mode) = output_mode {
        validate_output_mode(mode).map_err(String::from)?;
    }

    // Determine effective output mode.
    let effective_mode: Result<String, BackendError> = {
        let manifest_path =
            find_go_manifest(&abs_task_dir).ok_or_else(|| BackendError::ManifestError {
                message: format!(
                    "No Go manifest (task_manifest.json or manifest.json) found in: {}",
                    abs_task_dir.display()
                ),
            })?;
        output_mode.clone().map(Ok).unwrap_or_else(|| {
            read_manifest_output_mode(&manifest_path).ok_or_else(|| BackendError::ManifestError {
                message: format!(
                    "Manifest {} is missing a valid 'output_mode' field.",
                    manifest_path.display()
                ),
            })
        })
    };

    let effective_mode = effective_mode.map_err(String::from)?;
    validate_output_mode(&effective_mode).map_err(String::from)?;
    let desktop_settings =
        load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
            .map_err(String::from)?;
    validate_asr_selection(&desktop_settings.asr).map_err(String::from)?;
    if effective_mode == "transcript_and_generate"
        || effective_mode == "minutes"
        || effective_mode == "notes"
    {
        validate_llm_selection(&desktop_settings.llm).map_err(String::from)?;
    }

    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    let credentials = resolve_run_credentials(
        &KeyringCredentialStore,
        &effective_mode,
        &desktop_settings.asr.provider_id,
        runtime_mode,
        mimo_api_key.as_deref(),
        deepseek_api_key.as_deref(),
        &environment,
    )
    .map_err(String::from)?;
    let env_file = (runtime_mode == RuntimeMode::Development)
        .then(find_env_file)
        .flatten();
    let cmd = match backend_kind {
        BackendKind::Go => {
            let binary = discover_go_binary(runtime_mode, packaged_executable_dir.as_deref())
                .map_err(String::from)?;
            validate_go_binary(&binary).map_err(String::from)?;

            let mut c = Command::new(binary);
            c.arg("resume").arg(abs_task_dir.as_os_str());
            c.arg("--local-asr-language")
                .arg(&desktop_settings.local_asr_language);
            c.arg("--app-data-root")
                .arg(model_app_data_root(&app).map_err(String::from)?);
            c.arg("--templates-file")
                .arg(custom_templates_path(&app).map_err(String::from)?);
            c.arg("--asr-provider")
                .arg(&desktop_settings.asr.provider_id)
                .arg("--asr-model")
                .arg(&desktop_settings.asr.model_id)
                .arg("--llm-provider")
                .arg(&desktop_settings.llm.provider_id)
                .arg("--llm-model")
                .arg(&desktop_settings.llm.model_id);
            if let Some(ref mode) = output_mode {
                c.arg("--output-mode").arg(mode);
            }
            if runtime_mode == RuntimeMode::Development {
                if let Some(env) = env_file.as_ref() {
                    c.arg("--env-file").arg(env);
                } else {
                    let isolated_root = isolated_go_project_root()?;
                    c.arg("--project-root").arg(&isolated_root);
                    c.current_dir(&isolated_root);
                }
            } else {
                let isolated_root = isolated_go_project_root()?;
                c.arg("--project-root").arg(&isolated_root);
                c.current_dir(&isolated_root);
            }
            c.env_remove("MIMO_API_KEY");
            c.env_remove("DEEPSEEK_API_KEY");
            if !credentials.mimo.is_empty() {
                c.env("MIMO_API_KEY", &credentials.mimo);
            }
            if let Some(key) = credentials.deepseek.as_deref() {
                c.env("DEEPSEEK_API_KEY", key);
            }
            c
        }
    };

    let secrets = std::iter::once(credentials.mimo.clone())
        .chain(credentials.deepseek.iter().cloned())
        .collect::<Vec<_>>();

    run_and_check(&app, &active, cmd, &effective_mode, &output_root, &secrets)
}

#[allow(clippy::too_many_arguments)]
#[tauri::command]
async fn resume_meeting_asr(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_dir: String,
    output_mode: Option<String>,
    backend: Option<String>,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
) -> Result<RunResult, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        resume_meeting_asr_blocking(
            app,
            active,
            task_dir,
            output_mode,
            backend,
            mimo_api_key,
            deepseek_api_key,
        )
    })
    .await
    .map_err(|error| {
        String::from(BackendError::ProcessFailed {
            message: format!("Resume worker failed: {error}"),
            code: None,
        })
    })?
}

// ---------------------------------------------------------------------------
// Read-only Task Library command
// ---------------------------------------------------------------------------

fn decode_task_library_process_output(
    success: bool,
    code: Option<i32>,
    stdout: &[u8],
) -> Result<DiscoveryResult, BackendError> {
    if !success {
        return Err(BackendError::ProcessFailed {
            message: format!(
                "Task Library sidecar exited with code {}.",
                code.unwrap_or(-1)
            ),
            code,
        });
    }
    serde_json::from_slice(stdout).map_err(|_| BackendError::ProcessFailed {
        message: "Task Library sidecar returned invalid JSON.".to_string(),
        code: Some(0),
    })
}

fn run_task_library_sidecar(
    binary: &Path,
    output_root: &Path,
) -> Result<DiscoveryResult, BackendError> {
    let output = Command::new(binary)
        .arg("list-tasks")
        .arg("--output-root")
        .arg(output_root)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .output()
        .map_err(|_| BackendError::ProcessFailed {
            message: "Could not start the Task Library sidecar.".to_string(),
            code: None,
        })?;
    decode_task_library_process_output(
        output.status.success(),
        output.status.code(),
        &output.stdout,
    )
}

fn discover_tasks_for_root<F>(
    output_root: Option<PathBuf>,
    discover: F,
) -> Result<DiscoveryResult, BackendError>
where
    F: FnOnce(&Path) -> Result<DiscoveryResult, BackendError>,
{
    match output_root {
        Some(root) => discover(&root),
        None => Ok(DiscoveryResult {
            tasks: Vec::new(),
            issues: Vec::new(),
        }),
    }
}

fn list_tasks_from_existing_root(output_root: &Path) -> Result<DiscoveryResult, BackendError> {
    let runtime_mode = RuntimeMode::current();
    let packaged_executable_dir = if runtime_mode == RuntimeMode::Production {
        Some(
            packaged_go_executable_dir().map_err(|_| BackendError::BackendNotFound {
                message: "Go sidecar is unavailable for Task Library discovery.".to_string(),
            })?,
        )
    } else {
        None
    };
    let binary =
        discover_go_binary(runtime_mode, packaged_executable_dir.as_deref()).map_err(|_| {
            BackendError::BackendNotFound {
                message: "Go sidecar is unavailable for Task Library discovery.".to_string(),
            }
        })?;
    validate_go_binary(&binary).map_err(|_| BackendError::BackendNotFound {
        message: "Go sidecar is unavailable for Task Library discovery.".to_string(),
    })?;
    run_task_library_sidecar(&binary, output_root)
}

fn list_tasks_blocking(app: AppHandle) -> Result<DiscoveryResult, String> {
    let output_root = resolve_task_library_output_root(&app).map_err(|_| {
        String::from(BackendError::InvalidInput {
            message: "Task Library output root is unavailable.".to_string(),
        })
    })?;
    discover_tasks_for_root(output_root, list_tasks_from_existing_root).map_err(String::from)
}

#[tauri::command]
async fn list_tasks(app: AppHandle) -> Result<DiscoveryResult, String> {
    tauri::async_runtime::spawn_blocking(move || list_tasks_blocking(app))
        .await
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Task Library worker failed.".to_string(),
                code: None,
            })
        })?
}

#[derive(Debug, serde::Serialize, serde::Deserialize)]
struct CacheFile {
    reference: String,
    size_bytes: u64,
}

#[derive(Debug, serde::Serialize, serde::Deserialize)]
struct CachePreview {
    #[serde(default)]
    scope: String,
    can_clean: bool,
    #[serde(default)]
    reason: String,
    files: Vec<CacheFile>,
    total_bytes: u64,
    #[serde(default)]
    token: String,
    cache_status: String,
    removed_bytes: u64,
}

fn task_cache_blocking(
    app: AppHandle,
    active: ActiveProcess,
    task_folder: String,
    expected_token: Option<String>,
    text_only: bool,
) -> Result<CachePreview, String> {
    validate_task_metadata_mutation_state(active.is_active()).map_err(String::from)?;
    let root = task_metadata_root(&app).map_err(String::from)?;
    let binary = go_sidecar_binary().map_err(String::from)?;
    let mut command = Command::new(binary);
    command
        .arg(if expected_token.is_some() {
            "clean-task-cache"
        } else {
            "preview-task-cache"
        })
        .arg("--output-root")
        .arg(root)
        .arg("--task-folder")
        .arg(task_folder)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .stdin(Stdio::null())
        .stderr(Stdio::null());
    if text_only {
        command.arg("--scope").arg("text_only");
    }
    if let Some(token) = expected_token {
        command.arg("--expected-token").arg(token);
    }
    let output = command
        .output()
        .map_err(|_| "Cache worker could not start.".to_string())?;
    decode_cache_output(output.status.success(), &output.stdout)
}

fn decode_cache_output(success: bool, stdout: &[u8]) -> Result<CachePreview, String> {
    if !success {
        return Err("Cache operation failed; refresh the preview and try again.".to_string());
    }
    serde_json::from_slice(stdout).map_err(|_| "Cache worker returned invalid data.".to_string())
}

#[tauri::command]
async fn preview_task_cache(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_folder: String,
    text_only: Option<bool>,
) -> Result<CachePreview, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        task_cache_blocking(app, active, task_folder, None, text_only.unwrap_or(false))
    })
    .await
    .map_err(|_| "Cache worker failed.".to_string())?
}

#[tauri::command]
async fn clean_task_cache(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_folder: String,
    expected_token: String,
    text_only: Option<bool>,
) -> Result<CachePreview, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        task_cache_blocking(
            app,
            active,
            task_folder,
            Some(expected_token),
            text_only.unwrap_or(false),
        )
    })
    .await
    .map_err(|_| "Cache worker failed.".to_string())?
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct StorageSettings {
    auto_clean_chunks: bool,
    cleanup_temporary_on_startup: bool,
}

#[derive(Debug, Default, serde::Serialize, serde::Deserialize)]
struct TemporaryCleanup {
    removed_bytes: u64,
    removed_count: u64,
    skipped_count: u64,
}

#[derive(serde::Serialize, serde::Deserialize)]
struct StorageTask {
    folder: String,
    title: String,
    status: String,
    total_bytes: u64,
    chunk_bytes: u64,
}

#[derive(serde::Serialize, serde::Deserialize)]
struct StorageSummary {
    output_root: String,
    categories: std::collections::BTreeMap<String, u64>,
    total_bytes: u64,
    tasks: Vec<StorageTask>,
    issue_count: u64,
}

#[derive(serde::Serialize, serde::Deserialize)]
struct BatchCacheTask {
    folder: String,
    title: String,
    preview: CachePreview,
}

#[derive(serde::Serialize, serde::Deserialize)]
struct BatchCacheResult {
    output_root: String,
    tasks: Vec<BatchCacheTask>,
    removed_bytes: u64,
    cleaned_count: u64,
    skipped_count: u64,
    failed_count: u64,
}

fn storage_settings_at(path: &Path) -> Result<StorageSettings, BackendError> {
    let settings = load_desktop_settings(path)?;
    Ok(StorageSettings {
        auto_clean_chunks: settings.auto_clean_chunks,
        cleanup_temporary_on_startup: settings.cleanup_temporary_on_startup,
    })
}

#[tauri::command]
fn get_storage_settings(app: AppHandle) -> Result<StorageSettings, String> {
    storage_settings_at(&desktop_settings_path(&app).map_err(String::from)?).map_err(String::from)
}

#[tauri::command]
fn set_storage_settings(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    auto_clean_chunks: bool,
    cleanup_temporary_on_startup: bool,
) -> Result<StorageSettings, String> {
    validate_task_metadata_mutation_state(active.is_active()).map_err(String::from)?;
    let path = desktop_settings_path(&app).map_err(String::from)?;
    let mut settings = load_desktop_settings(&path).map_err(String::from)?;
    settings.auto_clean_chunks = auto_clean_chunks;
    settings.cleanup_temporary_on_startup = cleanup_temporary_on_startup;
    save_desktop_settings(&path, &settings).map_err(String::from)?;
    storage_settings_at(&path).map_err(String::from)
}

fn run_storage_sidecar<T: serde::de::DeserializeOwned>(
    app: &AppHandle,
    name: &str,
    request: Option<serde_json::Value>,
) -> Result<T, String> {
    let binary = go_sidecar_binary().map_err(String::from)?;
    let mut command = Command::new(binary);
    command
        .arg(name)
        .arg("--app-data-root")
        .arg(model_app_data_root(app).map_err(String::from)?);
    if name != "cleanup-model-temporary" {
        if let Some(root) = resolve_task_library_output_root(app).map_err(String::from)? {
            command.arg("--output-root").arg(root);
        }
    }
    command
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .stdin(if request.is_some() {
            Stdio::piped()
        } else {
            Stdio::null()
        })
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    let mut child = command
        .spawn()
        .map_err(|_| "Storage worker could not start.".to_string())?;
    if let Some(request) = request {
        let data =
            serde_json::to_vec(&request).map_err(|_| "Storage request is invalid.".to_string())?;
        child
            .stdin
            .take()
            .ok_or("Storage input is unavailable.")?
            .write_all(&data)
            .map_err(|_| "Storage request failed.".to_string())?;
    }
    let output = child
        .wait_with_output()
        .map_err(|_| "Storage worker failed.".to_string())?;
    if !output.status.success() {
        return Err("Storage operation failed; refresh and retry.".to_string());
    }
    serde_json::from_slice(&output.stdout)
        .map_err(|_| "Storage worker returned invalid data.".to_string())
}

#[tauri::command]
async fn get_storage_summary(app: AppHandle) -> Result<StorageSummary, String> {
    tauri::async_runtime::spawn_blocking(move || run_storage_sidecar(&app, "storage-summary", None))
        .await
        .map_err(|_| "Storage worker failed.".to_string())?
}

#[tauri::command]
async fn preview_cache_batch(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
) -> Result<BatchCacheResult, String> {
    validate_task_metadata_mutation_state(active.is_active()).map_err(String::from)?;
    tauri::async_runtime::spawn_blocking(move || {
        run_storage_sidecar(&app, "preview-cache-batch", None)
    })
    .await
    .map_err(|_| "Storage worker failed.".to_string())?
}

#[tauri::command]
async fn clean_cache_batch(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    request: serde_json::Value,
) -> Result<BatchCacheResult, String> {
    validate_task_metadata_mutation_state(active.is_active()).map_err(String::from)?;
    tauri::async_runtime::spawn_blocking(move || {
        run_storage_sidecar(&app, "clean-cache-batch", Some(request))
    })
    .await
    .map_err(|_| "Storage worker failed.".to_string())?
}

#[tauri::command]
async fn run_storage_maintenance(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    startup: bool,
) -> Result<TemporaryCleanup, String> {
    validate_task_metadata_mutation_state(active.is_active()).map_err(String::from)?;
    let settings = storage_settings_at(&desktop_settings_path(&app).map_err(String::from)?)
        .map_err(String::from)?;
    if startup && !settings.cleanup_temporary_on_startup {
        return Ok(TemporaryCleanup::default());
    }
    tauri::async_runtime::spawn_blocking(move || {
        run_storage_sidecar(&app, "cleanup-model-temporary", None)
    })
    .await
    .map_err(|_| "Storage worker failed.".to_string())?
}

fn decode_task_metadata_output(
    success: bool,
    code: Option<i32>,
    stdout: &[u8],
) -> Result<TaskMetadata, BackendError> {
    if !success {
        return Err(BackendError::ProcessFailed {
            message: format!(
                "Task metadata sidecar exited with code {}.",
                code.unwrap_or(-1)
            ),
            code,
        });
    }
    serde_json::from_slice(stdout).map_err(|_| BackendError::ProcessFailed {
        message: "Task metadata sidecar returned invalid JSON.".to_string(),
        code: Some(0),
    })
}

fn run_task_metadata_sidecar(
    binary: &Path,
    command_name: &str,
    output_root: &Path,
    task_id: &str,
    update: Option<&TaskMetadataUpdate>,
) -> Result<TaskMetadata, BackendError> {
    let mut command = Command::new(binary);
    command
        .arg(command_name)
        .arg("--output-root")
        .arg(output_root)
        .arg("--task-id")
        .arg(task_id)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    if update.is_some() {
        command.stdin(Stdio::piped());
    }
    let mut child = command.spawn().map_err(|_| BackendError::ProcessFailed {
        message: "Could not start the Task metadata sidecar.".to_string(),
        code: None,
    })?;
    if let Some(payload) = update {
        let data = serde_json::to_vec(payload).map_err(|_| BackendError::InvalidInput {
            message: "Task metadata is invalid.".to_string(),
        })?;
        child
            .stdin
            .take()
            .ok_or_else(|| BackendError::ProcessFailed {
                message: "Task metadata sidecar input is unavailable.".to_string(),
                code: None,
            })?
            .write_all(&data)
            .map_err(|_| BackendError::ProcessFailed {
                message: "Could not send Task metadata to the sidecar.".to_string(),
                code: None,
            })?;
    }
    let output = child
        .wait_with_output()
        .map_err(|_| BackendError::ProcessFailed {
            message: "Task metadata sidecar did not complete.".to_string(),
            code: None,
        })?;
    decode_task_metadata_output(
        output.status.success(),
        output.status.code(),
        &output.stdout,
    )
}

fn task_metadata_root(app: &AppHandle) -> Result<PathBuf, BackendError> {
    resolve_task_library_output_root(app)?.ok_or_else(|| BackendError::ArtifactUnavailable {
        message: "Task Library is empty.".to_string(),
    })
}

fn read_task_metadata_blocking(app: AppHandle, task_id: String) -> Result<TaskMetadata, String> {
    let root = task_metadata_root(&app).map_err(String::from)?;
    run_task_metadata_sidecar(
        &go_sidecar_binary().map_err(String::from)?,
        "get-task-metadata",
        &root,
        &task_id,
        None,
    )
    .map_err(String::from)
}

#[tauri::command]
async fn get_task_metadata(app: AppHandle, task_id: String) -> Result<TaskMetadata, String> {
    tauri::async_runtime::spawn_blocking(move || read_task_metadata_blocking(app, task_id))
        .await
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Task metadata worker failed.".to_string(),
                code: None,
            })
        })?
}

fn update_task_metadata_blocking(
    app: AppHandle,
    active: ActiveProcess,
    task_id: String,
    metadata: TaskMetadataUpdate,
) -> Result<TaskMetadata, String> {
    validate_task_metadata_mutation_state(active.is_active()).map_err(String::from)?;
    let root = task_metadata_root(&app).map_err(String::from)?;
    run_task_metadata_sidecar(
        &go_sidecar_binary().map_err(String::from)?,
        "update-task-metadata",
        &root,
        &task_id,
        Some(&metadata),
    )
    .map_err(String::from)
}

fn validate_task_metadata_mutation_state(is_active: bool) -> Result<(), BackendError> {
    if is_active {
        return Err(BackendError::InvalidInput {
            message: "Task metadata cannot be changed while processing.".to_string(),
        });
    }
    Ok(())
}

#[tauri::command]
async fn update_task_metadata(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_id: String,
    metadata: TaskMetadataUpdate,
) -> Result<TaskMetadata, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        update_task_metadata_blocking(app, active, task_id, metadata)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Task metadata worker failed.".to_string(),
            code: None,
        })
    })?
}

fn go_sidecar_binary() -> Result<PathBuf, BackendError> {
    let runtime_mode = RuntimeMode::current();
    let packaged = if runtime_mode == RuntimeMode::Production {
        Some(
            packaged_go_executable_dir().map_err(|_| BackendError::BackendNotFound {
                message: "Go sidecar is unavailable.".to_string(),
            })?,
        )
    } else {
        None
    };
    let binary = discover_go_binary(runtime_mode, packaged.as_deref()).map_err(|_| {
        BackendError::BackendNotFound {
            message: "Go sidecar is unavailable.".to_string(),
        }
    })?;
    validate_go_binary(&binary).map_err(|_| BackendError::BackendNotFound {
        message: "Go sidecar is unavailable.".to_string(),
    })?;
    Ok(binary)
}

fn list_prompt_templates_blocking(app: AppHandle) -> Result<PromptTemplateCatalog, String> {
    let output = Command::new(go_sidecar_binary().map_err(String::from)?)
        .arg("list-templates")
        .arg("--templates-file")
        .arg(custom_templates_path(&app).map_err(String::from)?)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .output()
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Could not start the template sidecar.".to_string(),
                code: None,
            })
        })?;
    if !output.status.success() {
        return Err(BackendError::ProcessFailed {
            message: "Template sidecar failed.".to_string(),
            code: output.status.code(),
        }
        .into());
    }
    serde_json::from_slice(&output.stdout).map_err(|_| {
        BackendError::ProcessFailed {
            message: "Template sidecar returned invalid JSON.".to_string(),
            code: output.status.code(),
        }
        .into()
    })
}

#[tauri::command]
async fn list_prompt_templates(app: AppHandle) -> Result<PromptTemplateCatalog, String> {
    tauri::async_runtime::spawn_blocking(move || list_prompt_templates_blocking(app))
        .await
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Template worker failed.".to_string(),
                code: None,
            })
        })?
}

fn run_custom_template_sidecar<T: serde::Serialize, R: serde::de::DeserializeOwned>(
    app: &AppHandle,
    command_name: &str,
    payload: &T,
) -> Result<R, String> {
    let mut command = Command::new(go_sidecar_binary().map_err(String::from)?);
    command
        .arg(command_name)
        .arg("--templates-file")
        .arg(custom_templates_path(app).map_err(String::from)?)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let mut child = command.spawn().map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Could not start the custom template sidecar.".to_string(),
            code: None,
        })
    })?;
    let data = serde_json::to_vec(payload).map_err(|_| {
        String::from(BackendError::InvalidInput {
            message: "Custom template input is invalid.".to_string(),
        })
    })?;
    child
        .stdin
        .take()
        .ok_or_else(|| {
            String::from(BackendError::ProcessFailed {
                message: "Custom template sidecar input is unavailable.".to_string(),
                code: None,
            })
        })?
        .write_all(&data)
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Could not send custom template input.".to_string(),
                code: None,
            })
        })?;
    let output = child.wait_with_output().map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Custom template sidecar did not complete.".to_string(),
            code: None,
        })
    })?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        let message = if stderr.contains("revision_conflict") {
            "Custom template changed. Refresh and try again."
        } else if stderr.contains("builtin_mutation_forbidden") {
            "Built-in templates cannot be changed."
        } else if stderr.contains("deleted_template") {
            "This custom template has been deleted."
        } else if stderr.contains("unknown_template") {
            "Custom template was not found."
        } else if stderr.contains("invalid_template_input") {
            "Custom template input is invalid."
        } else if stderr.contains("corrupted_template_storage")
            || stderr.contains("unsupported_template_schema")
        {
            "Custom template storage is invalid."
        } else if stderr.contains("template_storage") {
            "Custom template storage is unavailable."
        } else {
            "Custom template operation failed."
        };
        return Err(String::from(BackendError::InvalidInput {
            message: message.to_string(),
        }));
    }
    serde_json::from_slice(&output.stdout).map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Custom template sidecar returned invalid JSON.".to_string(),
            code: output.status.code(),
        })
    })
}

#[tauri::command]
async fn create_custom_template(
    app: AppHandle,
    input: CustomTemplateInput,
) -> Result<PromptTemplateSummary, String> {
    tauri::async_runtime::spawn_blocking(move || {
        run_custom_template_sidecar(&app, "create-custom-template", &input)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Custom template worker failed.".to_string(),
            code: None,
        })
    })?
}

#[tauri::command]
async fn update_custom_template(
    app: AppHandle,
    input: UpdateCustomTemplateInput,
) -> Result<PromptTemplateSummary, String> {
    tauri::async_runtime::spawn_blocking(move || {
        run_custom_template_sidecar(&app, "update-custom-template", &input)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Custom template worker failed.".to_string(),
            code: None,
        })
    })?
}

#[tauri::command]
async fn delete_custom_template(
    app: AppHandle,
    input: DeleteCustomTemplateInput,
) -> Result<serde_json::Value, String> {
    tauri::async_runtime::spawn_blocking(move || {
        run_custom_template_sidecar(&app, "delete-custom-template", &input)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Custom template worker failed.".to_string(),
            code: None,
        })
    })?
}

fn list_providers_blocking() -> Result<ProviderCatalog, String> {
    let output = Command::new(go_sidecar_binary().map_err(String::from)?)
        .arg("list-providers")
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .output()
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Could not start the provider sidecar.".to_string(),
                code: None,
            })
        })?;
    if !output.status.success() {
        return Err(BackendError::ProcessFailed {
            message: "Provider sidecar failed.".to_string(),
            code: output.status.code(),
        }
        .into());
    }
    serde_json::from_slice(&output.stdout).map_err(|_| {
        BackendError::ProcessFailed {
            message: "Provider sidecar returned invalid JSON.".to_string(),
            code: output.status.code(),
        }
        .into()
    })
}

#[tauri::command]
async fn list_providers() -> Result<ProviderCatalog, String> {
    tauri::async_runtime::spawn_blocking(list_providers_blocking)
        .await
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Provider worker failed.".to_string(),
                code: None,
            })
        })?
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ModelCatalog {
    schema_version: String,
    catalog_revision: String,
    models: Vec<ManagedModel>,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ManagedModel {
    model_id: String,
    display_mode: String,
    display_name: std::collections::HashMap<String, String>,
    engine: String,
    engine_runtime: String,
    runtime_version: Option<String>,
    model_version: String,
    platforms: Vec<String>,
    source: ManagedModelSource,
    archive: ManagedArchive,
    installed_files: Vec<ManagedFile>,
    license: ManagedLicense,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ManagedModelSource {
    kind: String,
    repository: String,
    revision: String,
    url: String,
}
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ManagedArchive {
    filename: String,
    #[serde(rename = "type")]
    archive_type: String,
    size_bytes: u64,
    sha256: String,
    root_directory: String,
    entries: Vec<ManagedArchiveEntry>,
}
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ManagedArchiveEntry {
    path: String,
    #[serde(rename = "type")]
    entry_type: String,
    size_bytes: u64,
    sha256: String,
}
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ManagedFile {
    path: String,
    size_bytes: u64,
    sha256: String,
}
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct ManagedLicense {
    notice_id: String,
    redistribution_review_required: bool,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct LocalModelStatus {
    model_id: String,
    model_version: String,
    state: String,
    message: String,
    installed_size_bytes: u64,
    runtime_id: Option<String>,
    runtime_version: Option<String>,
}

#[derive(Debug, Clone, serde::Serialize)]
struct ModelDownloadSettings {
    huggingface_endpoint: String,
}

fn model_app_data_root(app: &AppHandle) -> Result<PathBuf, BackendError> {
    app.path()
        .app_data_dir()
        .map_err(|_| BackendError::InvalidInput {
            message: "Local model storage is unavailable.".to_string(),
        })
}

fn decode_model_output<T: serde::de::DeserializeOwned>(
    output: std::process::Output,
) -> Result<T, BackendError> {
    if !output.status.success() {
        return Err(BackendError::ProcessFailed {
            message: "Local model operation failed safely.".to_string(),
            code: output.status.code(),
        });
    }
    serde_json::from_slice(&output.stdout).map_err(|_| BackendError::ProcessFailed {
        message: "Local model sidecar returned invalid JSON.".to_string(),
        code: output.status.code(),
    })
}

fn run_model_read<T: serde::de::DeserializeOwned>(
    app: &AppHandle,
    command: &str,
    with_root: bool,
) -> Result<T, BackendError> {
    let mut process = Command::new(go_sidecar_binary()?);
    process
        .arg(command)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .stderr(Stdio::null());
    if with_root {
        process
            .arg("--app-data-root")
            .arg(model_app_data_root(app)?)
            .arg("--model-id")
            .arg(LOCAL_ASR_MODEL);
    }
    let output = process
        .output()
        .map_err(|_| BackendError::BackendNotFound {
            message: "Go sidecar is unavailable for local model management.".to_string(),
        })?;
    decode_model_output(output)
}

#[tauri::command]
async fn get_local_model_catalog(app: AppHandle) -> Result<ModelCatalog, String> {
    tauri::async_runtime::spawn_blocking(move || {
        run_model_read(&app, "model-catalog", false).map_err(String::from)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Model catalog worker failed.".to_string(),
            code: None,
        })
    })?
}

#[tauri::command]
async fn get_local_model_status(app: AppHandle) -> Result<LocalModelStatus, String> {
    tauri::async_runtime::spawn_blocking(move || {
        run_model_read(&app, "model-status", true).map_err(String::from)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Model status worker failed.".to_string(),
            code: None,
        })
    })?
}

fn run_model_mutation(
    app: AppHandle,
    active: ActiveProcess,
    command: &str,
    source: Option<String>,
) -> Result<LocalModelStatus, String> {
    if active
        .model_operation
        .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
        .is_err()
    {
        return Err(String::from(BackendError::ProcessFailed {
            message: "Another local model operation is already running.".to_string(),
            code: None,
        }));
    }
    let _operation = ModelOperationGuard(active.model_operation.clone());
    let mut process = Command::new(go_sidecar_binary().map_err(String::from)?);
    process
        .arg(command)
        .arg("--app-data-root")
        .arg(model_app_data_root(&app).map_err(String::from)?)
        .arg("--model-id")
        .arg(LOCAL_ASR_MODEL)
        .arg("--control-stdin")
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY");
    if let Some(source) = source {
        process.arg("--source-archive").arg(source);
    }
    let runner = spawn_and_watch(&app, &active, process, &[], true).map_err(String::from)?;
    let (code, stdout, _) = runner.wait().map_err(String::from)?;
    if let Some(error) = cancelled_failure(code, "The model operation was cancelled.") {
        return Err(String::from(error));
    }
    if code != Some(0) {
        return Err(String::from(BackendError::ProcessFailed {
            message: "Local model operation failed safely.".to_string(),
            code,
        }));
    }
    serde_json::from_slice(stdout.join("\n").as_bytes()).map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Local model sidecar returned invalid JSON.".to_string(),
            code,
        })
    })
}

async fn model_mutation_command(
    app: AppHandle,
    active: ActiveProcess,
    command: &'static str,
    source: Option<String>,
) -> Result<LocalModelStatus, String> {
    tauri::async_runtime::spawn_blocking(move || run_model_mutation(app, active, command, source))
        .await
        .map_err(|_| {
            String::from(BackendError::ProcessFailed {
                message: "Local model worker failed.".to_string(),
                code: None,
            })
        })?
}

#[tauri::command]
async fn start_model_download(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
) -> Result<LocalModelStatus, String> {
    model_mutation_command(app, active.inner().clone(), "download-model", None).await
}
#[tauri::command]
async fn import_local_model(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    source_archive: String,
) -> Result<LocalModelStatus, String> {
    model_mutation_command(
        app,
        active.inner().clone(),
        "import-model",
        Some(source_archive),
    )
    .await
}
#[tauri::command]
async fn delete_local_model(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
) -> Result<LocalModelStatus, String> {
    model_mutation_command(app, active.inner().clone(), "delete-model", None).await
}
/// cancel_task asks the running operation to stop. The sidecar stops
/// cooperatively and the runtime forces a stop only after the grace period, so a
/// cancel cannot destroy an artifact that was already completed.
#[tauri::command]
fn cancel_task(active: State<'_, ActiveProcess>) -> CancelOutcome {
    active.request_stop()
}

fn resume_task_blocking(
    app: AppHandle,
    active: ActiveProcess,
    task_folder: String,
    output_mode: Option<String>,
) -> Result<RunResult, String> {
    let output_root = resolve_task_library_output_root(&app)
        .map_err(String::from)?
        .ok_or_else(|| {
            String::from(BackendError::ArtifactUnavailable {
                message: "Task Library is empty.".to_string(),
            })
        })?;
    let discovery = list_tasks_from_existing_root(&output_root).map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Task Library discovery failed.".to_string(),
            code: None,
        })
    })?;
    let task_dir = resolve_resumable_task(&output_root, &discovery, &task_folder, true)
        .map_err(String::from)?;

    if let Some(ref mode) = output_mode {
        validate_output_mode(mode).map_err(String::from)?;
    }
    // The manifest already records the mode and the requested template. The
    // override is forwarded only when the caller asked for it, so resuming never
    // rewrites the Task's own processing selection.
    let mode_override = output_mode.clone();
    let effective_mode = match output_mode {
        Some(mode) => mode,
        None => read_manifest_output_mode(&task_dir).unwrap_or_else(|| "transcript".to_string()),
    };

    let runtime_mode = RuntimeMode::current();
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    // Resume may only need one of the two credentials, so both are resolved
    // leniently here: the runtime validates the credential its next stage
    // actually needs and reports the precise failure itself.
    let mimo = resolve_credential(
        &KeyringCredentialStore,
        CredentialKey::MimoApiKey,
        None,
        runtime_mode,
        &environment,
    )
    .map_err(BackendError::from)
    .map_err(String::from)?
    .map(|credential| credential.value);
    let deepseek = resolve_credential(
        &KeyringCredentialStore,
        CredentialKey::DeepseekApiKey,
        None,
        runtime_mode,
        &environment,
    )
    .map_err(BackendError::from)
    .map_err(String::from)?
    .map(|credential| credential.value);
    let settings = load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
        .map_err(String::from)?;
    let mut cmd = Command::new(go_sidecar_binary().map_err(String::from)?);
    cmd.arg("resume")
        .arg(task_dir.as_os_str())
        .arg("--local-asr-language")
        .arg(&settings.local_asr_language)
        .arg("--app-data-root")
        .arg(model_app_data_root(&app).map_err(String::from)?)
        .arg("--templates-file")
        .arg(custom_templates_path(&app).map_err(String::from)?)
        // The speech identity belongs to the Task: resume adopts the identity the
        // manifest recorded instead of the current selection.
        .arg("--llm-provider")
        .arg(&settings.llm.provider_id)
        .arg("--llm-model")
        .arg(&settings.llm.model_id)
        .arg("--control-stdin");
    cmd.arg(format!(
        "--auto-clean-chunks={}",
        settings.auto_clean_chunks
    ));
    if let Some(mode) = mode_override.as_deref() {
        cmd.arg("--output-mode").arg(mode);
    }
    let isolated_root = isolated_go_project_root()?;
    cmd.arg("--project-root")
        .arg(&isolated_root)
        .current_dir(&isolated_root);
    cmd.env_remove("MIMO_API_KEY");
    cmd.env_remove("DEEPSEEK_API_KEY");
    let mut secrets: Vec<String> = Vec::new();
    if let Some(key) = mimo.as_deref() {
        if !key.is_empty() {
            cmd.env("MIMO_API_KEY", key);
            secrets.push(key.to_string());
        }
    }
    if let Some(key) = deepseek.as_deref() {
        if !key.is_empty() {
            cmd.env("DEEPSEEK_API_KEY", key);
            secrets.push(key.to_string());
        }
    }
    run_and_check(&app, &active, cmd, &effective_mode, &task_dir, &secrets)
}

/// resume_task continues an interrupted Task from the Task Library. It takes a
/// Task folder, revalidates membership, containment and the Task lock in Rust,
/// and lets the runtime decide which work is left.
#[tauri::command]
async fn resume_task(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_folder: String,
    output_mode: Option<String>,
) -> Result<RunResult, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        resume_task_blocking(app, active, task_folder, output_mode)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Task resume worker failed.".to_string(),
            code: None,
        })
    })?
}

/// RetryGenerationResult reports which generation was retried and how it ended.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
struct RetryGenerationResult {
    generation_id: String,
    status: String,
}

fn retry_task_generation_blocking(
    app: AppHandle,
    active: ActiveProcess,
    task_folder: String,
    generation_id: String,
    deepseek_api_key: Option<String>,
) -> Result<RetryGenerationResult, String> {
    if generation_id.trim().is_empty() {
        return Err(String::from(BackendError::InvalidInput {
            message: "A generation id is required.".to_string(),
        }));
    }
    let output_root = resolve_task_library_output_root(&app)
        .map_err(String::from)?
        .ok_or_else(|| {
            String::from(BackendError::ArtifactUnavailable {
                message: "Task Library is empty.".to_string(),
            })
        })?;
    let discovery = list_tasks_from_existing_root(&output_root).map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Task Library discovery failed.".to_string(),
            code: None,
        })
    })?;
    let task_dir =
        resolve_retryable_generation(&output_root, &discovery, &task_folder, &generation_id)
            .map_err(String::from)?;

    let runtime_mode = RuntimeMode::current();
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    let credential = resolve_credential(
        &KeyringCredentialStore,
        CredentialKey::DeepseekApiKey,
        deepseek_api_key.as_deref(),
        runtime_mode,
        &environment,
    )
    .map_err(BackendError::from)
    .map_err(String::from)?
    .ok_or_else(|| String::from(missing_api_key(CredentialKey::DeepseekApiKey)))?;
    let mut cmd = Command::new(go_sidecar_binary().map_err(String::from)?);
    cmd.arg("retry-generation")
        .arg("--task-dir")
        .arg(task_dir.as_os_str())
        .arg("--generation-id")
        .arg(&generation_id)
        // The retried generation keeps its recorded provider and model, so no
        // provider selection is passed here.
        .arg("--control-stdin");
    let isolated_root = isolated_go_project_root()?;
    cmd.arg("--project-root")
        .arg(&isolated_root)
        .current_dir(&isolated_root);
    cmd.env_remove("MIMO_API_KEY");
    cmd.env_remove("DEEPSEEK_API_KEY");
    cmd.env("DEEPSEEK_API_KEY", &credential.value);
    let secrets = vec![credential.value];

    let runner = spawn_and_watch(&app, &active, cmd, &secrets, true).map_err(String::from)?;
    let (code, stdout_lines, stderr_lines) = runner.wait()?;
    let _ = app.emit("process-done", DoneEvent { code });
    match code {
        Some(0) => {
            let parsed = ParsedOutput::parse_lines(&stdout_lines);
            let status = parsed
                .generation_status
                .unwrap_or_else(|| "completed".to_string());
            Ok(RetryGenerationResult {
                generation_id,
                status,
            })
        }
        Some(c) => Err(String::from(classify_process_failure(
            Some(c),
            &stderr_lines,
            &secrets,
            "The generation was cancelled before it finished.",
        ))),
        None => Err(String::from(BackendError::Interrupted {
            message: "Process was terminated by a signal.".to_string(),
        })),
    }
}

/// retry_task_generation retries one user-selected failed generation under its
/// original identity. It never creates a new generation and never retries a
/// different record than the one requested.
#[tauri::command]
async fn retry_task_generation(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_folder: String,
    generation_id: String,
    deepseek_api_key: Option<String>,
) -> Result<RetryGenerationResult, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        retry_task_generation_blocking(app, active, task_folder, generation_id, deepseek_api_key)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Generation retry worker failed.".to_string(),
            code: None,
        })
    })?
}

#[tauri::command]
fn cancel_model_download(active: State<'_, ActiveProcess>) -> CancelOutcome {
    active.cancel_model_operation()
}

#[tauri::command]
fn get_model_download_settings(app: AppHandle) -> Result<ModelDownloadSettings, String> {
    let settings = load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
        .map_err(String::from)?;
    Ok(ModelDownloadSettings {
        huggingface_endpoint: settings
            .huggingface_endpoint
            .unwrap_or_else(|| "https://huggingface.co".to_string()),
    })
}

fn normalize_hf_endpoint(endpoint: String) -> Result<String, BackendError> {
    let output = Command::new(go_sidecar_binary()?)
        .arg("normalize-hf-endpoint")
        .arg("--endpoint")
        .arg(endpoint)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .stderr(Stdio::null())
        .output()
        .map_err(|_| BackendError::BackendNotFound {
            message: "Go sidecar is unavailable for endpoint validation.".to_string(),
        })?;
    let value: serde_json::Value = decode_model_output(output)?;
    value
        .get("endpoint")
        .and_then(|value| value.as_str())
        .map(str::to_string)
        .ok_or_else(|| BackendError::ProcessFailed {
            message: "Endpoint validator returned invalid JSON.".to_string(),
            code: Some(0),
        })
}

#[tauri::command]
async fn update_model_download_settings(
    app: AppHandle,
    endpoint: String,
) -> Result<ModelDownloadSettings, String> {
    tauri::async_runtime::spawn_blocking(move || {
        let normalized = normalize_hf_endpoint(endpoint).map_err(String::from)?;
        let path = desktop_settings_path(&app).map_err(String::from)?;
        let mut settings = load_desktop_settings(&path).map_err(String::from)?;
        settings.huggingface_endpoint = Some(normalized.clone());
        save_desktop_settings(&path, &settings).map_err(String::from)?;
        Ok(ModelDownloadSettings {
            huggingface_endpoint: normalized,
        })
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Endpoint settings worker failed.".to_string(),
            code: None,
        })
    })?
}

#[tauri::command]
async fn test_huggingface_endpoint(endpoint: String) -> Result<serde_json::Value, String> {
    tauri::async_runtime::spawn_blocking(move || {
        let output = Command::new(go_sidecar_binary().map_err(String::from)?)
            .arg("test-hf-endpoint")
            .arg("--endpoint")
            .arg(endpoint)
            .env_remove("MIMO_API_KEY")
            .env_remove("DEEPSEEK_API_KEY")
            .stderr(Stdio::null())
            .output()
            .map_err(|_| {
                String::from(BackendError::BackendNotFound {
                    message: "Go sidecar is unavailable for endpoint testing.".to_string(),
                })
            })?;
        decode_model_output(output).map_err(String::from)
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Endpoint test worker failed.".to_string(),
            code: None,
        })
    })?
}

#[derive(Clone, serde::Serialize)]
struct GenerateContentResult {
    generation_id: String,
}

fn resolve_generation_task(
    output_root: &Path,
    discovery: &DiscoveryResult,
    task_folder: &str,
) -> Result<PathBuf, BackendError> {
    let hint = validated_relative_hint(task_folder, "Task folder", false)?;
    let summary = discovery
        .tasks
        .iter()
        .find(|task| task.task_folder == task_folder && task.can_generate)
        .ok_or_else(|| BackendError::ArtifactUnavailable {
            message: "Task is not eligible for generation. Refresh the Task Library.".to_string(),
        })?;
    // A Task another process is working on must not be overlapped, even when the
    // frontend believes generation is available.
    if let Some(actions) = summary.actions.as_ref() {
        if actions.locked || actions.has_reason("task_locked") {
            return Err(BackendError::TaskLocked {
                message: "Task is locked by another process.".to_string(),
            });
        }
    }
    let root = output_root
        .canonicalize()
        .map_err(|_| BackendError::ArtifactValidationFailed {
            message: "Task Library output root is unavailable.".to_string(),
        })?;
    let task = root
        .join(hint)
        .canonicalize()
        .map_err(|_| BackendError::ArtifactUnavailable {
            message: "Task is no longer available.".to_string(),
        })?;
    if !task.is_dir() || !task.starts_with(&root) {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Task folder failed containment validation.".to_string(),
        });
    }
    let transcript_reference = summary
        .canonical_transcript
        .as_ref()
        .and_then(|artifact| {
            (artifact.status == "completed")
                .then_some(artifact.reference.as_deref())
                .flatten()
        })
        .ok_or_else(|| BackendError::ArtifactUnavailable {
            message: "Canonical transcript is no longer available.".to_string(),
        })?;
    let transcript_hint =
        validated_relative_hint(transcript_reference, "Canonical transcript", true)?;
    let transcript = task.join(transcript_hint).canonicalize().map_err(|_| {
        BackendError::ArtifactUnavailable {
            message: "Canonical transcript is no longer available.".to_string(),
        }
    })?;
    if !transcript.is_file() || !transcript.starts_with(&task) || !transcript.starts_with(&root) {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Canonical transcript failed containment validation.".to_string(),
        });
    }
    Ok(task)
}

fn generate_task_content_blocking(
    app: AppHandle,
    active: ActiveProcess,
    task_folder: String,
    template_id: String,
    deepseek_api_key: Option<String>,
    llm_config: ProviderSelection,
) -> Result<GenerateContentResult, String> {
    let output_root = resolve_task_library_output_root(&app)
        .map_err(|_| {
            String::from(BackendError::ArtifactValidationFailed {
                message: "Task Library output root is unavailable.".to_string(),
            })
        })?
        .ok_or_else(|| {
            String::from(BackendError::ArtifactUnavailable {
                message: "Task Library is empty.".to_string(),
            })
        })?;
    let discovery = list_tasks_from_existing_root(&output_root).map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Task Library discovery failed.".to_string(),
            code: None,
        })
    })?;
    let task_dir =
        resolve_generation_task(&output_root, &discovery, &task_folder).map_err(String::from)?;
    let desktop_settings =
        load_desktop_settings(&desktop_settings_path(&app).map_err(String::from)?)
            .map_err(String::from)?;
    validate_llm_selection(&llm_config).map_err(String::from)?;
    if llm_config != desktop_settings.llm {
        return Err(String::from(BackendError::InvalidInput {
            message: "Generated-content settings changed. Refresh and try again.".to_string(),
        }));
    }
    let runtime_mode = RuntimeMode::current();
    let environment = credential_environment(runtime_mode).map_err(String::from)?;
    let credential = resolve_credential(
        &KeyringCredentialStore,
        CredentialKey::DeepseekApiKey,
        deepseek_api_key.as_deref(),
        runtime_mode,
        &environment,
    )
    .map_err(BackendError::from)
    .map_err(String::from)?
    .ok_or_else(|| String::from(missing_api_key(CredentialKey::DeepseekApiKey)))?;
    let mut cmd = Command::new(go_sidecar_binary().map_err(String::from)?);
    cmd.arg("generate-content")
        .arg("--task-dir")
        .arg(&task_dir)
        .arg("--template-id")
        .arg(&template_id)
        .arg("--templates-file")
        .arg(custom_templates_path(&app).map_err(String::from)?)
        .arg("--llm-provider")
        .arg(&llm_config.provider_id)
        .arg("--llm-model")
        .arg(&llm_config.model_id)
        .arg("--control-stdin");
    let isolated = isolated_go_project_root().map_err(String::from)?;
    cmd.arg("--project-root")
        .arg(&isolated)
        .current_dir(&isolated)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .env("DEEPSEEK_API_KEY", &credential.value);
    let secrets = vec![credential.value];
    let runner = spawn_and_watch(&app, &active, cmd, &secrets, true).map_err(String::from)?;
    let (code, stdout, stderr) = runner.wait()?;
    let _ = app.emit("process-done", DoneEvent { code });
    if code != Some(0) {
        return Err(String::from(classify_process_failure(
            code,
            &stderr,
            &secrets,
            "The generation was cancelled before it finished.",
        )));
    }
    let generation_id = stdout
        .iter()
        .find_map(|line| {
            line.trim()
                .strip_prefix("Generation ID:")
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_string)
        })
        .ok_or_else(|| {
            String::from(BackendError::ProcessFailed {
                message: "Generation sidecar returned an incomplete result.".to_string(),
                code,
            })
        })?;
    Ok(GenerateContentResult { generation_id })
}

#[tauri::command]
async fn generate_task_content(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_folder: String,
    template_id: String,
    deepseek_api_key: Option<String>,
    llm_config: ProviderSelection,
) -> Result<GenerateContentResult, String> {
    let active = active.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        generate_task_content_blocking(
            app,
            active,
            task_folder,
            template_id,
            deepseek_api_key,
            llm_config,
        )
    })
    .await
    .map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Generation worker failed.".to_string(),
            code: None,
        })
    })?
}

// ---------------------------------------------------------------------------
// Secure Task Library artifact actions
// ---------------------------------------------------------------------------

#[derive(Clone, Copy)]
enum TaskArtifactAction {
    Open,
    Reveal,
}

fn validated_relative_hint(
    value: &str,
    label: &str,
    allow_nested: bool,
) -> Result<PathBuf, BackendError> {
    if value.is_empty() || value.contains(['\0', '\\', ':']) {
        return Err(BackendError::InvalidInput {
            message: format!("{label} is invalid."),
        });
    }
    let path = Path::new(value);
    if path.is_absolute() {
        return Err(BackendError::InvalidInput {
            message: format!("{label} must be a relative identifier."),
        });
    }
    let components = path.components().collect::<Vec<_>>();
    if components.is_empty()
        || components
            .iter()
            .any(|component| !matches!(component, Component::Normal(_)))
        || (!allow_nested && components.len() != 1)
    {
        return Err(BackendError::InvalidInput {
            message: format!("{label} contains invalid path components."),
        });
    }
    Ok(path.to_path_buf())
}

fn resolve_validated_task_artifact(
    output_root: &Path,
    discovery: &DiscoveryResult,
    task_folder: &str,
    artifact_reference: &str,
) -> Result<PathBuf, BackendError> {
    let task_hint = validated_relative_hint(task_folder, "Task folder", false)?;
    let artifact_hint = validated_relative_hint(artifact_reference, "Artifact reference", true)?;

    let task = discovery
        .tasks
        .iter()
        .find(|task| task.task_folder == task_folder)
        .ok_or_else(|| BackendError::ArtifactUnavailable {
            message: "Task is no longer available. Refresh the Task Library.".to_string(),
        })?;
    let listed_primary = task.primary_artifacts.iter().any(|artifact| {
        artifact.status == "completed" && artifact.reference.as_deref() == Some(artifact_reference)
    });
    let listed_canonical = task.canonical_transcript.as_ref().is_some_and(|artifact| {
        artifact.status == "completed" && artifact.reference.as_deref() == Some(artifact_reference)
    });
    let listed_generation = task
        .generated_content
        .iter()
        .flat_map(|group| group.versions.iter())
        .any(|version| {
            // A completed generation is a deliverable. A failed generation is only
            // openable when the runtime explicitly preserved a usable partial
            // output for it, and it stays labelled as incomplete in the UI.
            version.reference.as_deref() == Some(artifact_reference)
                && (version.status == "completed" || version.partial_output)
        });
    if !listed_primary && !listed_canonical && !listed_generation {
        return Err(BackendError::ArtifactUnavailable {
            message: "Artifact is no longer listed for this task. Refresh the Task Library."
                .to_string(),
        });
    }

    let canonical_root =
        output_root
            .canonicalize()
            .map_err(|_| BackendError::ArtifactValidationFailed {
                message: "Task Library output root is unavailable.".to_string(),
            })?;
    if !canonical_root.is_dir() {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Task Library output root is unavailable.".to_string(),
        });
    }

    let canonical_task = canonical_root.join(task_hint).canonicalize().map_err(|_| {
        BackendError::ArtifactUnavailable {
            message: "Task folder is no longer available. Refresh the Task Library.".to_string(),
        }
    })?;
    if !canonical_task.is_dir() || !canonical_task.starts_with(&canonical_root) {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Task folder failed containment validation.".to_string(),
        });
    }

    let canonical_artifact = canonical_task
        .join(artifact_hint)
        .canonicalize()
        .map_err(|_| BackendError::ArtifactUnavailable {
            message: "Artifact is no longer available. Refresh the Task Library.".to_string(),
        })?;
    if !canonical_artifact.starts_with(&canonical_task)
        || !canonical_artifact.starts_with(&canonical_root)
    {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Artifact failed containment validation.".to_string(),
        });
    }
    if !canonical_artifact.is_file() {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Artifact target is not a regular file.".to_string(),
        });
    }
    Ok(canonical_artifact)
}

#[cfg(target_os = "macos")]
fn perform_task_artifact_action(
    path: &Path,
    action: TaskArtifactAction,
) -> Result<(), BackendError> {
    let mut command = Command::new("open");
    if matches!(action, TaskArtifactAction::Reveal) {
        command.arg("-R");
    }
    let status = command
        .arg(path)
        .status()
        .map_err(|_| BackendError::ArtifactActionFailed {
            message: "Could not start the system artifact action.".to_string(),
        })?;
    if !status.success() {
        return Err(BackendError::ArtifactActionFailed {
            message: "The system artifact action failed.".to_string(),
        });
    }
    Ok(())
}

#[cfg(target_os = "windows")]
fn perform_task_artifact_action(
    path: &Path,
    action: TaskArtifactAction,
) -> Result<(), BackendError> {
    let mut command = Command::new("explorer.exe");
    if matches!(action, TaskArtifactAction::Reveal) {
        command.arg("/select,");
    }
    let status = command
        .arg(path)
        .status()
        .map_err(|_| BackendError::ArtifactActionFailed {
            message: "Could not start the system artifact action.".to_string(),
        })?;
    if !status.success() {
        return Err(BackendError::ArtifactActionFailed {
            message: "The system artifact action failed.".to_string(),
        });
    }
    Ok(())
}

#[cfg(target_os = "linux")]
fn perform_task_artifact_action(
    path: &Path,
    action: TaskArtifactAction,
) -> Result<(), BackendError> {
    let target = match action {
        TaskArtifactAction::Open => path,
        TaskArtifactAction::Reveal => {
            path.parent()
                .ok_or_else(|| BackendError::ArtifactActionFailed {
                    message: "Could not resolve the artifact folder.".to_string(),
                })?
        }
    };
    let status = Command::new("xdg-open").arg(target).status().map_err(|_| {
        BackendError::ArtifactActionFailed {
            message: "Could not start the system artifact action.".to_string(),
        }
    })?;
    if !status.success() {
        return Err(BackendError::ArtifactActionFailed {
            message: "The system artifact action failed.".to_string(),
        });
    }
    Ok(())
}

/// resolve_resumable_task revalidates everything the runtime owns before a
/// resume is dispatched: current membership in the Task Library, directory
/// containment, and the Task lock. A path handed over by the frontend is never
/// trusted on its own.
fn resolve_resumable_task(
    output_root: &Path,
    discovery: &DiscoveryResult,
    task_folder: &str,
    require_can_resume: bool,
) -> Result<PathBuf, BackendError> {
    let hint = validated_relative_hint(task_folder, "Task folder", false)?;
    let summary = discovery
        .tasks
        .iter()
        .find(|task| task.task_folder == task_folder)
        .ok_or_else(|| BackendError::ArtifactUnavailable {
            message: "Task is no longer available. Refresh the Task Library.".to_string(),
        })?;
    if let Some(actions) = summary.actions.as_ref() {
        if actions.locked || actions.has_reason("task_locked") {
            return Err(BackendError::TaskLocked {
                message: "Task is locked by another process.".to_string(),
            });
        }
        // The runtime already decided this action is unavailable; re-checking it
        // here keeps a stale Task Library view from dispatching doomed work.
        if require_can_resume && !actions.can_resume {
            let message = actions
                .blocking_reasons
                .first()
                .map(|reason| reason.message.clone())
                .unwrap_or_else(|| {
                    "Task cannot continue right now. Refresh the Task Library.".to_string()
                });
            return Err(BackendError::ArtifactUnavailable { message });
        }
    }
    let root = output_root
        .canonicalize()
        .map_err(|_| BackendError::ArtifactValidationFailed {
            message: "Task Library output root is unavailable.".to_string(),
        })?;
    if !root.is_dir() {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Task Library output root is unavailable.".to_string(),
        });
    }
    let task = root
        .join(hint)
        .canonicalize()
        .map_err(|_| BackendError::ArtifactUnavailable {
            message: "Task is no longer available.".to_string(),
        })?;
    if !task.is_dir() || !task.starts_with(&root) {
        return Err(BackendError::ArtifactValidationFailed {
            message: "Task folder failed containment validation.".to_string(),
        });
    }
    Ok(task)
}

/// resolve_retryable_generation finds one generation that the runtime currently
/// reports as retryable for this Task.
fn resolve_retryable_generation(
    output_root: &Path,
    discovery: &DiscoveryResult,
    task_folder: &str,
    generation_id: &str,
) -> Result<PathBuf, BackendError> {
    // A retry only needs the Task itself and its lock; the speech-side reasons
    // that may block a resume do not apply to a generation retry.
    let task = resolve_resumable_task(output_root, discovery, task_folder, false)?;
    let summary = discovery
        .tasks
        .iter()
        .find(|task| task.task_folder == task_folder)
        .ok_or_else(|| BackendError::ArtifactUnavailable {
            message: "Task is no longer available. Refresh the Task Library.".to_string(),
        })?;
    let retryable = summary
        .generated_content
        .iter()
        .flat_map(|group| group.versions.iter())
        .any(|version| version.generation_id == generation_id && version.retryable == Some(true));
    if !retryable {
        return Err(BackendError::ArtifactUnavailable {
            message: "Generation is not retryable. Refresh the Task Library.".to_string(),
        });
    }
    Ok(task)
}

fn run_validated_task_artifact_action<F>(
    output_root: &Path,
    discovery: &DiscoveryResult,
    task_folder: &str,
    artifact_reference: &str,
    requested_action: TaskArtifactAction,
    action: F,
) -> Result<(), BackendError>
where
    F: FnOnce(&Path, TaskArtifactAction) -> Result<(), BackendError>,
{
    let artifact =
        resolve_validated_task_artifact(output_root, discovery, task_folder, artifact_reference)?;
    // Keep validation adjacent to the action. Platform APIs cannot eliminate
    // every filesystem race, so every request revalidates and nothing is cached.
    action(&artifact, requested_action)
}

fn task_artifact_action_blocking(
    app: AppHandle,
    task_folder: String,
    artifact_reference: String,
    action: TaskArtifactAction,
) -> Result<(), String> {
    let output_root = resolve_task_library_output_root(&app)
        .map_err(|_| {
            String::from(BackendError::ArtifactValidationFailed {
                message: "Task Library output root is unavailable.".to_string(),
            })
        })?
        .ok_or_else(|| {
            String::from(BackendError::ArtifactUnavailable {
                message: "Task Library is empty. Refresh the Task Library.".to_string(),
            })
        })?;
    let discovery = list_tasks_from_existing_root(&output_root).map_err(|_| {
        String::from(BackendError::ProcessFailed {
            message: "Task Library discovery failed.".to_string(),
            code: None,
        })
    })?;
    run_validated_task_artifact_action(
        &output_root,
        &discovery,
        &task_folder,
        &artifact_reference,
        action,
        perform_task_artifact_action,
    )
    .map_err(String::from)
}

async fn task_artifact_action(
    app: AppHandle,
    task_folder: String,
    artifact_reference: String,
    action: TaskArtifactAction,
) -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(move || {
        task_artifact_action_blocking(app, task_folder, artifact_reference, action)
    })
    .await
    .map_err(|_| task_artifact_worker_error())?
}

fn task_artifact_worker_error() -> String {
    String::from(BackendError::ProcessFailed {
        message: "Task artifact worker failed.".to_string(),
        code: None,
    })
}

#[tauri::command]
async fn open_task_artifact(
    app: AppHandle,
    task_folder: String,
    artifact_reference: String,
) -> Result<(), String> {
    task_artifact_action(
        app,
        task_folder,
        artifact_reference,
        TaskArtifactAction::Open,
    )
    .await
}

#[tauri::command]
async fn reveal_task_artifact(
    app: AppHandle,
    task_folder: String,
    artifact_reference: String,
) -> Result<(), String> {
    task_artifact_action(
        app,
        task_folder,
        artifact_reference,
        TaskArtifactAction::Reveal,
    )
    .await
}

// ---------------------------------------------------------------------------
// open_task_folder
// ---------------------------------------------------------------------------

#[tauri::command]
fn open_task_folder(app: AppHandle, task_folder: String) -> Result<(), String> {
    if task_folder.is_empty() {
        return Err("No task folder specified. Run a transcription first.".into());
    }

    let (abs_output, _) = resolve_output_root(&app).map_err(String::from)?;
    let abs_candidate =
        canonicalize_path_within(&abs_output, Path::new(&task_folder), "Task folder").map_err(
            |_| "Cannot open folders outside the configured output directory.".to_string(),
        )?;

    if !abs_candidate.is_dir() {
        return Err(format!("Path is not a directory: {}", task_folder));
    }

    #[cfg(target_os = "macos")]
    let opener = "open";
    #[cfg(target_os = "windows")]
    let opener = "explorer";
    #[cfg(target_os = "linux")]
    let opener = "xdg-open";

    let status = Command::new(opener)
        .arg(abs_candidate.as_os_str())
        .status()
        .map_err(|e| format!("Could not open output directory: {}", e))?;

    if !status.success() {
        return Err("Could not open output directory. Check your file manager settings.".into());
    }

    Ok(())
}

// ---------------------------------------------------------------------------
// save_env_key
// ---------------------------------------------------------------------------

const ALLOWED_ENV_KEYS: &[&str] = &["MIMO_API_KEY", "DEEPSEEK_API_KEY"];

fn ensure_development_env_compatibility(runtime_mode: RuntimeMode) -> Result<(), String> {
    if runtime_mode == RuntimeMode::Production {
        return Err("Saving credentials to the project-root .env is development-only.".into());
    }
    Ok(())
}

#[tauri::command]
fn save_env_key(key: String, value: String) -> Result<(), String> {
    ensure_development_env_compatibility(RuntimeMode::current())?;
    if key.is_empty() {
        return Err("Key name cannot be empty.".into());
    }
    if value.is_empty() {
        return Err("Key value cannot be empty.".into());
    }
    if !ALLOWED_ENV_KEYS.contains(&key.as_str()) {
        return Err(format!(
            "Key '{}' is not allowed. Allowed keys: {:?}",
            key, ALLOWED_ENV_KEYS
        ));
    }

    let root = resolve_project_root();
    let env_path = root.join(".env");
    let new_line = format!("{}={}", key, value);

    if !env_path.exists() {
        fs::write(&env_path, new_line + "\n")
            .map_err(|e| format!("Failed to create .env: {}", e))?;
        return Ok(());
    }

    let content =
        fs::read_to_string(&env_path).map_err(|e| format!("Failed to read .env: {}", e))?;
    let lines: Vec<&str> = content.split('\n').collect();
    let mut output: Vec<String> = Vec::new();
    let mut replaced = false;

    for line in &lines {
        let stripped = line.trim_start();
        if !stripped.starts_with('#') && stripped.starts_with(&format!("{}=", key)) {
            if !replaced {
                output.push(new_line.clone());
                replaced = true;
            }
            continue;
        }
        output.push(line.to_string());
    }

    if !replaced {
        if let Some(last) = output.last() {
            if !last.is_empty() {
                output.push(String::new());
            }
        }
        output.push(new_line);
    }

    let result = output.join("\n");
    let result = if result.ends_with('\n') {
        result
    } else {
        result + "\n"
    };

    fs::write(&env_path, result).map_err(|e| format!("Failed to write .env: {}", e))?;

    Ok(())
}

// ---------------------------------------------------------------------------
// App entry point
// ---------------------------------------------------------------------------

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let app = tauri::Builder::default()
        .manage(ActiveProcess::default())
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let config = app.path().app_config_dir();
            let data = app.path().app_data_dir();
            if let (Ok(config), Ok(data)) = (config, data) {
                if let Ok(paths) = AppNamespacePaths::from_current(config, data) {
                    let report = migrate_app_namespace(&paths);
                    if !report.completed || !report.issues.is_empty() {
                        eprintln!("Voxly app-data migration completed with a recoverable issue.");
                    }
                }
            }
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            get_runtime_info,
            get_runtime_preflight,
            get_provider_settings,
            set_provider_settings,
            get_output_root,
            set_output_root,
            reset_output_root,
            open_output_root,
            open_task_folder,
            run_meeting_asr,
            resume_meeting_asr,
            list_tasks,
            preview_task_cache,
            get_storage_settings,
            set_storage_settings,
            get_storage_summary,
            preview_cache_batch,
            clean_cache_batch,
            run_storage_maintenance,
            clean_task_cache,
            get_task_metadata,
            update_task_metadata,
            list_prompt_templates,
            create_custom_template,
            update_custom_template,
            delete_custom_template,
            list_providers,
            get_local_model_catalog,
            get_local_model_status,
            start_model_download,
            cancel_model_download,
            import_local_model,
            delete_local_model,
            get_model_download_settings,
            update_model_download_settings,
            test_huggingface_endpoint,
            generate_task_content,
            retry_task_generation,
            resume_task,
            cancel_task,
            open_task_artifact,
            reveal_task_artifact,
            get_credential_status,
            save_credential,
            delete_credential,
            save_env_key,
        ])
        .build(tauri::generate_context!())
        .expect("error while building Tauri application");

    app.run(|app_handle, event| {
        if matches!(event, RunEvent::ExitRequested { .. }) {
            // Leaving the app must end the processes this app owns. The stop is
            // cooperative first, so the sidecar can finish its current atomic
            // write and stop the ffmpeg it started; only a run that ignores the
            // request within the exit grace period is forced down.
            let active = app_handle.state::<ActiveProcess>();
            active.request_stop();
            let deadline = Instant::now() + Duration::from_secs(2);
            while Instant::now() < deadline && active.is_active() {
                thread::sleep(Duration::from_millis(50));
            }
            if active.is_active() {
                active.terminate();
            }
        }
    });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::sync::Mutex;

    #[test]
    fn voxly_branding_metadata_is_consistent() {
        let config: serde_json::Value =
            serde_json::from_str(include_str!("../tauri.conf.json")).unwrap();
        assert_eq!(config["productName"], "Voxly");
        assert_eq!(config["identifier"], "com.skybridgem.voxly");
        assert_eq!(config["version"], env!("CARGO_PKG_VERSION"));
        assert_eq!(
            config["app"]["windows"][0]["title"],
            "Voxly — Local Voice Workspace"
        );
        for icon in [
            "icons/32x32.png",
            "icons/128x128.png",
            "icons/128x128@2x.png",
            "icons/icon.icns",
            "icons/icon.ico",
        ] {
            assert!(Path::new(env!("CARGO_MANIFEST_DIR")).join(icon).is_file());
        }
    }

    #[derive(Default)]
    struct TestCredentialStore {
        values: Mutex<HashMap<CredentialKey, String>>,
    }

    impl CredentialStore for TestCredentialStore {
        fn get(&self, key: CredentialKey) -> Result<Option<String>, CredentialError> {
            Ok(self.values.lock().unwrap().get(&key).cloned())
        }

        fn save(&self, key: CredentialKey, value: &str) -> Result<(), CredentialError> {
            self.values.lock().unwrap().insert(key, value.to_string());
            Ok(())
        }

        fn delete(&self, key: CredentialKey) -> Result<(), CredentialError> {
            self.values.lock().unwrap().remove(&key);
            Ok(())
        }
    }

    // --- stdout parsing ---

    #[test]
    fn test_parsed_output_empty_lines() {
        let out = ParsedOutput::parse_lines(&[]);
        assert!(out.task_folder.is_none());
        assert!(out.transcript_path.is_none());
    }

    #[test]
    fn test_parsed_output_task_folder() {
        let lines = vec!["Task folder: /tmp/task".to_string()];
        let out = ParsedOutput::parse_lines(&lines);
        assert_eq!(out.task_folder.unwrap(), "/tmp/task");
    }

    #[test]
    fn test_parsed_output_all_fields() {
        let lines = vec![
            "Transcript saved: /tmp/transcript.md".to_string(),
            "Notes input saved: /tmp/notes_input.md".to_string(),
            "Meeting minutes saved: /tmp/meeting_minutes.md".to_string(),
            "Task folder: /tmp/task".to_string(),
        ];
        let out = ParsedOutput::parse_lines(&lines);
        assert_eq!(out.transcript_path.unwrap(), "/tmp/transcript.md");
        assert_eq!(out.notes_input_path.unwrap(), "/tmp/notes_input.md");
        assert_eq!(out.minutes_path.unwrap(), "/tmp/meeting_minutes.md");
    }

    #[test]
    fn test_validated_output_requires_status_and_expected_root() {
        let root = std::env::temp_dir().join("meeting-asr-desktop-output-contract");
        let task = root.join("task");
        fs::create_dir_all(&task).unwrap();
        let canonical_root = root.canonicalize().unwrap();
        let transcript = task.join("transcript.md");
        fs::write(&transcript, "[Part 001]\n\ntext\n").unwrap();
        let lines = vec![
            "Status: Done".to_string(),
            format!("Task folder: {}", task.display()),
            format!("Transcript saved: {}", transcript.display()),
        ];
        let out = ParsedOutput::parse_lines(&lines);
        assert!(out
            .validate_required_under("transcript", Some(&canonical_root))
            .is_ok());
        assert!(out
            .validate_required_under("transcript", Some(&std::env::temp_dir().join("other-root")),)
            .is_err());

        let without_status = ParsedOutput::parse_lines(&lines[1..]);
        assert!(without_status
            .validate_required_under("transcript", Some(&canonical_root))
            .is_err());
        fs::remove_dir_all(&root).ok();
    }

    #[test]
    fn test_parsed_output_ignores_incomplete_prefixes() {
        let lines = vec![
            "Task folder".to_string(),
            "Transcript saved: ".to_string(),
            "Meeting minutes saved: /tmp/mm.md".to_string(),
        ];
        let out = ParsedOutput::parse_lines(&lines);
        assert!(out.task_folder.is_none());
        assert!(out.transcript_path.is_none());
        assert_eq!(out.minutes_path.unwrap(), "/tmp/mm.md");
    }

    #[test]
    fn test_validate_required_transcript_missing_task_folder() {
        let out = ParsedOutput {
            transcript_path: Some("/tmp/t.md".into()),
            ..Default::default()
        };
        assert!(out.validate_required_under("transcript", None).is_err());
    }

    #[test]
    fn test_validate_required_minutes_missing_notes() {
        let out = ParsedOutput {
            task_folder: Some("/tmp".into()),
            transcript_path: Some("/tmp/transcript.md".into()),
            ..Default::default()
        };
        assert!(out.validate_required_under("minutes", None).is_err());
    }

    // --- status normalization ---

    #[test]
    fn test_normalize_status_splitting() {
        assert_eq!(normalize_status("Splitting audio"), "splitting");
    }

    #[test]
    fn test_normalize_status_transcribing() {
        assert_eq!(normalize_status("Transcribing 5 chunk(s)"), "transcribing");
    }

    #[test]
    fn test_normalize_status_all_transcribed() {
        assert_eq!(
            normalize_status("All chunks already transcribed"),
            "transcribing"
        );
    }

    #[test]
    fn test_normalize_status_merging() {
        assert_eq!(normalize_status("Merging transcript"), "merging");
    }

    #[test]
    fn test_normalize_status_notes() {
        assert_eq!(normalize_status("Preparing notes input"), "notes");
    }

    #[test]
    fn test_normalize_status_minutes() {
        assert_eq!(normalize_status("Generating meeting minutes"), "minutes");
    }

    #[test]
    fn test_normalize_status_generated_content() {
        assert_eq!(normalize_status("Generating content"), "generating");
    }

    #[test]
    fn test_normalize_status_done() {
        assert_eq!(normalize_status("Done"), "done");
    }

    #[test]
    fn test_normalize_status_nothing_to_resume() {
        assert_eq!(
            normalize_status("All outputs are valid — nothing to resume"),
            "done"
        );
    }

    #[test]
    fn test_normalize_status_unknown_passthrough() {
        assert_eq!(
            normalize_status("Some unknown status"),
            "Some unknown status"
        );
    }

    #[test]
    fn test_parse_progress_accepts_only_valid_counts() {
        assert_eq!(parse_progress("1/3"), Some((1, 3)));
        assert_eq!(parse_progress(" 3 / 3 "), Some((3, 3)));
        assert_eq!(parse_progress("0/3"), None);
        assert_eq!(parse_progress("4/3"), None);
        assert_eq!(parse_progress("1/0"), None);
        assert_eq!(parse_progress("not-progress"), None);
    }

    // --- error classification ---

    #[test]
    fn test_classify_manifest_error() {
        let err = classify_stderr(&["Error: no manifest found".to_string()], Some(1), &[]);
        match err {
            BackendError::ManifestError { .. } => {}
            _ => panic!("expected ManifestError, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_task_locked() {
        let err = classify_stderr(
            &["Error: acquiring task lock: task is already being resumed".to_string()],
            Some(1),
            &[],
        );
        match err {
            BackendError::TaskLocked { .. } => {}
            _ => panic!("expected TaskLocked, got {:?}", err),
        }
    }

    #[test]
    fn test_cancel_exit_code_is_never_a_provider_failure() {
        let stderr =
            vec!["Error: task cancelled: the task was cancelled before it finished".to_string()];
        let cancelled = classify_process_failure(
            Some(CANCELLED_EXIT_CODE),
            &stderr,
            &[],
            "The generation was cancelled before it finished.",
        );
        match cancelled {
            BackendError::Interrupted { .. } => {}
            other => panic!("expected Interrupted, got {:?}", other),
        }

        // A real failure still classifies from stderr.
        let failed = classify_process_failure(
            Some(1),
            &["Error: ffmpeg was not found on PATH".to_string()],
            &[],
            "The generation was cancelled before it finished.",
        );
        match failed {
            BackendError::DependencyMissing { .. } => {}
            other => panic!("expected DependencyMissing, got {:?}", other),
        }
    }

    #[test]
    fn test_classify_dependency_missing() {
        let err = classify_stderr(
            &["Error: ffmpeg was not found on PATH".to_string()],
            Some(1),
            &[],
        );
        match err {
            BackendError::DependencyMissing { .. } => {}
            _ => panic!("expected DependencyMissing, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_missing_api_key() {
        let err = classify_stderr(
            &["Error: MIMO_API_KEY is not configured".to_string()],
            Some(1),
            &[],
        );
        match err {
            BackendError::MissingApiKey { .. } => {}
            _ => panic!("expected MissingApiKey, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_invalid_input() {
        let err = classify_stderr(&["Error: invalid --output-mode".to_string()], Some(2), &[]);
        match err {
            BackendError::InvalidInput { .. } => {}
            _ => panic!("expected InvalidInput, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_process_failed_unknown() {
        let err = classify_stderr(&["some random error".to_string()], Some(3), &[]);
        match err {
            BackendError::ProcessFailed { code, .. } => {
                assert_eq!(code, Some(3));
            }
            _ => panic!("expected ProcessFailed, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_process_failed_no_stderr() {
        let err = classify_stderr(&[], Some(1), &[]);
        match err {
            BackendError::ProcessFailed { code, .. } => {
                assert_eq!(code, Some(1));
            }
            _ => panic!("expected ProcessFailed, got {:?}", err),
        }
    }

    #[test]
    fn test_run_credentials_notes_require_only_mimo() {
        let store = TestCredentialStore::default();
        store
            .save(CredentialKey::MimoApiKey, "mimo-secret")
            .unwrap();

        let credentials = resolve_run_credentials(
            &store,
            "notes",
            DEFAULT_ASR_PROVIDER,
            RuntimeMode::Development,
            None,
            None,
            &EnvironmentValues::default(),
        )
        .unwrap();

        assert_eq!(credentials.mimo, "mimo-secret");
        assert!(credentials.deepseek.is_none());
    }

    #[test]
    fn test_run_credentials_minutes_require_both_keys() {
        let store = TestCredentialStore::default();
        store
            .save(CredentialKey::MimoApiKey, "mimo-secret")
            .unwrap();

        let missing = match resolve_run_credentials(
            &store,
            "minutes",
            DEFAULT_ASR_PROVIDER,
            RuntimeMode::Development,
            None,
            None,
            &EnvironmentValues::default(),
        ) {
            Ok(_) => panic!("minutes should require DEEPSEEK_API_KEY"),
            Err(error) => error,
        };
        match missing {
            BackendError::MissingApiKey { message } => {
                assert!(message.contains("DEEPSEEK_API_KEY"));
                assert!(!message.contains("mimo-secret"));
            }
            other => panic!("expected MissingApiKey, got {other:?}"),
        }

        store
            .save(CredentialKey::DeepseekApiKey, "deepseek-secret")
            .unwrap();
        let credentials = resolve_run_credentials(
            &store,
            "minutes",
            DEFAULT_ASR_PROVIDER,
            RuntimeMode::Development,
            None,
            None,
            &EnvironmentValues::default(),
        )
        .unwrap();
        assert_eq!(credentials.deepseek.as_deref(), Some("deepseek-secret"));
    }

    #[test]
    fn test_run_credentials_v2_generation_requires_deepseek() {
        let store = TestCredentialStore::default();
        store
            .values
            .lock()
            .unwrap()
            .insert(CredentialKey::MimoApiKey, "mimo".to_string());
        store
            .values
            .lock()
            .unwrap()
            .insert(CredentialKey::DeepseekApiKey, "deepseek".to_string());
        let environment = EnvironmentValues::default();
        let credentials = resolve_run_credentials(
            &store,
            "transcript_and_generate",
            DEFAULT_ASR_PROVIDER,
            RuntimeMode::Development,
            None,
            None,
            &environment,
        )
        .unwrap();
        assert_eq!(credentials.mimo, "mimo");
        assert_eq!(credentials.deepseek.as_deref(), Some("deepseek"));
    }

    #[test]
    fn test_run_credentials_local_transcript_needs_no_cloud_keys() {
        let store = TestCredentialStore::default();
        let credentials = resolve_run_credentials(
            &store,
            "transcript_only",
            LOCAL_ASR_PROVIDER,
            RuntimeMode::Production,
            Some("unused-mimo"),
            None,
            &EnvironmentValues::default(),
        )
        .unwrap();
        assert!(credentials.mimo.is_empty());
        assert!(credentials.deepseek.is_none());
    }

    #[test]
    fn test_run_credentials_local_generation_requires_only_deepseek() {
        let store = TestCredentialStore::default();
        let environment = EnvironmentValues::default();
        let missing = resolve_run_credentials(
            &store,
            "transcript_and_generate",
            LOCAL_ASR_PROVIDER,
            RuntimeMode::Production,
            None,
            None,
            &environment,
        )
        .err()
        .expect("generation needs a key");
        assert!(
            matches!(missing, BackendError::MissingApiKey { message } if message.contains("DEEPSEEK_API_KEY"))
        );
        let credentials = resolve_run_credentials(
            &store,
            "transcript_and_generate",
            LOCAL_ASR_PROVIDER,
            RuntimeMode::Production,
            None,
            Some("deepseek-session"),
            &environment,
        )
        .unwrap();
        assert!(credentials.mimo.is_empty());
        assert_eq!(credentials.deepseek.as_deref(), Some("deepseek-session"));
    }

    #[test]
    fn test_run_credentials_cloud_transcript_still_requires_mimo() {
        let missing = resolve_run_credentials(
            &TestCredentialStore::default(),
            "transcript_only",
            DEFAULT_ASR_PROVIDER,
            RuntimeMode::Production,
            None,
            None,
            &EnvironmentValues::default(),
        )
        .err()
        .expect("cloud ASR needs a key");
        assert!(
            matches!(missing, BackendError::MissingApiKey { message } if message.contains("MIMO_API_KEY"))
        );
    }

    #[test]
    fn test_local_asr_start_checks_real_readiness() {
        let mut status = LocalModelStatus {
            model_id: LOCAL_ASR_MODEL.to_string(),
            model_version: "2024-07-17".to_string(),
            state: "ready".to_string(),
            message: "Ready.".to_string(),
            installed_size_bytes: 1,
            runtime_id: Some("sherpa-onnx".to_string()),
            runtime_version: Some("1.13.8".to_string()),
        };
        assert!(validate_local_asr_start(BackendKind::Go, &status).is_ok());
        for state in ["not_installed", "model_invalid", "runtime_missing"] {
            status.state = state.to_string();
            assert!(validate_local_asr_start(BackendKind::Go, &status).is_err());
        }
    }

    #[test]
    fn test_secret_values_are_redacted_from_errors() {
        let secret = "mimo-secret-value".to_string();
        let err = classify_stderr(
            &[format!("backend echoed {secret}")],
            Some(1),
            std::slice::from_ref(&secret),
        );
        let json = err.to_json_string();
        assert!(!json.contains(&secret));
        assert!(json.contains("[REDACTED]"));
    }

    // --- error JSON serialisation ---

    #[test]
    fn test_error_json_serialization() {
        let err = BackendError::BackendNotFound {
            message: "not found".to_string(),
        };
        let json = serde_json::to_string(&err).unwrap();
        assert!(json.contains("backend_not_found"));
        assert!(json.contains("not found"));
    }

    #[test]
    fn test_error_json_deserialization() {
        let json = r#"{"category":"backend_not_found","message":"test"}"#;
        let err: BackendError = serde_json::from_str(json).unwrap();
        match err {
            BackendError::BackendNotFound { message } => assert_eq!(message, "test"),
            _ => panic!("unexpected variant"),
        }
    }

    // --- backend kind ---

    #[test]
    fn test_backend_kind_serialization() {
        assert!(serde_json::from_str::<BackendKind>("\"python\"").is_err());
        let go: BackendKind = serde_json::from_str("\"go\"").unwrap();
        assert_eq!(go, BackendKind::Go);
    }

    #[test]
    fn test_backend_kind_invalid() {
        assert!(serde_json::from_str::<BackendKind>("\"rust\"").is_err());
    }

    #[test]
    fn test_development_backend_policy_uses_go_and_rejects_python() {
        assert_eq!(
            resolve_backend_kind(None, RuntimeMode::Development).unwrap(),
            BackendKind::Go
        );
        assert!(matches!(
            resolve_backend_kind(Some("python"), RuntimeMode::Development),
            Err(BackendError::InvalidInput { .. })
        ));
        assert_eq!(
            resolve_backend_kind(Some("go"), RuntimeMode::Development).unwrap(),
            BackendKind::Go
        );
        assert!(matches!(
            resolve_backend_kind(Some("invalid"), RuntimeMode::Development),
            Err(BackendError::InvalidInput { .. })
        ));
    }

    #[test]
    fn test_runtime_info_reports_go_as_development_policy_default() {
        assert_eq!(
            runtime_info_for(RuntimeMode::Development).backend,
            BackendKind::Go
        );
    }

    #[test]
    fn test_missing_development_go_binary_is_not_ready_and_does_not_fallback() {
        let check = preflight_development_go_result(Err(BackendError::BackendNotFound {
            message: "synthetic missing binary".to_string(),
        }));
        assert!(!check.ready);
        assert!(check.message.contains("Build core-go/meeting-asr-go"));
        assert!(check.message.contains("MEETING_ASR_GO_BINARY"));
        assert!(!check.message.contains("synthetic missing binary"));
    }

    #[cfg(unix)]
    #[test]
    fn test_available_development_go_binary_is_ready() {
        use std::os::unix::fs::PermissionsExt;

        let base = test_dir("meeting-asr-preflight-development-go");
        fs::create_dir_all(&base).unwrap();
        let binary = base.join("meeting-asr-go");
        fs::write(
            &binary,
            b"#!/bin/sh\nprintf '%s\\n' 'meeting-asr-go version test'\n",
        )
        .unwrap();
        let mut permissions = fs::metadata(&binary).unwrap().permissions();
        permissions.set_mode(0o755);
        fs::set_permissions(&binary, permissions).unwrap();

        let check = preflight_development_go_result(Ok(binary));
        assert!(check.ready);
        assert!(check.message.contains("version-valid"));
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_production_backend_policy_uses_packaged_go_by_default() {
        assert_eq!(
            resolve_backend_kind(None, RuntimeMode::Production).unwrap(),
            BackendKind::Go
        );
        assert_eq!(
            resolve_backend_kind(Some("go"), RuntimeMode::Production).unwrap(),
            BackendKind::Go
        );
    }

    #[test]
    fn test_production_backend_policy_rejects_python() {
        match resolve_backend_kind(Some("python"), RuntimeMode::Production) {
            Err(BackendError::InvalidInput { message }) => {
                assert!(message.contains("Only 'go' is supported"));
            }
            other => panic!("expected production Python rejection, got {other:?}"),
        }
    }

    #[test]
    fn test_runtime_info_exposes_only_safe_policy_metadata() {
        let info = runtime_info_for(RuntimeMode::Production);
        assert_eq!(info.backend, BackendKind::Go);
        let json = serde_json::to_string(&info).unwrap();
        assert_eq!(json, r#"{"runtime_mode":"production","backend":"go"}"#);
        assert!(!json.contains("API_KEY"));
        assert!(!json.contains("secret"));
    }

    #[test]
    fn test_runtime_preflight_metadata_exposes_no_secrets() {
        let secret = "preflight-secret";
        let preflight = runtime_preflight_for(
            RuntimeMode::Production,
            BackendKind::Go,
            preflight_check(true, "sidecar ready"),
            preflight_check(true, "ffmpeg ready"),
            OutputRootPreflight {
                path: "/tmp/meeting-asr-output".to_string(),
                configured: false,
                ready: true,
                message: "output ready".to_string(),
            },
            CredentialPreflight {
                status: CredentialStatus {
                    key: "MIMO_API_KEY".to_string(),
                    configured: true,
                    source: CredentialSource::Session,
                },
                message: "MIMO_API_KEY is configured for this session.".to_string(),
            },
            CredentialPreflight {
                status: CredentialStatus {
                    key: "DEEPSEEK_API_KEY".to_string(),
                    configured: false,
                    source: CredentialSource::Missing,
                },
                message: "DEEPSEEK_API_KEY is not configured.".to_string(),
            },
        );
        let json = serde_json::to_string(&preflight).unwrap();
        assert!(!json.contains(secret));
        assert!(!json.contains("\"value\""));
        assert!(json.contains("MIMO_API_KEY"));
        assert!(json.contains("session"));
        assert!(!json.contains("\"ready_for_minutes\":true"));
    }

    #[test]
    fn test_preflight_missing_sidecar_is_not_ready() {
        let executable_dir = test_dir("meeting-asr-preflight-sidecar-missing");
        fs::create_dir_all(&executable_dir).unwrap();
        let check = preflight_sidecar_at(&executable_dir);
        assert!(!check.ready);
        assert!(check.message.contains("Packaged Go sidecar"));
        fs::remove_dir_all(executable_dir).ok();
    }

    #[test]
    fn test_ffmpeg_production_path_never_uses_host_path() {
        let dir = Path::new("/Applications/Voxly.app/Contents/MacOS");
        assert_eq!(
            ffmpeg_binary_for(RuntimeMode::Production, Some(dir)).unwrap(),
            dir.join("ffmpeg")
        );
        assert_eq!(
            ffmpeg_binary_for(RuntimeMode::Development, Some(dir)).unwrap(),
            PathBuf::from("ffmpeg")
        );
    }

    #[test]
    fn test_preflight_missing_ffmpeg_is_not_ready() {
        let missing = test_dir("meeting-asr-preflight-ffmpeg-missing").join("ffmpeg");
        let check = check_ffmpeg_at(&missing);
        assert!(!check.ready);
        assert!(check.message.contains("reinstall Voxly"));
    }

    #[test]
    fn test_preflight_output_root_does_not_create_directory() {
        let base = test_dir("meeting-asr-preflight-output-root");
        fs::create_dir_all(&base).unwrap();
        let requested = base.join("output");
        assert!(!requested.exists());
        let check = output_root_preflight_for(&requested, false);
        assert!(check.ready);
        assert!(!requested.exists());
        fs::remove_dir_all(base).ok();
    }

    // --- path validation ---

    #[test]
    fn test_canonicalize_abs_rejects_relative() {
        assert!(canonicalize_abs("relative/path", "test").is_err());
    }

    #[test]
    fn test_canonicalize_abs_rejects_nonexistent() {
        assert!(canonicalize_abs("/nonexistent/abc/def/ghi", "test").is_err());
    }

    #[test]
    fn test_canonicalize_abs_file_rejects_dir() {
        assert!(canonicalize_abs_file("/tmp", "test").is_err());
    }

    fn test_dir(name: &str) -> PathBuf {
        let unique = format!("{name}-{}", std::process::id());
        std::env::temp_dir().join(unique)
    }

    #[test]
    fn test_production_default_output_root_uses_documents() {
        let documents = test_dir("meeting-asr-documents");
        let root = get_default_output_root(RuntimeMode::Production, Some(&documents)).unwrap();
        assert_eq!(root, documents.join("Meeting ASR"));
    }

    #[test]
    fn test_development_default_output_root_uses_project_root() {
        let root = get_default_output_root(RuntimeMode::Development, None).unwrap();
        assert_eq!(root, resolve_project_root().join("output"));
    }

    #[test]
    fn test_current_runtime_mode_matches_build_profile() {
        let expected = if cfg!(debug_assertions) {
            RuntimeMode::Development
        } else {
            RuntimeMode::Production
        };
        assert_eq!(RuntimeMode::current(), expected);
    }

    #[test]
    fn test_env_compatibility_save_is_development_only() {
        assert!(ensure_development_env_compatibility(RuntimeMode::Development).is_ok());
        assert!(ensure_development_env_compatibility(RuntimeMode::Production).is_err());
    }

    #[test]
    fn test_production_default_output_root_requires_documents_directory() {
        assert!(get_default_output_root(RuntimeMode::Production, None).is_err());
    }

    #[test]
    fn test_validate_output_root_creates_and_canonicalizes_directory() {
        let base = test_dir("meeting-asr-output-root");
        let requested = base.join("nested").join("output");
        let root = validate_output_root(&requested).unwrap();
        assert!(root.is_dir());
        assert_eq!(root, requested.canonicalize().unwrap());
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_validate_output_root_rejects_relative_path() {
        assert!(validate_output_root(Path::new("relative-output")).is_err());
    }

    #[test]
    fn test_settings_store_only_persists_non_sensitive_output_root() {
        let base = test_dir("meeting-asr-desktop-settings");
        let settings_path = base.join(DESKTOP_SETTINGS_FILE);
        let output_root = base.join("output");
        fs::create_dir_all(&output_root).unwrap();

        let saved = set_configured_output_root(&settings_path, &output_root).unwrap();
        let settings = load_desktop_settings(&settings_path).unwrap();
        assert_eq!(configured_output_root_path(&settings).unwrap(), Some(saved));

        let json = fs::read_to_string(&settings_path).unwrap();
        assert!(json.contains("output_root"));
        assert!(!json.contains("API_KEY"));
        assert!(!json.contains("transcript"));
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_provider_settings_defaults_migrate_and_round_trip_without_credentials() {
        let base = test_dir("meeting-asr-provider-settings");
        fs::create_dir_all(&base).unwrap();
        let path = base.join(DESKTOP_SETTINGS_FILE);

        fs::write(&path, r#"{"output_root":"/tmp/example","language":"zh","theme":"dark","deepseek_model":"deepseek-v4-flash"}"#).unwrap();
        let migrated = load_desktop_settings(&path).unwrap();
        assert_eq!(migrated.asr, ProviderSelection::default_asr());
        assert_eq!(migrated.llm, ProviderSelection::default_llm());
        assert_eq!(migrated.language.as_deref(), Some("zh"));
        assert_eq!(migrated.theme.as_deref(), Some("dark"));

        save_desktop_settings(&path, &migrated).unwrap();
        let json = fs::read_to_string(&path).unwrap();
        assert!(json.contains("\"asr\"") && json.contains("\"llm\""));
        assert!(!json.contains("deepseek_model"));
        assert!(!json.contains("API_KEY") && !json.contains("secret"));

        let saved = set_provider_settings_at(
            &path,
            &ActiveProcess::default(),
            ProviderSelection::default_asr(),
            ProviderSelection::default_llm(),
            "zh".to_string(),
            false,
        )
        .unwrap();
        assert_eq!(saved.asr, ProviderSelection::default_asr());
        assert_eq!(saved.llm, ProviderSelection::default_llm());
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_provider_settings_local_language_persists_and_validates() {
        let base = test_dir("voxly-local-language");
        let path = base.join(DESKTOP_SETTINGS_FILE);
        assert_eq!(
            load_desktop_settings(&path).unwrap().local_asr_language,
            "zh"
        );
        let saved = set_provider_settings_at(
            &path,
            &ActiveProcess::default(),
            ProviderSelection::default_asr(),
            ProviderSelection::default_llm(),
            "auto".to_string(),
            false,
        )
        .unwrap();
        assert_eq!(saved.local_asr_language, "auto");
        assert_eq!(
            load_desktop_settings(&path).unwrap().local_asr_language,
            "auto"
        );
        assert!(set_provider_settings_at(
            &path,
            &ActiveProcess::default(),
            ProviderSelection::default_asr(),
            ProviderSelection::default_llm(),
            "invalid".to_string(),
            false,
        )
        .is_err());
        assert_eq!(
            load_desktop_settings(&path).unwrap().local_asr_language,
            "auto"
        );
    }

    #[test]
    fn test_provider_settings_vad_preserves_storage_preferences() {
        let base = test_dir("voxly-vad-settings");
        let path = base.join(DESKTOP_SETTINGS_FILE);
        let mut initial = DesktopSettings::default();
        initial.auto_clean_chunks = true;
        save_desktop_settings(&path, &initial).unwrap();
        let saved = set_provider_settings_at(
            &path,
            &ActiveProcess::default(),
            ProviderSelection {
                provider_id: LOCAL_ASR_PROVIDER.to_string(),
                model_id: LOCAL_ASR_MODEL.to_string(),
            },
            ProviderSelection::default_llm(),
            "zh".to_string(),
            true,
        )
        .unwrap();
        assert!(saved.local_vad_enabled);
        let reloaded = load_desktop_settings(&path).unwrap();
        assert!(reloaded.local_vad_enabled && reloaded.auto_clean_chunks);
        assert!(reloaded.cleanup_temporary_on_startup);
    }

    #[test]
    fn test_provider_settings_reject_invalid_values_and_active_mutation() {
        let invalid_asr = ProviderSelection {
            provider_id: "other".to_string(),
            model_id: DEFAULT_ASR_MODEL.to_string(),
        };
        assert!(validate_asr_selection(&invalid_asr).is_err());
        let invalid_llm = ProviderSelection {
            provider_id: DEFAULT_LLM_PROVIDER.to_string(),
            model_id: "other".to_string(),
        };
        assert!(validate_llm_selection(&invalid_llm).is_err());

        assert!(validate_output_root_mutation_state(true).is_err());
    }

    #[test]
    fn test_provider_readiness_is_local_safe_and_provider_specific() {
        let ready_asr = provider_readiness(&ProviderSelection::default_asr(), true, true, None);
        assert!(ready_asr.ready && ready_asr.blocking_reasons.is_empty());
        let missing_llm = provider_readiness(&ProviderSelection::default_llm(), false, false, None);
        assert!(!missing_llm.ready);
        assert!(missing_llm
            .blocking_reasons
            .iter()
            .any(|reason| reason.contains("credential")));
        let invalid = provider_readiness(
            &ProviderSelection {
                provider_id: DEFAULT_ASR_PROVIDER.to_string(),
                model_id: "invalid".to_string(),
            },
            true,
            true,
            None,
        );
        assert!(!invalid.ready);
        let json = serde_json::to_string(&invalid).unwrap();
        assert!(!json.contains("API_KEY") && !json.contains("secret"));
        let local = ProviderSelection {
            provider_id: LOCAL_ASR_PROVIDER.to_string(),
            model_id: LOCAL_ASR_MODEL.to_string(),
        };
        assert!(validate_asr_selection(&local).is_ok());
        let local_status = LocalModelStatus {
            model_id: LOCAL_ASR_MODEL.to_string(),
            model_version: "2024-07-17".to_string(),
            state: "runtime_missing".to_string(),
            message: "Model installed; runtime missing.".to_string(),
            installed_size_bytes: 1,
            runtime_id: Some("sherpa-onnx".to_string()),
            runtime_version: None,
        };
        let readiness = provider_readiness(&local, false, true, Some(&local_status));
        assert!(!readiness.ready);
        assert!(readiness
            .blocking_reasons
            .iter()
            .any(|reason| reason.contains("runtime")));
        assert!(!readiness
            .blocking_reasons
            .iter()
            .any(|reason| reason.contains("credential")));
        let ready_status = LocalModelStatus {
            state: "ready".to_string(),
            runtime_version: Some("test".to_string()),
            ..local_status
        };
        assert!(provider_readiness(&local, false, true, Some(&ready_status)).ready);
    }

    #[test]
    fn test_local_model_transport_decodes_runtime_missing_without_paths() {
        let status: LocalModelStatus = serde_json::from_str(r#"{"model_id":"sensevoice-small-int8","model_version":"2024-07-17","state":"runtime_missing","message":"Model installed; runtime pending.","installed_size_bytes":239549910}"#).unwrap();
        assert_eq!(status.state, "runtime_missing");
        let serialized = serde_json::to_string(&status).unwrap();
        assert!(!serialized.contains("/Users/") && !serialized.contains("source_archive"));
    }

    #[test]
    fn test_huggingface_endpoint_setting_round_trips_without_readiness_or_source_path() {
        let base = test_dir("voxly-model-endpoint-settings");
        fs::create_dir_all(&base).unwrap();
        let path = base.join(DESKTOP_SETTINGS_FILE);
        let mut settings = DesktopSettings::default();
        settings.huggingface_endpoint = Some("https://hf.example".to_string());
        save_desktop_settings(&path, &settings).unwrap();
        let loaded = load_desktop_settings(&path).unwrap();
        assert_eq!(
            loaded.huggingface_endpoint.as_deref(),
            Some("https://hf.example")
        );
        let json = fs::read_to_string(&path).unwrap();
        assert!(
            !json.contains("readiness")
                && !json.contains("source_archive")
                && !json.contains("partial")
        );
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_selected_output_root_requires_existing_writable_directory() {
        let base = test_dir("meeting-asr-selected-output-root");
        let root = base.join("output");
        fs::create_dir_all(&root).unwrap();

        let canonical = validate_selected_output_root(&root).unwrap();
        assert_eq!(canonical, root.canonicalize().unwrap());
        assert!(fs::read_dir(&root).unwrap().next().is_none());

        assert!(validate_selected_output_root(Path::new("")).is_err());
        assert!(validate_selected_output_root(Path::new("relative-output")).is_err());
        assert!(validate_selected_output_root(&base.join("missing")).is_err());

        let file = base.join("file.txt");
        fs::write(&file, "not a directory").unwrap();
        assert!(validate_selected_output_root(&file).is_err());
        fs::remove_dir_all(base).ok();
    }

    #[cfg(unix)]
    #[test]
    fn test_selected_output_root_canonicalizes_symlink() {
        use std::os::unix::fs::symlink;

        let base = test_dir("meeting-asr-selected-output-root-symlink");
        let root = base.join("output");
        let link = base.join("output-link");
        fs::create_dir_all(&root).unwrap();
        symlink(&root, &link).unwrap();
        assert_eq!(
            validate_selected_output_root(&link).unwrap(),
            root.canonicalize().unwrap()
        );
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_reset_output_root_clears_only_configured_root() {
        let base = test_dir("meeting-asr-reset-output-root");
        let settings_path = base.join(DESKTOP_SETTINGS_FILE);
        let root = base.join("output");
        fs::create_dir_all(&root).unwrap();
        let settings = DesktopSettings {
            output_root: Some(root.to_string_lossy().to_string()),
            language: Some("zh".to_string()),
            ..Default::default()
        };
        save_desktop_settings(&settings_path, &settings).unwrap();

        reset_configured_output_root(&settings_path).unwrap();
        let reloaded = load_desktop_settings(&settings_path).unwrap();
        assert_eq!(reloaded.output_root, None);
        assert_eq!(reloaded.language.as_deref(), Some("zh"));

        let default = get_default_output_root(RuntimeMode::Development, None).unwrap();
        assert_eq!(
            select_output_root_path(&reloaded, RuntimeMode::Development, None).unwrap(),
            (default, false)
        );
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_output_root_mutation_rejects_active_processing() {
        assert!(validate_output_root_mutation_state(false).is_ok());
        let error = validate_output_root_mutation_state(true)
            .unwrap_err()
            .to_json_string();
        assert!(error.contains("Cannot change output folder while processing."));
        assert!(!error.contains("output_root"));
    }

    #[test]
    fn test_open_output_root_requires_existing_directory() {
        let base = test_dir("meeting-asr-open-output-root");
        fs::create_dir_all(&base).unwrap();
        assert_eq!(
            validate_output_root_for_open(&base).unwrap(),
            base.canonicalize().unwrap()
        );
        assert!(validate_output_root_for_open(&base.join("missing")).is_err());
        let file = base.join("file.txt");
        fs::write(&file, "file").unwrap();
        assert!(validate_output_root_for_open(&file).is_err());
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_configured_output_root_rejects_relative_path() {
        let settings = DesktopSettings {
            output_root: Some("relative-output".to_string()),
            ..Default::default()
        };
        assert!(configured_output_root_path(&settings).is_err());

        let preflight = configured_output_root_preflight(&settings).unwrap();
        assert!(preflight.configured);
        assert!(!preflight.ready);
        assert_eq!(preflight.path, "relative-output");
        assert_eq!(preflight.message, "Configured output root is invalid.");
    }

    #[test]
    fn test_task_library_missing_default_root_is_empty_without_creation() {
        let default_root = test_dir("meeting-asr-task-library-default");
        assert!(!default_root.exists());
        let resolved = validate_task_library_output_root(&default_root, false).unwrap();
        assert_eq!(resolved, None);
        assert!(!default_root.exists());
        let result = discover_tasks_for_root(resolved, |_| panic!("sidecar must not run")).unwrap();
        assert!(result.tasks.is_empty());
        assert!(result.issues.is_empty());
    }

    #[test]
    fn test_task_library_missing_configured_root_is_safe_error() {
        let configured = test_dir("meeting-asr-task-library-configured");
        let error = validate_task_library_output_root(&configured, true).unwrap_err();
        let encoded = error.to_json_string();
        assert!(encoded.contains("Task Library output root is unavailable."));
        assert!(!encoded.contains(&configured.to_string_lossy().to_string()));
        assert!(!configured.exists());
    }

    #[test]
    fn test_task_library_existing_empty_root_invokes_discovery() {
        let root = test_dir("meeting-asr-task-library-existing-empty");
        fs::create_dir_all(&root).unwrap();
        let resolved = validate_task_library_output_root(&root, false).unwrap();
        let mut called = false;
        let result = discover_tasks_for_root(resolved, |received| {
            called = true;
            assert_eq!(received, root.canonicalize().unwrap());
            Ok(DiscoveryResult {
                tasks: Vec::new(),
                issues: Vec::new(),
            })
        })
        .unwrap();
        assert!(called);
        assert!(result.tasks.is_empty());
        assert!(result.issues.is_empty());
        fs::remove_dir_all(root).ok();
    }

    #[test]
    fn test_settings_reject_unknown_fields() {
        assert!(serde_json::from_str::<DesktopSettingsWire>(
            r#"{"output_root":"/tmp/output","unexpected_value":"ignored"}"#
        )
        .is_err());
    }

    #[test]
    fn test_canonicalize_path_within_rejects_relative_and_traversal_paths() {
        let base = test_dir("meeting-asr-containment");
        let root = base.join("root");
        let outside = base.join("outside");
        fs::create_dir_all(&root).unwrap();
        fs::create_dir_all(&outside).unwrap();
        let canonical_root = root.canonicalize().unwrap();

        assert!(canonicalize_path_within(&canonical_root, Path::new("relative"), "Task").is_err());
        assert!(canonicalize_path_within(
            &canonical_root,
            &root.join("..").join("outside"),
            "Task"
        )
        .is_err());
        fs::remove_dir_all(base).ok();
    }

    #[cfg(unix)]
    #[test]
    fn test_canonicalize_path_within_rejects_symlink_escape() {
        use std::os::unix::fs::symlink;

        let base = test_dir("meeting-asr-symlink-containment");
        let root = base.join("root");
        let outside = base.join("outside");
        fs::create_dir_all(&root).unwrap();
        fs::create_dir_all(&outside).unwrap();
        symlink(&outside, root.join("escape")).unwrap();
        let canonical_root = root.canonicalize().unwrap();

        assert!(canonicalize_path_within(&canonical_root, &root.join("escape"), "Task").is_err());
        fs::remove_dir_all(base).ok();
    }

    #[cfg(unix)]
    #[test]
    fn test_output_validation_rejects_symlinked_artifact_escape() {
        use std::os::unix::fs::symlink;

        let base = test_dir("meeting-asr-artifact-symlink");
        let root = base.join("root");
        let task = root.join("task");
        let outside = base.join("outside.md");
        fs::create_dir_all(&task).unwrap();
        fs::write(&outside, "outside transcript").unwrap();
        let transcript = task.join("transcript.md");
        symlink(&outside, &transcript).unwrap();
        let canonical_root = root.canonicalize().unwrap();
        let lines = vec![
            "Status: Done".to_string(),
            format!("Task folder: {}", task.display()),
            format!("Transcript saved: {}", transcript.display()),
        ];

        assert!(ParsedOutput::parse_lines(&lines)
            .validate_required_under("transcript", Some(&canonical_root))
            .is_err());
        fs::remove_dir_all(base).ok();
    }

    // --- command construction: paths stay as discrete arguments ---

    #[cfg(unix)]
    #[test]
    fn test_build_go_command_paths_are_discrete() {
        use std::os::unix::fs::PermissionsExt;
        let base = test_dir("voxly-go-command-spaces");
        fs::create_dir_all(&base).unwrap();
        let binary = base.join(go_binary_filename());
        fs::write(
            &binary,
            b"#!/bin/sh\nprintf 'meeting-asr-go version test\\n'\n",
        )
        .unwrap();
        fs::set_permissions(&binary, fs::Permissions::from_mode(0o755)).unwrap();
        let settings = DesktopSettings::default();
        let providers = ProviderSettingsInfo {
            local_asr_language: settings.local_asr_language,
            local_vad_enabled: settings.local_vad_enabled,
            asr: settings.asr,
            llm: settings.llm,
        };
        let secret = "mimo-secret-value";
        let cmd = build_go_command(
            "/path/with spaces/audio.mp3",
            "transcript_only",
            "/output/dir",
            Some(secret),
            None,
            &providers,
            None,
            RuntimeMode::Production,
            Some(&base),
            None,
        )
        .unwrap();
        let args: Vec<_> = cmd.get_args().collect();
        // Verify that a path with spaces is NOT split into multiple args.
        let audio_arg_found = args
            .iter()
            .any(|a| a.to_string_lossy().contains("/path/with spaces/audio.mp3"));
        assert!(audio_arg_found, "audio path with spaces should be one arg");

        // Verify output dir arg is also discrete.
        let out_arg_found = args.iter().any(|a| a.to_string_lossy() == "/output/dir");
        assert!(out_arg_found, "output dir should be one discrete arg");
        assert!(
            !args.iter().any(|a| a.to_string_lossy().contains(secret)),
            "credential values must never be passed through argv"
        );
        fs::remove_dir_all(base).unwrap();
    }

    // --- misc ---

    #[test]
    fn test_project_root_is_not_empty() {
        assert!(!project_root().is_empty());
    }

    #[test]
    fn test_resolve_project_root_returns_path() {
        let root = resolve_project_root();
        assert!(!root.as_os_str().is_empty());
    }

    #[test]
    fn test_isolated_go_runtime_root_has_no_credentials() {
        let root = isolated_go_project_root().unwrap();
        let marker = fs::read_to_string(root.join(".env")).unwrap();
        assert!(marker.is_empty());
        assert!(!marker.contains("API_KEY"));
        fs::remove_dir_all(root).ok();
    }

    #[test]
    fn test_development_resolver_honors_absolute_override() {
        let base = test_dir("meeting-asr-go-dev-override");
        let binary = base.join("custom-go");
        fs::create_dir_all(&base).unwrap();
        fs::write(&binary, b"placeholder").unwrap();

        let previous = std::env::var_os("MEETING_ASR_GO_BINARY");
        std::env::set_var("MEETING_ASR_GO_BINARY", &binary);
        let result = discover_development_go_binary();
        match previous {
            Some(value) => std::env::set_var("MEETING_ASR_GO_BINARY", value),
            None => std::env::remove_var("MEETING_ASR_GO_BINARY"),
        }

        let resolved = result.unwrap();
        assert_eq!(resolved, binary.canonicalize().unwrap());
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_development_resolver_rejects_relative_override() {
        let result = discover_development_go_binary_from(
            Some("relative/meeting-asr-go"),
            Path::new("/tmp/project"),
        );
        match result {
            Err(BackendError::BackendNotFound { message }) => {
                assert!(message.contains("MEETING_ASR_GO_BINARY"));
                assert!(message.contains("absolute"));
            }
            other => panic!("expected a backend_not_found error, got {other:?}"),
        }
    }

    #[test]
    fn test_development_resolver_rejects_missing_binary() {
        let base = test_dir("meeting-asr-go-dev-missing");
        let result = discover_development_go_binary_from(None, &base);
        match result {
            Err(BackendError::BackendNotFound { message }) => {
                assert!(message.contains("Go development binary not found"));
                assert!(message.contains("meeting-asr-go"));
            }
            other => panic!("expected a backend_not_found error, got {other:?}"),
        }
    }

    #[test]
    fn test_production_resolver_ignores_development_sources() {
        let base = test_dir("meeting-asr-go-production-sources");
        let project_root = base.join("project");
        let project_binary = project_root.join("core-go").join(go_binary_filename());
        let executable_dir = base.join("packaged");
        fs::create_dir_all(project_binary.parent().unwrap()).unwrap();
        fs::create_dir_all(&executable_dir).unwrap();
        fs::write(&project_binary, b"development binary").unwrap();

        let result = discover_go_binary(RuntimeMode::Production, Some(&executable_dir));
        match result {
            Err(BackendError::BackendNotFound { message }) => {
                assert!(message.contains("Packaged Go sidecar not found"));
                assert!(!message.contains(project_binary.to_string_lossy().as_ref()));
            }
            other => panic!("expected a backend_not_found error, got {other:?}"),
        }
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_production_resolver_ignores_meeting_asr_go_binary() {
        let base = test_dir("meeting-asr-go-production-env");
        let executable_dir = base.join("packaged");
        let override_path = base.join("override-go");
        let packaged = executable_dir.join(go_binary_filename());
        fs::create_dir_all(&executable_dir).unwrap();
        fs::write(&override_path, b"development override").unwrap();
        fs::write(&packaged, b"packaged sidecar").unwrap();

        let previous = std::env::var_os("MEETING_ASR_GO_BINARY");
        std::env::set_var("MEETING_ASR_GO_BINARY", &override_path);
        let result = discover_go_binary(RuntimeMode::Production, Some(&executable_dir));
        match previous {
            Some(value) => std::env::set_var("MEETING_ASR_GO_BINARY", value),
            None => std::env::remove_var("MEETING_ASR_GO_BINARY"),
        }

        assert_eq!(result.unwrap(), packaged.canonicalize().unwrap());
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_production_missing_sidecar_is_backend_not_found() {
        let executable_dir = test_dir("meeting-asr-go-production-missing");
        fs::create_dir_all(&executable_dir).unwrap();
        let result = discover_go_binary(RuntimeMode::Production, Some(&executable_dir));
        assert!(matches!(result, Err(BackendError::BackendNotFound { .. })));
        fs::remove_dir_all(executable_dir).ok();
    }

    #[test]
    fn test_production_missing_sidecar_does_not_fall_back_to_python() {
        let executable_dir = test_dir("meeting-asr-go-production-no-python-fallback");
        fs::create_dir_all(&executable_dir).unwrap();

        let backend = resolve_backend_kind(None, RuntimeMode::Production).unwrap();
        assert_eq!(backend, BackendKind::Go);
        let result = discover_go_binary(RuntimeMode::Production, Some(&executable_dir));
        assert!(matches!(result, Err(BackendError::BackendNotFound { .. })));
        fs::remove_dir_all(executable_dir).ok();
    }

    #[test]
    fn test_production_resolver_matches_tauri_normalized_sidecar_name() {
        let executable_dir = test_dir("meeting-asr-go-tauri-normalized-name");
        fs::create_dir_all(&executable_dir).unwrap();

        // Tauri consumes the target suffix while copying an externalBin into
        // the app executable directory. A suffix left in the runtime directory
        // must not be mistaken for the packaged sidecar.
        let target_suffixed = executable_dir.join(format!(
            "{}-{}{}",
            GO_SIDECAR_NAME,
            option_env!("TAURI_ENV_TARGET_TRIPLE").unwrap_or("unknown-target"),
            if cfg!(target_os = "windows") {
                ".exe"
            } else {
                ""
            }
        ));
        fs::write(&target_suffixed, b"target-suffixed input").unwrap();
        assert!(matches!(
            discover_go_binary(RuntimeMode::Production, Some(&executable_dir)),
            Err(BackendError::BackendNotFound { .. })
        ));

        let normalized = executable_dir.join(go_binary_filename());
        fs::write(&normalized, b"normalized packaged sidecar").unwrap();
        let resolved = discover_go_binary(RuntimeMode::Production, Some(&executable_dir)).unwrap();
        assert_eq!(resolved, normalized.canonicalize().unwrap());
        fs::remove_dir_all(executable_dir).ok();
    }

    #[cfg(unix)]
    #[test]
    fn test_go_version_validation_remains_enforced() {
        use std::os::unix::fs::PermissionsExt;

        let base = test_dir("meeting-asr-go-version");
        fs::create_dir_all(&base).unwrap();
        let valid = base.join("valid-go");
        fs::write(
            &valid,
            b"#!/bin/sh\nprintf '%s\\n' 'meeting-asr-go version test'\n",
        )
        .unwrap();
        let mut permissions = fs::metadata(&valid).unwrap().permissions();
        permissions.set_mode(0o755);
        fs::set_permissions(&valid, permissions).unwrap();
        assert!(validate_go_binary(&valid).is_ok());

        let invalid = base.join("invalid-go");
        fs::write(&invalid, b"#!/bin/sh\nprintf '%s\\n' 'another binary'\n").unwrap();
        let mut permissions = fs::metadata(&invalid).unwrap().permissions();
        permissions.set_mode(0o755);
        fs::set_permissions(&invalid, permissions).unwrap();
        assert!(matches!(
            validate_go_binary(&invalid),
            Err(BackendError::BackendNotFound { .. })
        ));
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_process_failed_error_includes_code() {
        let err = BackendError::ProcessFailed {
            message: "exited".to_string(),
            code: Some(1),
        };
        let json = serde_json::to_string(&err).unwrap();
        assert!(json.contains("process_failed"));
        assert!(json.contains("\"code\":1"));
    }

    // --- go manifest discovery ---

    #[test]
    fn test_go_resume_requires_manifest() {
        let dir = std::env::temp_dir().join("test-go-resume-nomanifest");
        fs::create_dir_all(&dir).unwrap();
        let manifest = find_go_manifest(&dir);
        fs::remove_dir_all(&dir).ok();
        assert!(manifest.is_none());
    }

    #[test]
    fn test_go_resume_accepts_task_manifest() {
        let dir = std::env::temp_dir().join("test-go-resume-current");
        fs::create_dir_all(&dir).unwrap();
        let p = dir.join("task_manifest.json");
        fs::write(&p, r#"{"schema_version":1,"output_mode":"transcript"}"#).unwrap();
        let found = find_go_manifest(&dir).unwrap();
        assert_eq!(found.file_name().unwrap(), "task_manifest.json");
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_go_resume_accepts_legacy_manifest() {
        let dir = std::env::temp_dir().join("test-go-resume-legacy");
        fs::create_dir_all(&dir).unwrap();
        // Only legacy manifest.json present.
        let p = dir.join("manifest.json");
        fs::write(&p, r#"{"output_mode":"notes"}"#).unwrap();
        let found = find_go_manifest(&dir).unwrap();
        assert_eq!(found.file_name().unwrap(), "manifest.json");
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_go_resume_prefers_current_over_legacy() {
        let dir = std::env::temp_dir().join("test-go-resume-both");
        fs::create_dir_all(&dir).unwrap();
        fs::write(
            &dir.join("task_manifest.json"),
            r#"{"schema_version":1,"output_mode":"transcript"}"#,
        )
        .unwrap();
        fs::write(&dir.join("manifest.json"), r#"{"output_mode":"minutes"}"#).unwrap();
        let found = find_go_manifest(&dir).unwrap();
        assert_eq!(found.file_name().unwrap(), "task_manifest.json");
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_read_manifest_output_mode_transcript() {
        let dir = std::env::temp_dir().join("test-gomode-transcript");
        fs::create_dir_all(&dir).unwrap();
        let p = dir.join("task_manifest.json");
        fs::write(&p, r#"{"schema_version":1,"output_mode":"transcript"}"#).unwrap();
        assert_eq!(
            read_manifest_output_mode(&p),
            Some("transcript".to_string())
        );
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_read_manifest_output_mode_minutes() {
        let dir = std::env::temp_dir().join("test-gomode-minutes");
        fs::create_dir_all(&dir).unwrap();
        let p = dir.join("task_manifest.json");
        fs::write(&p, r#"{"schema_version":1,"output_mode":"minutes"}"#).unwrap();
        assert_eq!(read_manifest_output_mode(&p), Some("minutes".to_string()));
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_read_manifest_output_mode_invalid_returns_none() {
        let dir = std::env::temp_dir().join("test-gomode-invalid");
        fs::create_dir_all(&dir).unwrap();
        let p = dir.join("manifest.json");
        fs::write(&p, r#"{"output_mode":"bogus"}"#).unwrap();
        assert_eq!(read_manifest_output_mode(&p), None);
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_read_manifest_output_mode_not_json_returns_none() {
        let dir = std::env::temp_dir().join("test-gomode-badj");
        fs::create_dir_all(&dir).unwrap();
        let p = dir.join("manifest.json");
        fs::write(&p, b"not-json").unwrap();
        assert_eq!(read_manifest_output_mode(&p), None);
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_imported_task_without_manifest_is_detected() {
        // A folder with transcript artifacts alone cannot be resumed as a Go task.
        let dir = std::env::temp_dir().join("test-imported-nomanifest");
        fs::create_dir_all(&dir).unwrap();
        // Existing audio and transcript folders do not replace a manifest.
        fs::create_dir_all(dir.join("audio")).unwrap();
        fs::create_dir_all(dir.join("chunks")).unwrap();
        fs::create_dir_all(dir.join("transcripts")).unwrap();
        assert!(find_go_manifest(&dir).is_none());
        fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn test_validate_output_mode_rejects_invalid() {
        assert!(validate_output_mode("bogus").is_err());
        assert!(validate_output_mode("").is_err());
        assert!(validate_output_mode("transcript").is_ok());
        assert!(validate_output_mode("notes").is_ok());
        assert!(validate_output_mode("minutes").is_ok());
    }

    // --- Task Library transport ---

    fn task_library_json() -> &'static [u8] {
        br#"{
          "tasks": [{
            "task_id": "task-1",
            "title": "Interview",
            "title_source": "user",
            "status": "completed_with_errors",
            "created_at": "2026-07-15T01:00:00Z",
            "updated_at": "2026-07-15T02:00:00Z",
            "tags": ["work"],
            "output_mode": "minutes",
            "primary_artifacts": [
              {"kind":"minutes","status":"failed","has_error":true,"retryable":true},
              {"kind":"notes","status":"completed","reference":"notes.md","has_error":false},
              {"kind":"transcript","status":"completed","reference":"transcript.md","has_error":false}
            ],
            "has_partial_errors": true,
            "task_folder": "task-folder",
            "generated_content": [{
              "template_id": "future-template",
              "display_name": "Historical name",
              "latest": {
                "generation_id": "gen-1",
                "template_revision": 3,
                "template_display_name": "Historical name r3",
                "status": "completed",
                "created_at": "2026-07-15T02:00:00Z",
                "reference": "generated/future-template/gen-1.md",
                "has_error": false,
                "retryable": null,
                "legacy": false
              },
              "versions": []
            }]
          }],
          "issues": [{
            "task_folder": "invalid-task",
            "code": "invalid_manifest",
            "message": "Task manifest is invalid."
          }]
        }"#
    }

    #[test]
    fn test_task_library_successful_typed_decoding() {
        let result = decode_task_library_process_output(true, Some(0), task_library_json())
            .expect("decode Task Library output");
        assert_eq!(result.tasks.len(), 1);
        assert_eq!(result.tasks[0].status, "completed_with_errors");
        assert!(result.tasks[0].has_partial_errors);
        assert_eq!(result.tasks[0].primary_artifacts[0].kind, "minutes");
        assert_eq!(result.tasks[0].primary_artifacts[0].retryable, Some(true));
        let generation = result.tasks[0].generated_content[0]
            .latest
            .as_ref()
            .unwrap();
        assert_eq!(generation.template_revision, 3);
        assert_eq!(generation.template_display_name, "Historical name r3");
        assert_eq!(result.issues.len(), 1);
        assert_eq!(result.issues[0].code, "invalid_manifest");
    }

    #[test]
    fn test_task_library_transport_decodes_completed_failed_and_unavailable_states() {
        let json = br#"{
          "tasks": [
            {
              "task_id":"completed","title":"Completed","title_source":"filename",
              "status":"completed","created_at":"2026-07-15T01:00:00Z",
              "updated_at":"2026-07-15T02:00:00Z","tags":[],
              "output_mode":"transcript","primary_artifacts":[
                {"kind":"transcript","status":"completed","reference":"transcript.md","has_error":false}
              ],"has_partial_errors":false,"task_folder":"completed-task"
            },
            {
              "task_id":"failed","title":"Failed","title_source":"fallback",
              "status":"failed","created_at":"2026-07-15T01:00:00Z",
              "updated_at":"2026-07-15T02:00:00Z","tags":[],
              "output_mode":"notes","primary_artifacts":[
                {"kind":"notes","status":"unavailable","has_error":false},
                {"kind":"transcript","status":"failed","has_error":true,"retryable":false}
              ],"has_partial_errors":false,"task_folder":"failed-task"
            }
          ],
          "issues": []
        }"#;
        let result = decode_task_library_process_output(true, Some(0), json).unwrap();
        assert_eq!(result.tasks[0].status, "completed");
        assert_eq!(result.tasks[0].primary_artifacts[0].retryable, None);
        assert_eq!(result.tasks[1].status, "failed");
        assert_eq!(result.tasks[1].primary_artifacts[0].status, "unavailable");
        assert_eq!(result.tasks[1].primary_artifacts[0].retryable, None);
        assert_eq!(result.tasks[1].primary_artifacts[1].status, "failed");
        assert_eq!(result.tasks[1].primary_artifacts[1].retryable, Some(false));
    }

    #[test]
    fn test_task_library_transport_decodes_empty_result() {
        let result =
            decode_task_library_process_output(true, Some(0), br#"{"tasks":[],"issues":[]}"#)
                .unwrap();
        assert!(result.tasks.is_empty());
        assert!(result.issues.is_empty());
    }

    #[test]
    fn test_storage_preferences_migrate_old_settings_and_roundtrip() {
        let base = test_dir("voxly-storage-settings");
        fs::create_dir_all(&base).unwrap();
        let path = base.join(DESKTOP_SETTINGS_FILE);
        fs::write(&path, r#"{"output_root":null,"theme":"dark"}"#).unwrap();
        let defaults = storage_settings_at(&path).unwrap();
        assert!(!defaults.auto_clean_chunks);
        assert!(defaults.cleanup_temporary_on_startup);
        let mut settings = load_desktop_settings(&path).unwrap();
        settings.auto_clean_chunks = true;
        settings.cleanup_temporary_on_startup = false;
        save_desktop_settings(&path, &settings).unwrap();
        let stored = storage_settings_at(&path).unwrap();
        assert!(stored.auto_clean_chunks);
        assert!(!stored.cleanup_temporary_on_startup);
        assert_eq!(
            load_desktop_settings(&path).unwrap().theme.as_deref(),
            Some("dark")
        );
        fs::remove_dir_all(base).unwrap();
    }

    #[test]
    fn test_cache_transport_preserves_preview_and_blocking_reason() {
        let preview = br#"{"can_clean":true,"files":[{"reference":"chunks/part_001.wav","size_bytes":4096}],"total_bytes":4096,"token":"preview-token","cache_status":"present","removed_bytes":0}"#;
        let result = decode_cache_output(true, preview).unwrap();
        assert!(result.can_clean);
        assert_eq!(result.token, "preview-token");
        assert_eq!(result.files[0].reference, "chunks/part_001.wav");
        assert_eq!(result.total_bytes, 4096);
        let blocked = br#"{"can_clean":false,"reason":"task_busy","files":[],"total_bytes":0,"cache_status":"unknown","removed_bytes":0}"#;
        assert_eq!(
            decode_cache_output(true, blocked).unwrap().reason,
            "task_busy"
        );
        assert!(decode_cache_output(false, b"secret or internal path").is_err());
        assert!(decode_cache_output(true, b"{}").is_err());
    }

    #[test]
    fn test_task_library_malformed_json_is_safe_error() {
        let error = decode_task_library_process_output(true, Some(0), b"not-json").unwrap_err();
        let encoded = error.to_json_string();
        assert!(encoded.contains(r#""category":"process_failed""#));
        assert!(encoded.contains("invalid JSON"));
        assert!(!encoded.contains("not-json"));
    }

    #[test]
    fn test_task_library_nonzero_exit_is_safe_error() {
        let error = decode_task_library_process_output(
            false,
            Some(7),
            br#"{"secret":"must-not-be-exposed"}"#,
        )
        .unwrap_err();
        let encoded = error.to_json_string();
        assert!(encoded.contains(r#""category":"process_failed""#));
        assert!(encoded.contains(r#""code":7"#));
        assert!(!encoded.contains("secret"));
    }

    #[test]
    fn test_task_metadata_transport_decodes_user_fields() {
        let metadata = decode_task_metadata_output(
            true,
            Some(0),
            br#"{"task_id":"task-1","title":"Interview","description":"Research","tags":["work"],"created_at":"2026-07-15T01:00:00Z","updated_at":"2026-07-17T02:00:00Z"}"#,
        )
        .unwrap();
        assert_eq!(metadata.task_id, "task-1");
        assert_eq!(metadata.description, "Research");
        assert_eq!(metadata.tags, vec!["work"]);
    }

    #[test]
    fn test_task_metadata_transport_errors_are_safe() {
        let malformed = decode_task_metadata_output(true, Some(0), b"private manifest content")
            .unwrap_err()
            .to_json_string();
        assert!(malformed.contains("invalid JSON"));
        assert!(!malformed.contains("private manifest content"));

        let failed =
            decode_task_metadata_output(false, Some(9), br#"{"api_key":"must-not-be-exposed"}"#)
                .unwrap_err()
                .to_json_string();
        assert!(failed.contains(r#""code":9"#));
        assert!(!failed.contains("api_key"));
    }

    #[test]
    fn test_custom_template_transport_decodes_catalog_and_rejects_unknown_input() {
        let catalog: PromptTemplateCatalog = serde_json::from_str(r#"{
          "templates":[{"id":"custom-safe","revision":2,"display_name":{"en":"Safe","zh":"Safe"},"description":{"en":"","zh":""},"instructions":"Do work","content_type":"text/markdown","extension":".md","built_in":false,"ownership":"custom","status":"active","can_edit":true,"can_delete":true}],
          "issues":[]
        }"#).unwrap();
        assert_eq!(catalog.templates[0].revision, 2);
        assert!(catalog.templates[0].can_edit);
        assert!(serde_json::from_str::<CustomTemplateInput>(
            r#"{"display_name":"Name","description":"","instructions":"Work","unexpected":"value"}"#
        )
        .is_err());
    }

    #[test]
    fn test_task_metadata_mutation_rejects_active_processing() {
        assert!(validate_task_metadata_mutation_state(false).is_ok());
        assert!(matches!(
            validate_task_metadata_mutation_state(true),
            Err(BackendError::InvalidInput { .. })
        ));
    }

    fn artifact_discovery(task_folder: &str, reference: &str) -> DiscoveryResult {
        DiscoveryResult {
            tasks: vec![TaskSummary {
                task_id: "task-id".to_string(),
                title: "Task".to_string(),
                title_source: "fallback".to_string(),
                description: String::new(),
                status: "completed".to_string(),
                created_at: "2026-07-15T01:00:00Z".to_string(),
                updated_at: "2026-07-15T02:00:00Z".to_string(),
                tags: Vec::new(),
                output_mode: "transcript".to_string(),
                primary_artifacts: vec![ArtifactSummary {
                    kind: "transcript".to_string(),
                    status: "completed".to_string(),
                    reference: Some(reference.to_string()),
                    has_error: false,
                    retryable: None,
                    provider: String::new(),
                    model: String::new(),
                }],
                has_partial_errors: false,
                task_folder: task_folder.to_string(),
                canonical_transcript: None,
                generated_content: Vec::new(),
                can_generate: false,
                actions: None,
            }],
            issues: Vec::new(),
        }
    }

    #[test]
    fn test_generation_task_rechecks_membership_and_containment() {
        let base = test_dir("meeting-asr-generation-membership");
        let root = base.join("root");
        let task_dir = root.join("task");
        fs::create_dir_all(&task_dir).unwrap();
        fs::write(task_dir.join("transcript.md"), "text").unwrap();
        let mut discovery = artifact_discovery("task", "transcript.md");
        assert!(resolve_generation_task(&root, &discovery, "task").is_err());
        discovery.tasks[0].can_generate = true;
        discovery.tasks[0].canonical_transcript = Some(ArtifactSummary {
            kind: "transcript".to_string(),
            status: "completed".to_string(),
            reference: Some("transcript.md".to_string()),
            has_error: false,
            retryable: None,
            provider: "mimo".to_string(),
            model: "mimo-v2.5-asr".to_string(),
        });
        assert_eq!(
            resolve_generation_task(&root, &discovery, "task").unwrap(),
            task_dir.canonicalize().unwrap()
        );
        assert!(resolve_generation_task(&root, &discovery, "../task").is_err());
        let _ = fs::remove_dir_all(base);
    }

    #[test]
    fn test_resume_requires_free_task_and_retryable_generation() {
        let base = test_dir("meeting-asr-resume-lock-and-retry");
        let root = base.join("root");
        let task_dir = root.join("task");
        fs::create_dir_all(&task_dir).unwrap();
        fs::write(task_dir.join("transcript.md"), "text").unwrap();

        let mut discovery = artifact_discovery("task", "transcript.md");
        discovery.tasks[0].actions = Some(TaskActions {
            locked: false,
            next_stage: "TRANSCRIBING".to_string(),
            can_resume: true,
            can_resume_transcribe: true,
            can_resume_generation: false,
            can_retry_generation: true,
            pending_chunks: 1,
            blocking_reasons: Vec::new(),
        });
        assert_eq!(
            resolve_resumable_task(&root, &discovery, "task", true).unwrap(),
            task_dir.canonicalize().unwrap()
        );
        // A path the frontend invented is never trusted on its own.
        assert!(resolve_resumable_task(&root, &discovery, "../task", true).is_err());
        assert!(resolve_resumable_task(&root, &discovery, "missing-task", true).is_err());

        // Another process owning the Task blocks the action before it spawns.
        discovery.tasks[0].actions = Some(TaskActions {
            locked: true,
            next_stage: "TRANSCRIBING".to_string(),
            can_resume: false,
            can_resume_transcribe: false,
            can_resume_generation: false,
            can_retry_generation: false,
            pending_chunks: 1,
            blocking_reasons: vec![TaskBlockingReason {
                code: "task_locked".to_string(),
                message: "Another process is working on this task.".to_string(),
            }],
        });
        let locked = resolve_resumable_task(&root, &discovery, "task", true).unwrap_err();
        assert!(matches!(locked, BackendError::TaskLocked { .. }));

        // Only a generation the runtime reports as retryable may be retried.
        discovery.tasks[0].actions = None;
        discovery.tasks[0].generated_content = vec![GenerationGroup {
            template_id: "structured-notes".to_string(),
            display_name: "Structured notes".to_string(),
            latest: None,
            versions: vec![GenerationSummary {
                generation_id: "gen_retry".to_string(),
                template_revision: 1,
                template_display_name: "Structured notes".to_string(),
                status: "failed".to_string(),
                created_at: "2026-07-16T01:00:00Z".to_string(),
                reference: None,
                has_error: true,
                error_code: "generation_failed".to_string(),
                partial_output: false,
                retryable: Some(false),
                legacy: false,
                provider: "deepseek".to_string(),
                model: "model".to_string(),
            }],
        }];
        assert!(resolve_retryable_generation(&root, &discovery, "task", "gen_retry").is_err());
        discovery.tasks[0].generated_content[0].versions[0].retryable = Some(true);
        assert_eq!(
            resolve_retryable_generation(&root, &discovery, "task", "gen_retry").unwrap(),
            task_dir.canonicalize().unwrap()
        );
        assert!(resolve_retryable_generation(&root, &discovery, "task", "gen_other").is_err());

        let _ = fs::remove_dir_all(base);
    }

    #[cfg(unix)]
    #[test]
    fn test_stale_watchdog_identity_cannot_stop_a_later_run() {
        let active = ActiveProcess::default();

        let first = Command::new("sleep").arg("0.2").spawn().unwrap();
        let first_id = active.register(first, None).unwrap();
        let outcome = active.request_stop();
        assert_eq!(outcome.state, "stopping");

        // The cancelled run exits well before the watchdog deadline.
        thread::sleep(Duration::from_millis(400));
        active.finish(first_id);
        assert!(!active.is_run_active(first_id));

        // A new run takes the slot; the stale watchdog must not be able to reach it.
        let second = Command::new("sleep").arg("30").spawn().unwrap();
        let second_id = active.register(second, None).unwrap();
        assert_ne!(first_id, second_id);
        assert!(active.is_run_active(second_id));
        assert!(
            !active.terminate_run(first_id),
            "a stale run identity must not stop the current run"
        );
        assert!(
            active.is_run_active(second_id),
            "the current run must survive"
        );

        assert!(active.terminate_run(second_id));
        active.finish(second_id);
        assert!(!active.is_active());
    }

    #[test]
    fn test_cancel_without_a_running_process_is_idle() {
        let active = ActiveProcess::default();
        assert_eq!(active.request_stop().state, "idle");
        assert!(!active.is_active());
        assert!(!active.is_stopping());
    }

    #[test]
    fn test_task_artifact_allows_preserved_partial_generation_only() {
        let base = test_dir("meeting-asr-partial-generation-artifact");
        let root = base.join("root");
        let task_dir = root.join("task");
        let generated = task_dir.join("generated");
        fs::create_dir_all(&generated).unwrap();
        fs::write(generated.join("partial.md"), "truncated").unwrap();
        fs::write(generated.join("failed.md"), "nothing usable").unwrap();

        let mut discovery = artifact_discovery("task", "transcript.md");
        discovery.tasks[0].generated_content = vec![GenerationGroup {
            template_id: "structured-notes".to_string(),
            display_name: "Structured notes".to_string(),
            latest: None,
            versions: vec![
                GenerationSummary {
                    generation_id: "gen_partial".to_string(),
                    template_revision: 1,
                    template_display_name: "Structured notes".to_string(),
                    status: "failed".to_string(),
                    created_at: "2026-07-16T01:00:00Z".to_string(),
                    reference: Some("generated/partial.md".to_string()),
                    has_error: true,
                    error_code: "output_incomplete".to_string(),
                    partial_output: true,
                    retryable: Some(true),
                    legacy: false,
                    provider: "deepseek".to_string(),
                    model: "model".to_string(),
                },
                GenerationSummary {
                    generation_id: "gen_failed".to_string(),
                    template_revision: 1,
                    template_display_name: "Structured notes".to_string(),
                    status: "failed".to_string(),
                    created_at: "2026-07-16T01:05:00Z".to_string(),
                    reference: Some("generated/failed.md".to_string()),
                    has_error: true,
                    error_code: "generation_failed".to_string(),
                    partial_output: false,
                    retryable: Some(true),
                    legacy: false,
                    provider: "deepseek".to_string(),
                    model: "model".to_string(),
                },
            ],
        }];

        assert_eq!(
            resolve_validated_task_artifact(&root, &discovery, "task", "generated/partial.md")
                .unwrap(),
            generated.join("partial.md").canonicalize().unwrap()
        );
        // A failed generation without preserved output stays unopenable.
        assert!(
            resolve_validated_task_artifact(&root, &discovery, "task", "generated/failed.md")
                .is_err()
        );
        let _ = fs::remove_dir_all(base);
    }

    #[test]
    fn test_task_artifact_open_and_reveal_use_validated_regular_file() {
        use std::cell::Cell;

        let base = test_dir("meeting-asr-task-artifact-actions");
        let root = base.join("root");
        let nested = root.join("task").join("outputs");
        let artifact = nested.join("transcript.md");
        fs::create_dir_all(&nested).unwrap();
        fs::write(&artifact, "transcript").unwrap();
        let discovery = artifact_discovery("task", "outputs/transcript.md");

        for requested_action in [TaskArtifactAction::Open, TaskArtifactAction::Reveal] {
            let called = Cell::new(false);
            run_validated_task_artifact_action(
                &root,
                &discovery,
                "task",
                "outputs/transcript.md",
                requested_action,
                |validated, actual_action| {
                    assert_eq!(validated, artifact.canonicalize().unwrap());
                    assert_eq!(
                        matches!(actual_action, TaskArtifactAction::Reveal),
                        matches!(requested_action, TaskArtifactAction::Reveal)
                    );
                    called.set(true);
                    Ok(())
                },
            )
            .unwrap();
            assert!(called.get());
        }
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_task_artifact_rejects_hostile_and_unlisted_inputs() {
        let base = test_dir("meeting-asr-task-artifact-inputs");
        let root = base.join("root");
        let task = root.join("task");
        fs::create_dir_all(&task).unwrap();
        fs::write(task.join("transcript.md"), "transcript").unwrap();
        fs::write(task.join("other.md"), "other").unwrap();
        let discovery = artifact_discovery("task", "transcript.md");

        let invalid = [
            ("", "transcript.md"),
            ("/absolute-task", "transcript.md"),
            ("C:\\task", "transcript.md"),
            ("task/child", "transcript.md"),
            ("task", ""),
            ("task", "/absolute.md"),
            ("task", "C:/absolute.md"),
            ("task", "../outside.md"),
            ("task", "nested/../../outside.md"),
            ("task\0bad", "transcript.md"),
        ];
        for (task_folder, reference) in invalid {
            assert!(
                resolve_validated_task_artifact(&root, &discovery, task_folder, reference).is_err()
            );
        }
        assert!(resolve_validated_task_artifact(
            &root,
            &discovery,
            "missing-task",
            "transcript.md"
        )
        .is_err());
        assert!(resolve_validated_task_artifact(&root, &discovery, "task", "other.md").is_err());

        let mut failed_discovery = artifact_discovery("task", "transcript.md");
        failed_discovery.tasks[0].primary_artifacts[0].status = "failed".to_string();
        assert!(
            resolve_validated_task_artifact(&root, &failed_discovery, "task", "transcript.md")
                .is_err()
        );
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_task_artifact_rejects_missing_paths_and_directory_target() {
        let base = test_dir("meeting-asr-task-artifact-missing");
        let root = base.join("root");
        let task = root.join("task");
        fs::create_dir_all(&task).unwrap();
        let discovery = artifact_discovery("task", "artifact.md");

        assert!(resolve_validated_task_artifact(
            &base.join("missing-root"),
            &discovery,
            "task",
            "artifact.md"
        )
        .is_err());
        assert!(resolve_validated_task_artifact(
            &root,
            &artifact_discovery("missing-task", "artifact.md"),
            "missing-task",
            "artifact.md"
        )
        .is_err());
        assert!(resolve_validated_task_artifact(&root, &discovery, "task", "artifact.md").is_err());

        fs::create_dir(task.join("artifact.md")).unwrap();
        assert!(resolve_validated_task_artifact(&root, &discovery, "task", "artifact.md").is_err());
        fs::remove_dir_all(base).ok();
    }

    #[cfg(unix)]
    #[test]
    fn test_task_artifact_rejects_task_and_artifact_symlink_escapes() {
        use std::os::unix::fs::symlink;

        let base = test_dir("meeting-asr-task-artifact-symlinks");
        let root = base.join("root");
        let outside_task = base.join("root-evil");
        fs::create_dir_all(&root).unwrap();
        fs::create_dir_all(&outside_task).unwrap();
        fs::write(outside_task.join("artifact.md"), "outside").unwrap();
        symlink(&outside_task, root.join("task")).unwrap();
        assert!(resolve_validated_task_artifact(
            &root,
            &artifact_discovery("task", "artifact.md"),
            "task",
            "artifact.md"
        )
        .is_err());

        fs::remove_file(root.join("task")).unwrap();
        let task = root.join("task");
        fs::create_dir_all(task.join("nested")).unwrap();
        let outside_file = base.join("outside.md");
        fs::write(&outside_file, "outside").unwrap();
        symlink(&outside_file, task.join("artifact.md")).unwrap();
        symlink(&outside_task, task.join("nested").join("escape")).unwrap();
        assert!(resolve_validated_task_artifact(
            &root,
            &artifact_discovery("task", "artifact.md"),
            "task",
            "artifact.md"
        )
        .is_err());
        assert!(resolve_validated_task_artifact(
            &root,
            &artifact_discovery("task", "nested/escape/artifact.md"),
            "task",
            "nested/escape/artifact.md"
        )
        .is_err());
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_task_artifact_errors_are_structured_and_path_safe() {
        let error = resolve_validated_task_artifact(
            Path::new("/sensitive/private/root"),
            &artifact_discovery("task", "artifact.md"),
            "missing-task",
            "artifact.md",
        )
        .unwrap_err()
        .to_json_string();
        assert!(error.contains(r#""category":"artifact_unavailable""#));
        assert!(!error.contains("/sensitive/private/root"));

        let worker = task_artifact_worker_error();
        assert!(worker.contains(r#""category":"process_failed""#));
        assert!(worker.contains("Task artifact worker failed"));
    }

    #[test]
    fn test_task_artifact_resolution_uses_newly_configured_root() {
        let base = test_dir("meeting-asr-artifact-configured-root");
        let old_root = base.join("old-root");
        let new_root = base.join("new-root");
        let old_task = old_root.join("task");
        let new_task = new_root.join("task");
        fs::create_dir_all(&old_task).unwrap();
        fs::create_dir_all(&new_root).unwrap();
        fs::write(old_task.join("artifact.md"), "old").unwrap();

        let settings_path = base.join(DESKTOP_SETTINGS_FILE);
        let configured = set_configured_output_root(&settings_path, &new_root).unwrap();
        assert_eq!(configured, new_root.canonicalize().unwrap());

        let discovery = artifact_discovery("task", "artifact.md");
        assert!(
            resolve_validated_task_artifact(&configured, &discovery, "task", "artifact.md")
                .is_err()
        );

        fs::create_dir_all(&new_task).unwrap();
        fs::write(new_task.join("artifact.md"), "new").unwrap();
        assert_eq!(
            resolve_validated_task_artifact(&configured, &discovery, "task", "artifact.md")
                .unwrap(),
            new_task.join("artifact.md").canonicalize().unwrap()
        );
        fs::remove_dir_all(base).ok();
    }

    // --- interrupted ---

    #[test]
    fn test_interrupted_error_serialization() {
        let err = BackendError::Interrupted {
            message: "killed".to_string(),
        };
        let json = serde_json::to_string(&err).unwrap();
        assert!(json.contains("interrupted"));
        assert!(json.contains("killed"));
    }
}
