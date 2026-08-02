#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/deploy-modelsell.sh"
SEO_VERIFY_SCRIPT="$ROOT_DIR/scripts/verify-modelsell-seo.sh"

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

assert_remote_rollout_helpers() {
  local extracted functions sandbox
  extracted="$(mktemp)"
  functions="$(mktemp)"
  sandbox="$(mktemp -d)"
  awk '
    capture && $0 == "REMOTE_SCRIPT" { exit }
    capture { print }
    index($0, "<<") && index($0, "REMOTE_SCRIPT") { capture = 1 }
  ' "$SCRIPT" >"$extracted"
  awk '/^command -v curl >\/dev\/null/ { exit } { print }' "$extracted" >"$functions"

  (
    export REMOTE_DIR="$sandbox/remote"
    export SERVICE_NAME=modelsell
    export BINARY_NAME=new-api
    export APP_PORT=3000
    export RELEASE_ID=test-release
    export STREAM_LOGS=false
    export CADDY_CONFIG="$sandbox/Caddyfile"
    export PEER_UPSTREAM=''
    export CADDY_PROXY_SNIPPETS=''
    export DRAIN_HEALTH_PATH=/api/ready
    export EXPECT_NODE_TYPE=master
    export EXPECT_PEER_NODE_TYPE=''
    export IPV6_DRAIN_PROXY_PORT=39002
    export TRAFFIC_SETTLE_SECONDS=0
    export PEER_STABLE_CHECKS=1
    export POST_START_STABLE_CHECKS=1
    export POST_START_SOAK_SECONDS=0
    export SHUTDOWN_TIMEOUT=900
    export STOP_TIMEOUT=930
    export DIRECT_PORT_DRAIN=true
    export DIRECT_IPV6_DRAIN=true
    export APPLY_RELEASE=1
    export INSTALL_ENV=1
    # shellcheck disable=SC1090
    source "$functions"

    caddy() {
      [[ "$1" == validate ]]
    }

    ZERO_DOWNTIME=false
    validate_rollout_settings || fail 'single-machine rollout settings must not require peer configuration'
    ZERO_DOWNTIME=true
    EXPECT_NODE_TYPE=slave
    EXPECT_PEER_NODE_TYPE=master
    PEER_UPSTREAM=10.0.0.2:3000
    CADDY_PROXY_SNIPPETS=common_proxy
    validate_rollout_settings || fail 'multi-machine rollout settings must accept valid peer configuration'

    mkdir -p "$REMOTE_DIR"
    mkdir -p "$REMOTE_DIR/releases/previous"
    PREVIOUS_TARGET="$REMOTE_DIR/releases/previous"
    ENV_BACKUP="$REMOTE_DIR/.env.rollback.test-release"
    printf '%s\n' 'old-env' >"$ENV_BACKUP"
    printf '%s\n' \
      '(common_proxy) {' \
      '  reverse_proxy 127.0.0.1:3000 10.0.0.2:3000 {' \
      '    lb_policy first' \
      '  }' \
      '}' >"$CADDY_CONFIG"

    render_caddy_upstreams peer "$CADDY_CONFIG" "$sandbox/rendered"
    grep -Fq 'reverse_proxy 10.0.0.2:3000 {' "$sandbox/rendered"
    IPV6_DRAIN_REQUIRED=true
    render_caddy_upstreams peer "$CADDY_CONFIG" "$sandbox/rendered-ipv6"
    grep -Fq 'http://:39002 {' "$sandbox/rendered-ipv6"
    grep -Fq 'bind ::' "$sandbox/rendered-ipv6"
    grep -Fq 'reverse_proxy 10.0.0.2:3000' "$sandbox/rendered-ipv6"
    IPV6_DRAIN_REQUIRED=false

    printf '%s\n' '{"success":true,"data":{"node_type":"master","checks":{"database":"ok","log_database":"ok","redis":"ok"}}}' >"$sandbox/ready.json"
    verify_ready_payload "$sandbox/ready.json" master
    if verify_ready_payload "$sandbox/ready.json" slave >/dev/null 2>&1; then
      fail 'readiness role verification accepted the wrong node'
    fi

    prepare_drain_state
    grep -Fq 'phase=prepared' "$DRAIN_STATE_FILE"
    grep -Fq 'reverse_proxy 127.0.0.1:3000 10.0.0.2:3000 {' "$CADDY_BACKUP"
    grep -Fq 'reverse_proxy 10.0.0.2:3000 {' "$CADDY_DRAIN_CANDIDATE"

    CADDY_BACKUP=''
    CADDY_DRAIN_CANDIDATE=''
    DRAIN_STATE_PHASE=''
    PREVIOUS_TARGET="$REMOTE_DIR/releases/wrong"
    ENV_BACKUP="$REMOTE_DIR/.env.rollback.wrong"
    load_drain_state
    [[ "$DRAIN_STATE_PHASE" == prepared ]]
    [[ -f "$CADDY_BACKUP" && -f "$CADDY_DRAIN_CANDIDATE" ]]
    [[ "$PREVIOUS_TARGET" == "$REMOTE_DIR/releases/previous" ]]
    [[ "$ENV_BACKUP" == "$REMOTE_DIR/.env.rollback.test-release" ]]
  ) || fail 'remote rollout helper behavior check failed'

  rm -f "$extracted" "$functions"
  rm -rf -- "$sandbox"
}

