#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

[[ "${1:-}" == "--confirm" ]] || die "fault drill requires --confirm"
[[ "${SAKURA_DRILL_ENVIRONMENT:-}" == "staging" ]] || die "fault drill is restricted to SAKURA_DRILL_ENVIRONMENT=staging"
require_command docker; require_command curl; require_env V3_ENV_PATH
compose=(docker compose --env-file "$V3_ENV_PATH" -f "$ops_root/compose.yaml")
recover() { "${compose[@]}" start postgres redis worker >/dev/null 2>&1 || true; }
trap recover EXIT

log "drill 1/3: Redis outage keeps API live but removes readiness"
"${compose[@]}" stop redis
wait_http "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/health/live" 10
ready="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/health/ready" || true)"
[[ "$ready" == "503" ]] || die "API readiness should be 503 during Redis outage, got $ready"
"${compose[@]}" start redis; wait_http "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/health/ready"

log "drill 2/3: PostgreSQL outage is detected and recovers"
"${compose[@]}" stop postgres
wait_http "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/health/live" 10
ready="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/health/ready" || true)"
[[ "$ready" == "503" ]] || die "API readiness should be 503 during PostgreSQL outage, got $ready"
"${compose[@]}" start postgres; wait_http "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/health/ready"

log "drill 3/3: Worker restart does not affect API queries"
"${compose[@]}" stop worker
wait_http "http://127.0.0.1:${SAKURA_V3_API_BIND_PORT:-8080}/api/v3/system/info" 10
"${compose[@]}" start worker
sleep 5
[[ -n "$("${compose[@]}" ps --status running -q worker)" ]] || die "worker did not recover"
trap - EXIT
log "fault recovery drill passed"
