#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_ENV_FILE="${DEPLOY_ENV_FILE:-$ROOT_DIR/.env.deploy}"

log() {
  printf '\033[1;34m[deploy]\033[0m %s\n' "$*"
}

fail() {
  printf '\033[1;31m[deploy:error]\033[0m %s\n' "$*" >&2
  exit 1
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
    scp "${ssh_base_args[@]}" "$@"
  fi
}

load_env_file "$DEPLOY_ENV_FILE"

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

require_var DEPLOY_HOST
require_var DEPLOY_REMOTE_DIR
require_var DEPLOY_SERVICE_NAME
require_var DEPLOY_BINARY_NAME
require_var DEPLOY_APP_PORT
require_var DEPLOY_GOOS
require_var DEPLOY_GOARCH
require_var DEPLOY_ARCH_LABEL

APP_ENV_PATH="$ROOT_DIR/$DEPLOY_APP_ENV_FILE"
[[ -f "$APP_ENV_PATH" ]] || fail "Missing app env file: $APP_ENV_PATH. Copy .env.production.example to $DEPLOY_APP_ENV_FILE first."

require_cmd npx
require_cmd go
require_cmd tar
require_cmd ssh
require_cmd scp

if [[ -n "${DEPLOY_PASSWORD:-}" && -z "${DEPLOY_SSH_KEY:-}" ]]; then
  require_cmd sshpass
fi

ssh_base_args=(-p "$DEPLOY_PORT" -o StrictHostKeyChecking=accept-new)
scp_base_args=(-P "$DEPLOY_PORT" -o StrictHostKeyChecking=accept-new)

VERSION_FILE="$ROOT_DIR/VERSION"
APP_VERSION="$(cat "$VERSION_FILE" 2>/dev/null || true)"
if [[ -z "$APP_VERSION" ]]; then
  APP_VERSION="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo dev)"
fi

BUILD_DIR="$ROOT_DIR/build/modelsell"
ARCHIVE_NAME="${DEPLOY_BINARY_NAME}-${DEPLOY_GOOS}-${DEPLOY_ARCH_LABEL}.tar.gz"
ARCHIVE_PATH="$BUILD_DIR/$ARCHIVE_NAME"
BINARY_PATH="$BUILD_DIR/$DEPLOY_BINARY_NAME"

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

log "Package artifact: $ARCHIVE_PATH"
(
  cd "$BUILD_DIR"
  tar -czf "$ARCHIVE_NAME" "$DEPLOY_BINARY_NAME"
)

REMOTE_TMP_DIR="/tmp/${DEPLOY_SERVICE_NAME}-deploy"
REMOTE_ARCHIVE="$REMOTE_TMP_DIR/$ARCHIVE_NAME"
REMOTE_ENV="$REMOTE_TMP_DIR/.env"

log "Prepare remote directory: $DEPLOY_USER@$DEPLOY_HOST:$DEPLOY_REMOTE_DIR"
remote_ssh "mkdir -p '$REMOTE_TMP_DIR' '$DEPLOY_REMOTE_DIR' '$DEPLOY_REMOTE_DIR/logs'"

log "Upload package and runtime env"
remote_scp "$ARCHIVE_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ARCHIVE"
remote_scp "$APP_ENV_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ENV"

log "Install and restart service: $DEPLOY_SERVICE_NAME"
remote_ssh "REMOTE_DIR='$DEPLOY_REMOTE_DIR' SERVICE_NAME='$DEPLOY_SERVICE_NAME' BINARY_NAME='$DEPLOY_BINARY_NAME' APP_PORT='$DEPLOY_APP_PORT' ARCHIVE_PATH='$REMOTE_ARCHIVE' ENV_PATH='$REMOTE_ENV' bash -s" <<'REMOTE_SCRIPT'
set -Eeuo pipefail

mkdir -p "$REMOTE_DIR" "$REMOTE_DIR/logs"
tar -xzf "$ARCHIVE_PATH" -C "$REMOTE_DIR"
chmod +x "$REMOTE_DIR/$BINARY_NAME"
cp "$ENV_PATH" "$REMOTE_DIR/.env"
chmod 600 "$REMOTE_DIR/.env"

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
ExecStart=${REMOTE_DIR}/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
sleep 2
systemctl --no-pager --full status "$SERVICE_NAME" || journalctl -u "$SERVICE_NAME" -n 100 --no-pager
REMOTE_SCRIPT

log "Done. Local artifact: $ARCHIVE_PATH"
