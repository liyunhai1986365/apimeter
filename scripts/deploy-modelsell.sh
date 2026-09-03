#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_ENV_FILE="${DEPLOY_ENV_FILE:-$ROOT_DIR/.env.deploy}"

log() {
  printf '\033[1;34m[deploy]\033[0m %s\n' "$*"
}

warn() {
  printf '\033[1;33m[deploy:warn]\033[0m %s\n' "$*" >&2
}

fail() {
  printf '\033[1;31m[deploy:error]\033[0m %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  ./scripts/deploy-modelsell.sh
  ./scripts/deploy-modelsell.sh --full
  ./scripts/deploy-modelsell.sh --build-only
  ./scripts/deploy-modelsell.sh --upload-only
  ./scripts/deploy-modelsell.sh --config-only
  ./scripts/deploy-modelsell.sh --rolling-code
  ./scripts/deploy-modelsell.sh --rolling-full
  ./scripts/deploy-modelsell.sh --config-check
  ./scripts/deploy-modelsell.sh --preflight
  ./scripts/deploy-modelsell.sh --rolling-preflight
  ./scripts/deploy-modelsell.sh --manual-start
  ./scripts/deploy-modelsell.sh --manual-stop
  ./scripts/deploy-modelsell.sh --manual-rollback-list
  ./scripts/deploy-modelsell.sh --manual-rollback <release-id>
  ./scripts/deploy-modelsell.sh --manual-status
  ./scripts/deploy-modelsell.sh --manual-logs
  ./scripts/deploy-modelsell.sh --manual-service-start
  ./scripts/deploy-modelsell.sh --manual-service-stop
  ./scripts/deploy-modelsell.sh --zero-downtime-test

Modes:
  1, --full         Build, upload, update config, install systemd, restart service.
  2, --build-only   Build frontend and Linux x86_64 package locally only.
  3, --upload-only  Upload existing package, install release, restart service. Does not update .env.
  4, --config-only  Upload runtime .env, refresh systemd, restart service. Does not upload binary.
  14, --rolling-full Multi-machine only: build once, then deploy code and config to standby followed by primary.
  15, --rolling-code Multi-machine only: build once, then deploy code without changing runtime config (recommended).
  --config-check Validate the selected topology locally without connecting to any server.
  --preflight Read-only service, dependency, and role check; multi mode also checks WireGuard.
  --rolling-preflight Backward-compatible alias for --preflight.
  5, --manual-start  Start current/new-api directly on the server and health-check it.
  6, --manual-stop   Stop the manually started server process.
  7, --manual-rollback-list  List historical releases on the server.
  8, --manual-rollback <id>  Switch to a historical release and verify it.
  9, --manual-status  Show the current release, service state, and health status.
  10, --manual-logs  Follow the live systemd service log until interrupted.
  11, --manual-service-start  Start the systemd service and verify health.
  12, --manual-service-stop  Gracefully stop the systemd service.
  13, --zero-downtime-test  Drain new traffic to the selected node's peer, restart it, test it, then restore traffic.

Target selection:
  DEPLOY_TOPOLOGY=single uses DEPLOY_PRIMARY_* and DEPLOY_TARGET=primary.
  DEPLOY_TOPOLOGY=multi selects DEPLOY_PRIMARY_* or DEPLOY_STANDBY_*.
  Multi-machine rolling deployment always updates standby first, then primary.
EOF
}

load_env_file() {
  local file="$1"
  [[ -f "$file" ]] || fail "Missing deploy env file: $file. Copy .env.deploy.example to .env.deploy first."
  set -a
  # shellcheck disable=SC1090
  source "$file"
  set +a
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing command: $1"
}

require_var() {
  local name="$1"
  [[ -n "${!name:-}" ]] || fail "Missing required env: $name"
}

resolve_app_env_path() {
  local configured_path="$1"
  if [[ "$configured_path" == /* ]]; then
    printf '%s' "$configured_path"
  else
    printf '%s/%s' "$ROOT_DIR" "$configured_path"
  fi
}

read_app_env_value() {
  local env_file="$1"
  local key="$2"
  sed -n "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*//p" "$env_file" 2>/dev/null \
    | tail -1 | tr -d '\"'\''[:space:]'
}

validate_app_env_role() {
  local env_file="$1"
  local expected_role="$2"
  local actual_role
  [[ -f "$env_file" ]] || fail "Missing app env file: $env_file"
  actual_role="$(read_app_env_value "$env_file" NODE_TYPE)"
  [[ "$actual_role" == "$expected_role" ]] || \
    fail "Runtime env role mismatch: file=$env_file expected=$expected_role actual=${actual_role:-missing}"
}

validate_shared_backend_config() {
  [[ "$DEPLOY_TOPOLOGY" == "multi" ]] || return 0
  local primary_env standby_env
  primary_env="$(resolve_app_env_path "$DEPLOY_PRIMARY_APP_ENV_FILE")"
  standby_env="$(resolve_app_env_path "$DEPLOY_STANDBY_APP_ENV_FILE")"
  validate_app_env_role "$primary_env" master
  validate_app_env_role "$standby_env" slave
  require_cmd python3

  python3 - "$primary_env" "$standby_env" <<'PY'
import re
import sys
from pathlib import Path
from urllib.parse import urlsplit

primary_path, standby_path = sys.argv[1:]


def load_env(path):
    values = {}
    for raw in Path(path).read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
            value = value[1:-1]
        values[key.strip()] = value
    return values


def mysql_identity(value, key, path):
    match = re.fullmatch(
        r"([^:]+):(.*?)@tcp\(([^):]+)(?::(\d+))?\)/([^?]+)(?:\?(.*))?",
        value,
    )
    if not match:
        raise SystemExit(f"unsupported {key} format in {path}")
    user, password, host, port, database, options = match.groups()
    return (user, password, port or "3306", database, options or ""), host


def redis_identity(value, path):
    parsed = urlsplit(value)
    if parsed.scheme not in {"redis", "rediss"} or not parsed.hostname:
        raise SystemExit(f"unsupported REDIS_CONN_STRING format in {path}")
    identity = (
        parsed.scheme,
        parsed.username or "",
        parsed.password or "",
        parsed.port or 6379,
        parsed.path or "/0",
        parsed.query,
    )
    return identity, parsed.hostname


primary = load_env(primary_path)
standby = load_env(standby_path)
hosts = {}
for key in ("SQL_DSN", "LOG_SQL_DSN"):
    if not primary.get(key) or not standby.get(key):
        raise SystemExit(f"{key} must be configured in both runtime env files")
    primary_identity, primary_host = mysql_identity(primary[key], key, primary_path)
    standby_identity, standby_host = mysql_identity(standby[key], key, standby_path)
    if primary_identity != standby_identity:
        raise SystemExit(
            f"{key} credentials, port, database, or options differ between primary and standby"
        )
    hosts[key] = (primary_host, standby_host)

if not primary.get("REDIS_CONN_STRING") or not standby.get("REDIS_CONN_STRING"):
    raise SystemExit("REDIS_CONN_STRING must be configured in both runtime env files")
primary_identity, primary_host = redis_identity(primary["REDIS_CONN_STRING"], primary_path)
standby_identity, standby_host = redis_identity(standby["REDIS_CONN_STRING"], standby_path)
if primary_identity != standby_identity:
    raise SystemExit(
        "REDIS_CONN_STRING credentials, port, database, or options differ between primary and standby"
    )
hosts["REDIS_CONN_STRING"] = (primary_host, standby_host)

loopback = {"127.0.0.1", "localhost", "::1"}
for key, (primary_host, standby_host) in hosts.items():
    if primary_host in loopback and standby_host in loopback:
        raise SystemExit(
            f"{key} points both nodes at loopback; standby must use the shared backend's private endpoint"
        )
PY
  log "Shared backend config verified: primary and standby use matching credentials and logical databases"
}

validate_two_server_config() {
  local name
  for name in \
    DEPLOY_PRIMARY_HOST DEPLOY_PRIMARY_APP_ENV_FILE DEPLOY_PRIMARY_NODE_TYPE \
    DEPLOY_PRIMARY_WIREGUARD_UPSTREAM DEPLOY_PRIMARY_CADDY_PROXY_SNIPPETS \
    DEPLOY_STANDBY_HOST DEPLOY_STANDBY_APP_ENV_FILE DEPLOY_STANDBY_NODE_TYPE \
    DEPLOY_STANDBY_WIREGUARD_UPSTREAM DEPLOY_STANDBY_CADDY_PROXY_SNIPPETS; do
    require_var "$name"
  done

  [[ "$DEPLOY_PRIMARY_HOST" != "$DEPLOY_STANDBY_HOST" ]] || \
    fail "Primary and standby hosts must be different"
  [[ "$DEPLOY_PRIMARY_WIREGUARD_UPSTREAM" != "$DEPLOY_STANDBY_WIREGUARD_UPSTREAM" ]] || \
    fail "Primary and standby WireGuard upstreams must be different"
  [[ "$DEPLOY_PRIMARY_APP_ENV_FILE" != "$DEPLOY_STANDBY_APP_ENV_FILE" ]] || \
    fail "Primary and standby app env files must be different"
  [[ "$DEPLOY_PRIMARY_NODE_TYPE" == "master" ]] || \
    fail "DEPLOY_PRIMARY_NODE_TYPE must be master"
  [[ "$DEPLOY_STANDBY_NODE_TYPE" == "slave" ]] || \
    fail "DEPLOY_STANDBY_NODE_TYPE must be slave"

  DEPLOY_PRIMARY_PORT="${DEPLOY_PRIMARY_PORT:-22}"
  DEPLOY_STANDBY_PORT="${DEPLOY_STANDBY_PORT:-22}"
  [[ "$DEPLOY_PRIMARY_PORT" =~ ^[0-9]+$ && "$DEPLOY_STANDBY_PORT" =~ ^[0-9]+$ ]] || \
    fail "Primary and standby SSH ports must be numeric"
  (( 10#$DEPLOY_PRIMARY_PORT >= 1 && 10#$DEPLOY_PRIMARY_PORT <= 65535 )) || \
    fail "DEPLOY_PRIMARY_PORT must be between 1 and 65535"
  (( 10#$DEPLOY_STANDBY_PORT >= 1 && 10#$DEPLOY_STANDBY_PORT <= 65535 )) || \
    fail "DEPLOY_STANDBY_PORT must be between 1 and 65535"
}

validate_single_server_config() {
  local name
  for name in DEPLOY_PRIMARY_HOST DEPLOY_PRIMARY_APP_ENV_FILE; do
    require_var "$name"
  done

  DEPLOY_PRIMARY_NODE_TYPE="${DEPLOY_PRIMARY_NODE_TYPE:-master}"
  [[ "$DEPLOY_PRIMARY_NODE_TYPE" == "master" ]] || \
    fail "DEPLOY_PRIMARY_NODE_TYPE must be master in single topology"

  DEPLOY_PRIMARY_PORT="${DEPLOY_PRIMARY_PORT:-22}"
  [[ "$DEPLOY_PRIMARY_PORT" =~ ^[0-9]+$ ]] || \
    fail "Single-server SSH port must be numeric"
  (( 10#$DEPLOY_PRIMARY_PORT >= 1 && 10#$DEPLOY_PRIMARY_PORT <= 65535 )) || \
    fail "DEPLOY_PRIMARY_PORT must be between 1 and 65535"
}

select_deploy_target() {
  DEPLOY_TOPOLOGY="${DEPLOY_TOPOLOGY:-multi}"

  case "$DEPLOY_TOPOLOGY" in
    single)
      validate_single_server_config
      case "${DEPLOY_TARGET:-primary}" in
        primary|"") DEPLOY_TARGET="primary" ;;
        *) fail "DEPLOY_TOPOLOGY=single only supports DEPLOY_TARGET=primary" ;;
      esac
      DEPLOY_HOST="$DEPLOY_PRIMARY_HOST"
      DEPLOY_PORT="$DEPLOY_PRIMARY_PORT"
      DEPLOY_USER="${DEPLOY_PRIMARY_USER:-root}"
      DEPLOY_PASSWORD="${DEPLOY_PRIMARY_PASSWORD:-}"
      DEPLOY_SSH_KEY="${DEPLOY_PRIMARY_SSH_KEY:-}"
      DEPLOY_APP_ENV_FILE="$DEPLOY_PRIMARY_APP_ENV_FILE"
      DEPLOY_EXPECT_NODE_TYPE="$DEPLOY_PRIMARY_NODE_TYPE"
      DEPLOY_CADDY_PROXY_SNIPPETS=""
      DEPLOY_PEER_UPSTREAM=""
      DEPLOY_EXPECT_PEER_NODE_TYPE=""
      ;;
    multi)
      validate_two_server_config
      case "${DEPLOY_TARGET:-primary}" in
        primary|"")
          DEPLOY_TARGET="primary"
          DEPLOY_HOST="$DEPLOY_PRIMARY_HOST"
          DEPLOY_PORT="$DEPLOY_PRIMARY_PORT"
          DEPLOY_USER="${DEPLOY_PRIMARY_USER:-root}"
          DEPLOY_PASSWORD="${DEPLOY_PRIMARY_PASSWORD:-}"
          DEPLOY_SSH_KEY="${DEPLOY_PRIMARY_SSH_KEY:-}"
          DEPLOY_APP_ENV_FILE="$DEPLOY_PRIMARY_APP_ENV_FILE"
          DEPLOY_CADDY_PROXY_SNIPPETS="$DEPLOY_PRIMARY_CADDY_PROXY_SNIPPETS"
          DEPLOY_EXPECT_NODE_TYPE="$DEPLOY_PRIMARY_NODE_TYPE"
          DEPLOY_PEER_UPSTREAM="$DEPLOY_STANDBY_WIREGUARD_UPSTREAM"
          DEPLOY_EXPECT_PEER_NODE_TYPE="$DEPLOY_STANDBY_NODE_TYPE"
          ;;
        standby)
          DEPLOY_TARGET="standby"
          DEPLOY_HOST="$DEPLOY_STANDBY_HOST"
          DEPLOY_PORT="$DEPLOY_STANDBY_PORT"
          DEPLOY_USER="${DEPLOY_STANDBY_USER:-root}"
          DEPLOY_PASSWORD="${DEPLOY_STANDBY_PASSWORD:-}"
          DEPLOY_SSH_KEY="${DEPLOY_STANDBY_SSH_KEY:-}"
          DEPLOY_APP_ENV_FILE="$DEPLOY_STANDBY_APP_ENV_FILE"
          DEPLOY_CADDY_PROXY_SNIPPETS="$DEPLOY_STANDBY_CADDY_PROXY_SNIPPETS"
          DEPLOY_EXPECT_NODE_TYPE="$DEPLOY_STANDBY_NODE_TYPE"
          DEPLOY_PEER_UPSTREAM="$DEPLOY_PRIMARY_WIREGUARD_UPSTREAM"
          DEPLOY_EXPECT_PEER_NODE_TYPE="$DEPLOY_PRIMARY_NODE_TYPE"
          ;;
        backup|main)
          fail "DEPLOY_TARGET=$DEPLOY_TARGET is ambiguous and no longer supported; use primary or standby"
          ;;
        *)
          fail "DEPLOY_TOPOLOGY=multi requires DEPLOY_TARGET=primary or standby"
          ;;
      esac
      ;;
    *)
      fail "Unsupported DEPLOY_TOPOLOGY: $DEPLOY_TOPOLOGY (expected single or multi)"
  esac
}

