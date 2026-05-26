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
assert_contains 'CURRENT_LINK="$REMOTE_DIR/current"'
assert_contains 'PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"'
assert_contains 'ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"'
assert_contains 'rollback_service "$PREVIOUS_TARGET"'
assert_contains 'curl -fsS --max-time 3 "$url"'
assert_contains 'ExecStart=${REMOTE_DIR}/current/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs'

printf 'deploy-modelsell script safety checks passed\n'
