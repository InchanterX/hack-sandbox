#!/bin/sh
# Полная проверка на чистой базе: raw -> модель (дважды) -> самопроверки -> ответы -> контрольные цифры.
#   sh checks/run.sh                 итоговое решение: model/build.sql, model/checks.sql, mart/*.sql
#   sh checks/run.sh <папка>         решение участника: <папка>/build.sql и <папка>/answers_a.sql
# Запускается внутри сервиса dev из compose.checks.yaml (там заданы PGHOST и остальное).
set -eu

DIR=${1:-}
if [ -n "$DIR" ]; then
  BUILD="$DIR/build.sql"; SELF=""; ANSWERS="$DIR/answers_a.sql"
else
  BUILD="model/build.sql"; SELF="model/checks.sql"; ANSWERS=$(ls mart/*.sql 2>/dev/null | sort || true)
fi
PSQL="psql -X -q -v ON_ERROR_STOP=1"

step() { echo; echo "=== $*"; }
fail() { echo; echo "FAIL: $*"; exit 1; }
elapsed() { echo "    $(( $(date +%s) - $1 )) с"; }

# Отпечаток raw: колонки с типами и число строк каждой таблицы.
raw_state() {
  $PSQL -t -A -c "SELECT string_agg(table_name || '.' || column_name || ':' || data_type, ',' ORDER BY 1)
    FROM information_schema.columns WHERE table_schema = 'raw'"
  for t in $($PSQL -t -A -c "SELECT tablename FROM pg_tables WHERE schemaname = 'raw' ORDER BY 1"); do
    $PSQL -t -A -c "SELECT '$t', count(*) FROM raw.\"$t\""
  done
}

[ -f "$BUILD" ] || fail "нет $BUILD"

step "данные: скачать (если нет) и загрузить в raw"
bash scripts/download_data.sh >/dev/null
python scripts/load_raw.py --host "$PGHOST" >/dev/null || fail "загрузка raw"

RAW_BEFORE=$(raw_state)

step "сборка модели: $BUILD"
t=$(date +%s); $PSQL -f "$BUILD" || fail "сборка модели"; elapsed "$t"

step "повторная сборка (скрипт должен быть идемпотентным)"
t=$(date +%s); $PSQL -f "$BUILD" || fail "повторная сборка"; elapsed "$t"

step "raw не изменён"
[ "$(raw_state)" = "$RAW_BEFORE" ] || fail "сборка изменила схему raw (колонки, типы или число строк)"

step "строки по таблицам raw и core"
$PSQL -c "SELECT table_schema || '.' || table_name AS \"table\",
  (xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM %I.%I',
    table_schema, table_name), false, true, '')))[1]::text::bigint AS rows
  FROM information_schema.tables
  WHERE table_schema IN ('raw', 'core') AND table_type = 'BASE TABLE' ORDER BY 1"

if [ -n "$SELF" ] && [ -f "$SELF" ]; then
  step "самопроверки модели: $SELF"
  $PSQL -f "$SELF" || fail "самопроверки модели"
fi

for f in $ANSWERS; do
  step "ответы: $f"
  $PSQL -f "$f" || fail "ответы $f"
done

step "ответы в mart (первые 15 строк каждой витрины)"
for v in $($PSQL -t -A -c "SELECT table_name FROM information_schema.tables
    WHERE table_schema = 'mart' ORDER BY 1"); do
  echo "--- mart.$v"
  $PSQL -c "SELECT * FROM mart.\"$v\" LIMIT 15"
done

step "контрольные цифры блока А"
$PSQL -f checks/control.sql || fail "контрольные цифры"

echo; echo "OK"