build_frontend() {
  local dir="$1"
  local extra_env="$2"

  log "Install frontend dependencies: $dir"
  (cd "$dir" && npx --yes bun install)

  log "Build frontend: $dir"
  (cd "$dir" && env $extra_env VITE_REACT_APP_VERSION="$APP_VERSION" npx --yes bun run build)
}

ssh_base_args=()
scp_base_args=()

remote_ssh() {
  if [[ -n "${DEPLOY_SSH_KEY:-}" ]]; then
    ssh "${ssh_base_args[@]}" -i "$DEPLOY_SSH_KEY" "$DEPLOY_USER@$DEPLOY_HOST" "$@"
  elif [[ -n "${DEPLOY_PASSWORD:-}" ]]; then
    SSHPASS="$DEPLOY_PASSWORD" sshpass -e ssh "${ssh_base_args[@]}" \
      -o PreferredAuthentications=password \
      -o PubkeyAuthentication=no \
      -o NumberOfPasswordPrompts=1 \
      "$DEPLOY_USER@$DEPLOY_HOST" "$@"
  else
    ssh "${ssh_base_args[@]}" "$DEPLOY_USER@$DEPLOY_HOST" "$@"
  fi
}

remote_scp() {
  if [[ -n "${DEPLOY_SSH_KEY:-}" ]]; then
    scp "${scp_base_args[@]}" -i "$DEPLOY_SSH_KEY" "$@"
  elif [[ -n "${DEPLOY_PASSWORD:-}" ]]; then
    SSHPASS="$DEPLOY_PASSWORD" sshpass -e scp "${scp_base_args[@]}" \
      -o PreferredAuthentications=password \
      -o PubkeyAuthentication=no \
      -o NumberOfPasswordPrompts=1 \
      "$@"
  else
    scp "${scp_base_args[@]}" "$@"
  fi
}

init_context() {
  # Explicit invocation-time overrides must win over values stored in the env
  # file. Rolling deployment relies on this when it invokes each target.
  local requested_topology="${DEPLOY_TOPOLOGY:-}"
  local requested_target="${DEPLOY_TARGET:-}"
  local requested_release_id="${DEPLOY_RELEASE_ID:-}"
  local requested_zero_downtime="${DEPLOY_ZERO_DOWNTIME:-}"
  local requested_allow_backend_config_change="${DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE:-}"

  load_env_file "$DEPLOY_ENV_FILE"

  if [[ -n "$requested_topology" ]]; then
    DEPLOY_TOPOLOGY="$requested_topology"
    # Do not carry a target from a different topology unless this invocation
    # explicitly selected one as well.
    [[ -n "$requested_target" ]] || unset DEPLOY_TARGET
  fi
  [[ -z "$requested_target" ]] || DEPLOY_TARGET="$requested_target"
  [[ -z "$requested_release_id" ]] || DEPLOY_RELEASE_ID="$requested_release_id"
  [[ -z "$requested_zero_downtime" ]] || DEPLOY_ZERO_DOWNTIME="$requested_zero_downtime"
  [[ -z "$requested_allow_backend_config_change" ]] || \
    DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE="$requested_allow_backend_config_change"

  select_deploy_target

  DEPLOY_PORT="${DEPLOY_PORT:-22}"
  DEPLOY_USER="${DEPLOY_USER:-root}"
  DEPLOY_REMOTE_DIR="${DEPLOY_REMOTE_DIR:-/www/wwwroot/modelsell}"
  DEPLOY_SERVICE_NAME="${DEPLOY_SERVICE_NAME:-modelsell}"
  DEPLOY_BINARY_NAME="${DEPLOY_BINARY_NAME:-new-api}"
  DEPLOY_APP_PORT="${DEPLOY_APP_PORT:-3000}"
  DEPLOY_APP_ENV_FILE="${DEPLOY_APP_ENV_FILE:-.env.production}"
  DEPLOY_GOOS="${DEPLOY_GOOS:-linux}"
  DEPLOY_GOARCH="${DEPLOY_GOARCH:-amd64}"
  DEPLOY_GOAMD64="${DEPLOY_GOAMD64:-v1}"
  DEPLOY_ARCH_LABEL="${DEPLOY_ARCH_LABEL:-x86_64}"
  DEPLOY_HEALTH_PATH="${DEPLOY_HEALTH_PATH:-/api/ready}"
  DEPLOY_HEALTH_TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-180}"
  DEPLOY_KEEP_RELEASES="${DEPLOY_KEEP_RELEASES:-5}"
  DEPLOY_SEO_VERIFY="${DEPLOY_SEO_VERIFY:-true}"
  DEPLOY_ZERO_DOWNTIME="${DEPLOY_ZERO_DOWNTIME:-auto}"
  DEPLOY_CADDY_CONFIG="${DEPLOY_CADDY_CONFIG:-/etc/caddy/Caddyfile}"
  DEPLOY_PEER_UPSTREAM="${DEPLOY_PEER_UPSTREAM:-}"
  DEPLOY_CADDY_PROXY_SNIPPETS="${DEPLOY_CADDY_PROXY_SNIPPETS:-}"
  DEPLOY_EXPECT_NODE_TYPE="${DEPLOY_EXPECT_NODE_TYPE:?missing expected node type}"
  DEPLOY_EXPECT_PEER_NODE_TYPE="${DEPLOY_EXPECT_PEER_NODE_TYPE:-}"
  DEPLOY_CADDY_HEALTH_HOST="${DEPLOY_CADDY_HEALTH_HOST:-modelsell.com}"
  DEPLOY_DRAIN_HEALTH_PATH="${DEPLOY_DRAIN_HEALTH_PATH:-/api/ready}"
  DEPLOY_DIRECT_PORT_DRAIN="${DEPLOY_DIRECT_PORT_DRAIN:-true}"
  DEPLOY_DIRECT_IPV6_DRAIN="${DEPLOY_DIRECT_IPV6_DRAIN:-true}"
  DEPLOY_IPV6_DRAIN_PROXY_PORT="${DEPLOY_IPV6_DRAIN_PROXY_PORT:-39002}"
  DEPLOY_PUBLIC_INTERFACE="${DEPLOY_PUBLIC_INTERFACE:-auto}"
  DEPLOY_WIREGUARD_INTERFACE="${DEPLOY_WIREGUARD_INTERFACE:-wg0}"
  DEPLOY_TRAFFIC_SETTLE_SECONDS="${DEPLOY_TRAFFIC_SETTLE_SECONDS:-3}"
  DEPLOY_PEER_STABLE_CHECKS="${DEPLOY_PEER_STABLE_CHECKS:-5}"
  DEPLOY_POST_START_STABLE_CHECKS="${DEPLOY_POST_START_STABLE_CHECKS:-5}"
  DEPLOY_POST_START_SOAK_SECONDS="${DEPLOY_POST_START_SOAK_SECONDS:-15}"
  DEPLOY_SHUTDOWN_TIMEOUT="${DEPLOY_SHUTDOWN_TIMEOUT:-900}"
  DEPLOY_STOP_TIMEOUT="${DEPLOY_STOP_TIMEOUT:-930}"
  DEPLOY_SMOKE_TESTS="${DEPLOY_SMOKE_TESTS:-true}"
  DEPLOY_STREAM_LOGS="${DEPLOY_STREAM_LOGS:-false}"
  DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE="${DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE:-false}"

  case "$DEPLOY_ZERO_DOWNTIME" in
    auto)
      if [[ "$DEPLOY_TOPOLOGY" == "multi" ]]; then
        DEPLOY_ZERO_DOWNTIME_ACTIVE=true
      else
        DEPLOY_ZERO_DOWNTIME_ACTIVE=false
      fi
      ;;
    true)
      [[ "$DEPLOY_TOPOLOGY" == "multi" ]] || \
        fail "DEPLOY_ZERO_DOWNTIME=true requires DEPLOY_TOPOLOGY=multi"
      DEPLOY_ZERO_DOWNTIME_ACTIVE=true
      ;;
    false)
      DEPLOY_ZERO_DOWNTIME_ACTIVE="$DEPLOY_ZERO_DOWNTIME"
      ;;
    *)
      fail "Unsupported DEPLOY_ZERO_DOWNTIME: $DEPLOY_ZERO_DOWNTIME (expected auto, true, or false)"
      ;;
  esac
  case "$DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE" in
    true|false) ;;
    *) fail "DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE must be true or false" ;;
  esac

  require_var DEPLOY_HOST
  require_var DEPLOY_REMOTE_DIR
  require_var DEPLOY_SERVICE_NAME
  require_var DEPLOY_BINARY_NAME
  require_var DEPLOY_APP_PORT
  require_var DEPLOY_GOOS
  require_var DEPLOY_GOARCH
  require_var DEPLOY_ARCH_LABEL

  APP_ENV_PATH="$(resolve_app_env_path "$DEPLOY_APP_ENV_FILE")"
  BUILD_DIR="$ROOT_DIR/build/modelsell"
  ARCHIVE_NAME="${DEPLOY_BINARY_NAME}-${DEPLOY_GOOS}-${DEPLOY_ARCH_LABEL}.tar.gz"
  ARCHIVE_PATH="$BUILD_DIR/$ARCHIVE_NAME"
  BINARY_PATH="$BUILD_DIR/$DEPLOY_BINARY_NAME"
  REMOTE_TMP_DIR="/tmp/${DEPLOY_SERVICE_NAME}-deploy"
  REMOTE_ARCHIVE="$REMOTE_TMP_DIR/$ARCHIVE_NAME"
  REMOTE_ENV="$REMOTE_TMP_DIR/.env"

  ssh_base_args=(-p "$DEPLOY_PORT" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -o ServerAliveInterval=10 -o ServerAliveCountMax=12 -o TCPKeepAlive=yes)
  scp_base_args=(-P "$DEPLOY_PORT" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -o ServerAliveInterval=10 -o ServerAliveCountMax=12 -o TCPKeepAlive=yes)

  VERSION_FILE="$ROOT_DIR/VERSION"
  APP_VERSION="$(cat "$VERSION_FILE" 2>/dev/null || true)"
  if [[ -z "$APP_VERSION" ]]; then
    APP_VERSION="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo dev)"
  fi
  APP_VERSION_SAFE="$(printf '%s' "$APP_VERSION" | tr -c 'A-Za-z0-9._-' '_')"
  DEPLOY_RELEASE_ID="${DEPLOY_RELEASE_ID:-$(date -u +%Y%m%d%H%M%S)-$APP_VERSION_SAFE}"
  [[ "$DEPLOY_RELEASE_ID" =~ ^[A-Za-z0-9._-]+$ ]] || fail "Invalid DEPLOY_RELEASE_ID: $DEPLOY_RELEASE_ID"

  log "Deployment topology: $DEPLOY_TOPOLOGY; target: $DEPLOY_TARGET ($DEPLOY_USER@$DEPLOY_HOST:$DEPLOY_PORT), zero-downtime=$DEPLOY_ZERO_DOWNTIME_ACTIVE"
}

require_local_build_tools() {
  require_cmd npx
  require_cmd go
  require_cmd tar
}

require_remote_tools() {
  require_cmd ssh
  require_cmd scp

  if [[ -n "${DEPLOY_PASSWORD:-}" && -z "${DEPLOY_SSH_KEY:-}" ]]; then
    require_cmd sshpass
  fi
}

require_app_env() {
  [[ -f "$APP_ENV_PATH" ]] || fail "Missing app env file: $APP_ENV_PATH. Copy .env.production.example to $DEPLOY_APP_ENV_FILE first."
  validate_app_env_role "$APP_ENV_PATH" "$DEPLOY_EXPECT_NODE_TYPE"
  validate_shared_backend_config
}

require_artifact() {
  [[ -f "$ARCHIVE_PATH" ]] || fail "Missing artifact: $ARCHIVE_PATH. Run option 2 first, or use option 1."
}

