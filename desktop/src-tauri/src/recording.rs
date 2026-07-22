use super::{
    canonicalize_abs_dir, canonicalize_path_within, discover_go_binary, packaged_go_executable_dir,
    resolve_output_root, RuntimeMode,
};
use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use cpal::SampleFormat;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::atomic::{AtomicBool, AtomicU64, AtomicUsize, Ordering};
use std::sync::{mpsc, Arc, Mutex};
use std::thread::JoinHandle;
use tauri::{AppHandle, Emitter, Manager, State};

#[derive(Clone, Serialize)]
pub struct InputDeviceInfo {
    id: String,
    name: String,
    is_default: bool,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct RecordingTask {
    pub task_id: String,
    pub task_folder: String,
    pub checkpoints_dir: String,
    #[serde(default)]
    pub audio_path: String,
    pub status: String,
    pub duration_ms: u64,
    pub checkpoint_count: usize,
}

#[derive(Clone, Serialize)]
pub struct RecordingState {
    pub active: bool,
    pub paused: bool,
    pub task: Option<RecordingTask>,
}

#[derive(Clone, Serialize)]
struct RecordingProgress {
    duration_ms: u64,
    level: f32,
    checkpoint_count: usize,
}

struct CaptureControl {
    paused: AtomicBool,
    samples: AtomicU64,
    checkpoints: AtomicUsize,
    last_emit_samples: AtomicU64,
    buffer: Mutex<Vec<i16>>,
}

enum CaptureCommand {
    Pause(mpsc::Sender<Result<RecordingTask, String>>),
    Resume(mpsc::Sender<Result<RecordingTask, String>>),
    Finish(String, mpsc::Sender<Result<RecordingTask, String>>),
}

struct RecordingHandle {
    sender: mpsc::Sender<CaptureCommand>,
    control: Arc<CaptureControl>,
    task: Arc<Mutex<RecordingTask>>,
    join: Option<JoinHandle<()>>,
}

#[derive(Clone, Default)]
pub struct RecordingManager {
    session: Arc<Mutex<Option<RecordingHandle>>>,
    starting: Arc<AtomicBool>,
}

fn sidecar() -> Result<PathBuf, String> {
    let mode = RuntimeMode::current();
    let packaged = if mode == RuntimeMode::Production {
        Some(packaged_go_executable_dir().map_err(String::from)?)
    } else {
        None
    };
    discover_go_binary(mode, packaged.as_deref()).map_err(String::from)
}

fn run_recording_command(args: &[String]) -> Result<RecordingTask, String> {
    let output = Command::new(sidecar()?)
        .args(args)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .output()
        .map_err(|_| "Could not start the Voxly recording task runtime.".to_string())?;
    if !output.status.success() {
        return Err("The Voxly recording task runtime could not complete the request.".to_string());
    }
    serde_json::from_slice(&output.stdout)
        .map_err(|_| "The recording task runtime returned invalid data.".to_string())
}

fn find_input_device(device_id: Option<&str>) -> Result<cpal::Device, String> {
    let host = cpal::default_host();
    if let Some(id) = device_id {
        return host
            .input_devices()
            .map_err(|e| format!("Could not enumerate microphones: {e}"))?
            .enumerate()
            .find(|(index, _)| format!("input-{index}") == id)
            .map(|(_, device)| device)
            .ok_or_else(|| "The selected microphone is no longer available.".to_string());
    }
    host.default_input_device()
        .ok_or_else(|| "No microphone is available.".to_string())
}

fn list_input_devices_blocking() -> Result<Vec<InputDeviceInfo>, String> {
    let host = cpal::default_host();
    let default_name = host.default_input_device().and_then(|d| d.name().ok());
    host.input_devices()
        .map_err(|e| format!("Could not enumerate microphones: {e}"))?
        .enumerate()
        .map(|(index, device)| {
            let name = device
                .name()
                .unwrap_or_else(|_| format!("Microphone {}", index + 1));
            Ok(InputDeviceInfo {
                id: format!("input-{index}"),
                is_default: default_name.as_deref() == Some(name.as_str()),
                name,
            })
        })
        .collect()
}

#[tauri::command]
pub async fn list_input_devices() -> Result<Vec<InputDeviceInfo>, String> {
    tauri::async_runtime::spawn_blocking(list_input_devices_blocking)
        .await
        .map_err(|_| "The microphone worker stopped unexpectedly.".to_string())?
}

fn write_checkpoints(
    receiver: mpsc::Receiver<Vec<i16>>,
    directory: PathBuf,
    sample_rate: u32,
    control: Arc<CaptureControl>,
) -> Result<(), String> {
    for samples in receiver {
        if samples.is_empty() {
            continue;
        }
        let index = control.checkpoints.fetch_add(1, Ordering::SeqCst) + 1;
        let path = directory.join(format!("checkpoint_{index:04}.wav"));
        let spec = hound::WavSpec {
            channels: 1,
            sample_rate,
            bits_per_sample: 16,
            sample_format: hound::SampleFormat::Int,
        };
        let mut writer = hound::WavWriter::create(path, spec)
            .map_err(|e| format!("Could not create recording checkpoint: {e}"))?;
        for sample in samples {
            writer.write_sample(sample).map_err(|e| e.to_string())?;
        }
        writer.finalize().map_err(|e| e.to_string())?;
    }
    Ok(())
}

fn accept_samples(
    samples: impl Iterator<Item = f32>,
    channels: usize,
    control: &Arc<CaptureControl>,
    sender: &mpsc::Sender<Vec<i16>>,
    sample_rate: u32,
    app: &AppHandle,
) {
    if control.paused.load(Ordering::Relaxed) {
        return;
    }
    let input = samples.collect::<Vec<_>>();
    let mut peak = 0.0_f32;
    let mut buffer = match control.buffer.lock() {
        Ok(value) => value,
        Err(_) => return,
    };
    for frame in input.chunks(channels.max(1)) {
        let mono = frame.iter().copied().sum::<f32>() / frame.len().max(1) as f32;
        peak = peak.max(mono.abs());
        buffer.push((mono.clamp(-1.0, 1.0) * i16::MAX as f32) as i16);
    }
    let added = (input.len() / channels.max(1)) as u64;
    let total = control.samples.fetch_add(added, Ordering::Relaxed) + added;
    if buffer.len() >= sample_rate as usize * 30 {
        let _ = sender.send(std::mem::take(&mut *buffer));
    }
    let previous = control.last_emit_samples.load(Ordering::Relaxed);
    if total.saturating_sub(previous) >= sample_rate as u64 / 10 {
        control.last_emit_samples.store(total, Ordering::Relaxed);
        let _ = app.emit(
            "recording-progress",
            RecordingProgress {
                duration_ms: total.saturating_mul(1000) / sample_rate as u64,
                level: peak.min(1.0),
                checkpoint_count: control.checkpoints.load(Ordering::Relaxed),
            },
        );
    }
}

fn recording_update_args(
    task: &RecordingTask,
    control: &CaptureControl,
    sample_rate: u32,
    status: &str,
    final_audio: Option<&Path>,
) -> Vec<String> {
    let duration = control.samples.load(Ordering::SeqCst) * 1000 / sample_rate as u64;
    let count = control.checkpoints.load(Ordering::SeqCst);
    let mut args = vec![
        "recording-update".into(),
        "--task-dir".into(),
        task.task_folder.clone(),
        "--status".into(),
        status.into(),
        "--duration-ms".into(),
        duration.to_string(),
        "--checkpoint-count".into(),
        count.to_string(),
    ];
    if let Some(path) = final_audio {
        args.push("--final-audio".into());
        args.push(path.to_string_lossy().into_owned());
    }
    args
}

fn merge_checkpoints(directory: &Path, output: &Path, sample_rate: u32) -> Result<(), String> {
    let mut files = fs::read_dir(directory)
        .map_err(|e| e.to_string())?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| path.extension().and_then(|e| e.to_str()) == Some("wav"))
        .collect::<Vec<_>>();
    files.sort();
    if files.is_empty() {
        return Err("No durable audio was captured.".to_string());
    }
    let spec = hound::WavSpec {
        channels: 1,
        sample_rate,
        bits_per_sample: 16,
        sample_format: hound::SampleFormat::Int,
    };
    let mut writer = hound::WavWriter::create(output, spec).map_err(|e| e.to_string())?;
    for file in files {
        let mut reader = hound::WavReader::open(file).map_err(|e| e.to_string())?;
        for sample in reader.samples::<i16>() {
            writer
                .write_sample(sample.map_err(|e| e.to_string())?)
                .map_err(|e| e.to_string())?;
        }
    }
    writer.finalize().map_err(|e| e.to_string())
}

