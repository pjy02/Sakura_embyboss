#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

[[ "${1:-}" == "--confirm" ]] || die "restore drill requires --confirm"
require_command docker; require_command age; require_command tar; require_command sha256sum; require_command diff
require_env BACKUP_ARCHIVE; require_env AGE_IDENTITY_FILE
archive="$(readlink -m -- "$BACKUP_ARCHIVE")"; identity="$(readlink -m -- "$AGE_IDENTITY_FILE")"
[[ -f "$archive" && -f "$identity" ]] || die "backup archive or age identity is missing"
if [[ -f "${archive}.sha256" ]]; then (cd "$(dirname "$archive")" && sha256sum -c "$(basename "${archive}.sha256")"); fi
stamp="$(date -u +%Y%m%d%H%M%S)"; project="sakura-restore-${stamp}"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/sakura-restore.XXXXXX")"
trap 'docker compose -f "${ops_root}/deploy/compose.restore-drill.yaml" -p "${project}" down -v >/dev/null 2>&1 || true; [[ -n "${tmp:-}" && "$tmp" == "${TMPDIR:-/tmp}"/sakura-restore.* ]] && rm -rf -- "$tmp"' EXIT
age -d -i "$identity" -o "$tmp/backup.tar.gz" "$archive"
tar -C "$tmp" -xzf "$tmp/backup.tar.gz"
bundle_dir="$(find "$tmp" -mindepth 1 -maxdepth 1 -type d -name 'sakura-cutover-*' -print -quit)"
[[ -n "$bundle_dir" ]] || die "invalid backup bundle"
(cd "$bundle_dir" && sha256sum -c SHA256SUMS)
docker compose -f "${ops_root}/deploy/compose.restore-drill.yaml" -p "$project" up -d --wait
log "restoring MySQL into isolated drill project $project"
docker compose -f "${ops_root}/deploy/compose.restore-drill.yaml" -p "$project" exec -T mysql sh -ceu 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' <"$bundle_dir/v2-mysql.sql"
docker compose -f "${ops_root}/deploy/compose.restore-drill.yaml" -p "$project" exec -T mysql sh -ceu 'exec mysql -N -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' <"$ops_root/ops/mysql-table-counts.sql" >"$tmp/restored-counts.tsv"
diff -u "$bundle_dir/v2-counts.tsv" "$tmp/restored-counts.tsv"
if [[ -f "$bundle_dir/v3-postgres.dump" ]]; then docker compose -f "${ops_root}/deploy/compose.restore-drill.yaml" -p "$project" exec -T postgres pg_restore -U sakura -d sakura --clean --if-exists <"$bundle_dir/v3-postgres.dump"; fi
log "restore drill passed; checksums and exact v2 row counts match"