build_package() {
  require_local_build_tools

  rm -rf "$BUILD_DIR"
  mkdir -p "$BUILD_DIR"

  build_frontend "$ROOT_DIR/web/default" "DISABLE_ESLINT_PLUGIN=true"
  build_frontend "$ROOT_DIR/web/classic" ""

  log "Build binary: GOOS=$DEPLOY_GOOS GOARCH=$DEPLOY_GOARCH GOAMD64=${DEPLOY_GOAMD64:-}"
  (
    cd "$ROOT_DIR"
    build_env=(CGO_ENABLED=0 GOOS="$DEPLOY_GOOS" GOARCH="$DEPLOY_GOARCH")
    if [[ "$DEPLOY_GOARCH" == "amd64" && -n "${DEPLOY_GOAMD64:-}" ]]; then
      build_env+=(GOAMD64="$DEPLOY_GOAMD64")
    fi
    env "${build_env[@]}" go build \
      -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$APP_VERSION'" \
      -o "$BINARY_PATH"
  )
  file "$BINARY_PATH"

  install -m 0755 "$ROOT_DIR/scripts/start-modelsell.sh" "$BUILD_DIR/start-modelsell.sh"
  install -m 0755 "$ROOT_DIR/scripts/rollback-modelsell.sh" "$BUILD_DIR/rollback-modelsell.sh"
  install -m 0755 "$ROOT_DIR/scripts/verify-modelsell-seo.sh" "$BUILD_DIR/verify-modelsell-seo.sh"

  log "Package artifact: $ARCHIVE_PATH"
  (
    cd "$BUILD_DIR"
    COPYFILE_DISABLE=1 tar --no-xattrs -czf "$ARCHIVE_NAME" \
      "$DEPLOY_BINARY_NAME" \
      start-modelsell.sh \
      rollback-modelsell.sh \
      verify-modelsell-seo.sh
  )
}

prepare_remote() {
  require_remote_tools
  log "Prepare remote directory: $DEPLOY_USER@$DEPLOY_HOST:$DEPLOY_REMOTE_DIR"
  remote_ssh "mkdir -p '$REMOTE_TMP_DIR' '$DEPLOY_REMOTE_DIR' '$DEPLOY_REMOTE_DIR/logs'"
}

upload_artifact() {
  require_artifact
  prepare_remote

  log "Upload package: $ARCHIVE_NAME"
  remote_scp "$ARCHIVE_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ARCHIVE"

  log "Install release and restart service with health-check rollback: $DEPLOY_SERVICE_NAME"
  run_remote_deploy 1 0
}

update_remote_config() {
  require_remote_tools
  require_app_env
  prepare_remote

  log "Upload runtime env: $DEPLOY_APP_ENV_FILE -> $DEPLOY_REMOTE_DIR/.env"
  remote_scp "$APP_ENV_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ENV"

  log "Install systemd config and restart service with health-check rollback: $DEPLOY_SERVICE_NAME"
  run_remote_deploy 0 1
}

run_remote_deploy() {
  local apply_release="$1"
  local install_env="$2"

  remote_ssh "REMOTE_DIR='$DEPLOY_REMOTE_DIR' SERVICE_NAME='$DEPLOY_SERVICE_NAME' BINARY_NAME='$DEPLOY_BINARY_NAME' APP_PORT='$DEPLOY_APP_PORT' ARCHIVE_PATH='$REMOTE_ARCHIVE' ENV_PATH='$REMOTE_ENV' RELEASE_ID='$DEPLOY_RELEASE_ID' APPLY_RELEASE='$apply_release' INSTALL_ENV='$install_env' HEALTH_PATH='$DEPLOY_HEALTH_PATH' HEALTH_TIMEOUT='$DEPLOY_HEALTH_TIMEOUT' KEEP_RELEASES='$DEPLOY_KEEP_RELEASES' SEO_VERIFY='$DEPLOY_SEO_VERIFY' ZERO_DOWNTIME='$DEPLOY_ZERO_DOWNTIME_ACTIVE' CADDY_CONFIG='$DEPLOY_CADDY_CONFIG' CADDY_PROXY_SNIPPETS='$DEPLOY_CADDY_PROXY_SNIPPETS' PEER_UPSTREAM='$DEPLOY_PEER_UPSTREAM' CADDY_HEALTH_HOST='$DEPLOY_CADDY_HEALTH_HOST' DRAIN_HEALTH_PATH='$DEPLOY_DRAIN_HEALTH_PATH' DIRECT_PORT_DRAIN='$DEPLOY_DIRECT_PORT_DRAIN' DIRECT_IPV6_DRAIN='$DEPLOY_DIRECT_IPV6_DRAIN' IPV6_DRAIN_PROXY_PORT='$DEPLOY_IPV6_DRAIN_PROXY_PORT' PUBLIC_INTERFACE='$DEPLOY_PUBLIC_INTERFACE' WIREGUARD_INTERFACE='$DEPLOY_WIREGUARD_INTERFACE' TRAFFIC_SETTLE_SECONDS='$DEPLOY_TRAFFIC_SETTLE_SECONDS' PEER_STABLE_CHECKS='$DEPLOY_PEER_STABLE_CHECKS' POST_START_STABLE_CHECKS='$DEPLOY_POST_START_STABLE_CHECKS' POST_START_SOAK_SECONDS='$DEPLOY_POST_START_SOAK_SECONDS' SHUTDOWN_TIMEOUT='$DEPLOY_SHUTDOWN_TIMEOUT' STOP_TIMEOUT='$DEPLOY_STOP_TIMEOUT' EXPECT_NODE_TYPE='$DEPLOY_EXPECT_NODE_TYPE' EXPECT_PEER_NODE_TYPE='$DEPLOY_EXPECT_PEER_NODE_TYPE' SMOKE_TESTS='$DEPLOY_SMOKE_TESTS' STREAM_LOGS='$DEPLOY_STREAM_LOGS' ALLOW_BACKEND_CONFIG_CHANGE='$DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE' flock -n '/run/lock/${DEPLOY_SERVICE_NAME}-deploy.lock' bash -s" <<'REMOTE_SCRIPT'
set -Eeuo pipefail

RELEASES_DIR="$REMOTE_DIR/releases"
CURRENT_LINK="$REMOTE_DIR/current"
RELEASE_DIR="$RELEASES_DIR/$RELEASE_ID"
STAGING_RELEASE_DIR="$RELEASES_DIR/.${RELEASE_ID}.staging.$$"
ENV_BACKUP=""
JOURNAL_PID=""
TRAFFIC_DRAINED=false
DIRECT_TRAFFIC_DRAINED=false
DIRECT_IPV6_TRAFFIC_DRAINED=false
IPV6_DRAIN_REQUIRED=false
CADDY_BACKUP=""
CADDY_DRAIN_CANDIDATE=""
DRAIN_STATE_PHASE=""
DRAIN_STATE_FILE="$REMOTE_DIR/ha/active-deploy-drain"

log() {
  printf '[remote-deploy] %s\n' "$*"
}

warn() {
  printf '[remote-deploy:warn] %s\n' "$*" >&2
}

show_service_debug() {
  systemctl --no-pager --full status "$SERVICE_NAME" || true
  journalctl -u "$SERVICE_NAME" -n 100 --no-pager || true
}

stop_service_log_stream() {
  [[ -n "$JOURNAL_PID" ]] || return 0
  kill "$JOURNAL_PID" 2>/dev/null || true
  wait "$JOURNAL_PID" 2>/dev/null || true
  JOURNAL_PID=""
}

cleanup_remote_processes() {
  stop_service_log_stream
  if [[ -n "${STAGING_RELEASE_DIR:-}" && -d "$STAGING_RELEASE_DIR" ]]; then
    rm -rf -- "$STAGING_RELEASE_DIR"
  fi
}

start_service_log_stream() {
  [[ "$STREAM_LOGS" == "true" ]] || return 0
  log "Live service log started (service=$SERVICE_NAME)"
  journalctl -u "$SERVICE_NAME" --since now --follow --no-pager -n 0 -o short-iso &
  JOURNAL_PID=$!
}

trap cleanup_remote_processes EXIT
trap 'cleanup_remote_processes; exit 130' HUP INT TERM

install_service_config() {
  cat >"/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=ModelSell API Service
After=network-online.target mysql.service mysqld.service redis.service redis-server.service
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${REMOTE_DIR}
EnvironmentFile=${REMOTE_DIR}/.env
Environment=SHUTDOWN_TIMEOUT_SECONDS=${SHUTDOWN_TIMEOUT}
ExecStart=${REMOTE_DIR}/current/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs
Restart=always
RestartSec=5
KillMode=control-group
KillSignal=SIGTERM
TimeoutStopSec=${STOP_TIMEOUT}s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME"
}

validate_rollout_settings() {
  local name value
  for name in TRAFFIC_SETTLE_SECONDS PEER_STABLE_CHECKS POST_START_STABLE_CHECKS POST_START_SOAK_SECONDS SHUTDOWN_TIMEOUT STOP_TIMEOUT IPV6_DRAIN_PROXY_PORT; do
    value="${!name}"
    if ! [[ "$value" =~ ^[0-9]+$ ]]; then
      warn "Invalid numeric rollout setting: $name=$value"
      return 1
    fi
  done
  if (( STOP_TIMEOUT <= SHUTDOWN_TIMEOUT )); then
    warn "STOP_TIMEOUT must be greater than SHUTDOWN_TIMEOUT"
    return 1
  fi
  if (( PEER_STABLE_CHECKS < 1 || POST_START_STABLE_CHECKS < 1 )); then
    warn "Stable health-check counts must be at least 1"
    return 1
  fi
  case "$EXPECT_NODE_TYPE" in
    master|slave) ;;
    *)
      warn "Invalid expected node type: $EXPECT_NODE_TYPE"
      return 1
      ;;
  esac
  if [[ "$ZERO_DOWNTIME" == "true" ]]; then
    [[ -n "$PEER_UPSTREAM" && -n "$CADDY_PROXY_SNIPPETS" ]] || {
      warn "Multi-machine zero-downtime deployment requires peer and Caddy routing configuration"
      return 1
    }
    case "$EXPECT_PEER_NODE_TYPE" in
      master|slave) ;;
      *)
        warn "Invalid expected peer node type: $EXPECT_PEER_NODE_TYPE"
        return 1
        ;;
    esac
    if [[ "$EXPECT_PEER_NODE_TYPE" == "$EXPECT_NODE_TYPE" ]]; then
      warn "Local and peer node roles must differ: $EXPECT_NODE_TYPE"
      return 1
    fi
  fi
  case "$DIRECT_PORT_DRAIN" in
    true|false) ;;
    *)
      warn "Invalid direct-port drain setting: $DIRECT_PORT_DRAIN"
      return 1
      ;;
  esac
  case "$DIRECT_IPV6_DRAIN" in
    true|false) ;;
    *)
      warn "Invalid IPv6 direct-port drain setting: $DIRECT_IPV6_DRAIN"
      return 1
      ;;
  esac
  case "$ALLOW_BACKEND_CONFIG_CHANGE" in
    true|false) ;;
    *)
      warn "Invalid backend config change setting: $ALLOW_BACKEND_CONFIG_CHANGE"
      return 1
      ;;
  esac
  if (( IPV6_DRAIN_PROXY_PORT < 1024 || IPV6_DRAIN_PROXY_PORT > 65535 || IPV6_DRAIN_PROXY_PORT == APP_PORT )); then
    warn "Invalid IPv6 drain proxy port: $IPV6_DRAIN_PROXY_PORT"
    return 1
  fi
}

read_node_type() {
  local env_file="$1"
  local node_type
  node_type="$(sed -n 's/^[[:space:]]*NODE_TYPE[[:space:]]*=[[:space:]]*//p' "$env_file" 2>/dev/null | tail -1 | tr -d '\"'\''[:space:]')"
  printf '%s' "${node_type:-master}"
}

verify_node_role() {
  local active_role candidate_role
  if [[ -f "$REMOTE_DIR/.env" ]]; then
    active_role="$(read_node_type "$REMOTE_DIR/.env")"
    if [[ "$active_role" != "$EXPECT_NODE_TYPE" ]]; then
      warn "Active node role mismatch: expected=$EXPECT_NODE_TYPE actual=$active_role"
      return 1
    fi
  fi
  if [[ "$INSTALL_ENV" == "1" ]]; then
    candidate_role="$(read_node_type "$ENV_PATH")"
    if [[ "$candidate_role" != "$EXPECT_NODE_TYPE" ]]; then
      warn "Candidate node role mismatch: expected=$EXPECT_NODE_TYPE actual=$candidate_role"
      return 1
    fi
  fi
  log "Node role verified: $EXPECT_NODE_TYPE"
}

verify_backend_config_change() {
  [[ "$INSTALL_ENV" == "1" ]] || return 0
  [[ -f "$REMOTE_DIR/.env" ]] || return 0
  local changed_keys
  changed_keys="$(python3 - "$REMOTE_DIR/.env" "$ENV_PATH" <<'PY'
import sys
from pathlib import Path


def load_env(path):
    values = {}
    for raw in Path(path).read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
            value = value[1:-1]
        values[key.strip()] = value
    return values


active = load_env(sys.argv[1])
candidate = load_env(sys.argv[2])
keys = ("SQL_DSN", "LOG_SQL_DSN", "REDIS_CONN_STRING")
print(",".join(key for key in keys if active.get(key) != candidate.get(key)))
PY
)"
  if [[ -n "$changed_keys" && "$ALLOW_BACKEND_CONFIG_CHANGE" != "true" ]]; then
    warn "Candidate changes protected backend settings: $changed_keys"
    warn "Refusing to stop the service. Verify the new backends separately, then set DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE=true for an intentional config rollout"
    return 1
  fi
  if [[ -n "$changed_keys" ]]; then
    warn "Intentional protected backend config change allowed: $changed_keys"
  else
    log "Protected backend settings match the active runtime env"
  fi
}

