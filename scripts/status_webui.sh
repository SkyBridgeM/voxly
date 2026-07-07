#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

HOST="127.0.0.1"
PORT="${MEETING_ASR_WEBUI_PORT:-8501}"
URL="http://${HOST}:${PORT}"
PID_FILE=".streamlit/webui.pid"

is_pid_running() {
  ps -p "$1" >/dev/null 2>&1 && return 0
  if command -v lsof >/dev/null 2>&1; then
    lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | grep -qx "$1"
    return
  fi
  return 1
}

if [ -f "$PID_FILE" ]; then
  PIDS="$(tr '\n' ' ' < "$PID_FILE")"
  ALIVE=""
  for PID in $PIDS; do
    if is_pid_running "$PID"; then
      ALIVE="${ALIVE}${ALIVE:+ }$PID"
    fi
  done
  if [ -n "$ALIVE" ]; then
    echo "running"
    echo "URL: $URL"
    echo "PID: $ALIVE"
    exit 0
  fi
  echo "Removing stale PID file: $PID_FILE"
  rm -f "$PID_FILE"
fi

if command -v lsof >/dev/null 2>&1; then
  PIDS="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN || true)"
  if [ -n "$PIDS" ]; then
    echo "port occupied"
    echo "URL: $URL"
    echo "PID: $PIDS"
    exit 0
  fi
fi

echo "not running"