assert_deploy_topologies() {
  local sandbox config single_config primary_output standby_output single_output override_output
  sandbox="$(mktemp -d)"
  config="$sandbox/deploy.env"
  printf '%s\n' \
    'DEPLOY_TOPOLOGY=multi' \
    'DEPLOY_TARGET=primary' \
    'DEPLOY_PRIMARY_HOST=primary.example' \
    'DEPLOY_PRIMARY_PORT=2201' \
    'DEPLOY_PRIMARY_USER=primary-user' \
    'DEPLOY_PRIMARY_APP_ENV_FILE=.env.production-primary' \
    'DEPLOY_PRIMARY_NODE_TYPE=master' \
    'DEPLOY_PRIMARY_WIREGUARD_UPSTREAM=10.0.0.1:3000' \
    'DEPLOY_PRIMARY_CADDY_PROXY_SNIPPETS=primary_proxy' \
    'DEPLOY_STANDBY_HOST=standby.example' \
    'DEPLOY_STANDBY_PORT=2202' \
    'DEPLOY_STANDBY_USER=standby-user' \
    'DEPLOY_STANDBY_APP_ENV_FILE=.env.production-standby' \
    'DEPLOY_STANDBY_NODE_TYPE=slave' \
    'DEPLOY_STANDBY_WIREGUARD_UPSTREAM=10.0.0.2:3000' \
    'DEPLOY_STANDBY_CADDY_PROXY_SNIPPETS=standby_proxy' >"$config"

  primary_output="$(DEPLOY_ENV_FILE="$config" DEPLOY_TARGET=primary "$SCRIPT" --config-check)"
  grep -Fq 'CONFIG_CHECK=ok' <<<"$primary_output"
  grep -Fq 'DEPLOY_TOPOLOGY=multi' <<<"$primary_output"
  grep -Fq 'SELECTED_TARGET=primary' <<<"$primary_output"
  grep -Fq 'SELECTED_HOST=primary.example' <<<"$primary_output"
  grep -Fq 'SELECTED_NODE_TYPE=master' <<<"$primary_output"
  grep -Fq 'SELECTED_PEER_UPSTREAM=10.0.0.2:3000' <<<"$primary_output"
  grep -Fq 'ZERO_DOWNTIME_ACTIVE=true' <<<"$primary_output"

  standby_output="$(DEPLOY_ENV_FILE="$config" DEPLOY_TARGET=standby "$SCRIPT" --config-check)"
  grep -Fq 'CONFIG_CHECK=ok' <<<"$standby_output"
  grep -Fq 'SELECTED_TARGET=standby' <<<"$standby_output"
  grep -Fq 'SELECTED_HOST=standby.example' <<<"$standby_output"
  grep -Fq 'SELECTED_NODE_TYPE=slave' <<<"$standby_output"
  grep -Fq 'SELECTED_PEER_UPSTREAM=10.0.0.1:3000' <<<"$standby_output"
  grep -Fq 'ZERO_DOWNTIME_ACTIVE=true' <<<"$standby_output"

  override_output="$(DEPLOY_ENV_FILE="$config" DEPLOY_TARGET=standby DEPLOY_ZERO_DOWNTIME=true "$SCRIPT" --config-check)"
  grep -Fq 'SELECTED_TARGET=standby' <<<"$override_output" || \
    fail 'invocation target override must win over the env file'
  grep -Fq 'ZERO_DOWNTIME_ACTIVE=true' <<<"$override_output" || \
    fail 'invocation zero-downtime override must win over the env file'

  if DEPLOY_ENV_FILE="$config" DEPLOY_TARGET=backup "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'legacy backup target must be rejected as ambiguous'
  fi

  sed 's/DEPLOY_STANDBY_HOST=standby.example/DEPLOY_STANDBY_HOST=primary.example/' "$config" >"$sandbox/invalid.env"
  if DEPLOY_ENV_FILE="$sandbox/invalid.env" "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'same host must not be accepted for primary and standby'
  fi

  sed 's/DEPLOY_STANDBY_NODE_TYPE=slave/DEPLOY_STANDBY_NODE_TYPE=master/' "$config" >"$sandbox/invalid.env"
  if DEPLOY_ENV_FILE="$sandbox/invalid.env" "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'swapped or duplicate node roles must not be accepted'
  fi

  sed 's#DEPLOY_STANDBY_APP_ENV_FILE=.env.production-standby#DEPLOY_STANDBY_APP_ENV_FILE=.env.production-primary#' "$config" >"$sandbox/invalid.env"
  if DEPLOY_ENV_FILE="$sandbox/invalid.env" "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'primary and standby must not share one runtime env file'
  fi

  sed 's/DEPLOY_STANDBY_PORT=2202/DEPLOY_STANDBY_PORT=70000/' "$config" >"$sandbox/invalid.env"
  if DEPLOY_ENV_FILE="$sandbox/invalid.env" "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'out-of-range SSH ports must not be accepted'
  fi

  single_config="$sandbox/single.env"
  printf '%s\n' \
    'DEPLOY_TOPOLOGY=single' \
    'DEPLOY_PRIMARY_HOST=single.example' \
    'DEPLOY_PRIMARY_PORT=2203' \
    'DEPLOY_PRIMARY_USER=single-user' \
    'DEPLOY_PRIMARY_APP_ENV_FILE=.env.production-single' >"$single_config"

  single_output="$(DEPLOY_ENV_FILE="$single_config" "$SCRIPT" --config-check)"
  grep -Fq 'CONFIG_CHECK=ok' <<<"$single_output"
  grep -Fq 'DEPLOY_TOPOLOGY=single' <<<"$single_output"
  grep -Fq 'SELECTED_TARGET=primary' <<<"$single_output"
  grep -Fq 'SELECTED_HOST=single.example' <<<"$single_output"
  grep -Fq 'SELECTED_NODE_TYPE=master' <<<"$single_output"
  grep -Fq 'SELECTED_APP_ENV_FILE=.env.production-single' <<<"$single_output"
  grep -Fq 'ZERO_DOWNTIME_ACTIVE=false' <<<"$single_output"
  if grep -Fq 'SELECTED_PEER_UPSTREAM=' <<<"$single_output"; then
    fail 'single topology must not expose or require a peer upstream'
  fi

  override_output="$(DEPLOY_ENV_FILE="$config" DEPLOY_TOPOLOGY=single "$SCRIPT" --config-check)"
  grep -Fq 'DEPLOY_TOPOLOGY=single' <<<"$override_output" || \
    fail 'invocation topology override must win over the env file'
  grep -Fq 'SELECTED_TARGET=primary' <<<"$override_output" || \
    fail 'a topology override must not inherit an incompatible file target'
  grep -Fq 'SELECTED_HOST=primary.example' <<<"$override_output"

  if DEPLOY_ENV_FILE="$single_config" DEPLOY_TARGET=standby "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'single topology must reject multi-machine targets'
  fi
  if DEPLOY_ENV_FILE="$single_config" DEPLOY_ZERO_DOWNTIME=true "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'single topology must reject peer-based zero-downtime mode'
  fi
  if DEPLOY_ENV_FILE="$single_config" "$SCRIPT" --rolling-full >/dev/null 2>&1; then
    fail 'single topology must reject rolling-full before any remote action'
  fi
  if DEPLOY_ENV_FILE="$single_config" "$SCRIPT" --zero-downtime-test >/dev/null 2>&1; then
    fail 'single topology must reject zero-downtime-test before any remote action'
  fi

  sed 's/DEPLOY_TOPOLOGY=single/DEPLOY_TOPOLOGY=cluster/' "$single_config" >"$sandbox/invalid.env"
  if DEPLOY_ENV_FILE="$sandbox/invalid.env" "$SCRIPT" --config-check >/dev/null 2>&1; then
    fail 'unknown deployment topology must not be accepted'
  fi

  DEPLOY_ENV_FILE="$sandbox/missing.env" "$SCRIPT" --help >/dev/null || \
    fail '--help must not require or load a deployment config'

  rm -rf -- "$sandbox"
}