normalize_health_path() {
  if [[ "$HEALTH_PATH" != /* ]]; then
    HEALTH_PATH="/$HEALTH_PATH"
  fi
}

readiness_summary() {
  local body_file="$1"
  python3 - "$body_file" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], "r", encoding="utf-8") as handle:
        payload = json.load(handle)
except (OSError, ValueError):
    print("payload=invalid")
    raise SystemExit(0)

data = payload.get("data") or {}
checks = data.get("checks") or {}
parts = [
    f"success={str(payload.get('success')).lower()}",
    f"role={data.get('node_type') or 'unknown'}",
    f"version={data.get('version') or 'unknown'}",
]
parts.extend(f"{name}={checks.get(name) or 'missing'}" for name in ("database", "log_database", "redis"))
print(" ".join(parts))
PY
}

wait_service_healthy() {
  normalize_health_path
  local url="http://127.0.0.1:${APP_PORT}${HEALTH_PATH}"
  local deadline=$((SECONDS + HEALTH_TIMEOUT))
  local started_at=$SECONDS
  local stable=0
  local body_file last_summary=""
  body_file="$(mktemp /tmp/modelsell-local-ready.XXXXXX)"

  while (( SECONDS < deadline )); do
    local state pid http_code summary payload_ok=false
    state="$(systemctl is-active "$SERVICE_NAME" 2>/dev/null || true)"
    pid="$(systemctl show "$SERVICE_NAME" -p MainPID --value 2>/dev/null || true)"
    : >"$body_file"
    http_code="$(curl -sS --max-time 3 -o "$body_file" -w '%{http_code}' "$url" 2>/dev/null || true)"
    summary="$(readiness_summary "$body_file")"
    if [[ "$summary" != "$last_summary" ]]; then
      log "Readiness detail: $summary"
      last_summary="$summary"
    fi
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]] && verify_ready_payload "$body_file" "$EXPECT_NODE_TYPE" >/dev/null 2>&1; then
      payload_ok=true
    fi
    log "Health check: elapsed=$((SECONDS - started_at))s state=${state:-unknown} pid=${pid:-0} http=${http_code:-000} stable=$stable/$POST_START_STABLE_CHECKS"
    if [[ "$state" == "active" && "$pid" =~ ^[1-9][0-9]*$ && "$payload_ok" == "true" ]]; then
      stable=$((stable + 1))
      if (( stable >= POST_START_STABLE_CHECKS )); then
        log "Health check passed: $url"
        rm -f "$body_file"
        return 0
      fi
    else
      stable=0
    fi
    sleep 1
  done

  warn "Health check failed: $url"
  warn "Last readiness detail: ${last_summary:-unavailable}"
  rm -f "$body_file"
  return 1
}

verify_service_soak() {
  local restart_baseline="$1"
  local expected_pid current_pid current_restarts state code
  local deadline=$((SECONDS + POST_START_SOAK_SECONDS))
  local url="http://127.0.0.1:${APP_PORT}${HEALTH_PATH}"

  expected_pid="$(systemctl show "$SERVICE_NAME" -p MainPID --value 2>/dev/null || true)"
  [[ "$expected_pid" =~ ^[1-9][0-9]*$ ]] || {
    warn "Cannot begin post-start soak without a running PID"
    return 1
  }

  while :; do
    state="$(systemctl is-active "$SERVICE_NAME" 2>/dev/null || true)"
    current_pid="$(systemctl show "$SERVICE_NAME" -p MainPID --value 2>/dev/null || true)"
    current_restarts="$(systemctl show "$SERVICE_NAME" -p NRestarts --value 2>/dev/null || true)"
    code="$(curl -sS --max-time 3 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [[ "$state" != "active" || "$current_pid" != "$expected_pid" ||
          "$current_restarts" != "$restart_baseline" || ! "$code" =~ ^2[0-9][0-9]$ ]]; then
      warn "Post-start soak failed: state=${state:-unknown} pid=${current_pid:-0}/$expected_pid restarts=${current_restarts:-unknown}/$restart_baseline http=${code:-000}"
      return 1
    fi
    (( SECONDS >= deadline )) && break
    sleep 1
  done
  log "Post-start soak passed: pid=$expected_pid restarts=$current_restarts duration=${POST_START_SOAK_SECONDS}s"
}

verify_seo() {
  [[ "$SEO_VERIFY" == "true" ]] || return 0
  local verifier="$CURRENT_LINK/verify-modelsell-seo.sh"
  [[ -x "$verifier" ]] || {
    warn "SEO verifier is missing: $verifier"
    return 1
  }
  log "SEO verification: origin=http://127.0.0.1:${APP_PORT}"
  "$verifier" "http://127.0.0.1:${APP_PORT}"
}

verify_smoke_tests() {
  [[ "$SMOKE_TESTS" == "true" ]] || return 0

  local origin="http://127.0.0.1:${APP_PORT}"
  local code

  code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "$origin/api/ready" 2>/dev/null || true)"
  [[ "$code" =~ ^2[0-9][0-9]$ ]] || {
    warn "Smoke test failed: /api/ready returned ${code:-000}"
    return 1
  }

  code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "$origin/api/status" 2>/dev/null || true)"
  [[ "$code" =~ ^2[0-9][0-9]$ ]] || {
    warn "Smoke test failed: /api/status returned ${code:-000}"
    return 1
  }

  code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "$origin/v1/models" 2>/dev/null || true)"
  [[ "$code" == "401" ]] || {
    warn "Smoke test failed: unauthenticated /v1/models returned ${code:-000}, expected 401"
    return 1
  }

  code="$(curl -sS --max-time 5 -H "Host: $CADDY_HEALTH_HOST" -H 'X-Forwarded-Proto: https' -o /dev/null -w '%{http_code}' "$origin/" 2>/dev/null || true)"
  [[ "$code" =~ ^[23][0-9][0-9]$ ]] || {
    warn "Smoke test failed: frontend root returned ${code:-000}"
    return 1
  }

  log "Smoke tests passed: ready/status=2xx, root=2xx/3xx, unauthenticated models=401"
}

render_caddy_upstreams() {
  local mode="$1"
  local input="$2"
  local output="$3"

  python3 - "$input" "$output" "$mode" "$APP_PORT" "$PEER_UPSTREAM" "$CADDY_PROXY_SNIPPETS" "$IPV6_DRAIN_REQUIRED" "$IPV6_DRAIN_PROXY_PORT" <<'PY'
from pathlib import Path
import re
import sys

(
    source_path,
    output_path,
    mode,
    app_port,
    peer,
    snippet_list,
    ipv6_drain,
    ipv6_proxy_port,
) = sys.argv[1:]
source = Path(source_path).read_text()
lines = source.splitlines(keepends=True)

if mode == "peer":
    upstreams = peer
elif mode == "local":
    upstreams = f"127.0.0.1:{app_port} {peer}"
else:
    raise SystemExit(f"unsupported traffic mode: {mode}")

snippets = [item.strip() for item in re.split(r"[ ,]+", snippet_list) if item.strip()]
if not snippets:
    raise SystemExit("no Caddy proxy snippets configured")

for snippet in snippets:
    start = next(
        (index for index, line in enumerate(lines) if line.strip() == f"({snippet}) {{"),
        None,
    )
    if start is None:
        raise SystemExit(f"missing Caddy snippet: {snippet}")

    depth = 0
    end = None
    for index in range(start, len(lines)):
        depth += lines[index].count("{") - lines[index].count("}")
        if depth == 0:
            end = index + 1
            break
    if end is None:
        raise SystemExit(f"unterminated Caddy snippet: {snippet}")

    block = "".join(lines[start:end])
    updated, replacements = re.subn(
        r"(?m)^(\s*)reverse_proxy\s+[^\n{]+\s*\{",
        rf"\1reverse_proxy {upstreams} {{",
        block,
        count=1,
    )
    if replacements != 1:
        raise SystemExit(f"unexpected reverse_proxy shape in: {snippet}")
    lines[start:end] = [updated]

rendered = "".join(lines)
local_ask = rf"(?m)^(\s*ask\s+)http://(?:127\.0\.0\.1|localhost):{re.escape(app_port)}([^\s]*)[ \t]*$"
peer_ask = rf"(?m)^(\s*ask\s+)http://{re.escape(peer)}([^\s]*)[ \t]*$"
if mode == "peer":
    rendered = re.sub(local_ask, rf"\1http://{peer}\2", rendered)
else:
    rendered = re.sub(peer_ask, rf"\1http://127.0.0.1:{app_port}\2", rendered)
lines = rendered.splitlines(keepends=True)

if mode == "peer" and ipv6_drain == "true":
    if lines and not lines[-1].endswith("\n"):
        lines[-1] += "\n"
    lines.append(
        "\n"
        "# Temporary listener used only while direct IPv6 :"
        f"{app_port} traffic is drained.\n"
        f"http://:{ipv6_proxy_port} {{\n"
        "\tbind ::\n"
        f"\treverse_proxy {peer}\n"
        "}\n"
    )

Path(output_path).write_text("".join(lines))
PY
}

verify_ready_payload() {
  local body_file="$1"
  local expected_role="$2"

  python3 - "$body_file" "$expected_role" <<'PY'
import json
import sys

body_file, expected_role = sys.argv[1:]
try:
    with open(body_file, "r", encoding="utf-8") as handle:
        payload = json.load(handle)
except (OSError, ValueError) as exc:
    raise SystemExit(f"invalid readiness payload: {exc}")

data = payload.get("data") or {}
checks = data.get("checks") or {}
if data.get("node_type") != expected_role:
    raise SystemExit(
        f"unexpected node_type: expected={expected_role} actual={data.get('node_type')}"
    )
for name in ("database", "log_database", "redis"):
    if checks.get(name) != "ok":
        raise SystemExit(f"readiness dependency is not ok: {name}={checks.get(name)}")
if payload.get("success") is not True:
    raise SystemExit("readiness success is not true")
PY
}

verify_caddy_health() {
  local expected_role="${1:-}"
  local body_file code
  [[ "$DRAIN_HEALTH_PATH" == /* ]] || DRAIN_HEALTH_PATH="/$DRAIN_HEALTH_PATH"
  body_file="$(mktemp /tmp/modelsell-caddy-ready.XXXXXX)"
  code="$(curl -ksS --max-time 8 --resolve "${CADDY_HEALTH_HOST}:443:127.0.0.1" -o "$body_file" -w '%{http_code}' "https://${CADDY_HEALTH_HOST}${DRAIN_HEALTH_PATH}" 2>/dev/null || true)"
  if ! [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
    rm -f "$body_file"
    warn "Caddy health check failed: host=$CADDY_HEALTH_HOST http=${code:-000}"
    return 1
  fi
  if [[ -n "$expected_role" ]] && ! verify_ready_payload "$body_file" "$expected_role"; then
    rm -f "$body_file"
    warn "Caddy readiness reached the wrong node or an unhealthy dependency: expected=$expected_role"
    return 1
  fi
  rm -f "$body_file"
}

verify_peer_once() {
  local body_file code
  [[ "$DRAIN_HEALTH_PATH" == /* ]] || DRAIN_HEALTH_PATH="/$DRAIN_HEALTH_PATH"
  body_file="$(mktemp /tmp/modelsell-peer-ready.XXXXXX)"
  code="$(curl -sS --max-time 5 -o "$body_file" -w '%{http_code}' "http://${PEER_UPSTREAM}${DRAIN_HEALTH_PATH}" 2>/dev/null || true)"
  if ! [[ "$code" =~ ^2[0-9][0-9]$ ]] || ! verify_ready_payload "$body_file" "$EXPECT_PEER_NODE_TYPE"; then
    rm -f "$body_file"
    warn "Peer readiness failed: upstream=$PEER_UPSTREAM expected_role=$EXPECT_PEER_NODE_TYPE http=${code:-000}"
    return 1
  fi
  rm -f "$body_file"
}

verify_peer_stable() {
  local stable=0
  while (( stable < PEER_STABLE_CHECKS )); do
    verify_peer_once || return 1
    stable=$((stable + 1))
    log "Peer health: role=$EXPECT_PEER_NODE_TYPE stable=$stable/$PEER_STABLE_CHECKS"
    sleep 1
  done
}

write_drain_state() {
  local phase="$1"
  local state_tmp
  state_tmp="$(mktemp "$REMOTE_DIR/ha/.active-deploy-drain.XXXXXX")"
  {
    printf 'version=2\n'
    printf 'phase=%s\n' "$phase"
    printf 'backup=%s\n' "$CADDY_BACKUP"
    printf 'candidate=%s\n' "$CADDY_DRAIN_CANDIDATE"
    # Keep this legacy key name so an interrupted version-2 transaction remains resumable.
    printf 'standby=%s\n' "$PEER_UPSTREAM"
    printf 'snippets=%s\n' "$CADDY_PROXY_SNIPPETS"
    printf 'app_port=%s\n' "$APP_PORT"
    printf 'previous_target=%s\n' "$PREVIOUS_TARGET"
    printf 'env_backup=%s\n' "$ENV_BACKUP"
    printf 'apply_release=%s\n' "$APPLY_RELEASE"
    printf 'install_env=%s\n' "$INSTALL_ENV"
    printf 'ipv6_drain_required=%s\n' "$IPV6_DRAIN_REQUIRED"
    printf 'ipv6_proxy_port=%s\n' "$IPV6_DRAIN_PROXY_PORT"
  } >"$state_tmp"
  chmod 600 "$state_tmp"
  mv -f "$state_tmp" "$DRAIN_STATE_FILE"
  DRAIN_STATE_PHASE="$phase"
}

read_drain_state_value() {
  local key="$1"
  sed -n "s/^${key}=//p" "$DRAIN_STATE_FILE" | tail -1
}

load_drain_state() {
  local version state_peer state_snippets state_port state_apply_release state_install_env state_ipv6_required state_ipv6_port
  version="$(read_drain_state_value version)"

  if [[ -z "$version" ]]; then
    warn "Legacy drain state cannot prove the original release and environment; refusing automatic recovery"
    return 1
  fi

  [[ "$version" == "2" ]] || {
    warn "Unsupported drain state version: $version"
    return 1
  }
  DRAIN_STATE_PHASE="$(read_drain_state_value phase)"
  CADDY_BACKUP="$(read_drain_state_value backup)"
  CADDY_DRAIN_CANDIDATE="$(read_drain_state_value candidate)"
  # The persisted key stays named "standby" for compatibility with an
  # interrupted transaction created by the previous script version.
  state_peer="$(read_drain_state_value standby)"
  state_snippets="$(read_drain_state_value snippets)"
  state_port="$(read_drain_state_value app_port)"
  PREVIOUS_TARGET="$(read_drain_state_value previous_target)"
  ENV_BACKUP="$(read_drain_state_value env_backup)"
  state_apply_release="$(read_drain_state_value apply_release)"
  state_install_env="$(read_drain_state_value install_env)"
  state_ipv6_required="$(read_drain_state_value ipv6_drain_required)"
  state_ipv6_port="$(read_drain_state_value ipv6_proxy_port)"

  case "$DRAIN_STATE_PHASE" in prepared|caddy|drained) ;; *)
    warn "Invalid drain state phase: $DRAIN_STATE_PHASE"
    return 1
  esac
  case "$CADDY_BACKUP" in "$REMOTE_DIR"/ha/*) ;; *)
    warn "Drain state contains an unsafe backup path"
    return 1
  esac
  case "$CADDY_DRAIN_CANDIDATE" in "$REMOTE_DIR"/ha/*) ;; *)
    warn "Drain state contains an unsafe candidate path"
    return 1
  esac
  [[ -f "$CADDY_BACKUP" && -f "$CADDY_DRAIN_CANDIDATE" ]] || {
    warn "Drain state is missing its Caddy backup or candidate"
    return 1
  }
  if [[ "$state_peer" != "$PEER_UPSTREAM" ||
        "$state_snippets" != "$CADDY_PROXY_SNIPPETS" ||
        "$state_port" != "$APP_PORT" ||
        "$state_apply_release" != "$APPLY_RELEASE" ||
        "$state_install_env" != "$INSTALL_ENV" ||
        "$state_ipv6_required" != "$IPV6_DRAIN_REQUIRED" ||
        "$state_ipv6_port" != "$IPV6_DRAIN_PROXY_PORT" ]]; then
    warn "Drain state does not match the current target; traffic remains unchanged"
    return 1
  fi
  if [[ -n "$PREVIOUS_TARGET" ]]; then
    case "$PREVIOUS_TARGET" in "$RELEASES_DIR"/*) ;; *)
      warn "Drain state contains an unsafe previous release path"
      return 1
    esac
    [[ -d "$PREVIOUS_TARGET" ]] || {
      warn "Drain state previous release no longer exists: $PREVIOUS_TARGET"
      return 1
    }
  fi
  if [[ -n "$ENV_BACKUP" ]]; then
    case "$ENV_BACKUP" in "$REMOTE_DIR"/.env.rollback.*) ;; *)
      warn "Drain state contains an unsafe environment backup path"
      return 1
    esac
    [[ -f "$ENV_BACKUP" ]] || {
      warn "Drain state environment backup no longer exists: $ENV_BACKUP"
      return 1
    }
  fi
}

prepare_drain_state() {
  mkdir -p "$REMOTE_DIR/ha"
  if [[ -s "$DRAIN_STATE_FILE" ]]; then
    load_drain_state || return 1
    log "Resuming interrupted drain: phase=$DRAIN_STATE_PHASE backup=$CADDY_BACKUP"
    return 0
  fi

  CADDY_BACKUP="$REMOTE_DIR/ha/Caddyfile.before-deploy-$RELEASE_ID"
  CADDY_DRAIN_CANDIDATE="$REMOTE_DIR/ha/Caddyfile.during-deploy-$RELEASE_ID"
  install -m 0600 "$CADDY_CONFIG" "$CADDY_BACKUP"
  render_caddy_upstreams peer "$CADDY_BACKUP" "$CADDY_DRAIN_CANDIDATE" || return 1
  chmod 600 "$CADDY_DRAIN_CANDIDATE"
  caddy validate --config "$CADDY_DRAIN_CANDIDATE" --adapter caddyfile || return 1
  write_drain_state prepared
}

resolve_direct_drain_network() {
  PEER_HOST="${PEER_UPSTREAM%:*}"
  if ! [[ "$PEER_HOST" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    warn "Direct-port drain requires an IPv4 peer upstream: $PEER_UPSTREAM"
    return 1
  fi
  if [[ "$PUBLIC_INTERFACE" == "auto" ]]; then
    PUBLIC_INTERFACE="$(ip -4 route show default | awk 'NR == 1 { print $5 }')"
  fi
  [[ -n "$PUBLIC_INTERFACE" ]] || {
    warn "Cannot resolve public network interface"
    return 1
  }
  ip link show "$PUBLIC_INTERFACE" >/dev/null 2>&1 || {
    warn "Public network interface does not exist: $PUBLIC_INTERFACE"
    return 1
  }
  ip link show "$WIREGUARD_INTERFACE" >/dev/null 2>&1 || {
    warn "WireGuard network interface does not exist: $WIREGUARD_INTERFACE"
    return 1
  }
  IPV6_DRAIN_REQUIRED=false
  if ip -6 -o addr show dev "$PUBLIC_INTERFACE" scope global 2>/dev/null | grep -q .; then
    if [[ "$DIRECT_IPV6_DRAIN" != "true" ]]; then
      warn "Public IPv6 is active on $PUBLIC_INTERFACE, but IPv6 direct-port draining is disabled"
      return 1
    fi
    command -v ip6tables >/dev/null 2>&1 || {
      warn "Public IPv6 is active, but ip6tables is unavailable"
      return 1
    }
    IPV6_DRAIN_REQUIRED=true
  fi
  DIRECT_DRAIN_COMMENT="modelsell-deploy-${APP_PORT}"
}

verify_ipv6_drain_proxy() {
  [[ "$IPV6_DRAIN_REQUIRED" == "true" ]] || return 0
  local body_file code
  body_file="$(mktemp /tmp/modelsell-ipv6-ready.XXXXXX)"
  code="$(curl -g -6 -sS --max-time 5 -o "$body_file" -w '%{http_code}' "http://[::1]:${IPV6_DRAIN_PROXY_PORT}${DRAIN_HEALTH_PATH}" 2>/dev/null || true)"
  if ! [[ "$code" =~ ^2[0-9][0-9]$ ]] || ! verify_ready_payload "$body_file" "$EXPECT_PEER_NODE_TYPE"; then
    rm -f "$body_file"
    warn "IPv6 drain proxy did not reach the peer: port=$IPV6_DRAIN_PROXY_PORT http=${code:-000}"
    return 1
  fi
  rm -f "$body_file"
}

switch_ipv6_direct_traffic_to_peer() {
  [[ "$IPV6_DRAIN_REQUIRED" == "true" ]] || return 0
  verify_ipv6_drain_proxy || return 1

  local comment="${DIRECT_DRAIN_COMMENT}-ipv6"
  local redirect=(-i "$PUBLIC_INTERFACE" -p tcp --dport "$APP_PORT" -m comment --comment "$comment" -j REDIRECT --to-ports "$IPV6_DRAIN_PROXY_PORT")
  local input=(-i "$PUBLIC_INTERFACE" -p tcp --dport "$IPV6_DRAIN_PROXY_PORT" -m comment --comment "$comment" -j ACCEPT)

  DIRECT_IPV6_TRAFFIC_DRAINED=true
  if ! { ip6tables -w 5 -t nat -C PREROUTING "${redirect[@]}" >/dev/null 2>&1 || ip6tables -w 5 -t nat -I PREROUTING 1 "${redirect[@]}"; } ||
     ! { ip6tables -w 5 -C INPUT "${input[@]}" >/dev/null 2>&1 || ip6tables -w 5 -I INPUT 1 "${input[@]}"; }; then
    warn "Failed to install one or more IPv6 direct-port drain rules"
    restore_ipv6_direct_traffic || true
    return 1
  fi
  log "Direct IPv6 port drained through Caddy: interface=$PUBLIC_INTERFACE port=$APP_PORT proxy_port=$IPV6_DRAIN_PROXY_PORT"
}

restore_ipv6_direct_traffic() {
  [[ "$DIRECT_IPV6_TRAFFIC_DRAINED" == "true" ]] || return 0
  resolve_direct_drain_network || return 1

  local comment="${DIRECT_DRAIN_COMMENT}-ipv6"
  local redirect=(-i "$PUBLIC_INTERFACE" -p tcp --dport "$APP_PORT" -m comment --comment "$comment" -j REDIRECT --to-ports "$IPV6_DRAIN_PROXY_PORT")
  local input=(-i "$PUBLIC_INTERFACE" -p tcp --dport "$IPV6_DRAIN_PROXY_PORT" -m comment --comment "$comment" -j ACCEPT)

  while ip6tables -w 5 -t nat -C PREROUTING "${redirect[@]}" >/dev/null 2>&1; do
    ip6tables -w 5 -t nat -D PREROUTING "${redirect[@]}" || return 1
  done
  while ip6tables -w 5 -C INPUT "${input[@]}" >/dev/null 2>&1; do
    ip6tables -w 5 -D INPUT "${input[@]}" || return 1
  done
  DIRECT_IPV6_TRAFFIC_DRAINED=false
  log "Direct IPv6 port routing restored to the local application"
}

switch_direct_traffic_to_peer() {
  [[ "$DIRECT_PORT_DRAIN" == "true" ]] || return 0
  command -v ip >/dev/null 2>&1 || {
    warn "Missing remote command: ip"
    return 1
  }
  command -v iptables >/dev/null 2>&1 || {
    warn "Missing remote command: iptables"
    return 1
  }
  [[ "$(sysctl -n net.ipv4.ip_forward 2>/dev/null || true)" == "1" ]] || {
    warn "IPv4 forwarding is disabled; cannot drain direct port traffic"
    return 1
  }
  resolve_direct_drain_network || return 1

  local prerouting=(-i "$PUBLIC_INTERFACE" -p tcp --dport "$APP_PORT" -m comment --comment "$DIRECT_DRAIN_COMMENT" -j DNAT --to-destination "$PEER_UPSTREAM")
  local postrouting=(-o "$WIREGUARD_INTERFACE" -p tcp -d "$PEER_HOST" --dport "$APP_PORT" -m comment --comment "$DIRECT_DRAIN_COMMENT" -j MASQUERADE)
  local forwarding=(-i "$PUBLIC_INTERFACE" -o "$WIREGUARD_INTERFACE" -p tcp -d "$PEER_HOST" --dport "$APP_PORT" -m conntrack --ctstate NEW -m comment --comment "$DIRECT_DRAIN_COMMENT" -j ACCEPT)

  DIRECT_TRAFFIC_DRAINED=true
  if ! { iptables -w 5 -t nat -C PREROUTING "${prerouting[@]}" >/dev/null 2>&1 || iptables -w 5 -t nat -I PREROUTING 1 "${prerouting[@]}"; } ||
     ! { iptables -w 5 -t nat -C POSTROUTING "${postrouting[@]}" >/dev/null 2>&1 || iptables -w 5 -t nat -I POSTROUTING 1 "${postrouting[@]}"; } ||
     ! { iptables -w 5 -C FORWARD "${forwarding[@]}" >/dev/null 2>&1 || iptables -w 5 -I FORWARD 1 "${forwarding[@]}"; }; then
    warn "Failed to install one or more direct-port drain rules"
    restore_direct_traffic || true
    return 1
  fi
  if ! switch_ipv6_direct_traffic_to_peer; then
    restore_direct_traffic || true
    return 1
  fi

  log "Direct port drained: interface=$PUBLIC_INTERFACE port=$APP_PORT peer=$PEER_UPSTREAM"
}

restore_direct_traffic() {
  [[ "$DIRECT_PORT_DRAIN" == "true" ]] || return 0
  [[ "$DIRECT_TRAFFIC_DRAINED" == "true" ]] || return 0
  resolve_direct_drain_network || return 1

  local prerouting=(-i "$PUBLIC_INTERFACE" -p tcp --dport "$APP_PORT" -m comment --comment "$DIRECT_DRAIN_COMMENT" -j DNAT --to-destination "$PEER_UPSTREAM")
  local postrouting=(-o "$WIREGUARD_INTERFACE" -p tcp -d "$PEER_HOST" --dport "$APP_PORT" -m comment --comment "$DIRECT_DRAIN_COMMENT" -j MASQUERADE)
  local forwarding=(-i "$PUBLIC_INTERFACE" -o "$WIREGUARD_INTERFACE" -p tcp -d "$PEER_HOST" --dport "$APP_PORT" -m conntrack --ctstate NEW -m comment --comment "$DIRECT_DRAIN_COMMENT" -j ACCEPT)

  while iptables -w 5 -t nat -C PREROUTING "${prerouting[@]}" >/dev/null 2>&1; do
    iptables -w 5 -t nat -D PREROUTING "${prerouting[@]}" || return 1
  done
  while iptables -w 5 -t nat -C POSTROUTING "${postrouting[@]}" >/dev/null 2>&1; do
    iptables -w 5 -t nat -D POSTROUTING "${postrouting[@]}" || return 1
  done
  while iptables -w 5 -C FORWARD "${forwarding[@]}" >/dev/null 2>&1; do
    iptables -w 5 -D FORWARD "${forwarding[@]}" || return 1
  done

  DIRECT_TRAFFIC_DRAINED=false
  log "Direct port routing restored to the local application"
}

settle_peer_routing() {
  local connections="unknown"
  if command -v ss >/dev/null 2>&1; then
    connections="$(ss -Htn state established "sport = :${APP_PORT}" 2>/dev/null | wc -l | tr -d ' ')"
  fi
  log "Traffic route settled for ${TRAFFIC_SETTLE_SECONDS}s; existing local TCP connections=${connections} will drain through SIGTERM"
  sleep "$TRAFFIC_SETTLE_SECONDS"
}

restore_local_traffic() {
  [[ "$ZERO_DOWNTIME" == "true" ]] || return 0
  [[ "$TRAFFIC_DRAINED" == "true" ]] || return 0
  [[ -n "$CADDY_BACKUP" && -f "$CADDY_BACKUP" ]] || {
    warn "Caddy backup is missing; traffic remains on the peer"
    return 1
  }
  if ! restore_ipv6_direct_traffic; then
    warn "Direct IPv6 traffic remains on the peer proxy"
    return 1
  fi
  if [[ -n "$CADDY_DRAIN_CANDIDATE" && -f "$CADDY_DRAIN_CANDIDATE" ]] &&
     ! cmp -s "$CADDY_DRAIN_CANDIDATE" "$CADDY_CONFIG"; then
    warn "Caddy config changed outside this deployment; refusing to overwrite it during cutback"
    return 1
  fi

  if ! caddy validate --config "$CADDY_BACKUP" --adapter caddyfile ||
     ! install -m 0644 "$CADDY_BACKUP" "$CADDY_CONFIG" ||
     ! caddy reload --config "$CADDY_CONFIG" --force; then
    warn "Failed to restore local traffic; the peer continues serving requests"
    return 1
  fi
  if ! cmp -s "$CADDY_BACKUP" "$CADDY_CONFIG"; then
    warn "Restored Caddy config does not match the recorded backup"
    return 1
  fi

  if ! verify_caddy_health; then
    warn "Local Caddy health verification failed; Caddy configuration was restored but deployment remains failed"
    return 1
  fi
  if ! restore_direct_traffic; then
    warn "Caddy routing was restored, but direct port traffic remains on the peer"
    return 1
  fi

  rm -f "$DRAIN_STATE_FILE"
  [[ -n "$CADDY_DRAIN_CANDIDATE" ]] && rm -f "$CADDY_DRAIN_CANDIDATE"
  TRAFFIC_DRAINED=false
  DRAIN_STATE_PHASE=""
  log "Original Caddy routing restored after local release verification"
}

switch_traffic_to_peer() {
  [[ "$ZERO_DOWNTIME" == "true" ]] || return 0

  command -v caddy >/dev/null 2>&1 || {
    warn "Missing remote command: caddy"
    return 1
  }
  command -v python3 >/dev/null 2>&1 || {
    warn "Missing remote command: python3"
    return 1
  }
  [[ -f "$CADDY_CONFIG" ]] || {
    warn "Missing Caddy config: $CADDY_CONFIG"
    return 1
  }

  verify_peer_stable || return 1
  if [[ "$DIRECT_PORT_DRAIN" == "true" ]]; then
    resolve_direct_drain_network || return 1
  else
    IPV6_DRAIN_REQUIRED=false
  fi
  prepare_drain_state || return 1

  if ! caddy validate --config "$CADDY_DRAIN_CANDIDATE" --adapter caddyfile ||
     ! install -m 0644 "$CADDY_DRAIN_CANDIDATE" "$CADDY_CONFIG" ||
     ! caddy reload --config "$CADDY_CONFIG" --force ||
     ! cmp -s "$CADDY_DRAIN_CANDIDATE" "$CADDY_CONFIG"; then
    if install -m 0644 "$CADDY_BACKUP" "$CADDY_CONFIG" &&
       caddy reload --config "$CADDY_CONFIG" --force &&
       cmp -s "$CADDY_BACKUP" "$CADDY_CONFIG"; then
      rm -f "$DRAIN_STATE_FILE" "$CADDY_DRAIN_CANDIDATE"
    fi
    warn "Failed to switch traffic to the peer"
    return 1
  fi
  TRAFFIC_DRAINED=true
  write_drain_state caddy

  if ! verify_caddy_health "$EXPECT_PEER_NODE_TYPE"; then
    warn "Peer route did not pass Caddy health verification"
    restore_local_traffic || true
    return 1
  fi
  if ! switch_direct_traffic_to_peer; then
    warn "Failed to drain direct port traffic"
    restore_local_traffic || true
    return 1
  fi
  write_drain_state drained
  settle_peer_routing

  log "Traffic drained: new connections use peer $PEER_UPSTREAM; SIGTERM closes idle keep-alives and waits for active requests"
}

stop_service() {
  log "Gracefully stopping service with SIGTERM (application=${SHUTDOWN_TIMEOUT}s, systemd=${STOP_TIMEOUT}s): $SERVICE_NAME"
  local stop_rc=0
  local started_at=$SECONDS
  systemctl stop "$SERVICE_NAME" || stop_rc=$?

  if systemctl is-active --quiet "$SERVICE_NAME"; then
    warn "Service is still active after stop (exit=$stop_rc)"
    return 1
  fi
  if (( stop_rc != 0 )); then
    warn "systemctl stop returned $stop_rc, but the service is stopped; continuing safely"
  fi
  systemctl reset-failed "$SERVICE_NAME" 2>/dev/null || true
  log "Service stopped after $((SECONDS - started_at))s: $SERVICE_NAME"
}

ensure_app_port_available() {
  command -v ss >/dev/null 2>&1 || return 0
  if ss -ltn "sport = :$APP_PORT" 2>/dev/null | tail -n +2 | grep -q .; then
    warn "Port $APP_PORT is still occupied after stopping $SERVICE_NAME; refusing to start a second process"
    ss -ltnp "sport = :$APP_PORT" 2>/dev/null || true
    return 1
  fi
}

start_and_verify() {
  local verify_release_seo="${1:-false}"
  local restart_baseline
  restart_baseline="$(systemctl show "$SERVICE_NAME" -p NRestarts --value 2>/dev/null || true)"
  [[ "$restart_baseline" =~ ^[0-9]+$ ]] || restart_baseline=0
  stop_service || return 1
  ensure_app_port_available || return 1
  log "Starting service with release: $(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
  if ! systemctl start "$SERVICE_NAME"; then
    warn "systemctl start failed: $SERVICE_NAME"
    return 1
  fi
  wait_service_healthy || return 1
  verify_smoke_tests || return 1
  if [[ "$verify_release_seo" == "true" ]]; then
    verify_seo || return 1
  fi
  verify_service_soak "$restart_baseline"
}

rollback_service() {
  local previous_target="$1"
  warn "Rolling back service: $SERVICE_NAME"

  if [[ -n "$ENV_BACKUP" && -f "$ENV_BACKUP" ]]; then
    cp "$ENV_BACKUP" "$REMOTE_DIR/.env"
    chmod 600 "$REMOTE_DIR/.env"
    warn "Restored previous runtime env"
  fi

  if [[ -n "$previous_target" && -d "$previous_target" ]]; then
    ln -sfnT "$previous_target" "$CURRENT_LINK"
    warn "Restored previous release: $previous_target"
  else
    rm -f "$CURRENT_LINK"
    stop_service || true
    warn "No previous release is available; service left stopped"
    return 0
  fi

  systemctl daemon-reload || true
  if start_and_verify false; then
    warn "Rollback completed"
    return 0
  else
    warn "Rollback health check failed"
    show_service_debug
    return 1
  fi
}

cleanup_old_releases() {
  local keep="$KEEP_RELEASES"
  if ! [[ "$keep" =~ ^[0-9]+$ ]] || (( keep < 1 )); then
    keep=5
  fi

  local current_target
  current_target="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
  local index=0
  local release
  while IFS= read -r release; do
    [[ -n "$release" ]] || continue
    index=$((index + 1))
    if (( index <= keep )); then
      continue
    fi
    if [[ -n "$current_target" && "$(readlink -f "$release" 2>/dev/null || true)" == "$current_target" ]]; then
      continue
    fi
    rm -rf "$release"
  done < <(ls -1dt "$RELEASES_DIR"/* 2>/dev/null || true)
}

ensure_legacy_release() {
  if [[ -L "$CURRENT_LINK" || ! -x "$REMOTE_DIR/$BINARY_NAME" ]]; then
    return 0
  fi

  local legacy_dir="$RELEASES_DIR/legacy-$(date -u +%Y%m%d%H%M%S)"
  mkdir -p "$legacy_dir"
  cp -p "$REMOTE_DIR/$BINARY_NAME" "$legacy_dir/$BINARY_NAME"
  chmod +x "$legacy_dir/$BINARY_NAME"
  ln -sfnT "$legacy_dir" "$CURRENT_LINK"
  log "Imported legacy flat binary as release: $legacy_dir"
}

backup_runtime_env() {
  [[ "$INSTALL_ENV" == "1" ]] || return 0
  if [[ -s "$DRAIN_STATE_FILE" ]]; then
    log "Existing deployment transaction found; preserving its original environment backup"
    return 0
  fi
  if [[ -f "$REMOTE_DIR/.env" ]]; then
    ENV_BACKUP="$REMOTE_DIR/.env.rollback.$RELEASE_ID"
    cp -- "$REMOTE_DIR/.env" "$ENV_BACKUP"
    chmod 600 "$ENV_BACKUP"
  fi
}

install_runtime_env() {
  [[ "$INSTALL_ENV" == "1" ]] || return 0
  log "Installing runtime environment"
  cp "$ENV_PATH" "$REMOTE_DIR/.env"
  chmod 600 "$REMOTE_DIR/.env"
}

command -v curl >/dev/null 2>&1 || {
  echo "Missing remote command: curl" >&2
  exit 1
}
[[ "$RELEASE_ID" =~ ^[A-Za-z0-9._-]+$ ]] || {
  echo "Invalid release id: $RELEASE_ID" >&2
  exit 1
}
[[ "$BINARY_NAME" =~ ^[A-Za-z0-9._-]+$ ]] || {
  echo "Invalid binary name: $BINARY_NAME" >&2
  exit 1
}

mkdir -p "$REMOTE_DIR" "$REMOTE_DIR/logs" "$RELEASES_DIR"
ensure_legacy_release
validate_rollout_settings
verify_node_role
verify_backend_config_change

if ! [[ "$HEALTH_TIMEOUT" =~ ^[0-9]+$ ]] || (( HEALTH_TIMEOUT < 1 )); then
  HEALTH_TIMEOUT=180
fi

PREVIOUS_TARGET=""
if [[ -L "$CURRENT_LINK" ]]; then
  PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
fi

if [[ "$APPLY_RELEASE" == "1" ]]; then
  log "Validating and staging release: $RELEASE_ID"
  archive_entries="$(tar -tzf "$ARCHIVE_PATH" | sed 's#^\./##' | LC_ALL=C sort)"
  expected_entries="$(printf '%s\n' "$BINARY_NAME" rollback-modelsell.sh start-modelsell.sh verify-modelsell-seo.sh | LC_ALL=C sort)"
  [[ "$archive_entries" == "$expected_entries" ]] || {
    warn "Release archive contains unexpected or missing files"
    exit 1
  }

  rm -rf -- "$STAGING_RELEASE_DIR"
  mkdir -p "$STAGING_RELEASE_DIR"
  tar --no-same-owner --no-same-permissions -xzf "$ARCHIVE_PATH" -C "$STAGING_RELEASE_DIR"
  chmod +x "$STAGING_RELEASE_DIR/$BINARY_NAME"
  [[ -e "$STAGING_RELEASE_DIR/start-modelsell.sh" ]] && chmod +x "$STAGING_RELEASE_DIR/start-modelsell.sh"
  [[ -e "$STAGING_RELEASE_DIR/rollback-modelsell.sh" ]] && chmod +x "$STAGING_RELEASE_DIR/rollback-modelsell.sh"
  [[ -e "$STAGING_RELEASE_DIR/verify-modelsell-seo.sh" ]] && chmod +x "$STAGING_RELEASE_DIR/verify-modelsell-seo.sh"

  if [[ -e "$RELEASE_DIR" ]]; then
    existing_target="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
    if [[ "$existing_target" == "$(readlink -f "$RELEASE_DIR" 2>/dev/null || true)" ]]; then
      if ! diff -qr "$STAGING_RELEASE_DIR" "$RELEASE_DIR" >/dev/null; then
        warn "Release id is already active with different content; use a new release id"
        exit 1
      fi
      rm -rf -- "$STAGING_RELEASE_DIR"
      log "Identical release is already active; reusing it without deleting live files"
    else
      rm -rf -- "$RELEASE_DIR"
      mv "$STAGING_RELEASE_DIR" "$RELEASE_DIR"
    fi
  else
    mv "$STAGING_RELEASE_DIR" "$RELEASE_DIR"
  fi

  mkdir -p "$REMOTE_DIR/bin"
  [[ -e "$RELEASE_DIR/start-modelsell.sh" ]] && install -m 0755 "$RELEASE_DIR/start-modelsell.sh" "$REMOTE_DIR/bin/start-modelsell.sh"
  [[ -e "$RELEASE_DIR/rollback-modelsell.sh" ]] && install -m 0755 "$RELEASE_DIR/rollback-modelsell.sh" "$REMOTE_DIR/bin/rollback-modelsell.sh"
  [[ -e "$RELEASE_DIR/verify-modelsell-seo.sh" ]] && install -m 0755 "$RELEASE_DIR/verify-modelsell-seo.sh" "$REMOTE_DIR/bin/verify-modelsell-seo.sh"
fi

backup_runtime_env
install_service_config

if [[ "$APPLY_RELEASE" == "1" ]]; then
  if ! switch_traffic_to_peer; then
    warn "Deployment canceled before touching the running release"
    exit 1
  fi
  install_runtime_env
  start_service_log_stream
  log "Switching current release: ${PREVIOUS_TARGET:-<none>} -> $RELEASE_DIR"
  ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"
  if ! start_and_verify true; then
    if rollback_service "$PREVIOUS_TARGET"; then
      restore_local_traffic || true
    else
      warn "Rollback is unhealthy; the peer continues serving domain traffic"
    fi
    exit 1
  fi
  if ! restore_local_traffic; then
    warn "New release is healthy, but traffic remains on the peer because local cutback failed"
    exit 1
  fi
  cleanup_old_releases
  exit 0
fi

if [[ -x "$CURRENT_LINK/$BINARY_NAME" ]]; then
  if ! switch_traffic_to_peer; then
    warn "Deployment canceled before stopping the running service"
    exit 1
  fi
  install_runtime_env
  start_service_log_stream
  if ! start_and_verify false; then
    if rollback_service "$PREVIOUS_TARGET"; then
      restore_local_traffic || true
    else
      warn "Rollback is unhealthy; the peer continues serving domain traffic"
    fi
    exit 1
  fi
  if ! restore_local_traffic; then
    warn "Service is healthy, but traffic remains on the peer because local cutback failed"
    exit 1
  fi
else
  install_runtime_env
  log "Config updated. Current binary not found yet: $CURRENT_LINK/$BINARY_NAME"
fi
REMOTE_SCRIPT
}

full_deploy() {
  build_package
  install_artifact_with_config
}

install_artifact_with_config() {
  require_artifact
  require_app_env
  prepare_remote

  log "Upload package and runtime env"
  remote_scp "$ARCHIVE_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ARCHIVE"
  remote_scp "$APP_ENV_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ENV"

  log "Install release, systemd config, and restart service with health-check rollback: $DEPLOY_SERVICE_NAME"
  run_remote_deploy 1 1
}

rolling_deploy() {
  local include_config="$1"
  local rolling_release_id="$DEPLOY_RELEASE_ID"
  local deploy_script="$ROOT_DIR/scripts/deploy-modelsell.sh"
  local primary_version standby_version child_mode description

  [[ "$DEPLOY_TOPOLOGY" == "multi" ]] || \
    fail "Rolling deployment requires DEPLOY_TOPOLOGY=multi"

  if [[ "$include_config" == "true" ]]; then
    child_mode="--install-artifact-with-config"
    description="code and runtime config"
    validate_shared_backend_config
  else
    child_mode="--upload-only"
    description="code only; active runtime config is preserved"
  fi

  standby_version="$(collect_rollout_preflight standby "$deploy_script")"
  primary_version="$(collect_rollout_preflight primary "$deploy_script")"
  if [[ "$primary_version" != "$standby_version" ]]; then
    if [[ "$primary_version" != "$APP_VERSION" && "$standby_version" != "$APP_VERSION" ]]; then
      fail "Nodes run different unexpected versions: primary=$primary_version standby=$standby_version local=$APP_VERSION"
    fi
    warn "Resuming a partial rollout: primary=$primary_version standby=$standby_version local=$APP_VERSION"
  fi
  warn "Rolling deployment requires backward-compatible database migrations; automatic schema compatibility cannot be proven by this script"

  build_package
  log "Rolling release $rolling_release_id ($description): standby first, primary second"

  DEPLOY_TARGET=standby DEPLOY_RELEASE_ID="$rolling_release_id" DEPLOY_ZERO_DOWNTIME=true DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE="$DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE" \
    "$deploy_script" "$child_mode"
  log "Standby release verified; proceeding to primary"

  DEPLOY_TARGET=primary DEPLOY_RELEASE_ID="$rolling_release_id" DEPLOY_ZERO_DOWNTIME=true DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE="$DEPLOY_ALLOW_BACKEND_CONFIG_CHANGE" \
    "$deploy_script" "$child_mode"
  log "Rolling release completed on both nodes: $rolling_release_id"
}

rolling_code_deploy() {
  rolling_deploy false
}

rolling_full_deploy() {
  rolling_deploy true
}

config_check() {
  validate_app_env_role "$APP_ENV_PATH" "$DEPLOY_EXPECT_NODE_TYPE"
  validate_shared_backend_config
  printf 'CONFIG_CHECK=ok\n'
  printf 'DEPLOY_TOPOLOGY=%s\n' "$DEPLOY_TOPOLOGY"
  printf 'SELECTED_TARGET=%s\n' "$DEPLOY_TARGET"
  printf 'SELECTED_HOST=%s\n' "$DEPLOY_HOST"
  printf 'SELECTED_NODE_TYPE=%s\n' "$DEPLOY_EXPECT_NODE_TYPE"
  printf 'SELECTED_APP_ENV_FILE=%s\n' "$DEPLOY_APP_ENV_FILE"
  printf 'ZERO_DOWNTIME_ACTIVE=%s\n' "$DEPLOY_ZERO_DOWNTIME_ACTIVE"
  if [[ "$DEPLOY_TOPOLOGY" == "multi" ]]; then
    printf 'SELECTED_PEER_UPSTREAM=%s\n' "$DEPLOY_PEER_UPSTREAM"
    printf 'PRIMARY_HOST=%s\n' "$DEPLOY_PRIMARY_HOST"
    printf 'PRIMARY_NODE_TYPE=%s\n' "$DEPLOY_PRIMARY_NODE_TYPE"
    printf 'STANDBY_HOST=%s\n' "$DEPLOY_STANDBY_HOST"
    printf 'STANDBY_NODE_TYPE=%s\n' "$DEPLOY_STANDBY_NODE_TYPE"
  else
    printf 'PRIMARY_HOST=%s\n' "$DEPLOY_PRIMARY_HOST"
    printf 'PRIMARY_NODE_TYPE=%s\n' "$DEPLOY_PRIMARY_NODE_TYPE"
  fi
}

rolling_preflight() {
  local check_multi_dependencies=false
  [[ "$DEPLOY_TOPOLOGY" == "multi" ]] && check_multi_dependencies=true
  require_remote_tools
  remote_ssh "SERVICE_NAME='$DEPLOY_SERVICE_NAME' APP_PORT='$DEPLOY_APP_PORT' HEALTH_PATH='$DEPLOY_HEALTH_PATH' EXPECT_NODE_TYPE='$DEPLOY_EXPECT_NODE_TYPE' CHECK_MULTI_DEPENDENCIES='$check_multi_dependencies' WIREGUARD_INTERFACE='$DEPLOY_WIREGUARD_INTERFACE' bash -s" <<'REMOTE_PREFLIGHT'
set -Eeuo pipefail
[[ "$HEALTH_PATH" == /* ]] || HEALTH_PATH="/$HEALTH_PATH"
systemctl is-active --quiet "$SERVICE_NAME" || {
  echo "Service is not active: $SERVICE_NAME" >&2
  exit 1
}
if [[ "$CHECK_MULTI_DEPENDENCIES" == "true" ]]; then
  systemctl is-active --quiet caddy || {
    echo "Caddy is not active" >&2
    exit 1
  }
  ip link show "$WIREGUARD_INTERFACE" >/dev/null 2>&1 || {
    echo "WireGuard interface is missing: $WIREGUARD_INTERFACE" >&2
    exit 1
  }
fi
body="$(mktemp /tmp/modelsell-preflight-ready.XXXXXX)"
trap 'rm -f "$body"' EXIT
code="$(curl -sS --max-time 5 -o "$body" -w '%{http_code}' "http://127.0.0.1:${APP_PORT}${HEALTH_PATH}" 2>/dev/null || true)"
[[ "$code" =~ ^2[0-9][0-9]$ ]] || {
  echo "Local readiness failed: HTTP ${code:-000}" >&2
  exit 1
}
version="$(python3 - "$body" "$EXPECT_NODE_TYPE" <<'PY'
import json
import sys

body_file, expected_role = sys.argv[1:]
with open(body_file, "r", encoding="utf-8") as handle:
    payload = json.load(handle)
data = payload.get("data") or {}
checks = data.get("checks") or {}
if payload.get("success") is not True or data.get("node_type") != expected_role:
    raise SystemExit("readiness role mismatch")
for name in ("database", "log_database", "redis"):
    if checks.get(name) != "ok":
        raise SystemExit(f"readiness dependency failed: {name}")
print(data.get("version") or "unknown")
PY
)"
printf 'PREFLIGHT_VERSION=%s\n' "$version"
printf 'ROLLING_PREFLIGHT_VERSION=%s\n' "$version"
REMOTE_PREFLIGHT
}

collect_rollout_preflight() {
  local target="$1"
  local deploy_script="$2"
  local output version
  output="$(DEPLOY_TARGET="$target" "$deploy_script" --preflight)"
  printf '%s\n' "$output" >&2
  version="$(printf '%s\n' "$output" | sed -n 's/^ROLLING_PREFLIGHT_VERSION=//p' | tail -1)"
  [[ -n "$version" ]] || fail "Cannot read rollout preflight version for target: $target"
  printf '%s' "$version"
}

zero_downtime_test() {
  [[ "$DEPLOY_TOPOLOGY" == "multi" ]] || \
    fail "--zero-downtime-test requires DEPLOY_TOPOLOGY=multi"
  [[ "$DEPLOY_ZERO_DOWNTIME_ACTIVE" == "true" ]] || fail "Zero-downtime routing is disabled for target: $DEPLOY_TARGET"
  require_remote_tools
  prepare_remote
  log "Test zero-downtime flow with the current release: drain, restart, smoke-test, restore"
  run_remote_deploy 0 0
}

manual_start() {
  require_remote_tools
  prepare_remote
  log "Start current release directly: $DEPLOY_SERVICE_NAME"
  remote_ssh "cd '$DEPLOY_REMOTE_DIR' && if [[ -x '$DEPLOY_REMOTE_DIR/bin/start-modelsell.sh' ]]; then '$DEPLOY_REMOTE_DIR/bin/start-modelsell.sh' --force; else '$DEPLOY_REMOTE_DIR/current/start-modelsell.sh' --force; fi"
}

manual_stop() {
  require_remote_tools
  log "Stop manually started process: $DEPLOY_SERVICE_NAME"
  remote_ssh "if [[ -x '$DEPLOY_REMOTE_DIR/bin/start-modelsell.sh' ]]; then '$DEPLOY_REMOTE_DIR/bin/start-modelsell.sh' --stop; else '$DEPLOY_REMOTE_DIR/current/start-modelsell.sh' --stop; fi"
}

manual_rollback_list() {
  require_remote_tools
  log "List historical releases: $DEPLOY_SERVICE_NAME"
  remote_ssh "if [[ -x '$DEPLOY_REMOTE_DIR/bin/rollback-modelsell.sh' ]]; then '$DEPLOY_REMOTE_DIR/bin/rollback-modelsell.sh' --list; else '$DEPLOY_REMOTE_DIR/current/rollback-modelsell.sh' --list; fi"
}

manual_rollback() {
  local release_id="$1"
  [[ "$release_id" =~ ^[A-Za-z0-9._-]+$ ]] || fail "Invalid release id: $release_id"
  require_remote_tools
  log "Manually rollback to release: $release_id"
  remote_ssh "if [[ -x '$DEPLOY_REMOTE_DIR/bin/rollback-modelsell.sh' ]]; then '$DEPLOY_REMOTE_DIR/bin/rollback-modelsell.sh' --release '$release_id' --yes; else '$DEPLOY_REMOTE_DIR/current/rollback-modelsell.sh' --release '$release_id' --yes; fi"
}

manual_status() {
  require_remote_tools
  log "Show deployment status: $DEPLOY_SERVICE_NAME"
  remote_ssh "REMOTE_DIR='$DEPLOY_REMOTE_DIR' SERVICE_NAME='$DEPLOY_SERVICE_NAME' APP_PORT='$DEPLOY_APP_PORT' HEALTH_PATH='$DEPLOY_HEALTH_PATH' bash -s" <<'REMOTE_STATUS'
set -u
[[ "$HEALTH_PATH" == /* ]] || HEALTH_PATH="/$HEALTH_PATH"
printf 'Current release: %s\n' "$(readlink -f "$REMOTE_DIR/current" 2>/dev/null || printf '<none>')"
systemctl --no-pager --full status "$SERVICE_NAME" | sed -n '1,16p' || true
printf 'Health: '
curl -sS --max-time 3 -o /dev/null -w 'HTTP %{http_code} in %{time_total}s\n' "http://127.0.0.1:${APP_PORT}${HEALTH_PATH}" || true
REMOTE_STATUS
}

manual_logs() {
  require_remote_tools
  log "Follow live service logs; press Ctrl-C to stop: $DEPLOY_SERVICE_NAME"
  remote_ssh "journalctl -u '$DEPLOY_SERVICE_NAME' -n 100 --follow --no-pager -o short-iso"
}

manual_service_start() {
  require_remote_tools
  log "Start systemd service and verify health: $DEPLOY_SERVICE_NAME"
  remote_ssh "SERVICE_NAME='$DEPLOY_SERVICE_NAME' APP_PORT='$DEPLOY_APP_PORT' HEALTH_PATH='$DEPLOY_HEALTH_PATH' HEALTH_TIMEOUT='$DEPLOY_HEALTH_TIMEOUT' bash -s" <<'REMOTE_START'
set -Eeuo pipefail
[[ "$HEALTH_PATH" == /* ]] || HEALTH_PATH="/$HEALTH_PATH"
journalctl -u "$SERVICE_NAME" --since now --follow --no-pager -n 0 -o short-iso &
journal_pid=$!
trap 'kill "$journal_pid" 2>/dev/null || true; wait "$journal_pid" 2>/dev/null || true' EXIT
systemctl start "$SERVICE_NAME"
url="http://127.0.0.1:${APP_PORT}${HEALTH_PATH}"
deadline=$((SECONDS + HEALTH_TIMEOUT))
while (( SECONDS < deadline )); do
  state="$(systemctl is-active "$SERVICE_NAME" 2>/dev/null || true)"
  pid="$(systemctl show "$SERVICE_NAME" -p MainPID --value 2>/dev/null || true)"
  code="$(curl -sS --max-time 3 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
  printf '[manual-service] state=%s pid=%s http=%s\n' "${state:-unknown}" "${pid:-0}" "${code:-000}"
  if [[ "$state" == active && "$pid" =~ ^[1-9][0-9]*$ && "$code" =~ ^2[0-9][0-9]$ ]]; then
    printf '[manual-service] start verified: %s\n' "$url"
    exit 0
  fi
  sleep 1
done
printf '[manual-service:error] health check timed out: %s\n' "$url" >&2
exit 1
REMOTE_START
}

manual_service_stop() {
  require_remote_tools
  log "Gracefully stop systemd service: $DEPLOY_SERVICE_NAME"
  remote_ssh "SERVICE_NAME='$DEPLOY_SERVICE_NAME' APP_PORT='$DEPLOY_APP_PORT' bash -s" <<'REMOTE_STOP'
set -Eeuo pipefail
journalctl -u "$SERVICE_NAME" --since now --follow --no-pager -n 0 -o short-iso &
journal_pid=$!
trap 'kill "$journal_pid" 2>/dev/null || true; wait "$journal_pid" 2>/dev/null || true' EXIT
stop_rc=0
systemctl stop "$SERVICE_NAME" || stop_rc=$?
if systemctl is-active --quiet "$SERVICE_NAME"; then
  printf '[manual-service:error] service is still active (exit=%s)\n' "$stop_rc" >&2
  exit 1
fi
printf '[manual-service] service stopped (systemctl exit=%s)\n' "$stop_rc"
if command -v ss >/dev/null 2>&1 && ss -ltn "sport = :$APP_PORT" 2>/dev/null | tail -n +2 | grep -q .; then
  printf '[manual-service:error] port %s is still occupied\n' "$APP_PORT" >&2
  ss -ltnp "sport = :$APP_PORT" 2>/dev/null || true
  exit 1
fi
printf '[manual-service] port %s is free\n' "$APP_PORT"
REMOTE_STOP
}

choose_mode() {
  local menu_target="$DEPLOY_TARGET"
  local choice release_id

  while true; do
    cat >&2 <<EOF

ModelSell 部署管理
  拓扑：$DEPLOY_TOPOLOGY
  当前单节点操作目标：${menu_target}
  完整发布策略：$([[ "$DEPLOY_TOPOLOGY" == "multi" ]] && printf 'standby -> primary 滚动发布' || printf '单机优雅重启')

  1) 发布与更新
  2) 检查与观察
  3) 服务控制
  4) 版本与回滚
  5) 本地构建
EOF
    if [[ "$DEPLOY_TOPOLOGY" == "multi" ]]; then
      printf '  6) 切换单节点操作目标（primary / standby）\n' >&2
    fi
    printf '  q) 退出\n\n请选择功能: ' >&2
    read -r choice || choice="q"

    case "${choice:-q}" in
      1)
        while true; do
          if [[ "$DEPLOY_TOPOLOGY" == "multi" ]]; then
            cat >&2 <<EOF

发布与更新（多机）
  1) 双机滚动发布代码（推荐：保留线上配置，standby -> primary）
  2) 双机滚动发布代码和配置（会校验共享数据库配置）
  3) 仅完整发布当前目标 ${menu_target}（代码和配置，不更新另一台）
  4) 仅上传已有构建并发布到 ${menu_target}（不更新运行配置）
  5) 仅更新 ${menu_target} 的运行配置并重启
  6) 在 ${menu_target} 测试对端切流、重启和自动切回
  0) 返回主菜单
EOF
          else
            cat >&2 <<'EOF'

发布与更新（单机）
  1) 完整发布（构建、上传配置、优雅重启；新请求可能短暂不可用）
  2) 仅上传已有构建并发布（不更新运行配置）
  3) 仅更新运行配置并优雅重启
  0) 返回主菜单
EOF
          fi
          printf '请选择发布操作: ' >&2
          read -r choice || choice="0"
          if [[ "$DEPLOY_TOPOLOGY" == "multi" ]]; then
            case "$choice" in
              1) printf 'target:%s:rolling-code\n' "$menu_target"; return 0 ;;
              2) printf 'target:%s:rolling-full\n' "$menu_target"; return 0 ;;
              3) printf 'target:%s:full\n' "$menu_target"; return 0 ;;
              4) printf 'target:%s:upload-only\n' "$menu_target"; return 0 ;;
              5) printf 'target:%s:config-only\n' "$menu_target"; return 0 ;;
              6) printf 'target:%s:zero-downtime-test\n' "$menu_target"; return 0 ;;
              0) break ;;
              *) printf '无效选项，请重新选择。\n' >&2 ;;
            esac
          else
            case "$choice" in
              1) printf 'target:%s:full\n' "$menu_target"; return 0 ;;
              2) printf 'target:%s:upload-only\n' "$menu_target"; return 0 ;;
              3) printf 'target:%s:config-only\n' "$menu_target"; return 0 ;;
              0) break ;;
              *) printf '无效选项，请重新选择。\n' >&2 ;;
            esac
          fi
        done
        ;;
      2)
        while true; do
          cat >&2 <<EOF

检查与观察（目标：${menu_target}）
  1) 本地检查部署配置（不连接服务器）
  2) 远程发布前检查（服务、依赖、角色$([[ "$DEPLOY_TOPOLOGY" == "multi" ]] && printf '、WireGuard' || true)）
  3) 查看服务与当前版本状态
  4) 实时查看服务日志（Ctrl-C 退出）
  0) 返回主菜单
EOF
          printf '请选择检查操作: ' >&2
          read -r choice || choice="0"
          case "$choice" in
            1) printf 'target:%s:config-check\n' "$menu_target"; return 0 ;;
            2) printf 'target:%s:preflight\n' "$menu_target"; return 0 ;;
            3) printf 'target:%s:manual-status\n' "$menu_target"; return 0 ;;
            4) printf 'target:%s:manual-logs\n' "$menu_target"; return 0 ;;
            0) break ;;
            *) printf '无效选项，请重新选择。\n' >&2 ;;
          esac
        done
        ;;
      3)
        while true; do
          cat >&2 <<EOF

服务控制（目标：${menu_target}）
  1) 启动 systemd 服务并检查健康
  2) 优雅停止 systemd 服务（会停止对外服务）
  3) 绕过 systemd，直接启动当前版本（故障处理）
  4) 停止直接启动的进程（故障处理）
  0) 返回主菜单
EOF
          printf '请选择服务操作: ' >&2
          read -r choice || choice="0"
          case "$choice" in
            1) printf 'target:%s:manual-service-start\n' "$menu_target"; return 0 ;;
            2) printf 'target:%s:manual-service-stop\n' "$menu_target"; return 0 ;;
            3) printf 'target:%s:manual-start\n' "$menu_target"; return 0 ;;
            4) printf 'target:%s:manual-stop\n' "$menu_target"; return 0 ;;
            0) break ;;
            *) printf '无效选项，请重新选择。\n' >&2 ;;
          esac
        done
        ;;
      4)
        while true; do
          cat >&2 <<EOF

版本与回滚（目标：${menu_target}）
  1) 列出历史版本
  2) 回滚到指定 release id
  0) 返回主菜单
EOF
          printf '请选择版本操作: ' >&2
          read -r choice || choice="0"
          case "$choice" in
            1) printf 'target:%s:manual-rollback-list\n' "$menu_target"; return 0 ;;
            2)
              printf '请输入 release id: ' >&2
              read -r release_id || release_id=""
              if [[ -z "$release_id" ]]; then
                printf 'release id 不能为空。\n' >&2
              else
                printf 'target:%s:manual-rollback:%s\n' "$menu_target" "$release_id"
                return 0
              fi
              ;;
            0) break ;;
            *) printf '无效选项，请重新选择。\n' >&2 ;;
          esac
        done
        ;;
      5)
        cat >&2 <<'EOF'

本地构建
  1) 构建前端和 Linux 发布包（不连接服务器）
  0) 返回主菜单
EOF
        printf '请选择构建操作: ' >&2
        read -r choice || choice="0"
        case "$choice" in
          1) printf 'target:%s:build-only\n' "$menu_target"; return 0 ;;
          0) ;;
          *) printf '无效选项，请重新选择。\n' >&2 ;;
        esac
        ;;
      6)
        if [[ "$DEPLOY_TOPOLOGY" != "multi" ]]; then
          printf '单机拓扑没有可切换的目标。\n' >&2
          continue
        fi
        cat >&2 <<EOF

切换单节点操作目标
  1) primary$([[ "$menu_target" == "primary" ]] && printf '（当前）' || true)
  2) standby$([[ "$menu_target" == "standby" ]] && printf '（当前）' || true)
  0) 返回主菜单
EOF
        printf '请选择目标: ' >&2
        read -r choice || choice="0"
        case "$choice" in
          1) menu_target=primary ;;
          2) menu_target=standby ;;
          0) ;;
          *) printf '无效选项，目标未改变。\n' >&2 ;;
        esac
        ;;
      q|Q|quit|exit)
        printf 'q\n'
        return 0
        ;;
      *)
        printf '无效选项，请重新选择。\n' >&2
        ;;
    esac
  done
}

run_mode() {
  local mode="$1"

  case "$mode" in
    1|full|--full)
      full_deploy
      log "Done. Local artifact: $ARCHIVE_PATH"
      ;;
    2|build|build-only|--build-only)
      build_package
      log "Done. Local artifact: $ARCHIVE_PATH"
      ;;
    3|upload|upload-only|--upload-only)
      upload_artifact
      log "Done. Uploaded artifact: $ARCHIVE_NAME"
      ;;
    4|config|config-only|--config-only)
      update_remote_config
      log "Done. Remote config updated: $DEPLOY_REMOTE_DIR/.env"
      ;;
    5|manual-start|--manual-start)
      manual_start
      ;;
    6|manual-stop|--manual-stop)
      manual_stop
      ;;
    7|manual-rollback-list|--manual-rollback-list)
      manual_rollback_list
      ;;
    8|manual-rollback|--manual-rollback)
      [[ -n "${2:-}" ]] || fail "Usage: $0 --manual-rollback <release-id>"
      manual_rollback "$2"
      ;;
    manual-rollback:*)
      manual_rollback "${mode#manual-rollback:}"
      ;;
    9|manual-status|--manual-status)
      manual_status
      ;;
    10|manual-logs|--manual-logs)
      manual_logs
      ;;
    11|manual-service-start|--manual-service-start)
      manual_service_start
      ;;
    12|manual-service-stop|--manual-service-stop)
      manual_service_stop
      ;;
    13|zero-downtime-test|--zero-downtime-test)
      zero_downtime_test
      ;;
    14|rolling-full|--rolling-full)
      rolling_full_deploy
      log "Done. Rolling artifact: $ARCHIVE_PATH"
      ;;
    15|rolling-code|--rolling-code)
      rolling_code_deploy
      log "Done. Rolling code artifact: $ARCHIVE_PATH"
      ;;
    config-check|--config-check)
      config_check
      ;;
    preflight|--preflight|rolling-preflight|--rolling-preflight)
      rolling_preflight
      ;;
    --install-artifact-with-config)
      install_artifact_with_config
      ;;
    -h|--help|help)
      usage
      ;;
    q|Q|quit|exit)
      log "Canceled."
      ;;
    *)
      usage
      fail "Unknown option: $mode"
      ;;
  esac
}

main() {
  local mode="${1:-}"
  local menu_payload menu_target
  case "$mode" in
    -h|--help|help)
      usage
      return 0
      ;;
  esac

  init_context

  if [[ -z "$mode" ]]; then
    if [[ -t 0 ]]; then
      mode="$(choose_mode)"
    else
      usage
      fail "No interactive terminal detected. Use --rolling-full, --full, --build-only, --upload-only, --config-only, or a manual operation option."
    fi
  fi

  if [[ $# -eq 0 ]]; then
    if [[ "$mode" == target:*:* ]]; then
      menu_payload="${mode#target:}"
      menu_target="${menu_payload%%:*}"
      mode="${menu_payload#*:}"
      case "$menu_target" in primary|standby) ;;
        *) fail "Invalid menu target: $menu_target" ;;
      esac
      if [[ "$menu_target" != "$DEPLOY_TARGET" ]]; then
        DEPLOY_TARGET="$menu_target"
        init_context
      fi
    fi
    run_mode "$mode"
  else
    run_mode "$@"
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
