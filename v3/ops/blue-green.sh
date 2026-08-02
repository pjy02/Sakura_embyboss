#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

require_command docker; require_command curl
ensure_state_dir
action="${1:-status}"; color="${2:-}"
active_file="$ops_state_dir/active-color"

case "$action" in
  stage)
    [[ "$color" == "blue" || "$color" == "green" ]] || die "usage: blue-green.sh stage blue|green"
    log "pulling and staging $color API/Web without Bot or Worker"
    color_compose "$color" pull api web
    color_compose "$color" up -d --remove-orphans api web
    wait_http "http://127.0.0.1:$(color_port "$color" api)/health/ready"
    wait_http "http://127.0.0.1:$(color_port "$color" web)/healthz"
    log "$color candidate is healthy"
    ;;
  activate)
    [[ "$color" == "blue" || "$color" == "green" ]] || die "usage: blue-green.sh activate blue|green"
    "$0" stage "$color"
    previous=""; [[ -f "$active_file" ]] && previous="$(<"$active_file")"
    if [[ "$previous" == "blue" || "$previous" == "green" ]]; then
      log "stopping singleton Bot/Worker on $previous"
      color_compose "$previous" --profile active stop bot worker || true
    fi
    if ! color_compose "$color" --profile active up -d worker bot; then
      [[ -n "$previous" ]] && color_compose "$previous" --profile active up -d worker bot || true
      die "failed to start singleton services on $color"
    fi
    switch_proxy_target "$(color_port "$color" web)"
    printf '%s\n' "$color" >"$active_file"
    log "$color is active; previous color remains available for read-only rollback inspection"
    ;;
  maintenance)
    docker compose -f "$ops_root/deploy/compose.maintenance.yaml" up -d --wait
    switch_proxy_target "${SAKURA_MAINTENANCE_PORT:-18090}"
    log "maintenance page is active"
    ;;
  retire)
    [[ "$color" == "blue" || "$color" == "green" ]] || die "usage: blue-green.sh retire blue|green"
    current=""; [[ -f "$active_file" ]] && current="$(<"$active_file")"
    [[ "$color" != "$current" ]] || die "refusing to retire active color $color"
    color_compose "$color" --profile active down
    log "$color retired; shared PostgreSQL/Redis were not touched"
    ;;
  status)
    current="none"; [[ -f "$active_file" ]] && current="$(<"$active_file")"
    printf 'active=%s\n' "$current"
    for candidate in blue green; do
      printf '%s_api=' "$candidate"; curl -sS -o /dev/null -w '%{http_code}\n' --connect-timeout 1 --max-time 2 "http://127.0.0.1:$(color_port "$candidate" api)/health/ready" || printf 'down\n'
      printf '%s_web=' "$candidate"; curl -sS -o /dev/null -w '%{http_code}\n' --connect-timeout 1 --max-time 2 "http://127.0.0.1:$(color_port "$candidate" web)/healthz" || printf 'down\n'
    done
    ;;
  *) die "usage: blue-green.sh {stage|activate|maintenance|retire|status} [blue|green]" ;;
esac
