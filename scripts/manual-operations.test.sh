#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail() { printf 'manual operations test failed: %s\n' "$*" >&2; exit 1; }

bash -n "$ROOT_DIR/scripts/start-apimeter.sh" || fail 'start script has invalid bash syntax'
bash -n "$ROOT_DIR/scripts/rollback-apimeter.sh" || fail 'rollback script has invalid bash syntax'

for needle in 'nohup "$BINARY_PATH"' '--log-dir "$LOG_DIR"' 'wait_healthy' '--stop' 'port $APP_PORT is already occupied' 'systemctl is-active --quiet "$SERVICE_NAME"'; do
  grep -Fq -- "$needle" "$ROOT_DIR/scripts/start-apimeter.sh" || fail "start script missing: $needle"
done
for needle in 'ln -sfnT "$TARGET" "$CURRENT_LINK"' 'PREVIOUS_TARGET=' 'restore' 'systemctl start "$SERVICE_NAME"' 'refusing direct fallback while a unit manages this port' 'APIMETER_STOP_TIMEOUT'; do
  grep -Fq -- "$needle" "$ROOT_DIR/scripts/rollback-apimeter.sh" || fail "rollback script missing: $needle"
done

printf 'manual operations script safety checks passed\n'
