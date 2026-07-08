use std::io::{BufRead, BufReader};
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

                    if trimmed.starts_with("Status: ") {
                        let status_val = trimmed["Status: ".len()..].trim();
                        let _ = app_stdout.emit("progress", ProgressEvent {
                            status: status_val.to_string(),
                            current: 0,
                            total: 0,
                        });
                    } else if trimmed.starts_with("Progress: ") {
                        let progress_str = trimmed["Progress: ".len()..].trim();
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
        if trimmed.starts_with("Task folder:") {
            let val = trimmed["Task folder:".len()..].trim();
            if !val.is_empty() {
                task_folder = Some(val.to_string());
            }
        } else if trimmed.starts_with("Transcript saved:") {
            let val = trimmed["Transcript saved:".len()..].trim();
            if !val.is_empty() {
                transcript_path = Some(val.to_string());
            }
        } else if trimmed.starts_with("Notes input saved:") {
            let val = trimmed["Notes input saved:".len()..].trim();
            if !val.is_empty() {
                notes_input_path = Some(val.to_string());
            }
        } else if trimmed.starts_with("Meeting minutes saved:") {
            let val = trimmed["Meeting minutes saved:".len()..].trim();
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

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![open_task_folder, run_meeting_asr])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
