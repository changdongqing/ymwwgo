#!/usr/bin/env bash
# 数据库结构漂移守卫：goose 迁移链 vs Ent schema 全量建表，两库结构必须一致。
# 原理与语义见 docs/database/design/ent-goose-migrations.md §7。
#
# 用法: drift_check.sh <host> <port> [pg_container]
#   本地容器栈:   backend$ scripts/database/drift_check.sh 127.0.0.1 25432 ymwwgo-postgres
#   CI(见 .github/workflows/db-schema-drift.yml): scripts/database/drift_check.sh 127.0.0.1 5432 drift-pg
#
# 给了 pg_container 时 psql/pg_dump 走 docker exec（宿主机无需安装 PG 客户端），否则用 PATH 里的。
# 凭据与官方 compose 一致（postgres / *Abcd123456），不要用于暴露在公网的实例。
set -euo pipefail

PG_HOST=${1:?用法: drift_check.sh <host> <port> [pg_container]}
PG_PORT=${2:?用法: drift_check.sh <host> <port> [pg_container]}
PG_CONTAINER=${3:-}
PG_PASSWORD=${PG_PASSWORD:-*Abcd123456}
DB_GOOSE=gwa_drift_goose
DB_ENT=gwa_drift_ent
BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

psql_exec() {
  if [ -n "$PG_CONTAINER" ]; then
    docker exec -i "$PG_CONTAINER" psql -U postgres -v ON_ERROR_STOP=1 -q "$@"
  else
    psql -h "$PG_HOST" -p "$PG_PORT" -U postgres -v ON_ERROR_STOP=1 -q "$@"
  fi
}

pgdump_exec() {
  if [ -n "$PG_CONTAINER" ]; then
    docker exec "$PG_CONTAINER" pg_dump -U postgres "$@"
  else
    pg_dump -h "$PG_HOST" -p "$PG_PORT" -U postgres "$@"
  fi
}

DSN_COMMON="host=$PG_HOST port=$PG_PORT user=postgres password=$PG_PASSWORD sslmode=disable"
TMP_DIR="$(mktemp -d)"

cleanup() {
  psql_exec -c "DROP DATABASE IF EXISTS $DB_GOOSE" >/dev/null 2>&1 || true
  psql_exec -c "DROP DATABASE IF EXISTS $DB_ENT" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "== 数据库结构漂移守卫：goose 迁移链 vs Ent schema =="

psql_exec \
  -c "DROP DATABASE IF EXISTS $DB_GOOSE" \
  -c "DROP DATABASE IF EXISTS $DB_ENT" \
  -c "CREATE DATABASE $DB_GOOSE" \
  -c "CREATE DATABASE $DB_ENT"

echo "== 库A($DB_GOOSE)：应用 goose 迁移链 =="
(cd "$BACKEND_DIR" && go run ./app/admin/service/cmd/migrate up -dsn "$DSN_COMMON dbname=$DB_GOOSE")

echo "== 库B($DB_ENT)：Ent schema 全量建表 =="
(cd "$BACKEND_DIR" && go run ./app/admin/service/cmd/migrate automigrate-ent -dsn "$DSN_COMMON dbname=$DB_ENT")

echo "== 双库 dump + diff（排除 goose 版本表）=="
# 过滤 pg_dump 新版头部/尾部的 \restrict \unrestrict psql 元命令（内含随机 token，每次 dump 都不同）；
# 归一整表内的列顺序：ADD COLUMN 只能追加到表尾，而 Ent 会把边字段（如 parent_id）排在常规字段之后，
# 两者列序天然不同。列序不是逻辑结构契约，归一后再 diff，仍能发现缺列/多列/类型差异。
filter_dump() {
  sed '/^\\restrict /d;/^\\unrestrict /d' | python3 "$BACKEND_DIR/scripts/database/normalize_dump.py"
}
pgdump_exec --schema-only --no-owner --no-privileges -T 'goose_db_version*' "$DB_GOOSE" | filter_dump >"$TMP_DIR/goose.sql"
pgdump_exec --schema-only --no-owner --no-privileges "$DB_ENT" | filter_dump >"$TMP_DIR/ent.sql"

# 真实性护栏：两侧 dump 都必须真的建出了表，防止"迁移空跑/连错库"类的假阳性通过。
if ! grep -q '^CREATE TABLE' "$TMP_DIR/goose.sql" || ! grep -q '^CREATE TABLE' "$TMP_DIR/ent.sql"; then
  echo "== 漂移守卫失败：至少一侧 dump 没有 CREATE TABLE（迁移可能空跑或连错了库），拒绝判通过 ==" >&2
  exit 1
fi

diff -u "$TMP_DIR/ent.sql" "$TMP_DIR/goose.sql" >"$TMP_DIR/diff.txt" || true
cat "$TMP_DIR/diff.txt"
if [ -s "$TMP_DIR/diff.txt" ]; then
  echo "== 漂移守卫失败：goose 迁移链与 Ent schema 不一致（左 ent 期望 / 右 goose 实际），diff 见上 ==" >&2
  echo "== 处理办法见 docs/database/design/ent-goose-migrations.md FAQ ==" >&2
  exit 1
fi
echo "== 漂移守卫通过：迁移链与 Ent schema 一致 =="
