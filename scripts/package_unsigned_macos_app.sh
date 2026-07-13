#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/package_unsigned_macos_app.sh [--force]

Package an already-built unsigned Apple Silicon macOS .app into local zip,
SHA-256, and build-manifest files under dist/unsigned/.
EOF
}

force=false
case "${1:-}" in
  "") ;;
  --force) force=true ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [[ $# -gt 1 ]]; then
  usage >&2
  exit 2
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Unsigned app packaging requires macOS; found $(uname -s)." >&2
  exit 1
fi

if [[ "$(uname -m)" != "arm64" ]]; then
  echo "Unsigned app packaging currently supports Apple Silicon (arm64) only; found $(uname -m)." >&2
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  echo "Node.js is required to read the Tauri config and write the build manifest." >&2
  exit 1
fi

if ! command -v ditto >/dev/null 2>&1; then
  echo "The macOS ditto utility is required to create the app zip." >&2
  exit 1
fi

if ! command -v shasum >/dev/null 2>&1; then
  echo "The macOS shasum utility is required to create the SHA-256 file." >&2
  exit 1
fi

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "$repo_root" ]]; then
  echo "Run this script from the meeting-asr repository root." >&2
  exit 1
fi
repo_root="$(cd "$repo_root" && pwd -P)"
current_dir="$(pwd -P)"
if [[ "$current_dir" != "$repo_root" ]]; then
  echo "Run this script from the repository root: $repo_root" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
  echo "Tracked files must be clean before packaging. Commit or discard tracked changes first." >&2
  exit 1
fi

source_commit="$(git rev-parse HEAD)"
short_commit="$(git rev-parse --short HEAD)"
build_date="$(date -u +%Y-%m-%d)"

tauri_config="$repo_root/desktop/src-tauri/tauri.conf.json"
if [[ ! -f "$tauri_config" ]]; then
  echo "Tauri config not found: $tauri_config" >&2
  exit 1
fi

app_name="$(node -e 'const c=require(process.argv[1]); if (typeof c.productName !== "string" || !c.productName) process.exit(1); process.stdout.write(c.productName)' "$tauri_config")"
app_version="$(node -e 'const c=require(process.argv[1]); if (typeof c.version !== "string" || !c.version) process.exit(1); process.stdout.write(c.version)' "$tauri_config")"
if [[ ! "$app_version" =~ ^[0-9A-Za-z][0-9A-Za-z._-]*$ ]]; then
  echo "Tauri app version is not safe for an artifact filename: $app_version" >&2
  exit 1
fi

bundle_dir="$repo_root/desktop/src-tauri/target/release/bundle/macos"
if [[ ! -d "$bundle_dir" ]]; then
  echo "No built macOS app directory found: $bundle_dir" >&2
  exit 1
fi

app_paths=()
while IFS= read -r -d '' app_path; do
  app_paths+=("$app_path")
done < <(find "$bundle_dir" -maxdepth 1 -type d -name '*.app' -print0)

if [[ ${#app_paths[@]} -eq 0 ]]; then
  echo "No .app bundle found under $bundle_dir" >&2
  exit 1
fi
if [[ ${#app_paths[@]} -ne 1 ]]; then
  echo "Expected exactly one .app bundle under $bundle_dir; found ${#app_paths[@]}." >&2
  printf '  %s\n' "${app_paths[@]}" >&2
  exit 1
fi

app_path="${app_paths[0]}"
sidecar_name="meeting-asr-go"
sidecar_path="$app_path/Contents/MacOS/$sidecar_name"
if [[ ! -f "$sidecar_path" || ! -x "$sidecar_path" ]]; then
  echo "Packaged sidecar is missing or not executable: $sidecar_path" >&2
  exit 1
fi

output_dir="$repo_root/dist/unsigned"
artifact_filename="meeting-asr-desktop-experiment-macos-arm64-$app_version-unsigned.zip"
sha256_filename="$artifact_filename.sha256"
manifest_filename="build-manifest-$app_version-$short_commit.json"
artifact_path="$output_dir/$artifact_filename"
sha256_path="$output_dir/$sha256_filename"
manifest_path="$output_dir/$manifest_filename"

existing=()
for path in "$artifact_path" "$sha256_path" "$manifest_path"; do
  if [[ -e "$path" ]]; then
    existing+=("$path")
  fi
done
if [[ ${#existing[@]} -gt 0 && "$force" != true ]]; then
  echo "Refusing to overwrite existing output. Re-run with --force to replace:" >&2
  printf '  %s\n' "${existing[@]}" >&2
  exit 1
fi

mkdir -p "$output_dir"
temp_dir="$(mktemp -d "$output_dir/.package.XXXXXX")"
trap 'rm -rf "$temp_dir"' EXIT

temp_artifact="$temp_dir/$artifact_filename"
temp_sha256="$temp_dir/$sha256_filename"
temp_manifest="$temp_dir/$manifest_filename"

ditto -c -k --sequesterRsrc --keepParent "$app_path" "$temp_artifact"
(
  cd "$temp_dir"
  shasum -a 256 "$artifact_filename" > "$sha256_filename"
  shasum -a 256 -c "$sha256_filename" >/dev/null
)
sha256="$(awk '{print $1}' "$temp_sha256")"

node - "$temp_manifest" "$app_name" "$app_version" "$artifact_filename" \
  "$sha256_filename" "$sha256" "$source_commit" "$short_commit" \
  "$build_date" "$sidecar_name" <<'NODE'
const fs = require("fs");

const [
  manifestPath,
  appName,
  appVersion,
  artifactFilename,
  sha256Filename,
  sha256,
  sourceCommit,
  shortCommit,
  buildDate,
  sidecarName,
] = process.argv.slice(2);

const manifest = {
  app_name: appName,
  app_version: appVersion,
  artifact_filename: artifactFilename,
  sha256_filename: sha256Filename,
  sha256,
  source_commit: sourceCommit,
  short_commit: shortCommit,
  build_date: buildDate,
  architecture: "arm64",
  unsigned: true,
  notarized: false,
  sidecar_name: sidecarName,
  sidecar_present: true,
};

fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, {
  encoding: "utf8",
  flag: "wx",
});
NODE

mv -f "$temp_artifact" "$artifact_path"
mv -f "$temp_sha256" "$sha256_path"
mv -f "$temp_manifest" "$manifest_path"

echo "Created local unsigned macOS package files:"
printf '  %s\n' "$artifact_path" "$sha256_path" "$manifest_path"
