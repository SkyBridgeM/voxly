#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Internal app-bundle smoke requires macOS; found $(uname -s)." >&2
  exit 1
fi

# Tauri resolves the externalBin input by target triple, so always build the
# host sidecar before asking Tauri to create the internal .app bundle.
"$REPO_ROOT/scripts/build_desktop_sidecar.sh"

cd "$REPO_ROOT/desktop"
npm run tauri -- build --bundles app --no-sign --ci

bundle_dir="$REPO_ROOT/desktop/src-tauri/target/release/bundle/macos"
app_path="$(find "$bundle_dir" -maxdepth 1 -type d -name '*.app' -print -quit)"
if [[ -z "$app_path" || ! -d "$app_path" ]]; then
  echo "Internal app-bundle smoke did not produce a .app under $bundle_dir" >&2
  exit 1
fi

sidecar="$app_path/Contents/MacOS/meeting-asr-go"
test -x "$sidecar"
test -x "$app_path/Contents/MacOS/ffmpeg"
"$app_path/Contents/MacOS/ffmpeg" -version >/dev/null
test -f "$app_path/Contents/Resources/licenses/FFmpeg-LGPL-2.1.txt"
test -x "$app_path/Contents/MacOS/voxly-sensevoice-wrapper"
"$app_path/Contents/MacOS/voxly-sensevoice-wrapper" --version
version_output="$("$sidecar" version)"
case "$version_output" in
  *meeting-asr-go*) ;;
  *)
    echo "Bundled sidecar version output was unexpected: $version_output" >&2
    exit 1
    ;;
esac

echo "Internal app-bundle smoke passed: $app_path"
echo "Bundled sidecar: $sidecar"
