use std::fs;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;
use tauri::{AppHandle, Emitter, Manager, RunEvent, State};

// ---------------------------------------------------------------------------
// Shared event payloads
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Stderr-based error classification
// ---------------------------------------------------------------------------

fn classify_stderr(stderr_lines: &[String], exit_code: Option<i32>) -> BackendError {
    let combined = stderr_lines.join("\n").to_lowercase();

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
        message: if stderr_lines.is_empty() {
            format!(
                "Process exited with code {} and no stderr output.",
                exit_code.unwrap_or(-1)
            )
        } else {
            let first = stderr_lines
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
    Python,
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
enum RuntimeMode {
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

const DESKTOP_SETTINGS_FILE: &str = "desktop-settings.json";

/// Deliberately non-sensitive settings. API keys, input history, transcripts,
/// and task content must never be added to this structure.
#[derive(Debug, Clone, Default, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
// Reject unknown fields so unsupported or potentially sensitive values are
// never silently accepted and later persisted. Future schema changes require
// an explicit, reviewed migration rather than implicit forward compatibility.
#[serde(default, deny_unknown_fields)]
struct DesktopSettings {
    output_root: Option<String>,
    language: Option<String>,
    theme: Option<String>,
    deepseek_model: Option<String>,
    development_backend: Option<BackendKind>,
}

#[derive(serde::Serialize)]
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

fn load_desktop_settings(path: &Path) -> Result<DesktopSettings, BackendError> {
    if !path.exists() {
        return Ok(DesktopSettings::default());
    }
    let content = fs::read_to_string(path).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not read desktop settings: {e}"),
    })?;
    serde_json::from_str(&content).map_err(|e| BackendError::InvalidInput {
        message: format!("Desktop settings are invalid: {e}"),
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
    fs::write(path, format!("{json}\n")).map_err(|e| BackendError::InvalidInput {
        message: format!("Could not save desktop settings: {e}"),
    })
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

fn get_configured_output_root(settings: &DesktopSettings) -> Result<Option<PathBuf>, BackendError> {
    match settings.output_root.as_deref() {
        None => Ok(None),
        Some(value) if value.trim().is_empty() => Err(BackendError::InvalidInput {
            message: "Configured output root cannot be empty.".to_string(),
        }),
        Some(value) => validate_output_root(Path::new(value)).map(Some),
    }
}

fn set_configured_output_root(
    settings_path: &Path,
    output_root: &Path,
) -> Result<PathBuf, BackendError> {
    let canonical = validate_output_root(output_root)?;
    let mut settings = load_desktop_settings(settings_path)?;
    settings.output_root = Some(canonical.to_string_lossy().to_string());
    save_desktop_settings(settings_path, &settings)?;
    Ok(canonical)
}

fn resolve_output_root(app: &AppHandle) -> Result<(PathBuf, bool), BackendError> {
    let settings_path = desktop_settings_path(app)?;
    let settings = load_desktop_settings(&settings_path)?;
    if let Some(configured) = get_configured_output_root(&settings)? {
        return Ok((configured, true));
    }

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
    let default_root = get_default_output_root(runtime_mode, documents_dir.as_deref())?;
    validate_output_root(&default_root).map(|root| (root, false))
}

#[tauri::command]
fn get_output_root(app: AppHandle) -> Result<OutputRootInfo, String> {
    let runtime_mode = RuntimeMode::current();
    let (output_root, configured) = resolve_output_root(&app).map_err(String::from)?;
    Ok(OutputRootInfo {
        output_root: output_root.to_string_lossy().to_string(),
        configured,
        runtime_mode,
    })
}

#[tauri::command]
fn set_output_root(app: AppHandle, output_root: String) -> Result<OutputRootInfo, String> {
    let settings_path = desktop_settings_path(&app).map_err(String::from)?;
    let output_root = set_configured_output_root(&settings_path, Path::new(&output_root))
        .map_err(String::from)?;
    Ok(OutputRootInfo {
        output_root: output_root.to_string_lossy().to_string(),
        configured: true,
        runtime_mode: RuntimeMode::current(),
    })
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

fn discover_go_binary() -> Result<PathBuf, BackendError> {
    if let Ok(p) = std::env::var("MEETING_ASR_GO_BINARY") {
        if !p.is_empty() {
            let path = Path::new(&p);
            if !path.is_absolute() {
                return Err(BackendError::BackendNotFound {
                    message: format!("MEETING_ASR_GO_BINARY must be an absolute path, got: {}", p),
                });
            }
            return canonicalize_abs_file(&p, "Go binary");
        }
    }

    let root = resolve_project_root();
    let binary_name = if cfg!(target_os = "windows") {
        "meeting-asr-go.exe"
    } else {
        "meeting-asr-go"
    };
    let candidate = root.join("core-go").join(binary_name);
    let candidate = candidate
        .canonicalize()
        .map_err(|_| BackendError::BackendNotFound {
            message: format!(
                "Go binary not found at {}. Build it with: cd core-go && go build -o meeting-asr-go .\n\
                 Or set MEETING_ASR_GO_BINARY to the absolute path of the binary.",
                candidate.display()
            ),
        })?;
    if !candidate.is_file() {
        return Err(BackendError::BackendNotFound {
            message: format!(
                "Go binary not found at {}. Build it with: cd core-go && go build -o meeting-asr-go .",
                candidate.display()
            ),
        });
    }
    Ok(candidate)
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

#[derive(Clone, Default)]
struct ActiveProcess {
    child: Arc<Mutex<Option<Child>>>,
}

impl ActiveProcess {
    fn terminate(&self) {
        if let Ok(mut guard) = self.child.lock() {
            if let Some(child) = guard.as_mut() {
                let _ = child.kill();
            }
        }
    }
}

struct ProcessRunner {
    active: ActiveProcess,
    stdout_lines: Arc<Mutex<Vec<String>>>,
    stderr_lines: Arc<Mutex<Vec<String>>>,
    stdout_handle: Option<JoinHandle<()>>,
    stderr_handle: Option<JoinHandle<()>>,
}

fn spawn_and_watch(
    app: &AppHandle,
    active: &ActiveProcess,
    mut cmd: Command,
) -> Result<ProcessRunner, BackendError> {
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

    {
        let mut guard = active.child.lock().unwrap();
        if guard.is_some() {
            let _ = child.kill();
            return Err(BackendError::ProcessFailed {
                message: "A meeting transcription process is already running.".to_string(),
                code: None,
            });
        }
        *guard = Some(child);
    }

    let stdout_lines = Arc::new(Mutex::new(Vec::new()));
    let stderr_lines = Arc::new(Mutex::new(Vec::new()));
    let stdout_for_thread = stdout_lines.clone();
    let stderr_for_thread = stderr_lines.clone();

    let app_stdout = app.clone();
    let stdout_handle: JoinHandle<()> = thread::spawn(move || {
        let reader = BufReader::new(stdout_stream);
        for line in reader.lines() {
            match line {
                Ok(text) => {
                    let trimmed = text.trim();

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
                            line: text.clone(),
                        },
                    );
                    stdout_for_thread.lock().unwrap().push(text);
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
                    let _ = app_stderr.emit(
                        "log-line",
                        LogEvent {
                            stream: "stderr".into(),
                            line: text.clone(),
                        },
                    );
                    stderr_for_thread.lock().unwrap().push(text);
                }
                Err(_) => break,
            }
        }
    });

    Ok(ProcessRunner {
        active: active.clone(),
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
                let mut guard = self.active.child.lock().unwrap();
                let child = guard.as_mut().ok_or_else(|| BackendError::Interrupted {
                    message: "Process was no longer available.".to_string(),
                })?;
                child.try_wait().map_err(|e| BackendError::ProcessFailed {
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
        let _ = self.active.child.lock().unwrap().take();

        Ok((code, stdout, stderr))
    }
}

// ---------------------------------------------------------------------------
// Command construction helpers
// ---------------------------------------------------------------------------

fn build_python_command(
    audio_path: &str,
    output_mode: &str,
    output_dir: &str,
    mimo_key: Option<&str>,
    deepseek_key: Option<&str>,
    deepseek_model: Option<&str>,
) -> Command {
    let root = resolve_project_root();
    let mut cmd = Command::new("uv");
    cmd.arg("run")
        .arg("meeting-asr")
        .arg("transcribe")
        .arg(audio_path)
        .arg("--output-mode")
        .arg(output_mode)
        .arg("--output-dir")
        .arg(output_dir)
        .current_dir(&root);

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
    if let Some(model) = deepseek_model {
        if !model.is_empty() {
            cmd.arg("--llm-model").arg(model);
        }
    }

    cmd
}

fn build_go_command(
    audio_path: &str,
    output_mode: &str,
    output_dir: &str,
    mimo_key: Option<&str>,
    deepseek_key: Option<&str>,
    deepseek_model: Option<&str>,
    env_file: Option<&PathBuf>,
) -> Result<Command, BackendError> {
    let binary = discover_go_binary()?;
    validate_go_binary(&binary)?;

    let root = resolve_project_root();
    let mut cmd = Command::new(binary);
    cmd.arg("transcribe")
        .arg(audio_path)
        .arg("--output-mode")
        .arg(output_mode)
        .arg("--output-dir")
        .arg(output_dir);

    if let Some(env) = env_file {
        cmd.arg("--env-file").arg(env);
    } else {
        cmd.arg("--project-root").arg(&root);
    }

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
    if let Some(model) = deepseek_model {
        if !model.is_empty() {
            cmd.env("DEEPSEEK_MODEL", model);
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
) -> Result<RunResult, String> {
    let runner = spawn_and_watch(app, active, cmd).map_err(String::from)?;
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
            })
        }
        Some(c) => {
            let err = classify_stderr(&stderr_lines, Some(c));
            Err(err.into())
        }
    }
}

// ---------------------------------------------------------------------------
// Main command
// ---------------------------------------------------------------------------

#[allow(clippy::too_many_arguments)]
#[tauri::command]
fn run_meeting_asr(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    audio_path: String,
    output_mode: String,
    backend: Option<String>,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    deepseek_model: Option<String>,
) -> Result<RunResult, String> {
    // Validate output mode.
    if output_mode != "transcript" && output_mode != "notes" && output_mode != "minutes" {
        return Err(BackendError::InvalidInput {
            message: format!(
                "Invalid output_mode: {}. Must be 'transcript', 'notes', or 'minutes'.",
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
    let backend_kind = match backend.as_deref().unwrap_or("python") {
        "python" => BackendKind::Python,
        "go" => BackendKind::Go,
        other => {
            return Err(BackendError::InvalidInput {
                message: format!("Invalid backend: {}. Must be 'python' or 'go'.", other),
            }
            .into())
        }
    };

    // Resolve the runtime-aware output root, creating it when needed.
    let (abs_out, _) = resolve_output_root(&app).map_err(String::from)?;
    let out_dir = abs_out.to_string_lossy().to_string();

    let env_file = find_env_file();

    let cmd = match backend_kind {
        BackendKind::Python => build_python_command(
            &abs_audio_str,
            &output_mode,
            &out_dir,
            mimo_api_key.as_deref(),
            deepseek_api_key.as_deref(),
            deepseek_model.as_deref(),
        ),
        BackendKind::Go => build_go_command(
            &abs_audio_str,
            &output_mode,
            &out_dir,
            mimo_api_key.as_deref(),
            deepseek_api_key.as_deref(),
            deepseek_model.as_deref(),
            env_file.as_ref(),
        )
        .map_err(String::from)?,
    };

    run_and_check(&app, active.inner(), cmd, &output_mode, &abs_out)
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
        "transcript" | "notes" | "minutes" => Some(mode.to_string()),
        _ => None,
    }
}

/// Validates an output_mode string and returns it.
fn validate_output_mode(mode: &str) -> Result<String, BackendError> {
    match mode {
        "transcript" | "notes" | "minutes" => Ok(mode.to_string()),
        other => Err(BackendError::InvalidInput {
            message: format!(
                "Invalid output_mode: {}. Must be 'transcript', 'notes', or 'minutes'.",
                other
            ),
        }),
    }
}

#[allow(clippy::too_many_arguments)]
#[tauri::command]
fn resume_meeting_asr(
    app: AppHandle,
    active: State<'_, ActiveProcess>,
    task_dir: String,
    output_mode: Option<String>,
    backend: Option<String>,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    deepseek_model: Option<String>,
) -> Result<RunResult, String> {
    let backend_kind = match backend.as_deref().unwrap_or("python") {
        "python" => BackendKind::Python,
        "go" => BackendKind::Go,
        other => {
            return Err(BackendError::InvalidInput {
                message: format!("Invalid backend: {}. Must be 'python' or 'go'.", other),
            }
            .into())
        }
    };

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
    let effective_mode: Result<String, BackendError> = if backend_kind == BackendKind::Go {
        // Go: require a manifest; read output_mode from it unless overridden.
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
    } else {
        // Python: keep existing behaviour — use provided mode or default.
        Ok(output_mode
            .clone()
            .unwrap_or_else(|| "transcript".to_string()))
    };

    let effective_mode = effective_mode.map_err(String::from)?;
    validate_output_mode(&effective_mode).map_err(String::from)?;

    let root = resolve_project_root();
    let env_file = find_env_file();

    let mut cmd = match backend_kind {
        BackendKind::Python => {
            let mut c = Command::new("uv");
            c.arg("run")
                .arg("meeting-asr")
                .arg("resume")
                .arg(abs_task_dir.as_os_str())
                .current_dir(&root);
            if let Some(ref mode) = output_mode {
                c.arg("--output-mode").arg(mode);
            }
            c
        }
        BackendKind::Go => {
            let binary = discover_go_binary().map_err(String::from)?;
            validate_go_binary(&binary).map_err(String::from)?;

            let mut c = Command::new(binary);
            c.arg("resume").arg(abs_task_dir.as_os_str());
            if let Some(ref mode) = output_mode {
                c.arg("--output-mode").arg(mode);
            }
            if let Some(env) = env_file.as_ref() {
                c.arg("--env-file").arg(env);
            } else {
                c.arg("--project-root").arg(&root);
            }
            if let Some(key) = mimo_api_key.as_deref() {
                if !key.is_empty() {
                    c.env("MIMO_API_KEY", key);
                }
            }
            if let Some(key) = deepseek_api_key.as_deref() {
                if !key.is_empty() {
                    c.env("DEEPSEEK_API_KEY", key);
                }
            }
            if let Some(model) = deepseek_model.as_deref() {
                if !model.is_empty() {
                    c.env("DEEPSEEK_MODEL", model);
                }
            }
            c
        }
    };

    if backend_kind == BackendKind::Python {
        if let Some(key) = mimo_api_key.as_deref() {
            if !key.is_empty() {
                cmd.env("MIMO_API_KEY", key);
            }
        }
        if let Some(key) = deepseek_api_key.as_deref() {
            if !key.is_empty() {
                cmd.env("DEEPSEEK_API_KEY", key);
            }
        }
    }

    run_and_check(&app, active.inner(), cmd, &effective_mode, &output_root)
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

#[tauri::command]
fn save_env_key(key: String, value: String) -> Result<(), String> {
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
        .invoke_handler(tauri::generate_handler![
            get_output_root,
            set_output_root,
            open_task_folder,
            run_meeting_asr,
            resume_meeting_asr,
            save_env_key,
        ])
        .build(tauri::generate_context!())
        .expect("error while building Tauri application");

    app.run(|app_handle, event| {
        if matches!(event, RunEvent::ExitRequested { .. }) {
            app_handle.state::<ActiveProcess>().terminate();
        }
    });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

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
        let err = classify_stderr(&["Error: no manifest found".to_string()], Some(1));
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
        );
        match err {
            BackendError::TaskLocked { .. } => {}
            _ => panic!("expected TaskLocked, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_dependency_missing() {
        let err = classify_stderr(
            &["Error: ffmpeg was not found on PATH".to_string()],
            Some(1),
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
        );
        match err {
            BackendError::MissingApiKey { .. } => {}
            _ => panic!("expected MissingApiKey, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_invalid_input() {
        let err = classify_stderr(&["Error: invalid --output-mode".to_string()], Some(2));
        match err {
            BackendError::InvalidInput { .. } => {}
            _ => panic!("expected InvalidInput, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_process_failed_unknown() {
        let err = classify_stderr(&["some random error".to_string()], Some(3));
        match err {
            BackendError::ProcessFailed { code, .. } => {
                assert_eq!(code, Some(3));
            }
            _ => panic!("expected ProcessFailed, got {:?}", err),
        }
    }

    #[test]
    fn test_classify_process_failed_no_stderr() {
        let err = classify_stderr(&[], Some(1));
        match err {
            BackendError::ProcessFailed { code, .. } => {
                assert_eq!(code, Some(1));
            }
            _ => panic!("expected ProcessFailed, got {:?}", err),
        }
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
        let python: BackendKind = serde_json::from_str("\"python\"").unwrap();
        assert_eq!(python, BackendKind::Python);
        let go: BackendKind = serde_json::from_str("\"go\"").unwrap();
        assert_eq!(go, BackendKind::Go);
    }

    #[test]
    fn test_backend_kind_invalid() {
        assert!(serde_json::from_str::<BackendKind>("\"rust\"").is_err());
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

        let saved = set_configured_output_root(&settings_path, &output_root).unwrap();
        let settings = load_desktop_settings(&settings_path).unwrap();
        assert_eq!(get_configured_output_root(&settings).unwrap(), Some(saved));

        let json = fs::read_to_string(&settings_path).unwrap();
        assert!(json.contains("output_root"));
        assert!(!json.contains("API_KEY"));
        assert!(!json.contains("transcript"));
        fs::remove_dir_all(base).ok();
    }

    #[test]
    fn test_configured_output_root_rejects_relative_path() {
        let settings = DesktopSettings {
            output_root: Some("relative-output".to_string()),
            ..Default::default()
        };
        assert!(get_configured_output_root(&settings).is_err());
    }

    #[test]
    fn test_settings_reject_unknown_fields() {
        assert!(serde_json::from_str::<DesktopSettings>(
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

    #[test]
    fn test_build_go_command_paths_are_discrete() {
        // The build_go_command requires a real Go binary, but we can test
        // the Python command construction pattern for discrete arg safety.
        let cmd = build_python_command(
            "/path/with spaces/audio.mp3",
            "transcript",
            "/output/dir",
            None,
            None,
            None,
        );
        let args: Vec<_> = cmd.get_args().collect();
        // Verify that a path with spaces is NOT split into multiple args.
        let audio_arg_found = args
            .iter()
            .any(|a| a.to_string_lossy().contains("/path/with spaces/audio.mp3"));
        assert!(audio_arg_found, "audio path with spaces should be one arg");

        // Verify output dir arg is also discrete.
        let out_arg_found = args.iter().any(|a| a.to_string_lossy() == "/output/dir");
        assert!(out_arg_found, "output dir should be one discrete arg");
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
    fn test_discover_go_binary_env_var_nonexistent() {
        std::env::set_var("MEETING_ASR_GO_BINARY", "/nonexistent/go-binary");
        let result = discover_go_binary();
        std::env::remove_var("MEETING_ASR_GO_BINARY");
        assert!(result.is_err());
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
    fn test_python_resume_does_not_require_go_manifest() {
        // A Python task directory has no Go manifest files at all.
        // find_go_manifest must return None — Python resume skips the
        // Go manifest requirement and uses its own default mode.
        let dir = std::env::temp_dir().join("test-py-nomanifest");
        fs::create_dir_all(&dir).unwrap();
        // Create a Python-typical layout (just audio/chunks/transcripts).
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
