#!/usr/bin/env bash
# Isolated Tana stack: throwaway Postgres container, fake Panda, collector and
# local service built from the working tree. Never reads .env, so it cannot
# reach the production collector or real Panda.
#
# Usage: stack.sh start | stop | status | build
#        stack.sh restart-tana [VAR=value ...]   restart the local service with env overrides
#        stack.sh stop-collector | start-collector
set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
SKILL="$REPO/.claude/skills/run-tana"
RUN=${TANA_RUN_DIR:-/tmp/tana-run}
PG_NAME=tana-run-pg
PG_PORT=5499 COLLECTOR_PORT=8091 TANA_PORT=8090 PANDA_PORT=8092
TOKEN=runtoken

mkdir -p "$RUN/logs" "$RUN/pids" "$RUN/bin" "$RUN/control"

build() {
  (cd "$REPO/web" && npm run build >"$RUN/logs/web-build.log" 2>&1) || { tail -20 "$RUN/logs/web-build.log"; exit 1; }
  (cd "$REPO" && go build -o "$RUN/bin/" ./cmd/tana ./cmd/collector)
}

wait_http() { timeout "${2:-30}" bash -c "until curl -sf $1 >/dev/null; do sleep 0.5; done" || { echo "timeout: $1" >&2; return 1; }; }

launch() { # name, then command
  local name=$1; shift
  setsid "$@" >"$RUN/logs/$name.log" 2>&1 </dev/null &
  echo $! >"$RUN/pids/$name"
}

kill_one() {
  local f="$RUN/pids/$1"
  [ -f "$f" ] || return 0
  kill "$(cat "$f")" 2>/dev/null || true
  rm -f "$f"
}

start_collector() {
  launch collector env -i PATH="$PATH" HOME="$HOME" \
    TANA_COLLECTOR_HOST=127.0.0.1 TANA_COLLECTOR_PORT=$COLLECTOR_PORT TANA_COLLECTOR_API_TOKEN=$TOKEN \
    TANA_COLLECTOR_DATABASE_URL="postgres://postgres:run@127.0.0.1:$PG_PORT/collector?sslmode=disable" \
    TANA_COLLECTOR_DATA_DIR="$RUN/data/collector" \
    TANA_COLLECTOR_SITEMAP_URL=http://127.0.0.1:$PANDA_PORT/sitemap.xml \
    PANDA_FEED_URL=http://127.0.0.1:$PANDA_PORT/feed PANDA_API_URL=http://127.0.0.1:$PANDA_PORT/api.php \
    PANDA_FAVORITES_URL=http://127.0.0.1:$PANDA_PORT/favorites.php PANDA_ARCHIVER_URL=http://127.0.0.1:$PANDA_PORT/archiver.php \
    PANDA_FAVORITES_COOKIES='{"ipb_member_id":"1","ipb_pass_hash":"x","sp":"1"}' \
    "$RUN/bin/collector"
  wait_http http://127.0.0.1:$COLLECTOR_PORT/healthz
}

start_tana() { # extra VAR=value overrides; an empty value removes the variable
  local vars=(PATH="$PATH" HOME="$HOME" TANA_HOST=127.0.0.1 TANA_PORT=$TANA_PORT
    TANA_DATABASE_URL="postgres://postgres:run@127.0.0.1:$PG_PORT/tana?sslmode=disable"
    TANA_DATA_DIR="$RUN/data/tana" TANA_WEB_DIR="$REPO/web/dist"
    TANA_COLLECTOR_URL=http://127.0.0.1:$COLLECTOR_PORT TANA_COLLECTOR_API_TOKEN=$TOKEN "$@")
  launch tana env -i "${vars[@]}" "$RUN/bin/tana"
  wait_http http://127.0.0.1:$TANA_PORT/healthz
}

case "${1:-}" in
  build) build ;;
  start)
    [ "${SKIP_BUILD:-}" = 1 ] || build
    docker run -d --rm --name $PG_NAME -e POSTGRES_PASSWORD=run -p 127.0.0.1:$PG_PORT:5432 postgres:18-alpine >/dev/null
    timeout 60 bash -c "until docker exec $PG_NAME pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1; do sleep 1; done"
    docker exec $PG_NAME psql -qU postgres -c 'create database tana' -c 'create database collector'
    rm -rf "$RUN/data" "$RUN/control"/*
    launch panda python3 -I "$SKILL/fake_panda.py" $PANDA_PORT "$RUN/control"
    wait_http "http://127.0.0.1:$PANDA_PORT/favorites.php?favcat=0"
    start_collector
    start_tana
    echo "Tana: http://127.0.0.1:$TANA_PORT  collector: http://127.0.0.1:$COLLECTOR_PORT  fake Panda control: $RUN/control  logs: $RUN/logs"
    ;;
  stop)
    for n in tana collector panda; do kill_one $n; done
    docker stop $PG_NAME >/dev/null 2>&1 || true
    ;;
  status)
    for n in tana collector panda; do
      if [ -f "$RUN/pids/$n" ] && kill -0 "$(cat "$RUN/pids/$n")" 2>/dev/null; then echo "$n: running"; else echo "$n: stopped"; fi
    done
    docker ps --filter name=$PG_NAME --format 'postgres: {{.Status}}'
    ;;
  restart-tana) shift; kill_one tana; sleep 0.5; start_tana "$@" ;;
  stop-collector) kill_one collector ;;
  start-collector) start_collector ;;
  *) sed -n '2,9p' "$0"; exit 2 ;;
esac
