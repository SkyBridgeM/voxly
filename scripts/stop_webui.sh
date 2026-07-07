#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PORT="${MEETING_ASR_WEBUI_PORT:-8501}"
PID_FILE=".streamlit/webui.pid"

if [ -f "$PID_FILE" ]; then
  PID="$(cat "$PID_FILE")"
  if kill -0 "$PID" >/dev/null 2>&1; then
    echo "Stopping meeting-asr WebUI..."
    if ! kill "$PID" >/dev/null 2>&1; then
      echo "Could not stop PID $PID from this shell. Try: kill $PID"
      exit 1
    fi
    rm -f "$PID_FILE"
    echo "Stopped."
    exit 0
  fi
  rm -f "$PID_FILE"
fi

if command -v lsof >/dev/null 2>&1; then
  PIDS="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN || true)"
  if [ -n "$PIDS" ]; then
    echo "Stopping process on port $PORT..."
    if ! kill $PIDS >/dev/null 2>&1; then
      echo "Could not stop one or more processes from this shell. Try:"
      echo "kill $PIDS"
      exit 1
    fi
    echo "Stopped."
    exit 0
  fi
fi

echo "meeting-asr WebUI is not running."
