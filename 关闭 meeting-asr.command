#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$ROOT_DIR/scripts/stop_webui.sh"

echo
echo "Done. You can close this window."
echo "完成。可以关闭此窗口。"
