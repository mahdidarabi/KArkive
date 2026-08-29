set -eu
umask 0002
STAGE=redisrestore
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${WORKDIR}/.step-extract-done" "extract"
already_done_exit "${WORKDIR}/.step-job-done" "redisrestore"

NAME="${REDIS_NAME:-}"
HOST="${REDIS_HOST:-}"
PORT="${REDIS_PORT:-6379}"
: "${NAME:?REDIS_NAME required}"
: "${HOST:?REDIS_HOST required}"
export REDISCLI_AUTH="${REDISCLI_AUTH:-${REDIS_PASSWORD:-}}"
USER_ARGS=""
if [ -n "${REDIS_USERNAME:-}" ] && [ "${REDIS_USERNAME}" != "default" ]; then
  USER_ARGS="--user ${REDIS_USERNAME}"
fi

log "stage start: restore instance=${NAME} host=${HOST}:${PORT}"
# Scratch artifact is dump (engine-neutral) for all engines (fetch/decrypt/extract stay shared).
DUMP="${WORKDIR}/dump"
test -f "$DUMP"
log "dump file size=$(wc -c < "$DUMP") bytes"

drop_allowed() {
  case "${DROP_DATABASE_IF_EXISTS:-}" in
    yes|YES|true|TRUE|1) return 0 ;;
    *) return 1 ;;
  esac
}

redis_target() {
  # shellcheck disable=SC2086
  redis-cli -h "${HOST}" -p "${PORT}" ${USER_ARGS} "$@"
}

# Ephemeral redis has no requirepass; REDISCLI_AUTH (target password) must not
# leak into these calls or redis-cli will AUTH-fail and confuse readiness checks.
redis_ephemeral() {
  env -u REDISCLI_AUTH redis-cli -h 127.0.0.1 -p 6380 "$@"
}

log "ping target"
redis_target PING >/dev/null

TARGET_KEYS="$(redis_target DBSIZE | tr -d '\r')"
log "target DBSIZE=${TARGET_KEYS}"
if [ "${TARGET_KEYS}" != "0" ]; then
  if drop_allowed; then
    log "DROP_DATABASE_IF_EXISTS=${DROP_DATABASE_IF_EXISTS}; REPLICAOF will replace target dataset"
  else
    log "ERROR: target redis is not empty (DBSIZE=${TARGET_KEYS})" >&2
    log "ERROR: set DROP_DATABASE_IF_EXISTS=yes to allow full replace via REPLICAOF" >&2
    mark_failed
    exit 1
  fi
fi

RESTORE_DIR="${WORKDIR}/redis-data"
mkdir -p "${RESTORE_DIR}"
cp -f "$DUMP" "${RESTORE_DIR}/dump.rdb"
chmod 644 "${RESTORE_DIR}/dump.rdb"
log "starting ephemeral redis-server on :6380 with dump.rdb"
redis-server \
  --daemonize yes \
  --port 6380 \
  --bind 0.0.0.0 \
  --protected-mode no \
  --dir "${RESTORE_DIR}" \
  --dbfilename dump.rdb \
  --appendonly no \
  --save "" \
  --logfile "${WORKDIR}/redis-server.log"

i=0
while ! redis_ephemeral PING >/dev/null 2>&1; do
  i=$(( i + 1 ))
  if [ "$i" -gt 60 ]; then
    log "ERROR: ephemeral redis-server failed to become ready" >&2
    cat "${WORKDIR}/redis-server.log" >&2 || true
    mark_failed
    exit 1
  fi
  sleep 1
done

# PING can succeed while Redis is still loading dump.rdb into memory.
# REPLICAOF before loading:0 syncs a partial dataset (key counts look "wrong").
log "waiting for ephemeral redis to finish loading RDB (loading:0)"
i=0
while true; do
  loading="$(redis_ephemeral INFO persistence 2>/dev/null | tr -d '\r' | awk -F: '/^loading:/{print $2}')"
  if [ "${loading}" = "0" ]; then
    break
  fi
  i=$(( i + 1 ))
  if [ $(( i % 15 )) -eq 0 ]; then
    log "still loading RDB (~$(( i * 2 ))s) loading=${loading:-?}"
  fi
  if [ "$i" -gt 1800 ]; then
    log "ERROR: timeout waiting for ephemeral redis to finish loading RDB" >&2
    cat "${WORKDIR}/redis-server.log" >&2 || true
    mark_failed
    exit 1
  fi
  sleep 2
