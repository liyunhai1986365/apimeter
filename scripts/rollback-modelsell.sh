#!/usr/bin/env bash
set -Eeuo pipefail

# Switch the current release transactionally and verify it independently of
# the deploy script's automatic rollback path.

REMOTE_DIR="${MODELSELL_REMOTE_DIR:-/www/wwwroot/modelsell}"
BINARY_NAME="${MODELSELL_BINARY_NAME:-new-api}"
SERVICE_NAME="${MODELSELL_SERVICE_NAME:-modelsell}"
APP_PORT="${MODELSELL_APP_PORT:-3000}"
HEALTH_PATH="${MODELSELL_HEALTH_PATH:-/api/status}"
HEALTH_TIMEOUT="${MODELSELL_HEALTH_TIMEOUT:-30}"
LOG_DIR="${MODELSELL_LOG_DIR:-$REMOTE_DIR/logs}"
PID_FILE="${MODELSELL_PID_FILE:-$LOG_DIR/new-api-manual.pid}"
START_SCRIPT="${MODELSELL_START_SCRIPT:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/start-modelsell.sh}"
STOP_TIMEOUT="${MODELSELL_STOP_TIMEOUT:-135}"
RELEASE_ID=""
ASSUME_YES=0
LIST_ONLY=0

usage() {
  cat <<'EOF'
Usage:
  ./scripts/rollback-modelsell.sh --list
  ./scripts/rollback-modelsell.sh --release <release-id> [--yes]

The release id is a directory name under <remote-dir>/releases. The script
stops the current runtime, switches current atomically, starts and health-checks
the selected release, and restores the previous link if verification fails.
EOF
}

fail() {
  printf '[manual-rollback:error] %s\n' "$*" >&2
  exit 1
}

log() {
  printf '[manual-rollback] %s\n' "$*"
}

is_number() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

list_releases() {
  local current
  current="$(readlink -f "$REMOTE_DIR/current" 2>/dev/null || true)"
  printf 'Current: %s\n' "${current:-<none>}"
  printf 'Available releases:\n'
  find "$REMOTE_DIR/releases" -mindepth 1 -maxdepth 1 -type d -print 2>/dev/null \
    | sort -r \
    | while IFS= read -r release; do
        [[ -x "$release/$BINARY_NAME" ]] || continue
        marker=""
        [[ "$(readlink -f "$release" 2>/dev/null || true)" == "$current" ]] && marker=' (current)'
        printf '  %s%s\n' "$(basename "$release")" "$marker"
      done
}

