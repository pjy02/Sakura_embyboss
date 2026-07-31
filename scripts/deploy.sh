#!/usr/bin/env bash
set -Eeuo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="${1:-${project_dir}/.env}"
backup_dir="${project_dir}/db_backup/releases"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
compose=(docker compose --env-file "${env_file}")
rollback_image=""

cd "${project_dir}"

if [[ ! -f "${env_file}" || ! -f config.json ]]; then
  echo "缺少 ${env_file} 或 config.json，停止上线。"
  exit 1
fi

if command -v python3 >/dev/null 2>&1; then
  python3 scripts/preflight.py --env-file "${env_file}" --config config.json
else
  echo "提示：宿主机没有 python3，已跳过配置预检。"
fi

"${compose[@]}" config --quiet
mkdir -p "${backup_dir}"

if "${compose[@]}" ps --status running --services | grep -qx mysql; then
  database_backup="${backup_dir}/mysql-${timestamp}.sql"
  config_backup="${backup_dir}/config-${timestamp}.json"
  temp_backup="${database_backup}.tmp"
  trap 'rm -f "${temp_backup:-}"' EXIT
  echo "正在备份数据库和配置..."
  "${compose[@]}" exec -T mysql sh -c \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysqldump -uroot --single-transaction --routines --events "$MYSQL_DATABASE"' \
    > "${temp_backup}"
  mv "${temp_backup}" "${database_backup}"
  cp config.json "${config_backup}"
  echo "备份完成：${database_backup}"
else
  echo "未发现正在运行的 MySQL，按首次部署继续。"
fi

current_image_id="$("${compose[@]}" images -q bot 2>/dev/null | head -n 1 || true)"
if [[ -n "${current_image_id}" ]]; then
  rollback_image="sakura-embyboss:rollback-${timestamp}"
  docker image tag "${current_image_id}" "${rollback_image}"
fi

echo "正在拉取镜像并启动服务..."
"${compose[@]}" pull
if "${compose[@]}" up -d --remove-orphans --wait --wait-timeout 180; then
  "${compose[@]}" ps
  echo "上线完成，Web、Bot 和数据库均已通过健康检查。"
  [[ -n "${rollback_image}" ]] && echo "本次临时回滚镜像：${rollback_image}"
  exit 0
fi

echo "新版本健康检查失败。"
if [[ -z "${rollback_image}" ]]; then
  echo "没有可用的旧镜像，无法自动回退。"
  exit 1
fi

echo "正在回退到上线前镜像 ${rollback_image}..."
SAKURA_IMAGE="${rollback_image}" docker compose --env-file "${env_file}" \
  up -d --remove-orphans --wait --wait-timeout 180
docker compose --env-file "${env_file}" ps
echo "已回退。请把 .env 中的 SAKURA_IMAGE 固定为确认可用的版本标签后再上线。"
exit 1
