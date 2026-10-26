#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$(uname -s)" == Darwin && "$(uname -m)" == arm64 ]] || { echo 'This release workflow currently validates Apple Silicon macOS only.' >&2; exit 1; }
version="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' "$repo_root/desktop/src-tauri/tauri.conf.json")"
output="${VOXLY_RELEASE_OUTPUT:-$repo_root/.runtime-build/releases/$version-$(date -u +%Y%m%dT%H%M%SZ)}"
[[ "$output" == /* && ! -e "$output" ]] || { echo 'Release output must be a new absolute directory.' >&2; exit 1; }
if [[ "${1:-}" != --reuse-app ]]; then
  "$repo_root/scripts/build_desktop_sidecar.sh"
  (cd "$repo_root/desktop" && npm run tauri -- build --bundles app --no-sign --ci)
fi
app="$repo_root/desktop/src-tauri/target/release/bundle/macos/Voxly.app"
for binary in meeting-asr-go voxly-sensevoice-wrapper ffmpeg meeting-asr-desktop; do
  test -x "$app/Contents/MacOS/$binary"
  codesign --force --sign - --timestamp=none "$app/Contents/MacOS/$binary"
done
codesign --force --sign - --timestamp=none "$app"
codesign --verify --deep --strict "$app"
"$app/Contents/MacOS/ffmpeg" -version | head -n 1
mkdir -p "$output" "$output/staging"
ditto "$app" "$output/staging/Voxly.app"
ln -s /Applications "$output/staging/Applications"
cp "$repo_root/docs/releases/$version.md" "$output/RELEASE-NOTES.md"
cp "$repo_root/docs/releases/$version.md" "$output/staging/使用说明.md"
# A plain drag-to-Applications image avoids Finder/AppleScript automation.
hdiutil create -srcfolder "$output/staging" -volname "Voxly $version" -format UDZO -ov "$output/Voxly_${version}_macos_arm64.dmg"
hdiutil verify "$output/Voxly_${version}_macos_arm64.dmg"
sources="$repo_root/.runtime-build/distribution/sources"
tar -czf "$output/Voxly_${version}_corresponding-sources.tar.gz" \
  -C "$repo_root" scripts/build_ffmpeg_runtime.sh native/THIRD-PARTY-NOTICES.md docs/releases/ffmpeg-rebuild.md \
  -C "$sources" ffmpeg-9.0.2.tar.xz lame-3.100.tar.gz
(cd "$output" && shasum -a 256 ./*.dmg ./*.tar.gz > SHA256SUMS.txt)
echo "Release prepared at $output"
echo 'Not published. This ad-hoc-signed build is not Developer ID signed or notarized.'
