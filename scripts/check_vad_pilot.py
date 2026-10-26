#!/usr/bin/env python3
"""Focused native smoke checks; requires the official Chinese sample WAV."""
import argparse
import array
import json
from pathlib import Path
import subprocess
import tempfile
import wave

from vad_pilot import BINARY, CACHE, REPO, VAD, prepare, recognize, verified_model


def write_wave(path, samples):
    with wave.open(str(path), "wb") as writer:
        writer.setnchannels(1)
        writer.setsampwidth(2)
        writer.setframerate(16000)
        writer.writeframes(samples.tobytes())


def check_timeline(result, total_samples):
    previous = 0
    for segment in result["segments"]:
        assert previous <= segment["start_ms"] < segment["end_ms"] <= total_samples * 1000 // 16000
        assert segment["end_ms"] - segment["start_ms"] <= 30000
        previous = segment["end_ms"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("audio", type=Path, help="official test_wavs/zh.wav")
    parser.add_argument("--app-data-root", type=Path, default=Path.home() / "Library/Application Support/com.skybridgem.voxly")
    args = parser.parse_args()
    prepare()
    model = verified_model(args.app_data_root.resolve())
    original = json.loads(subprocess.check_output([
        str(REPO / "core-go/voxly-sensevoice-wrapper"),
        str(model / "model.int8.onnx"), str(model / "tokens.txt"), str(args.audio.resolve()),
    ], env={}))
    baseline, _ = recognize(model, args.audio.resolve(), language="auto")
    assert baseline == original, "The explicit auto mode changed the existing sample output"
    implicit = json.loads(subprocess.check_output([
        str(BINARY), str(model / "model.int8.onnx"), str(model / "tokens.txt"), str(args.audio.resolve()),
    ], env={}))
    chinese, _ = recognize(model, args.audio.resolve(), language="zh")
    assert implicit == chinese, "The default must match explicit Chinese"
    print("PASS: explicit auto is retained; default matches explicit Chinese")
    with wave.open(str(args.audio)) as reader:
        assert (reader.getnchannels(), reader.getsampwidth(), reader.getframerate()) == (1, 2, 16000)
        samples = array.array("h", reader.readframes(reader.getnframes()))
    # Trim quiet sample edges; ensure the final phrase is at EOF with no trailing
    # half-second silence, so only the EOF flush can finish that VAD segment.
    active = [i for i, value in enumerate(samples) if abs(value) > 500]
    speech = samples[max(0, active[0] - 320):min(len(samples), active[-1] + 321)]
    with tempfile.TemporaryDirectory(prefix="checks-", dir=CACHE) as temporary:
        root = Path(temporary)
        silence = array.array("h", [0]) * (2 * 16000 + 123)
        silence_path = root / "silence.wav"
        write_wave(silence_path, silence)
        silent, _ = recognize(model, silence_path, vad=True)
        assert silent["segments"] == [] and silent["text"] == "", "Silence should not produce transcription"
        print("PASS: pure silence (including partial final frame) emits no speech")
        combined = array.array("h", [0]) * 32000 + speech + array.array("h", [0]) * 32000 + speech
        # Make the file end deliberately unaligned to a 512-sample frame.
        combined += array.array("h", [0]) * ((123 - len(combined)) % 512)
        combined_path = root / "offset-and-tail.wav"
        write_wave(combined_path, combined)
        result, _ = recognize(model, combined_path, vad=True)
        check_timeline(result, len(combined))
        assert len(result["segments"]) >= 2, "Separated speech should retain distinct intervals"
        assert result["segments"][0]["start_ms"] >= 1500, "Leading silence was collapsed in timestamps"
        assert result["segments"][-1]["end_ms"] >= len(combined) * 1000 // 16000 - 500, "EOF speech was dropped"
        assert result["segments"][-1]["text"].strip(), "EOF speech has no transcript"
        print("PASS: original time offsets, separated speech, and EOF flush")
        missing = subprocess.run([str(BINARY), str(model / "model.int8.onnx"), str(model / "tokens.txt"), str(combined_path), "--vad-model", str(root / "missing.onnx")], capture_output=True, env={})
        assert missing.returncode != 0 and not missing.stdout.strip(), "Missing VAD must fail, without a baseline fallback"
        print("PASS: missing VAD fails explicitly without fallback")
    print("4 focused checks passed; real meeting accuracy is still unverified.")


if __name__ == "__main__":
    main()
