import { useEffect, useMemo, useState } from "react";
import {
  cancelRecording,
  getRecordingState,
  listInputDevices,
  listenRecordingError,
  listenRecordingProgress,
  pauseRecording,
  resumeRecording,
  startRecording,
  stopRecording,
  type InputDeviceInfo,
  type RecordingTask,
} from "../lib/meetingAsr";

interface Props {
  locale: "zh" | "en";
  disabled: boolean;
  onSaved: (task: RecordingTask) => void;
  onError: (message: string) => void;
}

function formatDuration(ms: number) {
  const total = Math.floor(ms / 1000);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  return [hours, minutes, seconds].map((value) => value.toString().padStart(2, "0")).join(":");
}

export function RecordingWorkspace({ locale, disabled, onSaved, onError }: Props) {
  const zh = locale === "zh";
  const [devices, setDevices] = useState<InputDeviceInfo[]>([]);
  const [deviceId, setDeviceId] = useState("");
  const [active, setActive] = useState(false);
  const [paused, setPaused] = useState(false);
  const [duration, setDuration] = useState(0);
  const [level, setLevel] = useState(0);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    let mounted = true;
    void Promise.all([listInputDevices(), getRecordingState()]).then(([available, state]) => {
      if (!mounted) return;
      setDevices(available);
      setDeviceId(available.find((device) => device.is_default)?.id ?? available[0]?.id ?? "");
      setActive(state.active);
      setPaused(state.paused);
      setDuration(state.task?.duration_ms ?? 0);
    }).catch((error) => onError(String(error)));
    const cleanups = Promise.all([
      listenRecordingProgress((progress) => {
        if (!mounted) return;
        setDuration(progress.duration_ms);
        setLevel(progress.level);
      }),
      listenRecordingError((message) => {
        if (mounted) onError(message);
      }),
    ]);
    return () => { mounted = false; void cleanups.then((items) => items.forEach((cleanup) => cleanup())); };
  }, [onError]);

  const selectedName = useMemo(() => devices.find((device) => device.id === deviceId)?.name, [devices, deviceId]);
  const run = async (operation: () => Promise<unknown>) => {
    setPending(true);
    try { await operation(); } catch (error) { onError(String(error)); } finally { setPending(false); }
  };

  const begin = () => run(async () => {
    await startRecording(deviceId || undefined);
    setActive(true); setPaused(false); setDuration(0); setLevel(0);
  });
  const togglePause = () => run(async () => {
    if (paused) await resumeRecording(); else await pauseRecording();
    setPaused(!paused);
  });
  const stop = () => run(async () => {
    const task = await stopRecording();
    setActive(false); setPaused(false); setLevel(0); onSaved(task);
  });
  const cancel = () => {
    if (!window.confirm(zh ? "停止录音并保留已经录制的音频？" : "Stop recording and keep the captured audio?")) return;
    void run(async () => {
      const task = await cancelRecording();
      setActive(false); setPaused(false); setLevel(0); onSaved(task);
    });
  };

  return (
    <section className={`recording-workspace ${active ? "is-active" : ""}`}>
      {!active ? <>
        <div className="recording-device-row">
          <label htmlFor="recording-device">{zh ? "麦克风" : "Microphone"}</label>
          <select id="recording-device" value={deviceId} onChange={(event) => setDeviceId(event.target.value)} disabled={disabled || pending}>
            {devices.map((device) => <option key={device.id} value={device.id}>{device.name}{device.is_default ? (zh ? "（默认）" : " (Default)") : ""}</option>)}
          </select>
        </div>
        <button className="btn-primary recording-start" onClick={begin} disabled={disabled || pending || !deviceId}>
          <span className="record-dot" aria-hidden="true"></span>{zh ? "开始录音" : "Start recording"}
        </button>
        {devices.length === 0 && <p className="recording-help">{zh ? "未发现可用麦克风，请检查系统权限。" : "No microphone found. Check system permissions."}</p>}
      </> : <>
        <div className="recording-live" role="status" aria-live="polite">
          <span className={`record-dot ${paused ? "paused" : "pulse"}`} aria-hidden="true"></span>
          <div><strong>{paused ? (zh ? "录音已暂停" : "Recording paused") : (zh ? "正在录音" : "Recording")}</strong><small>{selectedName}</small></div>
          <time>{formatDuration(duration)}</time>
        </div>
        <div className="level-meter" aria-label={zh ? "输入音量" : "Input level"}><span style={{ width: `${Math.max(2, level * 100)}%` }}></span></div>
        <div className="recording-actions">
          <button className="btn-secondary" onClick={togglePause} disabled={pending}>{paused ? (zh ? "继续" : "Resume") : (zh ? "暂停" : "Pause")}</button>
          <button className="btn-primary" onClick={stop} disabled={pending}>{zh ? "停止并保存" : "Stop and save"}</button>
          <button className="link-button danger" onClick={cancel} disabled={pending}>{zh ? "取消" : "Cancel"}</button>
        </div>
        <p className="recording-help">{zh ? "录音每 30 秒安全保存一次；停止后由你确认是否处理。" : "Audio is safely checkpointed every 30 seconds. You choose whether to process it after stopping."}</p>
      </>}
    </section>
  );
}