fn finish_capture(
    sender: mpsc::Sender<Vec<i16>>,
    writer: JoinHandle<Result<(), String>>,
    task: &RecordingTask,
    control: &CaptureControl,
    sample_rate: u32,
    status: &str,
) -> Result<RecordingTask, String> {
    if let Ok(mut buffer) = control.buffer.lock() {
        if !buffer.is_empty() {
            let _ = sender.send(std::mem::take(&mut *buffer));
        }
    }
    drop(sender);
    writer
        .join()
        .map_err(|_| "Recording writer stopped unexpectedly.".to_string())??;
    let output = PathBuf::from(&task.task_folder)
        .join("audio")
        .join("recording.wav");
    merge_checkpoints(Path::new(&task.checkpoints_dir), &output, sample_rate)?;
    run_recording_command(&recording_update_args(
        task,
        control,
        sample_rate,
        status,
        Some(&output),
    ))
}

fn capture_thread(
    app: AppHandle,
    device_id: Option<String>,
    task: Arc<Mutex<RecordingTask>>,
    control: Arc<CaptureControl>,
    commands: mpsc::Receiver<CaptureCommand>,
    ready: mpsc::Sender<Result<(), String>>,
) {
    let setup = (|| -> Result<_, String> {
        let device = find_input_device(device_id.as_deref())?;
        let supported = device
            .default_input_config()
            .map_err(|e| format!("Could not open the microphone: {e}"))?;
        let sample_format = supported.sample_format();
        let config: cpal::StreamConfig = supported.into();
        let sample_rate = config.sample_rate.0;
        let channels = config.channels as usize;
        let checkpoint_dir = task
            .lock()
            .map_err(|_| "Recording task state is unavailable.".to_string())?
            .checkpoints_dir
            .clone();
        let (audio_sender, audio_receiver) = mpsc::channel();
        let writer_control = control.clone();
        let writer = std::thread::spawn(move || {
            write_checkpoints(
                audio_receiver,
                PathBuf::from(checkpoint_dir),
                sample_rate,
                writer_control,
            )
        });
        let error_app = app.clone();
        let stream = match sample_format {
            SampleFormat::F32 => {
                let c = control.clone();
                let tx = audio_sender.clone();
                let a = app.clone();
                device.build_input_stream(
                    &config,
                    move |data: &[f32], _| {
                        accept_samples(data.iter().copied(), channels, &c, &tx, sample_rate, &a)
                    },
                    move |error| {
                        let _ = error_app.emit("recording-error", error.to_string());
                    },
                    None,
                )
            }
            SampleFormat::I16 => {
                let c = control.clone();
                let tx = audio_sender.clone();
                let a = app.clone();
                device.build_input_stream(
                    &config,
                    move |data: &[i16], _| {
                        accept_samples(
                            data.iter().map(|v| *v as f32 / i16::MAX as f32),
                            channels,
                            &c,
                            &tx,
                            sample_rate,
                            &a,
                        )
                    },
                    move |error| {
                        let _ = error_app.emit("recording-error", error.to_string());
                    },
                    None,
                )
            }
            SampleFormat::U16 => {
                let c = control.clone();
                let tx = audio_sender.clone();
                let a = app.clone();
                device.build_input_stream(
                    &config,
                    move |data: &[u16], _| {
                        accept_samples(
                            data.iter()
                                .map(|v| (*v as f32 / u16::MAX as f32) * 2.0 - 1.0),
                            channels,
                            &c,
                            &tx,
                            sample_rate,
                            &a,
                        )
                    },
                    move |error| {
                        let _ = error_app.emit("recording-error", error.to_string());
                    },
                    None,
                )
            }
            _ => return Err("The microphone sample format is not supported.".to_string()),
        }
        .map_err(|e| format!("Could not start microphone capture: {e}"))?;
        stream
            .play()
            .map_err(|e| format!("Could not start microphone capture: {e}"))?;
        Ok((stream, audio_sender, writer, sample_rate))
    })();

    let (stream, audio_sender, writer, sample_rate) = match setup {
        Ok(value) => {
            let _ = ready.send(Ok(()));
            value
        }
        Err(error) => {
            let _ = ready.send(Err(error));
            return;
        }
    };
    let mut stream = Some(stream);
    let mut writer = Some(writer);
    let mut sender = Some(audio_sender);
    while let Ok(command) = commands.recv() {
        match command {
            CaptureCommand::Pause(reply) => {
                let result = stream
                    .as_ref()
                    .unwrap()
                    .pause()
                    .map_err(|e| e.to_string())
                    .and_then(|_| {
                        control.paused.store(true, Ordering::SeqCst);
                        let current = task
                            .lock()
                            .map_err(|_| "Recording task state is unavailable.".to_string())?
                            .clone();
                        match run_recording_command(&recording_update_args(
                            &current,
                            &control,
                            sample_rate,
                            "paused",
                            None,
                        )) {
                            Ok(updated) => Ok(updated),
                            Err(error) => {
                                control.paused.store(false, Ordering::SeqCst);
                                let _ = stream.as_ref().unwrap().play();
                                Err(error)
                            }
                        }
                    });
                if let Ok(updated) = &result {
                    if let Ok(mut current) = task.lock() {
                        *current = updated.clone();
                    }
                }
                let _ = reply.send(result);
            }
            CaptureCommand::Resume(reply) => {
                let result = stream
                    .as_ref()
                    .unwrap()
                    .play()
                    .map_err(|e| e.to_string())
                    .and_then(|_| {
                        control.paused.store(false, Ordering::SeqCst);
                        let current = task
                            .lock()
                            .map_err(|_| "Recording task state is unavailable.".to_string())?
                            .clone();
                        match run_recording_command(&recording_update_args(
                            &current,
                            &control,
                            sample_rate,
                            "recording",
                            None,
                        )) {
                            Ok(updated) => Ok(updated),
                            Err(error) => {
                                control.paused.store(true, Ordering::SeqCst);
                                let _ = stream.as_ref().unwrap().pause();
                                Err(error)
                            }
                        }
                    });
                if let Ok(updated) = &result {
                    if let Ok(mut current) = task.lock() {
                        *current = updated.clone();
                    }
                }
                let _ = reply.send(result);
            }
            CaptureCommand::Finish(status, reply) => {
                if let Some(active_stream) = stream.take() {
                    let _ = active_stream.pause();
                    drop(active_stream);
                }
                let current = task
                    .lock()
                    .map(|v| v.clone())
                    .map_err(|_| "Recording task state is unavailable.".to_string());
                let result = current.and_then(|current| {
                    finish_capture(
                        sender.take().unwrap(),
                        writer.take().unwrap(),
                        &current,
                        &control,
                        sample_rate,
                        &status,
                    )
                });
                if let Ok(updated) = &result {
                    if let Ok(mut current) = task.lock() {
                        *current = updated.clone();
                    }
                }
                let _ = reply.send(result);
                break;
            }
        }
    }
}

