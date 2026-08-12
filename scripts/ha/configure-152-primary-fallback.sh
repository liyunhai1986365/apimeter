#!/usr/bin/env bash
set -Eeuo pipefail

[[ "${CONFIRM_APIMETER_152_ROUTE:-}" == "primary-fallback" ]] || {
  echo "set CONFIRM_APIMETER_152_ROUTE=primary-fallback" >&2
  exit 2
}

active_config="${CADDY_CONFIG:-/etc/caddy/Caddyfile}"
backup_config="${active_config}.before-primary-fallback"
candidate="$(mktemp /tmp/apimeter-caddy-152.XXXXXX)"
cleanup() {
  shred -u "$candidate" 2>/dev/null || true
}
trap cleanup EXIT

curl -fsS --max-time 5 http://10.38.145.1:3000/api/ready >/dev/null
curl -fsS --max-time 5 http://127.0.0.1:3000/api/ready >/dev/null

python3 - "$active_config" "$candidate" <<'PY'
from pathlib import Path
import sys

source = Path(sys.argv[1]).read_text()
lines = source.splitlines(keepends=True)

start = next(
    (index for index, line in enumerate(lines) if line.strip() == "(common_proxy) {"),
    None,
)
if start is None:
    raise SystemExit("missing common_proxy snippet")

depth = 0
end = None
for index in range(start, len(lines)):
    depth += lines[index].count("{") - lines[index].count("}")
    if depth == 0:
        end = index + 1
        break
if end is None:
    raise SystemExit("unterminated common_proxy snippet")

replacement = """(common_proxy) {
\treverse_proxy 10.38.145.1:3000 127.0.0.1:3000 {
\t\tlb_policy first
\t\tlb_try_duration 5s
\t\tlb_try_interval 250ms
\t\thealth_uri /api/ready
\t\thealth_interval 5s
\t\thealth_timeout 2s
\t\tfail_duration 30s
\t\tmax_fails 2
\t\theader_up Host {host}
\t\theader_up X-Forwarded-Host {host}
\t\theader_up X-Forwarded-Proto {scheme}
\t\theader_up X-Real-IP {remote_host}
\t}
}
"""
updated = "".join(lines[:start]) + replacement + "".join(lines[end:])

anchor = "\t\t\t\trequest>headers>New-Api-User delete\n"
extra = """\t\t\t\trequest>headers>X-Codex-Turn-Metadata delete
\t\t\t\trequest>headers>Session-Id delete
\t\t\t\trequest>headers>Thread-Id delete
\t\t\t\trequest>headers>X-Client-Request-Id delete
"""
if "request>headers>X-Codex-Turn-Metadata delete" not in updated:
    if anchor not in updated:
        raise SystemExit("missing access-log redaction anchor")
    updated = updated.replace(anchor, anchor + extra, 1)

Path(sys.argv[2]).write_text(updated)
PY

caddy validate --config "$candidate" --adapter caddyfile
install -m 0644 "$active_config" "$backup_config"
install -m 0644 "$candidate" "$active_config"

if ! caddy reload --config "$active_config" --force; then
  install -m 0644 "$backup_config" "$active_config"
  caddy reload --config "$active_config" --force
  exit 1
fi

curl --resolve apimeter.ai:443:127.0.0.1 \
  -fsS --max-time 8 https://apimeter.ai/api/ready >/dev/null

echo "caddy_152_route=38-first,152-fallback"
