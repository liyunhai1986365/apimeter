#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/deploy-modelsell.sh"

fail() {
  printf 'deploy-modelsell test failed: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local needle="$1"
  grep -Fq -- "$needle" "$SCRIPT" || fail "missing expected text: $needle"
}

assert_heredoc_syntax() {
  local marker="$1"
  local extracted
  extracted="$(mktemp)"
  awk -v marker="$marker" '
    capture && $0 == marker { exit }
    capture { print }
    index($0, "<<") && index($0, marker) { capture = 1 }
  ' "$SCRIPT" >"$extracted"
  [[ -s "$extracted" ]] || fail "could not extract heredoc: $marker"
  bash -n "$extracted" || fail "invalid remote bash syntax: $marker"
  rm -f "$extracted"
}

assert_contains 'DEPLOY_HEALTH_PATH="${DEPLOY_HEALTH_PATH:-/api/status}"'
assert_contains 'DEPLOY_HEALTH_TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-30}"'
assert_contains 'DEPLOY_STOP_TIMEOUT="${DEPLOY_STOP_TIMEOUT:-135}"'
assert_contains 'DEPLOY_KEEP_RELEASES="${DEPLOY_KEEP_RELEASES:-5}"'
assert_contains 'CURRENT_LINK="$REMOTE_DIR/current"'
assert_contains 'PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"'
assert_contains 'ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"'
assert_contains 'rollback_service "$PREVIOUS_TARGET"'
assert_contains "curl -sS --max-time 3 -o /dev/null -w '%{http_code}' \"\$url\""
assert_contains 'TimeoutStopSec=${STOP_TIMEOUT}s'
assert_contains 'systemctl stop "$SERVICE_NAME" || stop_rc=$?'
assert_contains 'systemctl reset-failed "$SERVICE_NAME"'
assert_contains 'ensure_app_port_available || return 1'
assert_contains 'journalctl -u "$SERVICE_NAME" --since now --follow'
assert_contains 'Health check: elapsed='
assert_contains 'install -m 0755 "$RELEASE_DIR/start-modelsell.sh" "$REMOTE_DIR/bin/start-modelsell.sh"'
assert_contains 'ExecStart=${REMOTE_DIR}/current/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs'
assert_contains 'install -m 0755 "$ROOT_DIR/scripts/start-modelsell.sh" "$BUILD_DIR/start-modelsell.sh"'
assert_contains 'COPYFILE_DISABLE=1 tar --no-xattrs -czf "$ARCHIVE_NAME"'
assert_contains 'rollback-modelsell.sh'
assert_contains '--manual-start'
assert_contains '--manual-stop'
assert_contains '--manual-rollback-list'
assert_contains '--manual-rollback <release-id>'
assert_contains '--manual-status'
assert_contains '--manual-logs'
assert_contains '--manual-service-start'
assert_contains '--manual-service-stop'
assert_contains 'manual-rollback:*)'

if grep -Fq 'systemctl restart "$SERVICE_NAME"' "$SCRIPT"; then
  fail 'automatic deploy must not treat a stop timeout from systemctl restart as a new-release failure'
fi

assert_heredoc_syntax REMOTE_SCRIPT
assert_heredoc_syntax REMOTE_STATUS
assert_heredoc_syntax REMOTE_START
assert_heredoc_syntax REMOTE_STOP

printf 'deploy-modelsell script safety checks passed\n'
