set -eu
umask 0002
STAGE=redisdump
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${DATA_DIR}/.step-cleanup-done" "cleanup"
log "stage start: dump redis=${REDIS_NAME} host=${REDIS_HOST}:${REDIS_PORT}"
log "scratch dir=${DATA_DIR}"
OUT="${DATA_DIR}/redisdump-${REDIS_NAME}-$(date '+%Y-%m-%d-%H-%M').rdb"
if [ -n "${REDIS_PASSWORD:-}" ]; then
  export REDISCLI_AUTH="${REDIS_PASSWORD}"
fi
USER_ARGS=""
if [ -n "${REDIS_USERNAME:-}" ] && [ "${REDIS_USERNAME}" != "default" ]; then
  USER_ARGS="--user ${REDIS_USERNAME}"
fi
log "running redis-cli --rdb -> ${OUT}"
# shellcheck disable=SC2086
redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" ${USER_ARGS} --rdb "${OUT}"
log "redis-cli --rdb finished size=$(wc -c < "${OUT}") bytes"
touch "${DATA_DIR}/.step-dump-done"
log "wrote marker .step-dump-done; stage work done"
hold_until_job_done
