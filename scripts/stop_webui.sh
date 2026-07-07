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

stop_pids() {
  local pids="$1"
  local label="$2"

  echo "$label"
  if ! kill $pids >/dev/null 2>&1; then
    echo "Could not stop one or more processes from this shell. Try:"
    echo "kill $pids"
    exit 1
  fi

  sleep 1
  local alive=""
  for pid in $pids; do
    if is_pid_running "$pid"; then
      alive="${alive}${alive:+ }$pid"
    fi
  done

  if [ -n "$alive" ]; then
    echo "Process still running after a normal stop; forcing it to quit..."
    if ! kill -9 $alive >/dev/null 2>&1; then
      echo "Could not force stop one or more processes. Try:"
      echo "kill -9 $alive"
      exit 1
    fi
    sleep 1
  fi

  alive=""
  for pid in $pids; do
    if is_pid_running "$pid"; then
      alive="${alive}${alive:+ }$pid"
    fi
  done

  if [ -n "$alive" ]; then
    echo "One or more processes are still running. Try:"
    echo "kill -9 $alive"
    exit 1
  fi
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
    stop_pids "$ALIVE" "Stopping meeting-asr WebUI..."
    rm -f "$PID_FILE"
    echo "Stopped."
    exit 0
  fi
  rm -f "$PID_FILE"
fi

if command -v lsof >/dev/null 2>&1; then
  PIDS="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN || true)"
  if [ -n "$PIDS" ]; then
    echo "No active PID file was found. Stopping process(es) listening on port $PORT."
    stop_pids "$PIDS" "This may stop another local service if it is using the same port."
    rm -f "$PID_FILE"
    echo "Stopped."
    exit 0
  fi
fi

echo "meeting-asr WebUI is not running."
