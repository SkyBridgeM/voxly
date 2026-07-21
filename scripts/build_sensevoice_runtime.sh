#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REVISION="142807252687d81b40d6315f23470a1512a00de3"
TAG="v1.13.4"
SOURCE="${VOXLY_SHERPA_SOURCE:-$ROOT/.runtime-build/sherpa-onnx-$TAG}"
BUILD="${VOXLY_SHERPA_BUILD:-$ROOT/.runtime-build/build-sensevoice}"
TARGET="${VOXLY_RUNTIME_TARGET:-}"

if [[ -z "$TARGET" ]]; then
  case "$(uname -s)-$(uname -m)" in
    Darwin-arm64) TARGET="aarch64-apple-darwin" ;;
    MINGW*-x86_64|MSYS*-x86_64) TARGET="x86_64-pc-windows-msvc" ;;
    *) echo "Unsupported runtime build host; set VOXLY_RUNTIME_TARGET explicitly." >&2; exit 2 ;;
  esac
fi

if [[ ! -d "$SOURCE/.git" ]]; then
  mkdir -p "$(dirname "$SOURCE")"
  git clone --filter=blob:none --no-checkout https://github.com/k2-fsa/sherpa-onnx.git "$SOURCE"
fi
git -C "$SOURCE" fetch --depth 1 origin "$REVISION"
git -C "$SOURCE" checkout --detach "$REVISION"
test "$(git -C "$SOURCE" rev-parse HEAD)" = "$REVISION"
test "$(git -C "$SOURCE" describe --tags --exact-match)" = "$TAG"

SUFFIX=""
[[ "$TARGET" == x86_64-pc-windows-msvc ]] && SUFFIX=".exe"
BUILT="$BUILD/voxly-sensevoice-wrapper$SUFFIX"

if command -v cmake >/dev/null 2>&1; then
  cmake -S "$ROOT/runtime/sensevoice" -B "$BUILD" \
    -DSHERPA_ONNX_SOURCE_DIR="$SOURCE" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_OSX_ARCHITECTURES="${CMAKE_OSX_ARCHITECTURES:-arm64}" \
    -DCMAKE_OSX_DEPLOYMENT_TARGET="${CMAKE_OSX_DEPLOYMENT_TARGET:-12.0}"
  cmake --build "$BUILD" --config Release --target voxly-sensevoice-wrapper --parallel
  [[ -f "$BUILT" ]] || BUILT="$BUILD/Release/voxly-sensevoice-wrapper$SUFFIX"
elif [[ "$TARGET" == aarch64-apple-darwin ]]; then
  SDK_ARCHIVE="$BUILD/sherpa-sdk.tar.bz2"
  JSON_ARCHIVE="$BUILD/json-3.12.0.tar.gz"
  mkdir -p "$BUILD/sdk" "$BUILD/json"
  if [[ ! -f "$SDK_ARCHIVE" ]] || [[ "$(shasum -a 256 "$SDK_ARCHIVE" | awk '{print $1}')" != "c003242369046d3c2adc6b48c3c96e0ff129e76738b7f3aa5342828ec8ba410d" ]]; then
    curl -L --fail --output "$SDK_ARCHIVE" \
      https://github.com/k2-fsa/sherpa-onnx/releases/download/v1.13.4/sherpa-onnx-v1.13.4-osx-arm64-shared-no-tts-lib.tar.bz2
  fi
  test "$(shasum -a 256 "$SDK_ARCHIVE" | awk '{print $1}')" = "c003242369046d3c2adc6b48c3c96e0ff129e76738b7f3aa5342828ec8ba410d"
  tar xjf "$SDK_ARCHIVE" -C "$BUILD/sdk" --strip-components=1
  if [[ ! -f "$JSON_ARCHIVE" ]] || [[ "$(shasum -a 256 "$JSON_ARCHIVE" | awk '{print $1}')" != "4b92eb0c06d10683f7447ce9406cb97cd4b453be18d7279320f7b2f025c10187" ]]; then
    curl -L --fail --output "$JSON_ARCHIVE" https://github.com/nlohmann/json/archive/refs/tags/v3.12.0.tar.gz
  fi
  test "$(shasum -a 256 "$JSON_ARCHIVE" | awk '{print $1}')" = "4b92eb0c06d10683f7447ce9406cb97cd4b453be18d7279320f7b2f025c10187"
  tar xzf "$JSON_ARCHIVE" -C "$BUILD/json" --strip-components=1
  xcrun clang++ -std=c++17 -O3 -DNDEBUG -arch arm64 -mmacosx-version-min=12.0 \
    -I"$SOURCE" -I"$BUILD/json/include" "$ROOT/runtime/sensevoice/wrapper.cc" \
    -L"$BUILD/sdk/lib" -lsherpa-onnx-cxx-api -lsherpa-onnx-c-api \
    -Wl,-rpath,@executable_path/sensevoice-runtime \
    -Wl,-rpath,@executable_path/../Resources/resources/sensevoice \
    -o "$BUILT"
