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

Modes:
  1, --full         Build, upload, update config, install systemd, restart service.
  2, --build-only   Build frontend and Linux x86_64 package locally only.
  3, --upload-only  Upload existing package, install release, restart service. Does not update .env.
  4, --config-only  Upload runtime .env, refresh systemd, restart service. Does not upload binary.
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
  DEPLOY_HEALTH_PATH="${DEPLOY_HEALTH_PATH:-/api/status}"
  DEPLOY_HEALTH_TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-30}"
  DEPLOY_KEEP_RELEASES="${DEPLOY_KEEP_RELEASES:-5}"

  require_var DEPLOY_HOST
  require_var DEPLOY_REMOTE_DIR
  require_var DEPLOY_SERVICE_NAME
  require_var DEPLOY_BINARY_NAME
  require_var DEPLOY_APP_PORT
  require_var DEPLOY_GOOS
  require_var DEPLOY_GOARCH
  require_var DEPLOY_ARCH_LABEL

  APP_ENV_PATH="$ROOT_DIR/$DEPLOY_APP_ENV_FILE"
  BUILD_DIR="$ROOT_DIR/build/modelsell"
  ARCHIVE_NAME="${DEPLOY_BINARY_NAME}-${DEPLOY_GOOS}-${DEPLOY_ARCH_LABEL}.tar.gz"
  ARCHIVE_PATH="$BUILD_DIR/$ARCHIVE_NAME"
  BINARY_PATH="$BUILD_DIR/$DEPLOY_BINARY_NAME"
  REMOTE_TMP_DIR="/tmp/${DEPLOY_SERVICE_NAME}-deploy"
  REMOTE_ARCHIVE="$REMOTE_TMP_DIR/$ARCHIVE_NAME"
  REMOTE_ENV="$REMOTE_TMP_DIR/.env"

  ssh_base_args=(-p "$DEPLOY_PORT" -o StrictHostKeyChecking=accept-new)
  scp_base_args=(-P "$DEPLOY_PORT" -o StrictHostKeyChecking=accept-new)

  VERSION_FILE="$ROOT_DIR/VERSION"
  APP_VERSION="$(cat "$VERSION_FILE" 2>/dev/null || true)"
  if [[ -z "$APP_VERSION" ]]; then
    APP_VERSION="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo dev)"
  fi
  APP_VERSION_SAFE="$(printf '%s' "$APP_VERSION" | tr -c 'A-Za-z0-9._-' '_')"
  DEPLOY_RELEASE_ID="${DEPLOY_RELEASE_ID:-$(date -u +%Y%m%d%H%M%S)-$APP_VERSION_SAFE}"
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

  log "Package artifact: $ARCHIVE_PATH"
  (
    cd "$BUILD_DIR"
    tar -czf "$ARCHIVE_NAME" "$DEPLOY_BINARY_NAME"
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

  remote_ssh "REMOTE_DIR='$DEPLOY_REMOTE_DIR' SERVICE_NAME='$DEPLOY_SERVICE_NAME' BINARY_NAME='$DEPLOY_BINARY_NAME' APP_PORT='$DEPLOY_APP_PORT' ARCHIVE_PATH='$REMOTE_ARCHIVE' ENV_PATH='$REMOTE_ENV' RELEASE_ID='$DEPLOY_RELEASE_ID' APPLY_RELEASE='$apply_release' INSTALL_ENV='$install_env' HEALTH_PATH='$DEPLOY_HEALTH_PATH' HEALTH_TIMEOUT='$DEPLOY_HEALTH_TIMEOUT' KEEP_RELEASES='$DEPLOY_KEEP_RELEASES' bash -s" <<'REMOTE_SCRIPT'
set -Eeuo pipefail

RELEASES_DIR="$REMOTE_DIR/releases"
CURRENT_LINK="$REMOTE_DIR/current"
RELEASE_DIR="$RELEASES_DIR/$RELEASE_ID"
ENV_BACKUP=""

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
ExecStart=${REMOTE_DIR}/current/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME"
}