fn start_recording_blocking(
    app: AppHandle,
    manager: RecordingManager,
    device_id: Option<String>,
) -> Result<RecordingTask, String> {
    if manager
        .starting
        .compare_exchange(false, true, Ordering::SeqCst, Ordering::SeqCst)
        .is_err()
    {
        return Err("A recording is already active.".to_string());
    }
    struct StartingGuard(Arc<AtomicBool>);
    impl Drop for StartingGuard {
        fn drop(&mut self) {
            self.0.store(false, Ordering::SeqCst);
        }
    }
    let _starting = StartingGuard(manager.starting.clone());
    if manager
        .session
        .lock()
        .map_err(|_| "Recording state is unavailable.")?
        .is_some()
    {
        return Err("A recording is already active.".to_string());
    }
    let (output_root, _) = resolve_output_root(&app).map_err(String::from)?;
    let task = run_recording_command(&[
        "recording-init".into(),
        "--output-root".into(),
        output_root.to_string_lossy().into_owned(),
    ])?;
    let task_state = Arc::new(Mutex::new(task.clone()));
    let control = Arc::new(CaptureControl {
        paused: AtomicBool::new(false),
        samples: AtomicU64::new(0),
        checkpoints: AtomicUsize::new(0),
        last_emit_samples: AtomicU64::new(0),
        buffer: Mutex::new(Vec::new()),
    });
    let (command_sender, command_receiver) = mpsc::channel();
    let (ready_sender, ready_receiver) = mpsc::channel();
    let thread_task = task_state.clone();
    let thread_control = control.clone();
    let join = std::thread::spawn(move || {
        capture_thread(
            app,
            device_id,
            thread_task,
            thread_control,
            command_receiver,
            ready_sender,
        )
    });
    let readiness = ready_receiver
        .recv()
        .map_err(|_| "Microphone capture stopped during startup.".to_string())?;
    if let Err(error) = readiness {
        let _ = run_recording_command(&[
            "recording-update".into(),
            "--task-dir".into(),
            task.task_folder.clone(),
            "--status".into(),
            "cancelled".into(),
            "--duration-ms".into(),
            "0".into(),
            "--checkpoint-count".into(),
            "0".into(),
        ]);
        let _ = join.join();
        return Err(error);
    }
    let mut slot = manager
        .session
        .lock()
        .map_err(|_| "Recording state is unavailable.")?;
    if slot.is_some() {
        let _ = command_sender.send(CaptureCommand::Finish(
            "cancelled".to_string(),
            mpsc::channel().0,
        ));
        let _ = join.join();
        return Err("A recording is already active.".to_string());
    }
    *slot = Some(RecordingHandle {
        sender: command_sender,
        control,
        task: task_state,
        join: Some(join),
    });
    Ok(task)
}