assert_interactive_menu() {
  local output

  output="$(bash -c 'source "$1"; DEPLOY_TOPOLOGY=single; DEPLOY_TARGET=primary; choose_mode' _ "$SCRIPT" \
    2>/dev/null <<'EOF'
1
1
EOF
)"
  [[ "$output" == 'target:primary:full' ]] || \
    fail "single release menu returned an unexpected action: $output"

  output="$(bash -c 'source "$1"; DEPLOY_TOPOLOGY=multi; DEPLOY_TARGET=primary; choose_mode' _ "$SCRIPT" \
    2>/dev/null <<'EOF'
1
1
EOF
)"
  [[ "$output" == 'target:primary:rolling-full' ]] || \
    fail "multi release menu returned an unexpected action: $output"

  output="$(bash -c 'source "$1"; DEPLOY_TOPOLOGY=multi; DEPLOY_TARGET=primary; choose_mode' _ "$SCRIPT" \
    2>/dev/null <<'EOF'
6
2
2
3
EOF
)"
  [[ "$output" == 'target:standby:manual-status' ]] || \
    fail "menu target switching returned an unexpected action: $output"

  output="$(bash -c 'source "$1"; DEPLOY_TOPOLOGY=single; DEPLOY_TARGET=primary; choose_mode' _ "$SCRIPT" \
    2>/dev/null <<'EOF'
