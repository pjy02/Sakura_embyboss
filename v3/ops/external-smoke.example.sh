#!/usr/bin/env bash
set -Eeuo pipefail

require_file() { [[ -r "${!1:-}" ]] || { printf 'missing readable secret file: %s\n' "$1" >&2; exit 1; }; }
require_file TELEGRAM_TOKEN_FILE
require_file TMDB_TOKEN_FILE
require_file MOVIEPILOT_TOKEN_FILE
require_file EMBY_TOKEN_FILE
: "${EMBY_BASE_URL:?set EMBY_BASE_URL to the exact Emby base URL reachable from the v3 network}"
: "${MOVIEPILOT_HEALTH_URL:?set MOVIEPILOT_HEALTH_URL to a read-only endpoint}"

telegram_token="$(<"$TELEGRAM_TOKEN_FILE")"
tmdb_token="$(<"$TMDB_TOKEN_FILE")"
moviepilot_token="$(<"$MOVIEPILOT_TOKEN_FILE")"
emby_token="$(<"$EMBY_TOKEN_FILE")"

curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  -H "X-Emby-Token: ${emby_token}" \
  "${EMBY_BASE_URL%/}/emby/System/Info" >/dev/null

curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  "https://api.telegram.org/bot${telegram_token}/getMe" >/dev/null
curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  -H "Authorization: Bearer ${tmdb_token}" \
  "${TMDB_API_BASE_URL:-https://api.themoviedb.org}/3/configuration" >/dev/null
curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  -H "Authorization: Bearer ${moviepilot_token}" \
  -H "X-API-KEY: ${moviepilot_token#Bearer }" \
  "$MOVIEPILOT_HEALTH_URL" >/dev/null
printf 'Emby, Telegram, TMDB and MoviePilot smoke checks passed\n'
