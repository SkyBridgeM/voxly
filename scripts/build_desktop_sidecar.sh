#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$REPO_ROOT/desktop/src-tauri/binaries"

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required to build the desktop sidecar. Install Go 1.21 or newer and try again." >&2
  exit 1
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Unsupported sidecar build host: $(uname -s). PR 10 currently supports macOS only." >&2
  exit 1
fi

case "$(uname -m)" in
  arm64)
    goarch="arm64"
    target_triple="aarch64-apple-darwin"
    ;;
  x86_64)
    # Keep the CI smoke usable on Intel macOS runners while Apple Silicon
    # remains the primary production target.
    goarch="amd64"
    target_triple="x86_64-apple-darwin"
    ;;
  *)
    echo "Unsupported macOS architecture: $(uname -m). Expected arm64 or x86_64." >&2
    exit 1
    ;;
esac

mkdir -p "$OUTPUT_DIR"
output="$OUTPUT_DIR/meeting-asr-go-$target_triple"

echo "Building offline Go sidecar for darwin/$goarch"
(
  cd "$REPO_ROOT/core-go"
  GOOS=darwin GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -o "$output" .
)

echo "Built $output"
