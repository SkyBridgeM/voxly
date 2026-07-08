use std::io::{BufRead, BufReader};
use std::process::{Command, Stdio};
use std::thread;
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

#[tauri::command]
fn run_meeting_asr(
    app: AppHandle,
    audio_path: String,
    output_mode: String,
) -> Result<String, String> {
    if output_mode != "transcript" && output_mode != "minutes" {
        return Err(format!(
            "Invalid output_mode: {}. Must be 'transcript' or 'minutes'.",
            output_mode
        ));
    }

    let project_root = concat!(env!("CARGO_MANIFEST_DIR"), "/../..");
    let python = format!("{}/.venv/bin/python", project_root);

    let mut cmd = Command::new(&python);
    cmd.arg("-m")
        .arg("meeting_asr.main")
        .arg("transcribe")
        .arg(&audio_path)
        .arg("--output-mode")
        .arg(&output_mode)
        .current_dir(project_root)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    let mut child = cmd
        .spawn()
        .map_err(|e| format!("Failed to start Python CLI: {}", e))?;

    let stdout = child.stdout.take().unwrap();
    let stderr = child.stderr.take().unwrap();

    let app_stdout = app.clone();
    thread::spawn(move || {
        let reader = BufReader::new(stdout);
        for line in reader.lines() {
            match line {
                Ok(text) => {
                    let _ = app_stdout.emit(
                        "log-line",
                        LogEvent {
                            stream: "stdout".into(),
                            line: text,
                        },
                    );
                }
                Err(_) => break,
            }
        }
    });

    let app_stderr = app.clone();
    thread::spawn(move || {
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

    let code = status.code();

    let _ = app.emit(
        "process-done",
        DoneEvent { code },
    );

    match code {
        Some(0) => Ok("success".into()),
        Some(c) => Ok(format!("exited with code {}", c)),
        None => Ok("terminated by signal".into()),
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![run_meeting_asr])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