#[tauri::command]
pub async fn start_recording(
    app: AppHandle,
    manager: State<'_, RecordingManager>,
    device_id: Option<String>,
) -> Result<RecordingTask, String> {
    let manager = manager.inner().clone();
    tauri::async_runtime::spawn_blocking(move || start_recording_blocking(app, manager, device_id))
        .await
        .map_err(|_| "The recording worker stopped unexpectedly.".to_string())?
}

fn send_control(
    manager: &RecordingManager,
    command: impl FnOnce(mpsc::Sender<Result<RecordingTask, String>>) -> CaptureCommand,
) -> Result<RecordingTask, String> {
    let sender = manager
        .session
        .lock()
        .map_err(|_| "Recording state is unavailable.")?
        .as_ref()
        .map(|handle| handle.sender.clone())
        .ok_or_else(|| "No recording is active.".to_string())?;
    let (reply_sender, reply_receiver) = mpsc::channel();
    sender
        .send(command(reply_sender))
        .map_err(|_| "Recording capture is unavailable.".to_string())?;
    reply_receiver
        .recv()
        .map_err(|_| "Recording capture stopped unexpectedly.".to_string())?
}

#[tauri::command]
pub async fn pause_recording(
    manager: State<'_, RecordingManager>,
) -> Result<RecordingTask, String> {
    let manager = manager.inner().clone();
    tauri::async_runtime::spawn_blocking(move || send_control(&manager, CaptureCommand::Pause))
        .await
        .map_err(|_| "The recording worker stopped unexpectedly.".to_string())?
}

