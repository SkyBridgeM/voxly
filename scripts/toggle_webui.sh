#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PORT="${MEETING_ASR_WEBUI_PORT:-8501}"
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
  for PID in $PIDS; do
    if is_pid_running "$PID"; then
      "$ROOT_DIR/scripts/stop_webui.sh"
      exit 0
    fi
  done
  if [ -n "$PIDS" ]; then
    rm -f "$PID_FILE"
  fi
fi

"$ROOT_DIR/scripts/start_webui.sh"
