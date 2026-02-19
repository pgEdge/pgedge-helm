#!/bin/bash
set -euo pipefail

# Template-level assertions for the pgEdge Helm chart.
# Runs without a cluster — just verifies helm template output.

CHART_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VALUES="$CHART_DIR/examples/configs/single/values.yaml"
STDERR_LOG=$(mktemp)
trap 'rm -f "$STDERR_LOG"' EXIT
PASS=0
FAIL=0

assert_contains() {
  local label="$1"
  local haystack="$2"
  local needle="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    echo "  PASS: $label"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $label"
    echo "    expected to find: $needle"
    FAIL=$((FAIL + 1))
  fi
}

assert_not_contains() {
  local label="$1"
  local haystack="$2"
  local needle="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    echo "  FAIL: $label"
    echo "    expected NOT to find: $needle"
    FAIL=$((FAIL + 1))
  else
    echo "  PASS: $label"
    PASS=$((PASS + 1))
  fi
}

# --- Test: default database name ---
echo "=== Default database name (app) ==="
DEFAULT=$(helm template pgedge "$CHART_DIR" -f "$VALUES" 2>"$STDERR_LOG") || {
  echo "FAIL: helm template (default) failed"; cat "$STDERR_LOG"; exit 1; }

assert_contains "init-spock DB_NAME is app" \
  "$(echo "$DEFAULT" | grep -A1 'name: DB_NAME')" \
  '"app"'

assert_contains "pg_hba references app database" \
  "$DEFAULT" \
  "hostssl app pgedge"

assert_contains "pg_hba references app owner" \
  "$DEFAULT" \
  "hostssl app app"

assert_contains "pg_hba includes streaming_replica rule" \
  "$DEFAULT" \
  "hostssl all streaming_replica all cert map=cnpg_streaming_replica"

assert_contains "pg_ident references app owner" \
  "$DEFAULT" \
  "local postgres app"

# --- Test: custom database name ---
echo ""
echo "=== Custom database name (mydb) ==="
CUSTOM=$(helm template pgedge "$CHART_DIR" -f "$VALUES" \
  --set pgEdge.clusterSpec.bootstrap.initdb.database=mydb 2>"$STDERR_LOG") || {
  echo "FAIL: helm template (custom db) failed"; cat "$STDERR_LOG"; exit 1; }

assert_contains "init-spock DB_NAME is mydb" \
  "$(echo "$CUSTOM" | grep -A1 'name: DB_NAME')" \
  '"mydb"'

assert_contains "pg_hba references mydb database" \
  "$CUSTOM" \
  "hostssl mydb pgedge"

assert_not_contains "pg_hba does not reference old app database" \
  "$(echo "$CUSTOM" | grep -F 'hostssl' | grep -F 'pgedge' | grep -F 'cert' | grep -vF 'streaming_replica' || true)" \
  "hostssl app"

assert_contains "pg_hba still includes streaming_replica rule" \
  "$CUSTOM" \
  "hostssl all streaming_replica all cert map=cnpg_streaming_replica"

assert_contains "pg_ident still references explicit owner (app)" \
  "$CUSTOM" \
  "local postgres app"

# --- Test: custom database + custom owner ---
echo ""
echo "=== Custom database (mydb) + custom owner (myuser) ==="
CUSTOM_OWNER=$(helm template pgedge "$CHART_DIR" -f "$VALUES" \
  --set pgEdge.clusterSpec.bootstrap.initdb.database=mydb \
  --set pgEdge.clusterSpec.bootstrap.initdb.owner=myuser 2>"$STDERR_LOG") || {
  echo "FAIL: helm template (custom owner) failed"; cat "$STDERR_LOG"; exit 1; }

assert_contains "init-spock DB_NAME is mydb not myuser" \
  "$(echo "$CUSTOM_OWNER" | grep -A1 'name: DB_NAME')" \
  '"mydb"'

assert_contains "pg_hba references myuser owner" \
  "$CUSTOM_OWNER" \
  "hostssl mydb myuser"

assert_not_contains "pg_hba does not reference old app owner" \
  "$(echo "$CUSTOM_OWNER" | grep -F 'hostssl mydb' | grep -F 'cert' | grep -vF pgedge | grep -vF admin || true)" \
  "hostssl mydb app"

assert_contains "pg_ident references myuser owner" \
  "$CUSTOM_OWNER" \
  "local postgres myuser"

assert_not_contains "pg_ident does not reference old app owner" \
  "$(echo "$CUSTOM_OWNER" | grep -F 'local postgres' | grep -vF admin || true)" \
  "local postgres app"

# --- Summary ---
echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] || exit 1
