#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

require_env V2_COMPOSE_DIR
if [[ -f "$ops_state_dir/traffic-opened-at" ]]; then
  [[ "${1:-}" == "--maintenance-only" ]] || die "v3 has accepted traffic; writable v2 rollback would lose v3 changes. Use --maintenance-only and perform forward recovery."
  "$ops_root/ops/blue-green.sh" maintenance
  log "traffic returned to maintenance. v2 remains read-only/stopped for safe forward recovery."
  exit 0
fi
[[ "${1:-}" == "--pre-traffic" ]] || die "pre-traffic rollback requires --pre-traffic"
v2_env_args=(); [[ -n "${V2_ENV_FILE:-}" ]] && v2_env_args=(--env-file "$V2_ENV_FILE")
v2_compose_file="${V2_COMPOSE_FILE:-${V2_COMPOSE_DIR}/docker-compose.yml}"
[[ -f "$v2_compose_file" ]] || die "v2 Compose file is missing: $v2_compose_file"
read -r -a v2_writers <<<"${V2_WRITER_SERVICES:-bot web}"
docker compose --project-directory "$V2_COMPOSE_DIR" -f "$v2_compose_file" "${v2_env_args[@]}" start "${v2_writers[@]}"
require_env V2_PROXY_PORT; switch_proxy_target "$V2_PROXY_PORT"
log "pre-traffic rollback completed; v2 writers and proxy are active"