1
0
5
1
EOF
)"
  [[ "$output" == 'target:primary:build-only' ]] || \
    fail "menu back navigation returned an unexpected action: $output"

  output="$(bash -c 'source "$1"; DEPLOY_TOPOLOGY=multi; DEPLOY_TARGET=standby; choose_mode' _ "$SCRIPT" \
    2>/dev/null <<'EOF'
4
2
release-123
EOF
)"
  [[ "$output" == 'target:standby:manual-rollback:release-123' ]] || \
    fail "rollback menu returned an unexpected action: $output"
}

assert_contains 'DEPLOY_HEALTH_PATH="${DEPLOY_HEALTH_PATH:-/api/ready}"'
assert_contains 'DEPLOY_HEALTH_TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-180}"'
assert_contains 'DEPLOY_KEEP_RELEASES="${DEPLOY_KEEP_RELEASES:-5}"'
assert_contains 'DEPLOY_SEO_VERIFY="${DEPLOY_SEO_VERIFY:-true}"'
assert_contains 'DEPLOY_SEO_CANONICAL_URL="${DEPLOY_SEO_CANONICAL_URL:-https://modelsell.com}"'
assert_contains 'DEPLOY_ZERO_DOWNTIME="${DEPLOY_ZERO_DOWNTIME:-auto}"'
assert_contains 'validate_two_server_config'
assert_contains 'validate_single_server_config'
assert_contains 'DEPLOY_TOPOLOGY="${DEPLOY_TOPOLOGY:-multi}"'
assert_contains 'DEPLOY_HOST="$DEPLOY_PRIMARY_HOST"'
assert_contains 'DEPLOY_HOST="$DEPLOY_STANDBY_HOST"'
assert_contains 'DEPLOY_PEER_UPSTREAM="$DEPLOY_STANDBY_WIREGUARD_UPSTREAM"'
assert_contains 'DEPLOY_PEER_UPSTREAM="$DEPLOY_PRIMARY_WIREGUARD_UPSTREAM"'
assert_contains 'DEPLOY_TRAFFIC_SETTLE_SECONDS="${DEPLOY_TRAFFIC_SETTLE_SECONDS:-3}"'
assert_contains 'DEPLOY_DIRECT_PORT_DRAIN="${DEPLOY_DIRECT_PORT_DRAIN:-true}"'
assert_contains 'DEPLOY_DIRECT_IPV6_DRAIN="${DEPLOY_DIRECT_IPV6_DRAIN:-true}"'
assert_contains 'DEPLOY_IPV6_DRAIN_PROXY_PORT="${DEPLOY_IPV6_DRAIN_PROXY_PORT:-39002}"'
assert_contains 'DEPLOY_PUBLIC_INTERFACE="${DEPLOY_PUBLIC_INTERFACE:-auto}"'
assert_contains 'DEPLOY_WIREGUARD_INTERFACE="${DEPLOY_WIREGUARD_INTERFACE:-wg0}"'
assert_contains 'DEPLOY_PEER_STABLE_CHECKS="${DEPLOY_PEER_STABLE_CHECKS:-5}"'
assert_contains 'DEPLOY_POST_START_STABLE_CHECKS="${DEPLOY_POST_START_STABLE_CHECKS:-5}"'
assert_contains 'DEPLOY_POST_START_SOAK_SECONDS="${DEPLOY_POST_START_SOAK_SECONDS:-15}"'
assert_contains 'DEPLOY_SHUTDOWN_TIMEOUT="${DEPLOY_SHUTDOWN_TIMEOUT:-900}"'
assert_contains 'DEPLOY_STOP_TIMEOUT="${DEPLOY_STOP_TIMEOUT:-930}"'
assert_contains 'DEPLOY_SMOKE_TESTS="${DEPLOY_SMOKE_TESTS:-true}"'
assert_contains 'select_deploy_target'
assert_contains 'DEPLOY_PASSWORD="${DEPLOY_PRIMARY_PASSWORD:-}"'
assert_contains 'DEPLOY_PASSWORD="${DEPLOY_STANDBY_PASSWORD:-}"'
assert_contains 'DEPLOY_SSH_KEY="${DEPLOY_PRIMARY_SSH_KEY:-}"'
assert_contains 'DEPLOY_SSH_KEY="${DEPLOY_STANDBY_SSH_KEY:-}"'
assert_contains 'DEPLOY_APP_ENV_FILE="$DEPLOY_PRIMARY_APP_ENV_FILE"'
assert_contains 'DEPLOY_APP_ENV_FILE="$DEPLOY_STANDBY_APP_ENV_FILE"'
assert_contains 'Unsupported DEPLOY_TOPOLOGY:'
assert_contains 'Deployment topology: $DEPLOY_TOPOLOGY; target: $DEPLOY_TARGET'
assert_contains 'flock -n '\''/run/lock/${DEPLOY_SERVICE_NAME}-deploy.lock'\'' bash -s'
assert_contains 'CURRENT_LINK="$REMOTE_DIR/current"'
assert_contains 'if [[ -L "$CURRENT_LINK" ]]; then'
assert_contains 'PREVIOUS_TARGET="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"'
assert_contains 'No previous release is available; service left stopped'
assert_contains 'ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"'
assert_contains 'rollback_service "$PREVIOUS_TARGET"'
assert_contains "curl -sS --max-time 3 -o /dev/null -w '%{http_code}' \"\$url\""
assert_contains 'KillMode=control-group'
assert_contains 'Environment=SHUTDOWN_TIMEOUT_SECONDS=${SHUTDOWN_TIMEOUT}'
assert_contains 'KillSignal=SIGTERM'
assert_contains 'TimeoutStopSec=${STOP_TIMEOUT}s'
assert_contains 'Gracefully stopping service with SIGTERM'
assert_contains 'systemctl stop "$SERVICE_NAME" || stop_rc=$?'
assert_contains 'systemctl reset-failed "$SERVICE_NAME"'
assert_contains 'ensure_app_port_available || return 1'
assert_contains 'journalctl -u "$SERVICE_NAME" --since now --follow'
assert_contains 'Health check: elapsed='
assert_contains 'install -m 0755 "$RELEASE_DIR/start-modelsell.sh" "$REMOTE_DIR/bin/start-modelsell.sh"'
assert_contains 'ExecStart=${REMOTE_DIR}/current/${BINARY_NAME} --port ${APP_PORT} --log-dir ${REMOTE_DIR}/logs'
assert_contains 'install -m 0755 "$ROOT_DIR/scripts/start-modelsell.sh" "$BUILD_DIR/start-modelsell.sh"'
assert_contains 'install -m 0755 "$ROOT_DIR/scripts/verify-modelsell-seo.sh" "$BUILD_DIR/verify-modelsell-seo.sh"'
assert_contains 'if ! start_and_verify true; then'
assert_contains 'if start_and_verify false; then'
assert_contains 'if ! start_and_verify false; then'
assert_contains 'switch_traffic_to_peer'
assert_contains 'verify_peer_stable'
assert_contains 'verify_ready_payload'
assert_contains 'EXPECT_PEER_NODE_TYPE'
assert_contains 'unexpected node_type:'
assert_contains 'settle_peer_routing'
assert_contains 'switch_direct_traffic_to_peer'
assert_contains 'switch_ipv6_direct_traffic_to_peer'
assert_contains 'restore_ipv6_direct_traffic'
assert_contains 'Direct IPv6 port drained through Caddy:'
assert_contains 'Public IPv6 is active on $PUBLIC_INTERFACE, but IPv6 direct-port draining is disabled'
assert_contains 'restore_direct_traffic'
assert_contains 'Failed to install one or more direct-port drain rules'
assert_contains 'iptables -w 5 -t nat -D PREROUTING "${prerouting[@]}" || return 1'
assert_contains 'modelsell-deploy-${APP_PORT}'
assert_contains 'active-deploy-drain'
assert_contains 'write_drain_state prepared'
assert_contains 'write_drain_state caddy'
assert_contains 'write_drain_state drained'
assert_contains 'previous_target=%s'
assert_contains 'env_backup=%s'
assert_contains 'Existing deployment transaction found; preserving its original environment backup'
assert_contains 'Resuming interrupted drain: phase='
assert_contains 'Legacy drain state cannot prove the original release and environment; refusing automatic recovery'
assert_contains 'Caddyfile.during-deploy-'
assert_contains 'cmp -s "$CADDY_DRAIN_CANDIDATE" "$CADDY_CONFIG"'
assert_contains 'Caddy config changed outside this deployment; refusing to overwrite it during cutback'
assert_contains 'SIGTERM closes idle keep-alives and waits for active requests'
assert_contains 'restore_local_traffic'
assert_contains 'install -m 0644 "$CADDY_BACKUP" "$CADDY_CONFIG"'
assert_contains 'the peer continues serving domain traffic'
assert_contains 'verify_smoke_tests || return 1'
assert_contains 'verify_service_soak "$restart_baseline"'
assert_contains 'Post-start soak passed:'
assert_contains 'Smoke tests passed: ready/status=2xx, root=2xx/3xx, unauthenticated models=401'
assert_contains '--zero-downtime-test'
assert_contains '--rolling-full'
assert_contains '--config-check'
assert_contains '--preflight'
assert_contains '--rolling-preflight'
assert_contains 'CHECK_MULTI_DEPENDENCIES='
assert_contains 'if [[ "$CHECK_MULTI_DEPENDENCIES" == "true" ]]; then'
assert_contains 'PREFLIGHT_VERSION=%s'
assert_contains 'ModelSell 部署管理'
assert_contains '发布与更新（多机）'
assert_contains '发布与更新（单机）'
assert_contains '检查与观察（目标：${menu_target}）'
assert_contains '版本与回滚（目标：${menu_target}）'
assert_contains 'if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then'

