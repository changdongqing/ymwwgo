#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════
# 一站式服务类型字典 · 测试数据导入脚本（【非】数据库迁移脚本！）
#
# 作用：把 docs/leixing/biaodan/README.md 的四级分类字典（大/中/小/细，共 299 条）
#       灌入 onestop_service_types 表，仅供本地/测试环境造数使用。
#
# 铁律声明（全仓铁律 6）：本脚本【不】是 goose 版本化迁移——迁移链只认
#   backend/app/admin/service/internal/data/migrations/000NN_*.sql，
#   本脚本及其等价 SQL 永远不得挪入迁移目录；它只插数据、不动表结构。
#
# 数据来源：docs/leixing/biaodan/README.md 的平铺清单表（每行一个细类，
#   四级编码/名称同行）。脚本运行时直接解析该文档 → 文档即唯一数据源，
#   文档更新后重跑本脚本即可同步（含改名），无需改脚本。
#
# 用法：
#   ./seed_onestop_service_types.sh up      # 导入/刷新（按 code 幂等，可重复执行）
#   ./seed_onestop_service_types.sh clean   # 仅删除字典内的 code（不动其他数据）
#   ./seed_onestop_service_types.sh status  # 查看库内各层级条数 vs 文档期望
#
# 连接（缺省走本机 docker 容器栈；装有 psql 时优先直连）：
#   环境变量 PG_CONTAINER(默认 ymwwgo-postgres) / PG_USER(postgres) / PG_DB(gwa)
#   或 GWA_DATABASE_DSN（host=... port=... user=... password=... dbname=... sslmode=disable）
# ═══════════════════════════════════════════════════════════════════════════
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOC="${SCRIPT_DIR}/../../leixing/biaodan/README.md"
MODE="${1:-up}"
PG_CONTAINER="${PG_CONTAINER:-ymwwgo-postgres}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-gwa}"

[[ -f "$DOC" ]] || { echo "错误: 找不到字典文档 $DOC" >&2; exit 1; }

run_sql() { # SQL 从 stdin 读入
  if command -v psql >/dev/null 2>&1; then
    psql "${GWA_DATABASE_DSN:-host=127.0.0.1 port=25432 user=postgres password=*Abcd123456 dbname=gwa sslmode=disable}" \
      -v ON_ERROR_STOP=1 -q
  else
    docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 -q
  fi
}

# 从字典文档解析去重后的 (code, name) 集合，按 code 升序（父级编码必为前缀，天然排在子级前）
# 输出: TSV 两列。文档表行形如 | 01 | 表单 | 0101 | 报修维修 | ... | 01010101 | xxx |
parse_doc() {
  awk -F'|' '
    /^\|[[:space:]]*[0-9][0-9][[:space:]]*\|/ {
      for (i = 2; i <= 8; i += 2) {
        code = $i;  gsub(/^[[:space:]]+|[[:space:]]+$/, "", code)
        name = $(i + 1); gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
        # 注意 mawk 不支持 {m,n} 区间正则，这里用可移植写法
        if (code ~ /^[0-9][0-9]*$/ && length(code) <= 8 && name != "") print code "\t" name
      }
    }' "$DOC" | sort -u
}

# TSV → SQL VALUES 元组（转义单引号；层级由编码位数推导）
gen_tuples() {
  awk -F'\t' '
    {
      name = $2; gsub(/\047/, "\047\047", name)
      lvl = "LEVEL_DETAIL"
      if (length($1) == 2) lvl = "LEVEL_MAJOR"
      else if (length($1) == 4) lvl = "LEVEL_MIDDLE"
      else if (length($1) == 6) lvl = "LEVEL_MINOR"
      printf "(\047%s\047, \047%s\047, \047%s\047, %d),\n", $1, name, lvl, ("0" substr($1, length($1) - 1)) + 0
    }'
}

EXPECTED=$(parse_doc | wc -l | tr -d ' ')

case "$MODE" in
  up)
    echo ">> 数据源: $DOC （$EXPECTED 条）"
    {
      echo "BEGIN;"
      echo "INSERT INTO onestop_service_types (code, name, level, description, is_enabled, sort_order, created_at, created_by)"
      echo "SELECT v.code, v.name, v.level, '表单分类字典·测试导入（docs/leixing/biaodan）', true, v.sort, now(), 1"
      echo "FROM (VALUES"
      parse_doc | gen_tuples | sed '$s/,$//'
      echo ") AS v(code, name, level, sort)"
      echo "ON CONFLICT (code) DO UPDATE SET"
      echo "  name = EXCLUDED.name, level = EXCLUDED.level, sort_order = EXCLUDED.sort_order,"
      echo "  is_enabled = true, updated_at = now(), updated_by = 1;"
      # 第二遍回填上级：同一语句内互相看不见，落库后按编码前缀关联
      echo "UPDATE onestop_service_types c SET parent_id = p.id"
      echo "FROM onestop_service_types p"
      echo "WHERE length(c.code) > 2 AND p.code = left(c.code, length(c.code) - 2);"
      echo "COMMIT;"
    } | run_sql
    echo ">> 导入完成，库内实际分布："
    {
      echo "SELECT CASE level WHEN 'LEVEL_MAJOR' THEN '大类' WHEN 'LEVEL_MIDDLE' THEN '中类'"
      echo "       WHEN 'LEVEL_MINOR' THEN '小类' WHEN 'LEVEL_DETAIL' THEN '细类' ELSE level END AS 层级,"
      echo "       count(*) AS 条数 FROM onestop_service_types GROUP BY level ORDER BY length(level::text);"
    } | run_sql
    echo ">> 文档期望: 大类 1 / 中类 12 / 小类 75 / 细类 211（合计 $EXPECTED）"
    ;;
  clean)
    echo ">> 仅删除字典文档内出现的 code（$EXPECTED 条，其他数据不动）"
    {
      echo "BEGIN;"
      # 先解掉父子引用，避免子级残留 parent_id 指向将删除的行
      echo "UPDATE onestop_service_types SET parent_id = NULL WHERE code IN ("
      parse_doc | cut -f1 | awk '{ printf "\047%s\047,\n", $0 }' | sed '$s/,$//'
      echo ");"
      echo "DELETE FROM onestop_service_types WHERE code IN ("
      parse_doc | cut -f1 | awk '{ printf "\047%s\047,\n", $0 }' | sed '$s/,$//'
      echo ");"
      echo "COMMIT;"
    } | run_sql
    echo ">> 清理完成"
    ;;
  status)
    {
      echo "SELECT CASE level WHEN 'LEVEL_MAJOR' THEN '大类' WHEN 'LEVEL_MIDDLE' THEN '中类'"
      echo "       WHEN 'LEVEL_MINOR' THEN '小类' WHEN 'LEVEL_DETAIL' THEN '细类' ELSE level END AS 层级,"
      echo "       count(*) AS 条数 FROM onestop_service_types GROUP BY level ORDER BY length(level::text);"
    } | run_sql
    echo ">> 文档期望: 大类 1 / 中类 12 / 小类 75 / 细类 211（合计 $EXPECTED）"
    ;;
  *)
    echo "用法: $0 [up|clean|status]" >&2
    exit 1
    ;;
esac
