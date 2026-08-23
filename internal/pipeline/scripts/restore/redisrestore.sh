set -eu
umask 0002
STAGE=redisrestore
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${WORKDIR}/.step-extract-done" "extract"
log "stage start: redisrestore name=${REDIS_NAME} host=${REDIS_HOST}:${REDIS_PORT}"
DUMP="${WORKDIR}/dump"
test -f "$DUMP"
log "dump file size=$(wc -c < "$DUMP") bytes"

drop_allowed() {
  case "${DROP_DATABASE_IF_EXISTS:-}" in
    yes|YES|true|TRUE|1) return 0 ;;
    *) return 1 ;;
  esac
}

remote_cli() {
  AUTH_ENV=""
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    AUTH_ENV="REDISCLI_AUTH=${REDIS_PASSWORD}"
  fi
  USER_ARGS=""
  if [ -n "${REDIS_USERNAME:-}" ] && [ "${REDIS_USERNAME}" != "default" ]; then
    USER_ARGS="--user ${REDIS_USERNAME}"
  fi
  # shellcheck disable=SC2086
  env ${AUTH_ENV} redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" ${USER_ARGS} "$@"
}

migrate_auth_args() {
  if [ -z "${REDIS_PASSWORD:-}" ]; then
    return 0
  fi
  if [ -n "${REDIS_USERNAME:-}" ] && [ "${REDIS_USERNAME}" != "default" ]; then
    printf 'AUTH2 %s %s' "${REDIS_USERNAME}" "${REDIS_PASSWORD}"
  else
    printf 'AUTH %s' "${REDIS_PASSWORD}"
  fi
}

LOCAL_DIR="${WORKDIR}/local-redis"
LOCAL_PORT=16379
mkdir -p "${LOCAL_DIR}"
cp "$DUMP" "${LOCAL_DIR}/dump.rdb"
chmod 660 "${LOCAL_DIR}/dump.rdb" 2>/dev/null || true

log "starting ephemeral redis-server from RDB on 127.0.0.1:${LOCAL_PORT}"
redis-server \
  --port "${LOCAL_PORT}" \
  --bind 127.0.0.1 \
  --protected-mode no \
  --dir "${LOCAL_DIR}" \
  --dbfilename dump.rdb \
  --save "" \
  --appendonly no \
  --daemonize yes \
  --logfile "${LOCAL_DIR}/redis.log" \
  --pidfile "${LOCAL_DIR}/redis.pid"

i=0
until redis-cli -h 127.0.0.1 -p "${LOCAL_PORT}" PING >/dev/null 2>&1; do
  i=$(( i + 1 ))
  if [ "$i" -gt 60 ]; then
    log "ERROR: local redis-server did not become ready" >&2
    cat "${LOCAL_DIR}/redis.log" >&2 || true
    mark_failed
    exit 1
  fi
  sleep 1
done
log "local redis-server is ready"

if drop_allowed; then
  log "DROP_DATABASE_IF_EXISTS=${DROP_DATABASE_IF_EXISTS}; FLUSHALL on target"
  remote_cli FLUSHALL >/dev/null
fi

MIGRATE_AUTH="$(migrate_auth_args || true)"
copied=0
db=0
while [ "$db" -le 15 ]; do
  while IFS= read -r key; do
    [ -n "$key" ] || continue
    # shellcheck disable=SC2086
    redis-cli -h 127.0.0.1 -p "${LOCAL_PORT}" -n "$db" \
      MIGRATE "${REDIS_HOST}" "${REDIS_PORT}" "$key" "$db" 120000 COPY REPLACE ${MIGRATE_AUTH} >/dev/null
    copied=$(( copied + 1 ))
  done <<EOF
$(redis-cli -h 127.0.0.1 -p "${LOCAL_PORT}" -n "$db" --scan)
EOF
  db=$(( db + 1 ))
done
log "migrated ${copied} keys to ${REDIS_HOST}:${REDIS_PORT}"

if [ -f "${LOCAL_DIR}/redis.pid" ]; then
  kill "$(cat "${LOCAL_DIR}/redis.pid")" 2>/dev/null || true
fi

rm -f "$DUMP" "${WORKDIR}/dump"*
touch "${WORKDIR}/.step-job-done"
log "wrote marker .step-job-done; releasing sibling containers"
log "waiting for sibling containers to observe job-done before removing scratch"
sleep 20
log "removing restore scratch dir ${WORKDIR}"
rm -rf "${WORKDIR}"
log "stage done"
