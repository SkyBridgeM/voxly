import { useRef, useState } from "react";
import { cleanTaskCache, previewTaskCache, type CachePreview } from "../lib/meetingAsr";

export interface CacheLabels {
  cacheScope: string;
  cacheChunksOnly: string;
  cacheTextOnly: string;
  cacheTextOnlyDescription: string;
  cacheTextOnlyConfirm: string;
  cacheTextOnlyCleaned: string;
  cacheNoAudio: string;

  cacheCleanup: string;
  cachePreview: string;
  cacheDescription: string;
  cacheConfirm: string;
  cacheWorking: string;
  cacheReclaimable: string;
  cacheCleaned: string;
  cacheNoFiles: string;
  cacheNotCompleted: string;
  cacheBusy: string;
  cacheSourceUnavailable: string;
  cacheTranscriptUnverified: string;
  cacheUnsafe: string;
  cacheChanged: string;
  cacheFailed: string;
}

export function cacheReasonLabel(reason: string, labels: CacheLabels) {
  const reasons: Record<string, string> = {
    no_cache: labels.cacheNoFiles,
    task_not_completed: labels.cacheNotCompleted,
    task_busy: labels.cacheBusy,
    source_unavailable: labels.cacheSourceUnavailable,
    transcript_unverified: labels.cacheTranscriptUnverified,
    unsafe_cache: labels.cacheUnsafe,
    preview_changed: labels.cacheChanged,
  };
  return reasons[reason] ?? labels.cacheFailed;
}

export function CacheCleanupPanel({ taskFolder, labels, locale, disabled, onBusyChange, onCleaned }: {
  taskFolder: string;
  labels: CacheLabels;
  locale: "zh" | "en";
  disabled: boolean;
  onBusyChange: (busy: boolean) => void;
  onCleaned: () => Promise<void>;
}) {
  const [textOnly, setTextOnly] = useState(false);
  const [preview, setPreview] = useState<CachePreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const inFlight = useRef(false);
  const bytes = (value: number) => `${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(value / 1024 / 1024)} MiB`;
  const run = async (clean: boolean) => {
    if (disabled || inFlight.current || (clean && !preview?.token)) return;
    inFlight.current = true;
    setBusy(true);
    setFailed(false);
    onBusyChange(true);
    try {
      const result = clean
        ? await cleanTaskCache(taskFolder, preview!.token!, textOnly)
        : await previewTaskCache(taskFolder, textOnly);
      setPreview(result);
      if (clean) await onCleaned();
    } catch {
      setPreview(null); // A failed operation must never leave an executable stale preview.
      setFailed(true);
    } finally {
      inFlight.current = false;
      setBusy(false);
      onBusyChange(false);
    }
  };
  const cleaned = !preview?.reason && preview?.cache_status === "cleaned" && !preview.can_clean && (!preview.reason || preview.reason === "no_cache");
  return <details className="library-details-section library-cache">
    <summary>{labels.cacheCleanup}</summary>
    <div className="form-group form-group-compact cache-scope">
      <label htmlFor={`cache-scope-${taskFolder}`}>{labels.cacheScope}</label>
      <select id={`cache-scope-${taskFolder}`} value={textOnly ? "text_only" : "chunks"} disabled={disabled || busy} onChange={(event) => { setTextOnly(event.target.value === "text_only"); setPreview(null); setFailed(false); }}>
        <option value="chunks">{labels.cacheChunksOnly}</option>
        <option value="text_only">{labels.cacheTextOnly}</option>
      </select>
    </div>
    <p>{textOnly ? labels.cacheTextOnlyDescription : labels.cacheDescription}</p>
    <button type="button" className="btn-secondary" disabled={disabled || busy} onClick={() => void run(false)}>{busy ? labels.cacheWorking : labels.cachePreview}</button>
    <div role="status" aria-live="polite" aria-busy={busy}>
      {failed && <p className="library-artifact-error">{labels.cacheFailed}</p>}
      {cleaned && <p>{textOnly ? labels.cacheTextOnlyCleaned : labels.cacheCleaned}{preview.removed_bytes > 0 ? ` · ${bytes(preview.removed_bytes)}` : ""}</p>}
      {preview?.reason && !cleaned && <p>{textOnly && preview.reason === "no_cache" ? labels.cacheNoAudio : cacheReasonLabel(preview.reason, labels)}</p>}
      {preview?.can_clean && <>
        <p>{labels.cacheReclaimable}: {bytes(preview.total_bytes)}</p>
        <ul>{preview.files.map((file) => <li key={file.reference}>{file.reference} · {bytes(file.size_bytes)}</li>)}</ul>
        <button type="button" className="btn-primary" disabled={disabled || busy} onClick={() => void run(true)}>{textOnly ? labels.cacheTextOnlyConfirm : labels.cacheConfirm}</button>
      </>}
    </div>
  </details>;
}
