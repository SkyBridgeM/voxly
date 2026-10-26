#!/usr/bin/env python3
"""Opt-in local comparison. Does not modify desktop settings or create Tasks."""
import argparse
import difflib
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import wave

REPO = Path(__file__).resolve().parents[1]
CACHE = REPO / ".runtime-build" / "vad-pilot"
BINARY = CACHE / "voxly-sensevoice-wrapper"
VAD = CACHE / "models" / "silero_vad.onnx"
VAD_SHA = "9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6"
VAD_URL = "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx"


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def checked(command):
    subprocess.run([str(arg) for arg in command], check=True)


def prepare():
    # Build a separate executable; never replace the desktop's sidecar.
    source = REPO / "native" / "sensevoice" / "main.cc"
    builder = REPO / "scripts" / "build_sensevoice_runtime.sh"
    if not BINARY.exists() or BINARY.stat().st_mtime < max(source.stat().st_mtime, builder.stat().st_mtime):
        checked([builder, "--output", BINARY])
    VAD.parent.mkdir(parents=True, exist_ok=True)
    if not VAD.exists():
        partial = VAD.with_suffix(".partial")
        checked(["curl", "--fail", "--location", "--retry", "2", VAD_URL, "-o", partial])
        if partial.stat().st_size != 643854 or sha256(partial) != VAD_SHA:
            raise ValueError("VAD download failed size/SHA-256 verification")
        partial.replace(VAD)
    if VAD.stat().st_size != 643854 or sha256(VAD) != VAD_SHA:
        raise ValueError("Cached VAD model failed size/SHA-256 verification")


def verified_model(root):
    # Check model files and receipt against the same catalog as the Go core.
    catalog = json.loads((REPO / "core-go/modelcatalog/models.v1.json").read_text())
    model = next(item for item in catalog["models"] if item["model_id"] == "sensevoice-small-int8")
    directory = root / "models" / model["model_id"] / model["model_version"]
    receipt = json.loads((directory / "receipt.json").read_text())
    expected = {
        "schema_version": "1.0", "model_id": model["model_id"],
        "model_version": model["model_version"], "catalog_revision": catalog["catalog_revision"],
        "archive_sha256": model["archive"]["sha256"], "installed_files": model["installed_files"],
    }
    if any(receipt.get(key) != value for key, value in expected.items()):
        raise ValueError("Installed SenseVoice receipt does not match the catalog")
    for identity in model["installed_files"]:
        path = directory / identity["path"]
        if path.is_symlink() or path.stat().st_size != identity["size_bytes"] or sha256(path) != identity["sha256"]:
            raise ValueError("Installed SenseVoice file verification failed")
    return directory


def recognize(model, audio, vad=False, language="zh"):
    command = [str(BINARY), str(model / "model.int8.onnx"), str(model / "tokens.txt"), str(audio)]
    command += ["--language", language]
    if vad:
        command += ["--vad-model", str(VAD)]
    start = time.monotonic()
    # No cloud credential is inherited by the inference worker.
    result = subprocess.run(command, capture_output=True, text=True, env={}, check=True)
    parsed = json.loads(result.stdout)
    if not isinstance(parsed.get("text"), str):
        raise ValueError("Inference returned an invalid text result")
    return parsed, time.monotonic() - start


def timestamp(ms):
    seconds, millis = divmod(ms, 1000)
    hours, seconds = divmod(seconds, 3600)
    minutes, seconds = divmod(seconds, 60)
    return f"{hours:02}:{minutes:02}:{seconds:02}.{millis:03}"


