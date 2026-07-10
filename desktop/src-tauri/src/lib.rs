use std::fs;
use std::fs::OpenOptions;
use std::io::{BufRead, BufReader, Write};
use std::path::Path;
use std::process::{Command, Stdio};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use tauri::{AppHandle, Emitter};

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
struct RunResult {
    status: String,
    code: Option<i32>,
    task_folder: Option<String>,
    transcript_path: Option<String>,
    notes_input_path: Option<String>,
    minutes_path: Option<String>,
}

fn normalize_status(status: &str) -> String {
    let normalized = status.trim().to_ascii_lowercase();
    if normalized.starts_with("splitting") {
        "splitting".into()
    } else if normalized.starts_with("transcribing") {
        "transcribing".into()
    } else if normalized.starts_with("merging") {
        "merging".into()
    } else if normalized.starts_with("preparing notes") {
        "notes".into()
    } else if normalized.starts_with("generating meeting minutes") {
        "minutes".into()
    } else if normalized == "done" {
        "done".into()
    } else {
        normalized
    }
}

fn project_root() -> &'static str {
    concat!(env!("CARGO_MANIFEST_DIR"), "/../..")
}

#[tauri::command]
fn open_task_folder(task_folder: String) -> Result<(), String> {
    if task_folder.is_empty() {
        return Err("No task folder specified. Run a transcription first.".into());
    }

    let root = Path::new(project_root());
    let output_dir = root.join("output");

    let abs_output = output_dir
        .canonicalize()
        .map_err(|_| "Output directory does not exist yet. Run a transcription first.".to_string())?;

    let candidate = root.join(&task_folder);
    let abs_candidate = candidate
        .canonicalize()
        .map_err(|_| format!("Task folder does not exist: {}", task_folder))?;

    if !abs_candidate.starts_with(&abs_output) {
        return Err("Cannot open folders outside the output directory.".into());
    }

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

#[tauri::command]
fn run_meeting_asr(
    app: AppHandle,
    audio_path: String,
    output_mode: String,
    mimo_api_key: Option<String>,
    deepseek_api_key: Option<String>,
    deepseek_model: Option<String>,
) -> Result<RunResult, String> {
    if output_mode != "transcript" && output_mode != "minutes" {
        return Err(format!(
            "Invalid output_mode: {}. Must be 'transcript' or 'minutes'.",
            output_mode
        ));
    }

    let ext = Path::new(&audio_path)
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_lowercase();
    if ext != "mp3" && ext != "wav" && ext != "m4a" {
        return Err("Only .mp3, .wav, and .m4a audio files are supported.".into());
    }

    let root = project_root();

    let mut cmd = Command::new("uv");
    cmd.arg("run")
        .arg("meeting-asr")
        .arg("transcribe")
        .arg(&audio_path)
        .arg("--output-mode")
        .arg(&output_mode)
        .current_dir(root)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    if let Some(ref key) = mimo_api_key {
        if !key.is_empty() {
            cmd.env("MIMO_API_KEY", key);
        }
    }

    if let Some(ref key) = deepseek_api_key {
        if !key.is_empty() {
            cmd.env("DEEPSEEK_API_KEY", key);
        }
    }

    if let Some(ref model) = deepseek_model {
        if !model.is_empty() {
            cmd.arg("--llm-model").arg(model);
        }
    }

    let mut child = cmd
        .spawn()
        .map_err(|e| {
            if e.kind() == std::io::ErrorKind::NotFound {
                "uv is not installed. Please install uv and run uv sync --all-extras first.".to_string()
            } else {
                format!("Failed to start uv: {}", e)
            }
        })?;

    let stdout = child.stdout.take().unwrap();
    let stderr = child.stderr.take().unwrap();

    let collected_lines: Arc<Mutex<Vec<String>>> = Arc::new(Mutex::new(Vec::new()));
    let collected_for_thread = collected_lines.clone();

    let app_stdout = app.clone();
    let stdout_handle: JoinHandle<()> = thread::spawn(move || {
        let reader = BufReader::new(stdout);
        for line in reader.lines() {
            match line {
                Ok(text) => {
                    let trimmed = text.trim();

                    if let Some(status_val) = trimmed.strip_prefix("Status: ") {
                        let _ = app_stdout.emit("progress", ProgressEvent {
                            status: normalize_status(status_val),
                            current: 0,
                            total: 0,
                        });
                    } else if let Some(progress_str) = trimmed.strip_prefix("Progress: ") {
                        let progress_str = progress_str.trim();
                        let parts: Vec<&str> = progress_str.split('/').collect();
                        if parts.len() == 2 {
                            let current = parts[0].parse::<u32>().unwrap_or(0);
                            let total = parts[1].parse::<u32>().unwrap_or(0);
                            let _ = app_stdout.emit("progress", ProgressEvent {
                                status: "transcribing".to_string(),
                                current,
                                total,
                            });
                        }
                    }

                    let _ = app_stdout.emit(
                        "log-line",
                        LogEvent {
                            stream: "stdout".into(),
                            line: text.clone(),
                        },
                    );
                    collected_for_thread.lock().unwrap().push(text);
                }
                Err(_) => break,
            }
        }
    });

    let app_stderr = app.clone();
    let stderr_handle: JoinHandle<()> = thread::spawn(move || {
        let reader = BufReader::new(stderr);
        for line in reader.lines() {
            match line {
                Ok(text) => {
                    let _ = app_stderr.emit(
                        "log-line",
                        LogEvent {
                            stream: "stderr".into(),
                            line: text,
                        },
                    );
                }
                Err(_) => break,
            }
        }
    });

    let status = child
        .wait()
        .map_err(|e| format!("Process error: {}", e))?;

    let _ = stdout_handle.join();
    let _ = stderr_handle.join();

    let code = status.code();

    let _ = app.emit(
        "process-done",
        DoneEvent { code },
    );

    let lines = collected_lines.lock().unwrap();
    let mut task_folder: Option<String> = None;
    let mut transcript_path: Option<String> = None;
    let mut notes_input_path: Option<String> = None;
    let mut minutes_path: Option<String> = None;

    for line in lines.iter() {
        let trimmed = line.trim();
        if let Some(val) = trimmed.strip_prefix("Task folder:") {
            let val = val.trim();
            if !val.is_empty() {
                task_folder = Some(val.to_string());
            }
        } else if let Some(val) = trimmed.strip_prefix("Transcript saved:") {
            let val = val.trim();
            if !val.is_empty() {
                transcript_path = Some(val.to_string());
            }
        } else if let Some(val) = trimmed.strip_prefix("Notes input saved:") {
            let val = val.trim();
            if !val.is_empty() {
                notes_input_path = Some(val.to_string());
            }
        } else if let Some(val) = trimmed.strip_prefix("Meeting minutes saved:") {
            let val = val.trim();
            if !val.is_empty() {
                minutes_path = Some(val.to_string());
            }
        }
    }

    let status_str = match code {
        Some(0) => "success".to_string(),
        Some(c) => format!("exited with code {}", c),
        None => "terminated by signal".to_string(),
    };

    Ok(RunResult {
        status: status_str,
        code,
        task_folder,
        transcript_path,
        notes_input_path,
        minutes_path,
    })
}

const ALLOWED_ENV_KEYS: &[&str] = &["MIMO_API_KEY", "DEEPSEEK_API_KEY"];

#[tauri::command]
fn save_env_key(key: String, value: String) -> Result<(), String> {
    if key.is_empty() {
        return Err("Key name cannot be empty.".into());
    }
    validate_env_value(&value)?;
    if !ALLOWED_ENV_KEYS.contains(&key.as_str()) {
        return Err(format!(
            "Key '{}' is not allowed. Allowed keys: {:?}",
            key, ALLOWED_ENV_KEYS
        ));
    }

    let env_path = Path::new(project_root()).join(".env");
    let new_line = format!("{}={}", key, value);

    if !env_path.exists() {
        write_env_securely(&env_path, &(new_line + "\n"))?;
        return Ok(());
    }

    let content = fs::read_to_string(&env_path)
        .map_err(|e| format!("Failed to read .env: {}", e))?;

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
    let result = if result.ends_with('\n') { result } else { result + "\n" };

    write_env_securely(&env_path, &result)?;

    Ok(())
}

fn validate_env_value(value: &str) -> Result<(), String> {
    if value.is_empty() {
        return Err("Key value cannot be empty.".into());
    }
    if value.trim() != value || value.contains(['\r', '\n']) {
        return Err("Key value cannot contain leading/trailing whitespace or newlines.".into());
    }
    Ok(())
}

fn write_env_securely(env_path: &Path, content: &str) -> Result<(), String> {
    let parent = env_path
        .parent()
        .ok_or_else(|| "Could not determine .env parent directory.".to_string())?;
    fs::create_dir_all(parent).map_err(|e| format!("Failed to create .env directory: {}", e))?;

    let temp_path = parent.join(format!(".env.{}.tmp", std::process::id()));
    let mut options = OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }

    let result = (|| -> Result<(), String> {
        let mut file = options
            .open(&temp_path)
            .map_err(|e| format!("Failed to create temporary .env: {}", e))?;
        file.write_all(content.as_bytes())
            .map_err(|e| format!("Failed to write temporary .env: {}", e))?;
        file.sync_all()
            .map_err(|e| format!("Failed to sync temporary .env: {}", e))?;
        drop(file);
        if let Err(error) = fs::rename(&temp_path, env_path) {
            #[cfg(not(target_os = "windows"))]
            return Err(format!("Failed to replace .env: {}", error));

            #[cfg(target_os = "windows")]
            {
                if env_path.exists() {
                    fs::remove_file(env_path)
                        .map_err(|e| format!("Failed to remove old .env: {}", e))?;
                }
                fs::rename(&temp_path, env_path)
                    .map_err(|e| format!("Failed to replace .env: {}", e))?;
            }
        }
        Ok(())
    })();

    if result.is_err() {
        let _ = fs::remove_file(&temp_path);
    }
    result
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![open_task_folder, run_meeting_asr, save_env_key])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(test)]
mod tests {
    use super::{normalize_status, validate_env_value};

    #[test]
    fn normalizes_python_and_go_status_lines() {
        assert_eq!(normalize_status("Splitting audio"), "splitting");
        assert_eq!(normalize_status("Transcribing 3 chunk(s)"), "transcribing");
        assert_eq!(normalize_status("Merging transcript"), "merging");
        assert_eq!(normalize_status("Preparing notes input"), "notes");
        assert_eq!(
            normalize_status("Generating meeting minutes via LLM"),
            "minutes"
        );
        assert_eq!(normalize_status("Done"), "done");
    }

    #[test]
    fn rejects_unsafe_env_values() {
        assert!(validate_env_value("").is_err());
        assert!(validate_env_value(" key").is_err());
        assert!(validate_env_value("key\nOTHER=injected").is_err());
        assert!(validate_env_value("valid-key").is_ok());
    }
}
