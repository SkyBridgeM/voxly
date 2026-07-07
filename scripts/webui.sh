#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PORT="${MEETING_ASR_WEBUI_PORT:-8501}"

if [ ! -x ".venv/bin/python" ]; then
  echo "Creating local Python environment..."
  python3 -m venv .venv
fi

if ! .venv/bin/python -c "import streamlit" >/dev/null 2>&1; then
  echo "Installing WebUI dependencies..."
  .venv/bin/python -m pip install -e '.[webui]'
fi

echo "Starting meeting-asr WebUI at http://127.0.0.1:${PORT}"
echo "Press Ctrl+C in this terminal to stop."

.venv/bin/streamlit run webui/app.py \
  --server.address 127.0.0.1 \
  --server.port "$PORT"