def main():
    parser = argparse.ArgumentParser(description="对比现有分段与 Silero VAD（本地试验，不改变桌面设置）")
    parser.add_argument("audio", type=Path, help="建议先使用 3–5 分钟的真实会议录音")
    parser.add_argument("--language", choices=["zh", "auto", "en", "yue", "ja", "ko"], default="zh", help="默认中文；多语种录音可显式选择 auto")
    parser.add_argument("--app-data-root", type=Path, default=Path.home() / "Library/Application Support/com.skybridgem.voxly")
    args = parser.parse_args()
    audio = args.audio.resolve(strict=True)
    if not audio.is_file():
        parser.error("audio must be a file")
    prepare()
    model = verified_model(args.app_data_root.expanduser().resolve())
    runs = CACHE / "runs"
    runs.mkdir(parents=True, exist_ok=True)
    output = Path(tempfile.mkdtemp(prefix=time.strftime("%Y%m%d-%H%M%S-"), dir=runs))
    print(f"对照结果目录：{output}", flush=True)
    # The same decoded waveform is used in both modes; no silence compression.
    # Temporary PCM is removed after the run; reports and transcripts remain.
    with tempfile.TemporaryDirectory(prefix="voxly-vad-", dir=CACHE) as temporary:
        wav = Path(temporary) / "audio.wav"
        checked(["ffmpeg", "-nostdin", "-v", "error", "-i", audio, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav])
        with wave.open(str(wav)) as reader:
            duration_ms = reader.getnframes() * 1000 // reader.getframerate()
        print("运行现有分段…", flush=True)
        baseline, baseline_seconds = recognize(model, wav, language=args.language)
        print("运行 VAD 分段…", flush=True)
        vad, vad_seconds = recognize(model, wav, vad=True, language=args.language)
    previous = 0
    for segment in vad["segments"]:
        if not (previous <= segment["start_ms"] < segment["end_ms"] <= duration_ms):
            raise ValueError("Invalid segment timeline")
        if segment["end_ms"] - segment["start_ms"] > 30000:
            raise ValueError("Segment exceeded the inference duration limit")
        previous = segment["end_ms"]
    for name, result in [("baseline", baseline), ("vad", vad)]:
        (output / f"{name}.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        (output / f"{name}.md").write_text(result["text"] + "\n")
    timed_text = "\n\n".join(f'[{timestamp(s["start_ms"])}–{timestamp(s["end_ms"])}] {s["text"]}' for s in vad["segments"])
    (output / "vad-timed.md").write_text(timed_text + "\n")
    covered_ms = sum(s["end_ms"] - s["start_ms"] for s in vad["segments"])
    metrics = {"audio_duration_ms": duration_ms, "vad_covered_ms": covered_ms,
               "vad_segments": len(vad["segments"]), "baseline_seconds": baseline_seconds,
               "vad_seconds": vad_seconds, "vad_sha256": VAD_SHA, "language": args.language}
    (output / "metrics.json").write_text(json.dumps(metrics, indent=2) + "\n")
    differences = "\n".join(difflib.unified_diff(baseline["text"].splitlines(), vad["text"].splitlines(), fromfile="现有分段", tofile="VAD分段", lineterm=""))
    report = f"""# VAD 对照试验

- 录音时长：{duration_ms / 1000:.2f} 秒
- 识别语言：{args.language}（默认 zh；auto 需显式选择）
- 现有分段推理：{baseline_seconds:.2f} 秒
- VAD＋识别：{vad_seconds:.2f} 秒
- VAD 片段：{len(vad['segments'])} 段，覆盖 {covered_ms / 1000:.2f} 秒（含边界余量）
- 两种模式共用同一份 16 kHz 单声道音频；耗时不含首次下载、编译和音频转换。
- 单次顺序运行会受缓存和机器负载影响，耗时仅供参考，不代表稳定加速比例。
- 参数：阈值 0.35，结束静音 0.5 秒，最短讲话 0.1 秒，边界余量 150 毫秒，硬上限 30 秒。
- 时间是片段边界，不是逐字对齐；未接入说话人区分。

## 人工检查

听原音，核对轻声、短回应、句首句尾和文件最后一句。两份文本的差异不等于识别错误率，不能据此自动判断哪份更准确。合成样音通过也不代表真实会议已验证。

## 文件

- baseline.md：原分段全文
- vad.md：VAD 分段全文
- vad-timed.md：VAD 片段时间与文字
- baseline.json、vad.json、metrics.json：结构化底稿

## 文本差异

```diff
{differences}
```
"""
    (output / "comparison.md").write_text(report)
    print(f"完成：{output / 'comparison.md'}")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(130)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"试验未完成：{error}", file=sys.stderr)
        sys.exit(1)
