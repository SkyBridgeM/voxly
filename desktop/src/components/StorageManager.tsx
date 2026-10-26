import { useEffect, useRef, useState } from "react";
import { cleanCacheBatch, getStorageSettings, getStorageSummary, previewCacheBatch, runStorageMaintenance, setStorageSettings, type BatchCacheResult, type StorageSettings, type StorageSummary, type TemporaryCleanup } from "../lib/meetingAsr";
import { cacheReasonLabel, type CacheLabels } from "./CacheCleanupPanel";

export interface StorageLabels extends CacheLabels {
  storageTitle: string;
  storageDescription: string;
  storageTotal: string;
  storageSource: string;
  storageChunks: string;
  storageTranscripts: string;
  storageGenerated: string;
  storageModels: string;
  storageTemporary: string;
  storageOther: string;
  storageRefresh: string;
  storageEstimate: string;
  storageIssues: string;
  storageAutoChunks: string;
  storageAutoChunksHelp: string;
  storageAutoTemporary: string;
  storageAutoTemporaryHelp: string;
  storageCleanTemporary: string;
  storageTemporaryResult: string;
  storageBatchPreview: string;
  storageBatchConfirm: string;
  storageBatchHelp: string;
  storageSelectAll: string;
  storageTask: string;
  storageSize: string;
  storageCleanedCount: string;
  storageSkippedCount: string;
  storageFailedCount: string;
  storageNoTasks: string;
  storageSaved: string;
}

