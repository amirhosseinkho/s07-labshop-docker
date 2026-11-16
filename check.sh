#!/usr/bin/env bash
# Self-check for the LabShop Docker exercise.
# Run from the repository root (Git Bash on Windows):  ./check.sh
# It starts from an EMPTY database (docker compose down -v) and takes about a minute.
set -u
cd "$(dirname "$0")"

RESULTS=()
PASS=0
FAIL=0
pass() { RESULTS+=("PASS  $1"); PASS=$((PASS + 1)); }
fail() { RESULTS+=("FAIL  $1"$'\n'"        $2"); FAIL=$((FAIL + 1)); }

[ -f .env ] || cp .env.example .env
SECRET=$(grep -E '^DB_PASSWORD=' .env | cut -d= -f2- | tr -d '\r')
IMAGE=labshop-api:dev

block() { # print one service block of the resolved compose file
  docker compose config 2>/dev/null | awk -v s="  $1:" '$0==s{f=1;next} /^  [a-z]/{f=0} f'
}

echo "[1/5] clean start from an empty database (build may take a minute) ..."
docker compose down -v --remove-orphans >/dev/null 2>&1
docker compose up -d --build --wait --wait-timeout 120 >/dev/null 2>&1

# DOCKER-1 and DOCKER-2: the API comes up on a fresh database.
if curl -fsS -m 5 http://localhost:8081/ready >/dev/null 2>&1; then
  pass "DOCKER-1/2  API is up and connected to the database on a fresh start"
else
  fail "DOCKER-1/2  API is not up on a fresh start" "see: docker compose ps -a   and   docker compose logs api"
fi
if block api | grep -q 'condition: service_healthy' && block db | grep -q 'healthcheck:'; then
  pass "DOCKER-2    api waits for a healthy db"
else
  fail "DOCKER-2    api does not wait for a healthy db" "depends_on alone only orders the start"
fi

echo "[2/5] web page and /api proxy ..."
if curl -fsS -m 5 http://localhost:8088/api/menu 2>/dev/null | grep -q Espresso; then
  pass "DOCKER-3    http://localhost:8088/api/menu returns the menu"
else
  fail "DOCKER-3    the web container cannot reach the API" "see: docker compose logs web"
fi

echo "[3/5] data survives docker compose down / up ..."
ID=$(curl -fsS -m 5 -X POST -H 'Content-Type: application/json' -d '{"item_id":2,"qty":3}' \
  http://localhost:8081/api/orders 2>/dev/null | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
docker compose down >/dev/null 2>&1
docker compose up -d --wait --wait-timeout 120 >/dev/null 2>&1
if [ -n "$ID" ] && curl -fsS -m 5 http://localhost:8081/api/orders 2>/dev/null | grep -q "\"id\":$ID,"; then
  pass "DOCKER-4    order #$ID is still there after down/up"
else
  fail "DOCKER-4    the order did not survive down/up" "where does PostgreSQL keep its data, and where is the volume mounted?"
fi

echo "[4/5] image size, user and secrets ..."
SIZE=$(docker image inspect $IMAGE --format '{{.Size}}' 2>/dev/null || echo 0)
MB=$((SIZE / 1000000))
if [ "$SIZE" -gt 0 ] && [ "$MB" -lt 50 ]; then
  pass "DOCKER-5    image is ${MB}MB (< 50MB)"
else
  fail "DOCKER-5    image is ${MB}MB (target < 50MB)" "the final image needs the binary, not the Go toolchain"
fi
USER_=$(docker image inspect $IMAGE --format '{{.Config.User}}' 2>/dev/null)
case "$USER_" in
  "" | root | 0 | 0:0) fail "DOCKER-5    image runs as root" "docker image inspect $IMAGE --format '{{.Config.User}}'" ;;
  *) pass "DOCKER-5    image runs as user '$USER_'" ;;
esac
LEAK=""
docker image inspect $IMAGE --format '{{json .Config.Env}}' 2>/dev/null | grep -q PASSWORD && LEAK="$LEAK ENV"
docker history --no-trunc $IMAGE 2>/dev/null | grep -qE "PASSWORD|${SECRET:-no-secret-set}" && LEAK="$LEAK history"
CID=$(docker create $IMAGE 2>/dev/null)
if [ -n "$CID" ]; then
  docker export "$CID" 2>/dev/null | tar -t 2>/dev/null | grep -qE '(^|/)\.env$' && LEAK="$LEAK .env-file"
  docker export "$CID" 2>/dev/null | tar -t 2>/dev/null | grep -qE '(^|/)\.git/' && LEAK="$LEAK .git-folder"
  docker rm "$CID" >/dev/null 2>&1
fi
if [ -z "$LEAK" ]; then
  pass "DOCKER-5    no password, .env or .git inside the image"
else
  fail "DOCKER-5    found in the image:$LEAK" "docker history --no-trunc $IMAGE ; what does COPY . . copy?"
fi

echo "[5/5] stopping the API (DOCKER-6 is optional) ..."
START=$SECONDS
docker compose stop api >/dev/null 2>&1
TOOK=$((SECONDS - START))
if [ "$TOOK" -le 4 ] && docker compose logs api 2>/dev/null | grep -q "shutting down gracefully"; then
  pass "DOCKER-6    API stopped gracefully in ${TOOK}s"
else
  fail "DOCKER-6    stop took ${TOOK}s, graceful shutdown log missing" "who is PID 1 in the api container?"
fi
docker compose up -d --wait --wait-timeout 60 >/dev/null 2>&1

echo
printf '%s\n' "${RESULTS[@]}"
echo
echo "$PASS passed, $FAIL failed.  Shop page: http://localhost:8088"