done

SRC_KEYS="$(redis_ephemeral DBSIZE | tr -d '\r')"
log "ephemeral source ready (RDB loaded); DBSIZE=${SRC_KEYS}"
redis_ephemeral INFO keyspace || true
log "WARNING: key counts after RDB load may be lower than backup-time live INFO (keys with TTL expire between dump and restore; live stats after dump are not the dump contents)"

# Prefer a cluster-routable address so the sandbox pod can pull RDB from us.
POD_IP="$(hostname -i 2>/dev/null | awk '{print $1}')"
if [ -z "${POD_IP}" ]; then
  POD_IP="$(redis_ephemeral CONFIG GET replica-announce-ip 2>/dev/null | tail -n 1 || true)"
fi
: "${POD_IP:?could not determine pod IP for REPLICAOF}"
log "announcing master at ${POD_IP}:6380"

# Clear any prior masterauth; ephemeral source has no requirepass.
redis_target CONFIG SET masterauth "" >/dev/null
log "REPLICAOF ${POD_IP} 6380"
redis_target REPLICAOF "${POD_IP}" 6380 >/dev/null

i=0
while true; do
  INFO="$(redis_target INFO replication | tr -d '\r')"
  LINK="$(printf '%s\n' "$INFO" | awk -F: '/^master_link_status:/{print $2}')"
  SYNC="$(printf '%s\n' "$INFO" | awk -F: '/^master_sync_in_progress:/{print $2}')"
  ROLE="$(printf '%s\n' "$INFO" | awk -F: '/^role:/{print $2}')"
  if [ "$ROLE" = "slave" ] || [ "$ROLE" = "replica" ]; then
    if [ "$LINK" = "up" ] && [ "${SYNC:-1}" = "0" ]; then
      log "replication sync complete (link=${LINK} sync_in_progress=${SYNC})"
      break
    fi
  fi
  i=$(( i + 1 ))
  if [ $(( i % 15 )) -eq 0 ]; then
    log "waiting for replication sync (~$(( i * 2 ))s) role=${ROLE} link=${LINK:-?} sync=${SYNC:-?}"
  fi
  if [ "$i" -gt 1800 ]; then
    log "ERROR: timeout waiting for REPLICAOF sync" >&2
    printf '%s\n' "$INFO" >&2
    redis_target REPLICAOF NO ONE >/dev/null 2>&1 || true
    mark_failed
    exit 1
  fi
  sleep 2
done

log "REPLICAOF NO ONE (promote sandbox to master with restored dataset)"
redis_target REPLICAOF NO ONE >/dev/null

FINAL_KEYS="$(redis_target DBSIZE | tr -d '\r')"
log "restore finished; target DBSIZE=${FINAL_KEYS} (ephemeral source DBSIZE=${SRC_KEYS})"
if [ "${FINAL_KEYS}" != "${SRC_KEYS}" ]; then
  log "WARNING: target DBSIZE (${FINAL_KEYS}) != ephemeral source (${SRC_KEYS}); TTLs may have expired during sync"
fi
redis_target INFO keyspace || true
redis_target INFO memory | grep -E '^(used_memory_human|maxmemory_human):' || true

redis_ephemeral SHUTDOWN NOSAVE >/dev/null 2>&1 || true
rm -f "$DUMP" "${WORKDIR}/dump"*
rm -rf "${RESTORE_DIR}"

touch "${WORKDIR}/.step-job-done"
log "wrote marker .step-job-done; releasing sibling containers"
log "waiting for sibling containers to observe job-done before removing scratch"
sleep 20
log "removing restore scratch dir ${WORKDIR}"
rm -rf "${WORKDIR}"
log "stage done"
