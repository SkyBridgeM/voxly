#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
runtime_output="$repo_root/core-go/voxly-sensevoice-wrapper"
if [[ $# -gt 0 ]]; then
  [[ $# -eq 2 && "$1" == --output && "$2" == /* ]] || { echo 'Usage: build_sensevoice_runtime.sh [--output ABSOLUTE_PATH]' >&2; exit 2; }
  runtime_output="$2"
fi
version=1.13.8
[[ "$(uname -s)" == Darwin ]] || { echo 'SenseVoice runtime build currently supports macOS only.' >&2; exit 1; }
case "$(uname -m)" in
  arm64) arch=arm64; triple=aarch64-apple-darwin; sha=3d7f9b8a496694af13d9802c33b8133231e397bdef302f543d19468765e83136 ;;
  x86_64) arch=x64; triple=x86_64-apple-darwin; sha=8ffc3ede9f997fec547b5c99b8e2073b8e04fd48a5f65e1a6d5d314ad0106ad9 ;;
  *) echo 'Unsupported architecture.' >&2; exit 1 ;;
esac
name="sherpa-onnx-v$version-osx-$arch-static-no-tts-lib"
cache="$repo_root/.runtime-build/sherpa-onnx-v$version"
mkdir -p "$cache" "$(dirname "$runtime_output")"
archive="$cache/$name.tar.bz2"
if [[ ! -f "$archive" ]]; then
  curl --fail --location --retry 3 "https://github.com/k2-fsa/sherpa-onnx/releases/download/v$version/$name.tar.bz2" -o "$archive.partial"
  mv "$archive.partial" "$archive"
fi
actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
[[ "$actual" == "$sha" ]] || { echo 'Runtime archive checksum mismatch.' >&2; exit 1; }
tar -xjf "$archive" -C "$cache"
sdk="$cache/$name"
header_dir="$cache/include/sherpa-onnx/c-api"
mkdir -p "$header_dir"
header="$header_dir/c-api.h"
if [[ ! -f "$header" ]]; then
  curl --fail --location --retry 3 "https://raw.githubusercontent.com/k2-fsa/sherpa-onnx/v$version/sherpa-onnx/c-api/c-api.h" -o "$header"
fi
header_sha="$(shasum -a 256 "$header" | awk '{print $1}')"
[[ "$header_sha" == 2a1b95084be8fd1deb3228fcad2fd3f7f0258b64582f7402281ec174c7b7f4ce ]] || { echo 'Runtime header checksum mismatch.' >&2; exit 1; }
# All sherpa/ONNX dependencies are static; only macOS system frameworks remain.
xcrun clang++ -std=c++17 -O2 -I "$cache/include" \
  "$repo_root/native/sensevoice/main.cc" "$sdk"/lib/*.a \
  -framework Accelerate -framework Foundation -framework CoreML \
  -o "$runtime_output"
"$runtime_output" --version
if [[ "$runtime_output" == "$repo_root/core-go/voxly-sensevoice-wrapper" ]]; then
  mkdir -p "$repo_root/desktop/src-tauri/binaries"
  cp "$runtime_output" "$repo_root/desktop/src-tauri/binaries/voxly-sensevoice-wrapper-$triple"
  vad="$cache/silero_vad.onnx"
  if [[ ! -f "$vad" ]]; then
    curl --fail --location --retry 3 "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx" -o "$vad.partial"
    mv "$vad.partial" "$vad"
  fi
  vad_sha="$(shasum -a 256 "$vad" | awk '{print $1}')"
  [[ "$vad_sha" == 9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6 ]] || { echo 'VAD model checksum mismatch.' >&2; exit 1; }
  cp "$vad" "$repo_root/core-go/silero_vad.onnx"
  cp "$vad" "$repo_root/desktop/src-tauri/binaries/silero_vad.onnx"
fi
echo 'Built local SenseVoice runtime.'
