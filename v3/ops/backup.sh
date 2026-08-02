#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"

require_command docker; require_command age; require_command sha256sum; require_command tar
require_env V2_COMPOSE_DIR; require_env V2_CONFIG_PATH; require_env V3_ENV_PATH; require_env AGE_RECIPIENT

v2_dir="$(readlink -m -- "$V2_COMPOSE_DIR")"
v2_compose_file="$(readlink -m -- "${V2_COMPOSE_FILE:-${v2_dir}/docker-compose.yml}")"
v2_config="$(readlink -m -- "$V2_CONFIG_PATH")"
v3_env="$(readlink -m -- "$V3_ENV_PATH")"
[[ -d "$v2_dir" && -f "$v2_compose_file" && -f "$v2_config" && -f "$v3_env" ]] || die "backup input path is missing"
output_dir="$(readlink -m -- "${SAKURA_BACKUP_DIR:-${ops_root}/backups}")"
mkdir -p -- "$output_dir"; chmod 700 "$output_dir"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"; bundle="sakura-cutover-${stamp}"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/sakura-backup.XXXXXX")"
trap '[[ -n "${tmp:-}" && "$tmp" == "${TMPDIR:-/tmp}"/sakura-backup.* ]] && rm -rf -- "$tmp"' EXIT
mkdir -p "$tmp/$bundle"

v2_env_args=(); [[ -n "${V2_ENV_FILE:-}" ]] && v2_env_args=(--env-file "$V2_ENV_FILE")
v2_compose=(docker compose --project-directory "$v2_dir" -f "$v2_compose_file" "${v2_env_args[@]}")
mysql_service="${V2_MYSQL_SERVICE:-mysql}"
log "creating consistent v2 MySQL dump"
"${v2_compose[@]}" exec -T "$mysql_service" sh -ceu 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --quick --routines --triggers --events --hex-blob --set-gtid-purged=OFF "$MYSQL_DATABASE"' >"$tmp/$bundle/v2-mysql.sql"
"${v2_compose[@]}" exec -T "$mysql_service" sh -ceu 'exec mysql -N -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' <"$ops_root/ops/mysql-table-counts.sql" >"$tmp/$bundle/v2-counts.tsv"

install -m 600 "$v2_config" "$tmp/$bundle/config.json"
install -m 600 "$v3_env" "$tmp/$bundle/v3.env"
if [[ -n "${V2_ENV_FILE:-}" ]]; then install -m 600 "$V2_ENV_FILE" "$tmp/$bundle/v2.env"; fi

if [[ -n "${V3_COMPOSE_DIR:-}" ]]; then
  v3_dir="$(readlink -m -- "$V3_COMPOSE_DIR")"
  log "creating v3 PostgreSQL safety dump"
  docker compose --project-directory "$v3_dir" --env-file "$v3_env" -f "$v3_dir/compose.yaml" exec -T postgres sh -ceu 'exec pg_dump -Fc -U "$POSTGRES_USER" "$POSTGRES_DB"' >"$tmp/$bundle/v3-postgres.dump"
fi

cat >"$tmp/$bundle/manifest.txt" <<EOF
created_at=${stamp}
hostname=$(hostname)
v2_compose_dir=${v2_dir}
git_commit=${SAKURA_RELEASE_COMMIT:-unknown}
image=${SAKURA_V3_IMAGE:-unknown}
EOF
(cd "$tmp/$bundle" && sha256sum ./* >SHA256SUMS)
tar -C "$tmp" -czf "$tmp/${bundle}.tar.gz" "$bundle"
archive="$output_dir/${bundle}.tar.gz.age"
age -r "$AGE_RECIPIENT" -o "$archive" "$tmp/${bundle}.tar.gz"
sha256sum "$archive" >"${archive}.sha256"
chmod 600 "$archive" "${archive}.sha256"
log "encrypted backup completed: $archive"
printf '%s\n' "$archive"
