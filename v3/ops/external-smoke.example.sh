#!/usr/bin/env bash
set -Eeuo pipefail

require_file() { [[ -r "${!1:-}" ]] || { printf 'missing readable secret file: %s\n' "$1" >&2; exit 1; }; }
require_file TELEGRAM_TOKEN_FILE
require_file TMDB_TOKEN_FILE
require_file MOVIEPILOT_TOKEN_FILE
: "${MOVIEPILOT_HEALTH_URL:?set MOVIEPILOT_HEALTH_URL to a read-only endpoint}"

telegram_token="$(<"$TELEGRAM_TOKEN_FILE")"
tmdb_token="$(<"$TMDB_TOKEN_FILE")"
moviepilot_token="$(<"$MOVIEPILOT_TOKEN_FILE")"

curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  "https://api.telegram.org/bot${telegram_token}/getMe" >/dev/null
curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  -H "Authorization: Bearer ${tmdb_token}" \
  "${TMDB_API_BASE_URL:-https://api.themoviedb.org}/3/configuration" >/dev/null
curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  -H "Authorization: Bearer ${moviepilot_token}" \
  "$MOVIEPILOT_HEALTH_URL" >/dev/null
printf 'Telegram, TMDB and MoviePilot smoke checks passed\n'