#[tauri::command]
pub async fn resume_recording(
    manager: State<'_, RecordingManager>,
) -> Result<RecordingTask, String> {
    let manager = manager.inner().clone();
    tauri::async_runtime::spawn_blocking(move || send_control(&manager, CaptureCommand::Resume))
        .await
        .map_err(|_| "The recording worker stopped unexpectedly.".to_string())?
}

fn finish(manager: &RecordingManager, status: &str) -> Result<RecordingTask, String> {
    let result = send_control(manager, |reply| {
        CaptureCommand::Finish(status.to_string(), reply)
    });
    if let Ok(mut slot) = manager.session.lock() {
        if let Some(mut handle) = slot.take() {
            if let Some(join) = handle.join.take() {
                let _ = join.join();
            }
        }
    }
    result
}

#[tauri::command]
pub async fn stop_recording(manager: State<'_, RecordingManager>) -> Result<RecordingTask, String> {
    let manager = manager.inner().clone();
    tauri::async_runtime::spawn_blocking(move || finish(&manager, "stopped"))
        .await
        .map_err(|_| "The recording worker stopped unexpectedly.".to_string())?
}

#[tauri::command]
pub async fn cancel_recording(
    manager: State<'_, RecordingManager>,
) -> Result<RecordingTask, String> {
    let manager = manager.inner().clone();
    tauri::async_runtime::spawn_blocking(move || finish(&manager, "cancelled"))
        .await
        .map_err(|_| "The recording worker stopped unexpectedly.".to_string())?
}

