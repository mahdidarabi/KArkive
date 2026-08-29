set -eu
umask 0002
STAGE=redisdump
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${DATA_DIR}/.step-cleanup-done" "cleanup"
already_done_hold "${DATA_DIR}/.step-dump-done" "dump"
clear_step_failed
NAME="${REDIS_NAME:-${MYSQL_DATABASE:-${PGDATABASE:-}}}"
HOST="${REDIS_HOST:-}"
PORT="${REDIS_PORT:-6379}"
: "${NAME:?REDIS_NAME required}"
: "${HOST:?REDIS_HOST required}"
# REDISCLI_AUTH is preferred (Redis 6+); falls through to empty for open instances.
export REDISCLI_AUTH="${REDISCLI_AUTH:-${REDIS_PASSWORD:-}}"
USER_ARGS=""
if [ -n "${REDIS_USERNAME:-}" ] && [ "${REDIS_USERNAME}" != "default" ]; then
  USER_ARGS="--user ${REDIS_USERNAME}"
fi
log "stage start: dump instance=${NAME} host=${HOST}:${PORT}"
log "scratch dir=${DATA_DIR}"
OUT="${DATA_DIR}/redisdump-${NAME}-$(date '+%Y-%m-%d-%H-%M').rdb"
log "running redis-cli --rdb -> ${OUT}"
err="${DATA_DIR}/.redisdump.stderr"
dump_heartbeat_start "${OUT}"
set +e
# shellcheck disable=SC2086
redis-cli -h "${HOST}" -p "${PORT}" ${USER_ARGS} --rdb "${OUT}" 2>"${err}"
ec=$?
set -e
dump_heartbeat_stop
log_file_lines "redis-cli: " "${err}"
rm -f "${err}"
if [ "$ec" -ne 0 ]; then
  log "ERROR: redis-cli --rdb failed exit=${ec}" >&2
  mark_failed
  exit "$ec"
fi
log "redis-cli --rdb finished size=$(wc -c < "${OUT}") bytes"
log "source INFO keyspace / memory"
# shellcheck disable=SC2086
redis-cli -h "${HOST}" -p "${PORT}" ${USER_ARGS} INFO keyspace || true
# shellcheck disable=SC2086
redis-cli -h "${HOST}" -p "${PORT}" ${USER_ARGS} INFO memory | grep -E '^(used_memory_human|maxmemory_human):' || true
log "WARNING: live source INFO above may differ from RDB contents (keyspace can change after snapshot)"
touch "${DATA_DIR}/.step-dump-done"
log "wrote marker .step-dump-done; stage work done"
hold_until_job_done
