#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

mode="${1:-rehearse}"; confirm="${2:-}"
[[ "$mode" == "rehearse" || "$mode" == "execute" ]] || die "usage: cutover.sh rehearse | execute --confirm"
[[ "$mode" != "execute" || "$confirm" == "--confirm" ]] || die "production cutover requires execute --confirm"
require_command docker; require_command curl
[[ "$mode" == "execute" ]] && require_command age
require_env V2_COMPOSE_DIR; require_env V3_ENV_PATH; require_env SAKURA_V2_DATABASE_DSN
[[ "$mode" == "execute" ]] && require_env SAKURA_EXTERNAL_SMOKE_COMMAND
ensure_state_dir
run_id="$(date -u +%Y%m%dT%H%M%SZ)-${mode}"; run_dir="$ops_state_dir/runs/$run_id"; mkdir -p "$run_dir"; chmod 700 "$run_dir"
checkpoint() { printf '%s\n' "$1" >>"$run_dir/checkpoints"; log "checkpoint: $1"; }

active=""; [[ -f "$ops_state_dir/active-color" ]] && active="$(<"$ops_state_dir/active-color")"
if [[ "$active" == "blue" ]]; then candidate=green; else candidate=blue; fi

log "preflight: validating immutable images, paths, database and proxy configuration"
require_env SAKURA_V3_IMAGE; require_env SAKURA_V3_WEB_IMAGE
[[ "$SAKURA_V3_IMAGE" == *@sha256:* || "$SAKURA_V3_IMAGE" == *:* ]] || die "SAKURA_V3_IMAGE must have an immutable tag or digest"
[[ -f "$V3_ENV_PATH" && -d "$V2_COMPOSE_DIR" ]] || die "deployment path is missing"
checkpoint preflight

"$ops_root/ops/blue-green.sh" stage "$candidate"
checkpoint candidate_staged

v3_compose=(docker compose --env-file "$V3_ENV_PATH" -f "$ops_root/compose.yaml")
"${v3_compose[@]}" up -d postgres redis
"${v3_compose[@]}" run --rm migrate
checkpoint migrations_applied

if [[ "$mode" == "execute" ]]; then
  "$ops_root/ops/blue-green.sh" maintenance
  checkpoint maintenance_enabled
  v2_env_args=(); [[ -n "${V2_ENV_FILE:-}" ]] && v2_env_args=(--env-file "$V2_ENV_FILE")
  v2_compose_file="${V2_COMPOSE_FILE:-${V2_COMPOSE_DIR}/docker-compose.yml}"
  [[ -f "$v2_compose_file" ]] || die "v2 Compose file is missing: $v2_compose_file"
  read -r -a v2_writers <<<"${V2_WRITER_SERVICES:-bot web}"
  docker compose --project-directory "$V2_COMPOSE_DIR" -f "$v2_compose_file" "${v2_env_args[@]}" stop "${v2_writers[@]}"
  # The shared data-plane Compose file must not leave a second v3 API, Web,
  # Worker or Bot running beside the selected color.
  "${v3_compose[@]}" stop bot worker web api || true
  checkpoint v2_writers_stopped
  backup_path="$("$ops_root/ops/backup.sh" | tail -n 1)"
  printf '%s\n' "$backup_path" >"$run_dir/backup-path"
  checkpoint encrypted_backup_completed
fi

log "running idempotent final v2 import"
"${v3_compose[@]}" --profile ops run --rm import-v2 | tee "$run_dir/import-report.json"
checkpoint final_import_completed
"${v3_compose[@]}" --profile ops run --rm reconcile-v2 | tee "$run_dir/reconciliation-report.json"
checkpoint financial_account_reconciliation_passed

if [[ "$mode" == "rehearse" ]]; then
  if [[ "${SAKURA_REHEARSAL_LOAD_TEST:-1}" == "1" ]]; then
    "$ops_root/ops/load-test.sh" "$candidate" | tee "$run_dir/load-test.log"
    checkpoint load_test_passed
  fi
  log "rehearsal passed. Production was not placed in maintenance and proxy was not switched."
  exit 0
fi

if [[ "$active" == "blue" || "$active" == "green" ]]; then color_compose "$active" --profile active stop bot worker || true; fi
color_compose "$candidate" --profile active up -d worker bot
[[ -n "$(color_compose "$candidate" --profile active ps --status running -q worker)" ]] || die "candidate Worker is not running"
[[ -n "$(color_compose "$candidate" --profile active ps --status running -q bot)" ]] || die "candidate Bot is not running"
checkpoint singleton_services_started

log "enqueueing one Emby reconciliation per enabled instance"
"${v3_compose[@]}" exec -T postgres sh -ceu 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<SQL
INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,created_by)
SELECT md5(random()::text || clock_timestamp()::text)::uuid, '\''emby.reconcile'\'', '\''cutover-emby-'\'' || id || '\''-'\'' || extract(epoch FROM clock_timestamp())::bigint, jsonb_build_object('\''instance_id'\'',id), '\''system:cutover'\''
FROM emby_instances WHERE enabled
ON CONFLICT(idempotency_key) DO NOTHING;
SQL'
deadline=$((SECONDS+${SAKURA_EMBY_RECONCILE_TIMEOUT_SECONDS:-600}))
while (( SECONDS < deadline )); do
  pending="$("${v3_compose[@]}" exec -T postgres sh -ceu 'psql -At -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT COUNT(*) FROM platform_tasks WHERE created_by='\''system:cutover'\'' AND status IN ('\''pending'\'','\''running'\'','\''retry'\'')"')"
  [[ "$pending" == "0" ]] && break
  sleep 5
done
failed="$("${v3_compose[@]}" exec -T postgres sh -ceu 'psql -At -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT COUNT(*) FROM platform_tasks WHERE created_by='\''system:cutover'\'' AND status IN ('\''failed'\'','\''dead'\'')"')"
[[ "$pending" == "0" && "$failed" == "0" ]] || die "Emby reconciliation did not complete cleanly (pending=$pending failed=$failed)"
checkpoint emby_reconciliation_passed

wait_http "http://127.0.0.1:$(color_port "$candidate" api)/health/ready"
wait_http "http://127.0.0.1:$(color_port "$candidate" web)/healthz"
bash -ceu "$SAKURA_EXTERNAL_SMOKE_COMMAND"
checkpoint runtime_and_integrations_verified

switch_proxy_target "$(color_port "$candidate" web)"
printf '%s\n' "$candidate" >"$ops_state_dir/active-color"
printf '%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$ops_state_dir/traffic-opened-at"
checkpoint proxy_switched
log "cutover completed. Keep v2 writers stopped and old MySQL plus the previous color untouched until the observation window ends."