else
  echo "cmake is required for the Windows runtime build." >&2
  exit 2
fi
test -f "$BUILT"

DEST="$ROOT/desktop/src-tauri/binaries/voxly-sensevoice-wrapper-$TARGET$SUFFIX"
mkdir -p "$(dirname "$DEST")" "$ROOT/desktop/src-tauri/resources/sensevoice"
install -m 0755 "$BUILT" "$DEST"
# Development resolves the trusted runtime beside the Go sidecar. Packaged
# builds place both external binaries together in the application directory.
install -m 0755 "$BUILT" "$ROOT/core-go/voxly-sensevoice-wrapper$SUFFIX"
install -m 0644 "$SOURCE/LICENSE" "$ROOT/desktop/src-tauri/resources/sensevoice/LICENSE.sherpa-onnx"
install -m 0644 "$ROOT/runtime/sensevoice/THIRD_PARTY_NOTICES.md" "$ROOT/desktop/src-tauri/resources/sensevoice/THIRD_PARTY_NOTICES.md"
install -m 0644 "$ROOT/runtime/sensevoice/runtime-manifest.json" "$ROOT/desktop/src-tauri/resources/sensevoice/runtime-manifest.json"

if [[ "$TARGET" == aarch64-apple-darwin && -d "$BUILD/sdk/lib" ]]; then
  install -m 0644 "$BUILD/sdk/lib/libsherpa-onnx-c-api.dylib" "$ROOT/desktop/src-tauri/resources/sensevoice/"
  install -m 0644 "$BUILD/sdk/lib/libsherpa-onnx-cxx-api.dylib" "$ROOT/desktop/src-tauri/resources/sensevoice/"
  install -m 0644 "$BUILD/sdk/lib/libonnxruntime.1.27.0.dylib" "$ROOT/desktop/src-tauri/resources/sensevoice/"
fi

if [[ "$TARGET" == aarch64-apple-darwin ]]; then
  cp "$BUILT" "$ROOT/desktop/src-tauri/target/debug/voxly-sensevoice-wrapper"
  chmod 0755 "$ROOT/desktop/src-tauri/target/debug/voxly-sensevoice-wrapper"
  mkdir -p "$ROOT/core-go/sensevoice-runtime" "$ROOT/desktop/src-tauri/target/debug/sensevoice-runtime"
  cp "$ROOT/desktop/src-tauri/resources/sensevoice/"*.dylib "$ROOT/core-go/sensevoice-runtime/"
  cp "$ROOT/desktop/src-tauri/resources/sensevoice/"*.dylib "$ROOT/desktop/src-tauri/target/debug/sensevoice-runtime/"
  codesign --force --sign - "$BUILT" "$DEST" "$ROOT/core-go/voxly-sensevoice-wrapper" "$ROOT/desktop/src-tauri/target/debug/voxly-sensevoice-wrapper"
  codesign --force --sign - "$ROOT/desktop/src-tauri/resources/sensevoice/"*.dylib
  codesign --force --sign - "$ROOT/core-go/sensevoice-runtime/"*.dylib
  codesign --force --sign - "$ROOT/desktop/src-tauri/target/debug/sensevoice-runtime/"*.dylib
fi

if [[ "$TARGET" == aarch64-apple-darwin && -d "$BUILD/sdk/lib" ]]; then
  "$ROOT/desktop/src-tauri/target/debug/voxly-sensevoice-wrapper" --version-json
else
  "$BUILT" --version-json
fi