normalize_health_path() {
  if [[ "$HEALTH_PATH" != /* ]]; then
    HEALTH_PATH="/$HEALTH_PATH"
  fi
}

wait_service_healthy() {
  normalize_health_path
  local url="http://127.0.0.1:${APP_PORT}${HEALTH_PATH}"
  local deadline=$((SECONDS + HEALTH_TIMEOUT))

  while (( SECONDS < deadline )); do
    if systemctl is-active --quiet "$SERVICE_NAME" && curl -fsS --max-time 3 "$url" >/dev/null; then
      log "Health check passed: $url"
      return 0
    fi
    sleep 1
  done

  warn "Health check failed: $url"
  return 1
}

restart_and_verify() {
  systemctl restart "$SERVICE_NAME"
  wait_service_healthy
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
  fi

  systemctl daemon-reload || true
  systemctl restart "$SERVICE_NAME" || true
  if wait_service_healthy; then
    warn "Rollback completed"
  else
    warn "Rollback health check failed"
    show_service_debug
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

command -v curl >/dev/null 2>&1 || {
  echo "Missing remote command: curl" >&2
  exit 1
}

mkdir -p "$REMOTE_DIR" "$REMOTE_DIR/logs" "$RELEASES_DIR"
ensure_legacy_release

if ! [[ "$HEALTH_TIMEOUT" =~ ^[0-9]+$ ]] || (( HEALTH_TIMEOUT < 1 )); then
  HEALTH_TIMEOUT=30
fi

PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"

if [[ "$APPLY_RELEASE" == "1" ]]; then
  rm -rf "$RELEASE_DIR"
  mkdir -p "$RELEASE_DIR"
  tar -xzf "$ARCHIVE_PATH" -C "$RELEASE_DIR"
  chmod +x "$RELEASE_DIR/$BINARY_NAME"
fi

if [[ "$INSTALL_ENV" == "1" ]]; then
  if [[ -f "$REMOTE_DIR/.env" ]]; then
    ENV_BACKUP="$REMOTE_DIR/.env.rollback.$RELEASE_ID"
    cp "$REMOTE_DIR/.env" "$ENV_BACKUP"
    chmod 600 "$ENV_BACKUP"
  fi
  cp "$ENV_PATH" "$REMOTE_DIR/.env"
  chmod 600 "$REMOTE_DIR/.env"
fi

install_service_config

if [[ "$APPLY_RELEASE" == "1" ]]; then
  ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"
  if ! restart_and_verify; then
    rollback_service "$PREVIOUS_TARGET"
    exit 1
  fi
  cleanup_old_releases
  exit 0
fi

if [[ -x "$CURRENT_LINK/$BINARY_NAME" ]]; then
  if ! restart_and_verify; then
    rollback_service "$PREVIOUS_TARGET"
    exit 1
  fi
else
  log "Config updated. Current binary not found yet: $CURRENT_LINK/$BINARY_NAME"
fi
REMOTE_SCRIPT
}

full_deploy() {
  build_package
  require_artifact
  require_app_env
  prepare_remote

  log "Upload package and runtime env"
  remote_scp "$ARCHIVE_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ARCHIVE"
  remote_scp "$APP_ENV_PATH" "$DEPLOY_USER@$DEPLOY_HOST:$REMOTE_ENV"

  log "Install release, systemd config, and restart service with health-check rollback: $DEPLOY_SERVICE_NAME"
  run_remote_deploy 1 1
}

choose_mode() {
  cat >&2 <<EOF

请选择部署操作：
  1) 自动化打包上传并配置
  2) 只打包
  3) 只上传
  4) 更新配置文件
  q) 退出

EOF
  printf "请输入选项 [1-4/q]: " >&2
  read -r choice || choice="q"
  if [[ -z "$choice" ]]; then
    choice="q"
  fi
  echo "$choice"
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
  init_context

  local mode="${1:-}"
  if [[ -z "$mode" ]]; then
    if [[ -t 0 ]]; then
      mode="$(choose_mode)"
    else
      usage
      fail "No interactive terminal detected. Use --full, --build-only, --upload-only, or --config-only."
    fi
  fi

  run_mode "$mode"
}

main "$@"
