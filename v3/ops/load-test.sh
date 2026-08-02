#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

require_command docker
color="${1:-blue}"; [[ "$color" == "blue" || "$color" == "green" ]] || die "usage: load-test.sh blue|green"
base_url="${BASE_URL:-http://127.0.0.1:$(color_port "$color" web)}"
log "running k6 against $color candidate at $base_url"
docker run --rm --network host \
  -e BASE_URL="$base_url" -e VUS="${VUS:-50}" -e RAMP_UP="${RAMP_UP:-30s}" -e HOLD="${HOLD:-2m}" \
  -v "$ops_root/ops/load:/scripts:ro" grafana/k6:0.57.0 run /scripts/k6.js
