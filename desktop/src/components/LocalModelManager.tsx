import { useEffect, useState } from "react";
import { open } from "@tauri-apps/plugin-dialog";
import {
  cancelModelDownload, deleteLocalModel, getLocalModelCatalog, getLocalModelStatus,
  getModelDownloadSettings, importLocalModel, listenModelProgress, startModelDownload,
  testHuggingFaceEndpoint, updateModelDownloadSettings,
  type LocalModelStatus, type ManagedModel, type ModelProgressEvent,
} from "../lib/meetingAsr";
import type { Locale } from "../i18n";

interface Labels {
  title: string; description: string; chineseFirst: string; status: string; notInstalled: string;
  runtimeMissing: string; invalid: string; unsupported: string; download: string; import: string;
  cancel: string; delete: string; source: string; size: string; advanced: string; endpoint: string;
  save: string; test: string; endpointReachable: string; operationFailed: string; confirmDelete: string;
  privacyTitle: string; privacyBody: string; technicalDetails: string; endpointScope: string;
}

export function LocalModelManager({ locale, disabled, labels, onStatusChanged }: { locale: Locale; disabled: boolean; labels: Labels; onStatusChanged: () => void }) {
  const [model, setModel] = useState<ManagedModel | null>(null);
  const [status, setStatus] = useState<LocalModelStatus | null>(null);
  const [progress, setProgress] = useState<ModelProgressEvent | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [endpoint, setEndpoint] = useState("https://huggingface.co");
  useEffect(() => {
    let active = true; let unlisten: (() => void) | undefined;
    void Promise.all([getLocalModelCatalog(), getLocalModelStatus(), getModelDownloadSettings()]).then(([catalog, state, settings]) => {
      if (!active) return; setModel(catalog.models[0] ?? null); setStatus(state); setEndpoint(settings.huggingface_endpoint);
    }).catch(() => active && setError(labels.operationFailed));
    void listenModelProgress((event) => active && setProgress(event)).then((stop) => { if (active) unlisten = stop; else stop(); });
    return () => { active = false; unlisten?.(); };
  }, [labels.operationFailed]);
  const run = async (operation: () => Promise<LocalModelStatus>) => {
    if (pending) return; setPending(true); setError(null); setProgress(null);
    try { const next = await operation(); setStatus(next); onStatusChanged(); } catch { setError(labels.operationFailed); }
    finally { setPending(false); }
  };
  const importArchive = async () => {
    const selected = await open({ multiple: false, directory: false, filters: [{ name: "Official SenseVoice archive", extensions: ["bz2"] }] });
    if (typeof selected === "string") await run(() => importLocalModel(selected));
  };
  const statusLabel = status?.state === "not_installed" ? labels.notInstalled : status?.state === "runtime_missing" ? labels.runtimeMissing : status?.state === "model_invalid" ? labels.invalid : status?.state === "unsupported_platform" ? labels.unsupported : status?.message ?? labels.notInstalled;
  const percent = progress && progress.total > 0 ? Math.min(100, Math.round(progress.completed / progress.total * 100)) : 0;
  return <>
  <div className="local-model-card">
    <div className="local-model-heading"><div><h4>{labels.title}</h4><p>{labels.description}</p></div><span className="privacy-chip">{labels.chineseFirst}</span></div>
    <strong>{model?.display_name[locale] ?? "SenseVoiceSmall INT8"}</strong>
    <dl className="local-model-facts"><div><dt>{labels.status}</dt><dd>{statusLabel}</dd></div><div><dt>{labels.size}</dt><dd>≈ 156 MB</dd></div><div><dt>{labels.source}</dt><dd>k2-fsa / sherpa-onnx · 2024-07-17</dd></div></dl>
    {pending && <div className="model-progress" role="status"><div style={{ width: `${percent}%` }} /><span>{progress?.state ?? labels.status} {percent > 0 ? `${percent}%` : ""}</span></div>}
    <p className="local-model-privacy"><strong>{labels.privacyTitle}</strong> {labels.privacyBody}</p>
    <div className="local-model-actions">
      {status?.state === "not_installed" || status?.state === "model_invalid" ? <><button className="btn-primary" disabled={disabled || pending} onClick={() => void run(startModelDownload)}>{labels.download}</button><button className="btn-secondary" disabled={disabled || pending} onClick={() => void importArchive()}>{labels.import}</button></> : null}
      {pending && <button className="btn-secondary" onClick={() => void cancelModelDownload()}>{labels.cancel}</button>}
      {(status?.state === "runtime_missing" || status?.state === "model_invalid") && !pending && <button className="btn-secondary" disabled={disabled} onClick={() => { if (confirm(labels.confirmDelete)) void run(deleteLocalModel); }}>{labels.delete}</button>}
    </div>
    <details><summary>{labels.technicalDetails}</summary><code>{model?.archive.sha256}</code></details>
    {error && <p className="provider-not-ready" role="status">{error}</p>}
  </div>
  <div className="model-network-settings-card">
    <h4>{labels.advanced}</h4><p>{labels.endpointScope}</p>
    <div className="form-group form-group-compact"><label htmlFor="hf-endpoint">{labels.endpoint}</label><input id="hf-endpoint" value={endpoint} onChange={(event) => setEndpoint(event.target.value)} /><div className="form-action-row"><button className="btn-secondary" onClick={() => void testHuggingFaceEndpoint(endpoint).then((result) => setError(result.reachable ? labels.endpointReachable : labels.operationFailed)).catch(() => setError(labels.operationFailed))}>{labels.test}</button><button className="btn-primary" onClick={() => void updateModelDownloadSettings(endpoint).then((result) => setEndpoint(result.huggingface_endpoint)).catch(() => setError(labels.operationFailed))}>{labels.save}</button></div></div>
  </div>
  </>;
}