if grep -Fq 'DEPLOY_SINGLE_' "$SCRIPT" || grep -Fq 'DEPLOY_SINGLE_' "$ROOT_DIR/.env.deploy.example"; then
  fail 'single topology must reuse DEPLOY_PRIMARY_* instead of defining DEPLOY_SINGLE_*'
fi
assert_contains 'collect_rollout_preflight primary'
assert_contains 'collect_rollout_preflight standby'
assert_contains 'Rolling deployment requires backward-compatible database migrations'
assert_contains 'ROLLING_PREFLIGHT_VERSION='
assert_contains 'DEPLOY_TARGET=standby DEPLOY_RELEASE_ID="$rolling_release_id" DEPLOY_ZERO_DOWNTIME=true'
assert_contains 'DEPLOY_TARGET=primary DEPLOY_RELEASE_ID="$rolling_release_id" DEPLOY_ZERO_DOWNTIME=true'
assert_contains 'verify_node_role'
assert_contains 'Validating and staging release:'
assert_contains 'Identical release is already active; reusing it without deleting live files'
assert_contains 'Release id is already active with different content; use a new release id'
assert_contains 'tar --no-same-owner --no-same-permissions -xzf'
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

first_drain_line="$(grep -nF 'if ! switch_traffic_to_peer; then' "$SCRIPT" | head -1 | cut -d: -f1)"
release_switch_line="$(grep -nF 'ln -sfnT "$RELEASE_DIR" "$CURRENT_LINK"' "$SCRIPT" | tail -1 | cut -d: -f1)"
[[ -n "$first_drain_line" && -n "$release_switch_line" && "$first_drain_line" -lt "$release_switch_line" ]] || \
  fail 'traffic must drain before switching the release symlink'