#[tauri::command]
pub fn get_recording_state(manager: State<'_, RecordingManager>) -> Result<RecordingState, String> {
    let slot = manager
        .session
        .lock()
        .map_err(|_| "Recording state is unavailable.")?;
    match slot.as_ref() {
        Some(handle) => Ok(RecordingState {
            active: true,
            paused: handle.control.paused.load(Ordering::SeqCst),
            task: handle.task.lock().ok().map(|v| v.clone()),
        }),
        None => Ok(RecordingState {
            active: false,
            paused: false,
            task: None,
        }),
    }
}

#[tauri::command]
fn scan_recordings_blocking(app: AppHandle) -> Result<Vec<RecordingTask>, String> {
    let (root, _) = resolve_output_root(&app).map_err(String::from)?;
    let output = Command::new(sidecar()?)
        .args(["recording-scan", "--output-root"])
        .arg(root)
        .env_remove("MIMO_API_KEY")
        .env_remove("DEEPSEEK_API_KEY")
        .output()
        .map_err(|_| "Could not scan recordings.".to_string())?;
    if !output.status.success() {
        return Err("Could not scan recordings.".to_string());
    }
    serde_json::from_slice(&output.stdout)
        .map_err(|_| "Recording scan returned invalid data.".to_string())
}

#[tauri::command]
pub async fn scan_recordings(app: AppHandle) -> Result<Vec<RecordingTask>, String> {
    tauri::async_runtime::spawn_blocking(move || scan_recordings_blocking(app))
        .await
        .map_err(|_| "The recording scan worker stopped unexpectedly.".to_string())?
}

#[tauri::command]
fn recover_recording_blocking(
    app: AppHandle,
    task_folder: String,
) -> Result<RecordingTask, String> {
    let (root, _) = resolve_output_root(&app).map_err(String::from)?;
    let task = canonicalize_abs_dir(&task_folder, "task_folder").map_err(String::from)?;
    let task = canonicalize_path_within(&root, &task, "Recording task")?;
    let checkpoints = task.join("audio").join("checkpoints");
    let mut files = fs::read_dir(&checkpoints)
        .map_err(|_| "No recording checkpoints are available.".to_string())?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| path.extension().and_then(|e| e.to_str()) == Some("wav"))
        .collect::<Vec<_>>();
    files.sort();
    let first = files
        .first()
        .ok_or_else(|| "No recording checkpoints are available.".to_string())?;
    let reader = hound::WavReader::open(first)
        .map_err(|_| "The first recording checkpoint is damaged.".to_string())?;
    let sample_rate = reader.spec().sample_rate;
    drop(reader);
    let output = task.join("audio").join("recording.wav");
    merge_checkpoints(&checkpoints, &output, sample_rate)?;
    let reader = hound::WavReader::open(&output).map_err(|e| e.to_string())?;
    let duration_ms = u64::from(reader.duration()).saturating_mul(1000) / u64::from(sample_rate);
    run_recording_command(&[
        "recording-update".into(),
        "--task-dir".into(),
        task.to_string_lossy().into_owned(),
        "--status".into(),
        "recovered".into(),
        "--duration-ms".into(),
        duration_ms.to_string(),
        "--checkpoint-count".into(),
        files.len().to_string(),
        "--final-audio".into(),
        output.to_string_lossy().into_owned(),
    ])
}

#[tauri::command]
pub async fn recover_recording(
    app: AppHandle,
    task_folder: String,
) -> Result<RecordingTask, String> {
    tauri::async_runtime::spawn_blocking(move || recover_recording_blocking(app, task_folder))
        .await
        .map_err(|_| "The recording recovery worker stopped unexpectedly.".to_string())?
}

pub fn interrupt_active_recording(app: &AppHandle) {
    if let Some(manager) = app.try_state::<RecordingManager>() {
        let _ = finish(manager.inner(), "interrupted");
    }
}