export function StorageManager({ locale, labels, disabled, onBusyChange, onChanged, startupResult, startupFailed }: {
  locale: "zh" | "en";
  labels: StorageLabels;
  disabled: boolean;
  onBusyChange: (busy: boolean) => void;
  onChanged: () => void;
  startupResult: TemporaryCleanup | null;
  startupFailed: boolean;
}) {
  const [summary, setSummary] = useState<StorageSummary | null>(null);
  const [settings, setSettings] = useState<StorageSettings | null>(null);
  const [preview, setPreview] = useState<BatchCacheResult | null>(null);
  const [result, setResult] = useState<BatchCacheResult | null>(null);
  const [selected, setSelected] = useState(new Set<string>());
  const [temporary, setTemporary] = useState(startupResult);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [saved, setSaved] = useState(false);
  useEffect(() => { if (startupResult) setTemporary(startupResult); }, [startupResult]);
  useEffect(() => { if (startupFailed) setFailed(true); }, [startupFailed]);
  const inFlight = useRef(false);
  const mounted = useRef(false);
  const format = (n: number) => {
    const unit = n >= 1024 ** 3 ? [1024 ** 3, "GiB"] as const : n >= 1024 ** 2 ? [1024 ** 2, "MiB"] as const : n >= 1024 ? [1024, "KiB"] as const : [1, "B"] as const;
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(n / unit[0])} ${unit[1]}`;
  };
  useEffect(() => {
    mounted.current = true;
    void Promise.all([getStorageSettings(), getStorageSummary()]).then(([nextSettings, nextSummary]) => {
      if (mounted.current) { setSettings(nextSettings); setSummary(nextSummary); }
    }).catch(() => { if (mounted.current) setFailed(true); });
    return () => { mounted.current = false; };
  }, []);

  const run = async (action: () => Promise<void>) => {
    if (disabled || inFlight.current) return;
    inFlight.current = true; setBusy(true); setFailed(false); setSaved(false); onBusyChange(true);
    try { await action(); }
    catch { if (mounted.current) { setFailed(true); setPreview(null); } }
    finally { inFlight.current = false; if (mounted.current) setBusy(false); onBusyChange(false); }
  };
  const refresh = async () => {
    const [nextSummary, nextSettings] = await Promise.all([getStorageSummary(), getStorageSettings()]);
    if (mounted.current) { setSummary(nextSummary); setSettings(nextSettings); }
  };
  const changeSetting = (next: StorageSettings) => void run(async () => {
    const previous = settings;
    setSettings(next);
    try {
      const stored = await setStorageSettings(next);
      if (mounted.current) { setSettings(stored); setSaved(true); }
    } catch (error) {
      if (mounted.current) setSettings(previous);
      throw error;
    }
  });
  const inspectBatch = () => void run(async () => {
    const next = await previewCacheBatch();
    if (mounted.current) { setPreview(next); setSelected(new Set(next.tasks.filter((task) => task.preview.can_clean).map((task) => task.folder))); setResult(null); }
  });
  const cleanSelected = () => void run(async () => {
    if (!preview) return;
    const next = await cleanCacheBatch({ output_root: preview.output_root, tasks: preview.tasks
      .filter((task) => selected.has(task.folder) && task.preview.can_clean && task.preview.token)
      .map((task) => ({ folder: task.folder, token: task.preview.token! })) });
    if (mounted.current) { setResult({ ...next, tasks: next.tasks.map((task) => ({ ...task, title: preview.tasks.find((original) => original.folder === task.folder)?.title ?? task.folder })) }); setPreview(null); setSelected(new Set()); }
    onChanged(); await refresh();
  });
  const eligible = preview?.tasks.filter((task) => task.preview.can_clean) ?? [];
  const selectedBytes = eligible.filter((task) => selected.has(task.folder)).reduce((sum, task) => sum + task.preview.total_bytes, 0);
  const categories: [string, string][] = [["source_audio", labels.storageSource], ["chunk_audio", labels.storageChunks], ["transcripts", labels.storageTranscripts], ["generated", labels.storageGenerated], ["models", labels.storageModels], ["temporary", labels.storageTemporary]];
  return <section className="settings-section storage-manager" aria-labelledby="storage-title">
    <div className="settings-section-header"><div className="settings-section-title"><span>08</span><div><h3 id="storage-title">{labels.storageTitle}</h3><p>{labels.storageDescription}</p></div></div></div>
    <div className="storage-actions"><button type="button" className="btn-secondary" disabled={disabled || busy} onClick={() => void run(async () => { setPreview(null); await refresh(); })}>{busy ? labels.cacheWorking : labels.storageRefresh}</button></div>
    {summary && <>
      <strong>{labels.storageTotal}: {format(summary.total_bytes)}</strong>
      <dl className="storage-sizes">{categories.map(([key, label]) => <div key={key}><dt>{label}</dt><dd>{format(summary.categories[key] ?? 0)}</dd></div>)}<div><dt>{labels.storageOther}</dt><dd>{format((summary.categories.other ?? 0) + (summary.categories.metadata ?? 0))}</dd></div></dl>
      <p className="storage-help">{labels.storageEstimate}</p>
      {summary.issue_count > 0 && <p className="storage-help" role="status">{labels.storageIssues}: {summary.issue_count}</p>}
      <div className="storage-table-wrap"><table><thead><tr><th>{labels.storageTask}</th><th>{labels.storageSize}</th><th>{labels.storageChunks}</th></tr></thead><tbody>{summary.tasks.map((task) => <tr key={task.folder}><td>{task.title}</td><td>{format(task.total_bytes)}</td><td>{format(task.chunk_bytes)}</td></tr>)}</tbody></table></div>
    </>}
    {settings && <div className="storage-preferences">
      <label><input type="checkbox" checked={settings.auto_clean_chunks} disabled={disabled || busy} onChange={(event) => changeSetting({ ...settings, auto_clean_chunks: event.target.checked })} />{labels.storageAutoChunks}</label><p className="storage-help">{labels.storageAutoChunksHelp}</p>
      <label><input type="checkbox" checked={settings.cleanup_temporary_on_startup} disabled={disabled || busy} onChange={(event) => changeSetting({ ...settings, cleanup_temporary_on_startup: event.target.checked })} />{labels.storageAutoTemporary}</label><p className="storage-help">{labels.storageAutoTemporaryHelp}</p>
      <button type="button" className="btn-secondary" disabled={disabled || busy} onClick={() => void run(async () => { const next = await runStorageMaintenance(); if (mounted.current) setTemporary(next); await refresh(); })}>{labels.storageCleanTemporary}</button>
    </div>}
    {temporary && <p role="status">{labels.storageTemporaryResult}: {temporary.removed_count} · {format(temporary.removed_bytes)}</p>}
    <div className="storage-batch"><h4>{labels.storageBatchPreview}</h4><p className="storage-help">{labels.storageBatchHelp}</p>
      <button type="button" className="btn-secondary" disabled={disabled || busy} onClick={inspectBatch}>{labels.storageBatchPreview}</button>
      {preview && <>
        {preview.tasks.length === 0 && <p>{labels.storageNoTasks}</p>}
        {eligible.length > 0 && <label className="storage-select-all"><input type="checkbox" disabled={disabled || busy} checked={eligible.every((task) => selected.has(task.folder))} onChange={(event) => setSelected(event.target.checked ? new Set(eligible.map((task) => task.folder)) : new Set())} />{labels.storageSelectAll}</label>}
        <div className="storage-batch-list">{preview.tasks.map((task) => <div key={task.folder} className="storage-batch-task">
          <label><input type="checkbox" checked={selected.has(task.folder)} disabled={disabled || busy || !task.preview.can_clean} onChange={(event) => { const checked = event.target.checked; setSelected((previous) => { const next = new Set(previous); if (checked) next.add(task.folder); else next.delete(task.folder); return next; }); }} />{task.title}</label>
          {task.preview.can_clean ? <details><summary>{format(task.preview.total_bytes)} · {task.preview.files.length}</summary><ul>{task.preview.files.map((file) => <li key={file.reference}>{file.reference} · {format(file.size_bytes)}</li>)}</ul></details> : <p>{cacheReasonLabel(task.preview.reason ?? "operation_failed", labels)}</p>}
        </div>)}</div>
        {eligible.length > 0 && <button type="button" className="btn-primary" disabled={disabled || busy || selected.size === 0} onClick={cleanSelected}>{labels.storageBatchConfirm} ({selected.size}) · {format(selectedBytes)}</button>}
      </>}
      {result && <div role="status"><p>{labels.storageCleanedCount}: {result.cleaned_count} · {labels.storageSkippedCount}: {result.skipped_count} · {labels.storageFailedCount}: {result.failed_count} · {format(result.removed_bytes)}</p>{result.tasks.filter((task) => task.preview.reason).map((task) => <p key={task.folder}>{task.title || task.folder}: {cacheReasonLabel(task.preview.reason!, labels)}</p>)}</div>}
    </div>
    <div role="status" aria-live="polite">{failed && <p className="library-artifact-error">{labels.cacheFailed}</p>}{saved && <p>{labels.storageSaved}</p>}</div>
  </section>;
}