standby_deploy_line="$(grep -nF 'DEPLOY_TARGET=standby DEPLOY_RELEASE_ID="$rolling_release_id"' "$SCRIPT" | cut -d: -f1)"
primary_deploy_line="$(grep -nF 'DEPLOY_TARGET=primary DEPLOY_RELEASE_ID="$rolling_release_id"' "$SCRIPT" | cut -d: -f1)"
[[ -n "$standby_deploy_line" && -n "$primary_deploy_line" && "$standby_deploy_line" -lt "$primary_deploy_line" ]] || \
  fail 'rolling deployment must update standby before primary'

standby_preflight_line="$(grep -nF 'standby_version="$(collect_rollout_preflight standby "$deploy_script")"' "$SCRIPT" | cut -d: -f1)"
primary_preflight_line="$(grep -nF 'primary_version="$(collect_rollout_preflight primary "$deploy_script")"' "$SCRIPT" | cut -d: -f1)"
[[ -n "$standby_preflight_line" && -n "$primary_preflight_line" && "$standby_preflight_line" -lt "$primary_preflight_line" ]] || \
  fail 'rolling preflight must inspect standby before primary'

if grep -Fq 'systemctl restart "$SERVICE_NAME"' "$SCRIPT"; then
  fail 'automatic deploy must not treat a stop timeout from systemctl restart as a new-release failure'
fi

if grep -Fq 'KillSignal=SIGKILL' "$SCRIPT"; then
  fail 'automatic deploy must not kill in-flight requests with SIGKILL'
fi

assert_heredoc_syntax REMOTE_SCRIPT
assert_heredoc_syntax REMOTE_STATUS
assert_heredoc_syntax REMOTE_START
assert_heredoc_syntax REMOTE_STOP
assert_heredoc_syntax REMOTE_PREFLIGHT
assert_remote_rollout_helpers
assert_deploy_topologies
assert_interactive_menu

[[ -x "$SEO_VERIFY_SCRIPT" ]] || fail 'SEO verification script must be executable'
bash -n "$SEO_VERIFY_SCRIPT" || fail 'invalid SEO verification script syntax'
grep -Fq 'match($0, /<loc>[^<]+\/pricing\/[^<]+/)' "$SEO_VERIFY_SCRIPT" || \
  fail 'SEO verifier must extract the first model URL without a SIGPIPE-prone pipeline'
if grep -Eq 'grep .*\|[[:space:]]*head' "$SEO_VERIFY_SCRIPT"; then
  fail 'SEO verifier must not use grep | head under pipefail'
fi

printf 'deploy-modelsell script safety checks passed\n'
