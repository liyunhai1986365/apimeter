#!/usr/bin/env bash
set -Eeuo pipefail

# Start the packaged binary directly when the systemd unit cannot be started.
# The binary loads .env itself, so this script must preserve the release cwd.

REMOTE_DIR="${APIMETER_REMOTE_DIR:-/www/wwwroot/apimeter}"
BINARY_NAME="${APIMETER_BINARY_NAME:-new-api}"
APP_PORT="${APIMETER_APP_PORT:-3000}"
HEALTH_PATH="${APIMETER_HEALTH_PATH:-/api/status}"
HEALTH_TIMEOUT="${APIMETER_HEALTH_TIMEOUT:-30}"
LOG_DIR="${APIMETER_LOG_DIR:-$REMOTE_DIR/logs}"
LOG_FILE="${APIMETER_LOG_FILE:-$LOG_DIR/new-api-manual.log}"
PID_FILE="${APIMETER_PID_FILE:-$LOG_DIR/new-api-manual.pid}"
SERVICE_NAME="${APIMETER_SERVICE_NAME:-apimeter}"
FOREGROUND=0
FORCE=0

usage() {
  cat <<'EOF'
Usage:
  ./scripts/start-apimeter.sh [--foreground] [--force]
  ./scripts/start-apimeter.sh --stop

Environment overrides:
  APIMETER_REMOTE_DIR, APIMETER_BINARY_NAME, APIMETER_APP_PORT
  APIMETER_HEALTH_PATH, APIMETER_HEALTH_TIMEOUT, APIMETER_LOG_DIR
  APIMETER_LOG_FILE, APIMETER_PID_FILE, APIMETER_SERVICE_NAME
EOF
}

fail() {
  printf '[manual-start:error] %s\n' "$*" >&2
  exit 1
}

log() {
  printf '[manual-start] %s\n' "$*"
}

is_number() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

pid_is_ours() {
  local pid="$1"
  if [[ -r "/proc/$pid/cmdline" ]]; then
    tr '\0' ' ' <"/proc/$pid/cmdline" | grep -Fq -- "$BINARY_NAME"
    return
  fi
  command -v ps >/dev/null 2>&1 || return 1
  ps -p "$pid" -o command= 2>/dev/null | grep -Fq -- "$BINARY_NAME"
}

stop_tracked_process() {
  [[ -f "$PID_FILE" ]] || return 0
  local pid
  pid="$(tr -d '[:space:]' <"$PID_FILE")"
  if ! is_number "$pid" || ! kill -0 "$pid" 2>/dev/null || ! pid_is_ours "$pid"; then
    rm -f "$PID_FILE"
    return 0
  fi

  log "Stopping manually started process: PID $pid"
  kill "$pid" 2>/dev/null || true
  for _ in {1..20}; do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.25
  done
  if kill -0 "$pid" 2>/dev/null; then
    kill -9 "$pid" 2>/dev/null || true
  fi
  rm -f "$PID_FILE"
}

wait_healthy() {
  local url="http://127.0.0.1:${APP_PORT}${HEALTH_PATH}"
  local deadline=$((SECONDS + HEALTH_TIMEOUT))
  while (( SECONDS < deadline )); do
    if curl -fsS --max-time 3 "$url" >/dev/null 2>&1; then
      log "Health check passed: $url"
      return 0
    fi
    sleep 1
  done
  return 1
}

while (($# > 0)); do
  case "$1" in
    --foreground) FOREGROUND=1 ;;
    --force) FORCE=1 ;;
    --stop) stop_tracked_process; exit 0 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; fail "Unknown option: $1" ;;
  esac
  shift
done

is_number "$APP_PORT" || fail "APIMETER_APP_PORT must be numeric: $APP_PORT"
is_number "$HEALTH_TIMEOUT" || fail "APIMETER_HEALTH_TIMEOUT must be numeric: $HEALTH_TIMEOUT"
[[ "$HEALTH_PATH" == /* ]] || HEALTH_PATH="/$HEALTH_PATH"

cd "$REMOTE_DIR" 2>/dev/null || fail "Working directory does not exist: $REMOTE_DIR"
BINARY_PATH="$REMOTE_DIR/current/$BINARY_NAME"
[[ -x "$BINARY_PATH" ]] || fail "Packaged binary is missing or not executable: $BINARY_PATH"
mkdir -p "$LOG_DIR"

if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$SERVICE_NAME"; then
  if (( FORCE == 0 )); then
    fail "systemd service $SERVICE_NAME is already active; use --force only if replacing it"
  fi
  log "Stopping systemd service before direct startup: $SERVICE_NAME"
  stop_rc=0
  systemctl stop "$SERVICE_NAME" || stop_rc=$?
  if systemctl is-active --quiet "$SERVICE_NAME"; then
    fail "systemd service $SERVICE_NAME is still active after stop (exit=$stop_rc)"
  fi
  if (( stop_rc != 0 )); then
    log "systemctl stop returned $stop_rc, but the service is stopped"
  fi
fi

stop_tracked_process

if command -v ss >/dev/null 2>&1 \
  && ss -ltn "sport = :$APP_PORT" 2>/dev/null | tail -n +2 | grep -q .; then
  ss -ltnp "sport = :$APP_PORT" 2>/dev/null >&2 || true
  fail "port $APP_PORT is already occupied; refusing to start a second process"
fi

if (( FOREGROUND == 1 )); then
  log "Starting $BINARY_PATH in foreground on port $APP_PORT"
  exec "$BINARY_PATH" --port "$APP_PORT" --log-dir "$LOG_DIR"
fi

log "Starting $BINARY_PATH in background on port $APP_PORT"
nohup "$BINARY_PATH" --port "$APP_PORT" --log-dir "$LOG_DIR" >>"$LOG_FILE" 2>&1 </dev/null &
pid=$!
printf '%s\n' "$pid" >"$PID_FILE"
sleep 1

if ! kill -0 "$pid" 2>/dev/null; then
  rm -f "$PID_FILE"
  tail -n 80 "$LOG_FILE" >&2 || true
  fail "new-api exited during startup"
fi

if ! wait_healthy; then
  tail -n 80 "$LOG_FILE" >&2 || true
  stop_tracked_process
  fail "new-api started but health check failed"
fi

log "Started successfully: PID $pid"
