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
  grep -Fq "$needle" "$SCRIPT" || fail "missing expected text: $needle"
}

assert_contains 'DEPLOY_HEALTH_PATH="${DEPLOY_HEALTH_PATH:-/api/status}"'
assert_contains 'DEPLOY_HEALTH_TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-30}"'
assert_contains 'DEPLOY_KEEP_RELEASES="${DEPLOY_KEEP_RELEASES:-5}"'
assert_contains 'DEPLOY_STANDBY_APP_PORT="${DEPLOY_STANDBY_APP_PORT:-$((DEPLOY_APP_PORT + 1))}"'
assert_contains 'DEPLOY_ZERO_DOWNTIME="${DEPLOY_ZERO_DOWNTIME:-true}"'
assert_contains 'DEPLOY_CADDY_UPSTREAM_FILE="${DEPLOY_CADDY_UPSTREAM_FILE:-/etc/caddy/modelsell-upstream.caddy}"'
assert_contains 'DEPLOY_DRAIN_TIMEOUT="${DEPLOY_DRAIN_TIMEOUT:-900}"'
assert_contains 'CURRENT_LINK="$REMOTE_DIR/current"'
assert_contains 'RELEASE_LINK="$REMOTE_DIR/release-$SLOT"'
assert_contains 'PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"'
assert_contains 'ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"'
assert_contains 'ln -sfnT "$RELEASE_DIR" "$RELEASE_LINK"'
assert_contains 'start_slot_service "$NEXT_SLOT" "$NEXT_PORT"'
assert_contains 'update_caddy_upstream "$NEXT_PORT"'
assert_contains 'stop_previous_slot_after_drain "$CURRENT_SLOT"'
assert_contains 'rollback_service "$PREVIOUS_TARGET"'
assert_contains 'rollback_zero_downtime "$CURRENT_SLOT" "$CURRENT_PORT" "$PREVIOUS_TARGET"'
assert_contains 'curl -fsS --max-time 3 "$url"'
assert_contains 'ExecStart=${REMOTE_DIR}/current/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs'
assert_contains 'ExecStart=${REMOTE_DIR}/release-%i/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs'

printf 'deploy-modelsell script safety checks passed\n'