resolve_release() {
  local candidate="$1"
  local releases_dir
  releases_dir="$(readlink -f "$REMOTE_DIR/releases" 2>/dev/null || true)"
  [[ -n "$releases_dir" && -d "$releases_dir" ]] || fail "Releases directory does not exist: $REMOTE_DIR/releases"
  [[ "$candidate" != */* ]] || fail "Release must be a directory name, not a path: $candidate"
  local resolved
  resolved="$(readlink -f "$releases_dir/$candidate" 2>/dev/null || true)"
  [[ -n "$resolved" && "$resolved" == "$releases_dir"/* ]] || fail "Release is outside releases directory: $candidate"
  [[ -d "$resolved" ]] || fail "Release does not exist: $candidate"
  [[ -x "$resolved/$BINARY_NAME" ]] || fail "Release binary is missing or not executable: $resolved/$BINARY_NAME"
  printf '%s' "$resolved"
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

stop_runtime() {
  if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files "${SERVICE_NAME}.service" >/dev/null 2>&1; then
    log "Stopping systemd service: $SERVICE_NAME"
    local stop_rc=0
    systemctl stop "$SERVICE_NAME" || stop_rc=$?
    if systemctl is-active --quiet "$SERVICE_NAME"; then
      fail "systemd service is still active after ${STOP_TIMEOUT}s (exit=$stop_rc)"
    fi
    if (( stop_rc != 0 )); then
      log "systemctl stop returned $stop_rc, but the service is stopped"
    fi
  fi
  if [[ -x "$START_SCRIPT" ]]; then
    "$START_SCRIPT" --stop || true
    return
  fi
  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid="$(tr -d '[:space:]' <"$PID_FILE")"
    if is_number "$pid" && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
  fi
}

start_runtime() {
  if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files "${SERVICE_NAME}.service" >/dev/null 2>&1; then
    systemctl daemon-reload
    if systemctl start "$SERVICE_NAME" && wait_healthy; then
      return 0
    fi
    log "systemd start/health check failed; refusing direct fallback while a unit manages this port"
    return 1
  fi
  if [[ -x "$START_SCRIPT" ]]; then
    "$START_SCRIPT" --force
    return
  fi

  # Releases created before the helper scripts were added can still be started
  # directly, which keeps historical rollback usable after an interrupted deploy.
  mkdir -p "$LOG_DIR"
  cd "$REMOTE_DIR"
  nohup "$CURRENT_LINK/$BINARY_NAME" --port "$APP_PORT" --log-dir "$LOG_DIR" \
    >>"$LOG_DIR/new-api-manual.log" 2>&1 </dev/null &
  printf '%s\n' "$!" >"$PID_FILE"
  sleep 1
}

show_debug() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --no-pager --full status "$SERVICE_NAME" || true
    journalctl -u "$SERVICE_NAME" -n 80 --no-pager || true
  fi
}

while (($# > 0)); do
  case "$1" in
    --release|-r) (($# >= 2)) || fail "--release requires a release id"; RELEASE_ID="$2"; shift ;;
    --list|-l) LIST_ONLY=1 ;;
    --yes|-y) ASSUME_YES=1 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; fail "Unknown option: $1" ;;
  esac
  shift
done

is_number "$APP_PORT" || fail "MODELSELL_APP_PORT must be numeric: $APP_PORT"
is_number "$HEALTH_TIMEOUT" || fail "MODELSELL_HEALTH_TIMEOUT must be numeric: $HEALTH_TIMEOUT"
is_number "$STOP_TIMEOUT" || fail "MODELSELL_STOP_TIMEOUT must be numeric: $STOP_TIMEOUT"
[[ "$HEALTH_PATH" == /* ]] || HEALTH_PATH="/$HEALTH_PATH"

[[ -d "$REMOTE_DIR" ]] || fail "Working directory does not exist: $REMOTE_DIR"
if (( LIST_ONLY == 1 )); then
  list_releases
  exit 0
fi
[[ -n "$RELEASE_ID" ]] || { usage >&2; fail "Specify --release or use --list"; }

TARGET="$(resolve_release "$RELEASE_ID")"
CURRENT_LINK="$REMOTE_DIR/current"
PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
# Resolve the helper before switching `current`; older releases may not contain
# the newly added helper scripts themselves.
if [[ -e "$START_SCRIPT" ]]; then
  START_SCRIPT="$(readlink -f "$START_SCRIPT" 2>/dev/null || printf '%s' "$START_SCRIPT")"
fi
if (( ASSUME_YES == 0 )); then
  printf 'Switch current from %s to %s and restart %s? [y/N] ' \
    "${PREVIOUS_TARGET:-<none>}" "$TARGET" "$SERVICE_NAME"
  read -r answer || answer=""
  [[ "$answer" =~ ^[Yy]$ ]] || { log "Cancelled"; exit 0; }
fi

LOCK_DIR="$REMOTE_DIR/.manual-rollback.lock"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  fail "Another manual rollback is already running: $LOCK_DIR"
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

stop_runtime
ln -sfnT "$TARGET" "$CURRENT_LINK"

if start_runtime && wait_healthy; then
  log "Rollback completed: $(basename "$TARGET")"
  exit 0
fi

log "Selected release failed verification; restoring previous release"
if [[ -n "$PREVIOUS_TARGET" && -d "$PREVIOUS_TARGET" ]]; then
  stop_runtime
  ln -sfnT "$PREVIOUS_TARGET" "$CURRENT_LINK"
  if start_runtime && wait_healthy; then
    show_debug
    fail "Rollback target failed; previous release restored successfully"
  fi
fi

show_debug
fail "Rollback failed and previous release could not be restored"
