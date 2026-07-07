#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

HOST="127.0.0.1"
PORT="${MEETING_ASR_WEBUI_PORT:-8501}"
URL="http://${HOST}:${PORT}"
PID_FILE=".streamlit/webui.pid"
LOG_FILE=".streamlit/webui.log"

mkdir -p .streamlit

if [ ! -x ".venv/bin/python" ]; then
  echo "Creating local Python environment..."
  python3 -m venv .venv
fi

if ! .venv/bin/python -c "import streamlit" >/dev/null 2>&1; then
  echo "Installing WebUI dependencies..."
  .venv/bin/python -m pip install -e '.[webui]'
fi

if [ -f "$PID_FILE" ]; then
  PID="$(cat "$PID_FILE")"
  if kill -0 "$PID" >/dev/null 2>&1; then
    echo "meeting-asr WebUI is already running:"
    echo "$URL"
    if command -v open >/dev/null 2>&1; then
      open "$URL"
    fi
    exit 0
  fi
  rm -f "$PID_FILE"
fi

if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Port $PORT is already in use. Open $URL if this is meeting-asr, or stop the other process first."
  exit 1
fi

echo "Starting meeting-asr WebUI..."
nohup .venv/bin/streamlit run webui/app.py \
  --server.address "$HOST" \
  --server.port "$PORT" \
  > "$LOG_FILE" 2>&1 &

STARTER_PID="$!"
PID=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if command -v lsof >/dev/null 2>&1; then
    PID="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN || true)"
    if [ -n "$PID" ]; then
      break
    fi
  elif kill -0 "$STARTER_PID" >/dev/null 2>&1; then
    PID="$STARTER_PID"
    break
  fi
  sleep 1
done

if [ -z "$PID" ]; then
  echo "WebUI failed to start. Recent log:"
  tail -n 40 "$LOG_FILE" || true
  rm -f "$PID_FILE"
  exit 1
fi

echo "$PID" > "$PID_FILE"
echo "meeting-asr WebUI is running:"
echo "$URL"
echo "Log file: $ROOT_DIR/$LOG_FILE"

if [ "${MEETING_ASR_NO_OPEN:-}" != "1" ] && command -v open >/dev/null 2>&1; then
  open "$URL"
fi
