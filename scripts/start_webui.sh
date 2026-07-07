#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

HOST="127.0.0.1"
PORT="${MEETING_ASR_WEBUI_PORT:-8501}"
URL="http://${HOST}:${PORT}"
PID_FILE=".streamlit/webui.pid"
LOG_FILE=".streamlit/webui.log"

is_pid_running() {
  ps -p "$1" >/dev/null 2>&1 && return 0
  if command -v lsof >/dev/null 2>&1; then
    lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | grep -qx "$1"
    return
  fi
  return 1
}

mkdir -p .streamlit

if [ ! -f ".env" ]; then
  echo "Warning: .env was not found. The WebUI can open, but transcription will fail until you create .env with MIMO_API_KEY."
fi

if [ ! -x ".venv/bin/python" ]; then
  echo "Creating local Python environment..."
  python3 -m venv .venv
fi

if ! .venv/bin/python -c "import streamlit" >/dev/null 2>&1; then
  echo "Installing WebUI dependencies..."
  .venv/bin/python -m pip install -e '.[webui]'
fi

if [ -f "$PID_FILE" ]; then
  PIDS="$(tr '\n' ' ' < "$PID_FILE")"
  ALIVE=""
  for PID in $PIDS; do
    if is_pid_running "$PID"; then
      ALIVE="${ALIVE}${ALIVE:+ }$PID"
    fi
  done
  if [ -n "$ALIVE" ]; then
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
    PID="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | head -n 1 || true)"
    if [ -n "$PID" ]; then
      break
    fi
  elif is_pid_running "$STARTER_PID"; then
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
