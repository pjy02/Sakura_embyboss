#!/usr/bin/env bash
set -Eeuo pipefail

ops_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
ops_state_dir="${SAKURA_OPS_STATE_DIR:-${ops_root}/.state}"

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
log() { printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
require_command() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }
require_env() { [[ -n "${!1:-}" ]] || die "required environment variable is empty: $1"; }
ensure_state_dir() { mkdir -p -- "$ops_state_dir"; chmod 700 "$ops_state_dir"; }

color_port() {
  case "$1:$2" in
    blue:api) printf '%s' "${SAKURA_BLUE_API_PORT:-18080}" ;;
    blue:web) printf '%s' "${SAKURA_BLUE_WEB_PORT:-18088}" ;;
    green:api) printf '%s' "${SAKURA_GREEN_API_PORT:-28080}" ;;
    green:web) printf '%s' "${SAKURA_GREEN_WEB_PORT:-28088}" ;;
    *) die "invalid color/port: $1/$2" ;;
  esac
}

wait_http() {
  local url="$1" attempts="${2:-60}" status
  while (( attempts > 0 )); do
    status="$(curl -sS -o /dev/null -w '%{http_code}' --connect-timeout 2 --max-time 5 "$url" || true)"
    [[ "$status" == "200" ]] && return 0
    attempts=$((attempts-1)); sleep 2
  done
  die "health check did not become ready: $url"
}

color_compose() {
  local color="$1"; shift
  SAKURA_V3_COLOR="$color" \
  SAKURA_V3_COLOR_API_PORT="$(color_port "$color" api)" \
  SAKURA_V3_COLOR_WEB_PORT="$(color_port "$color" web)" \
    docker compose --env-file "${SAKURA_V3_ENV_FILE:-${ops_root}/.env}" \
      -f "${ops_root}/deploy/compose.color.yaml" -p "sakura-v3-${color}" "$@"
}

switch_proxy_target() {
  local target_port="$1" target_file temp_file
  require_env SAKURA_PROXY_TARGET_FILE
  target_file="$(readlink -m -- "$SAKURA_PROXY_TARGET_FILE")"
  [[ "$target_file" != "/" && -n "$target_file" ]] || die "unsafe proxy target path"
  mkdir -p -- "$(dirname "$target_file")"
  temp_file="${target_file}.new.$$"
  printf 'set $sakura_upstream http://127.0.0.1:%s;\n' "$target_port" >"$temp_file"
  chmod 640 "$temp_file"
  mv -f -- "$temp_file" "$target_file"
  if [[ -n "${SAKURA_PROXY_TEST_COMMAND:-}" ]]; then bash -ceu "$SAKURA_PROXY_TEST_COMMAND"; fi
  if [[ -n "${SAKURA_PROXY_RELOAD_COMMAND:-}" ]]; then bash -ceu "$SAKURA_PROXY_RELOAD_COMMAND"; else die "SAKURA_PROXY_RELOAD_COMMAND is required"; fi
}
