set -eu
umask 0002
STAGE=mysqldump
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${DATA_DIR}/.step-cleanup-done" "cleanup"
log "stage start: dump database=${MYSQL_DATABASE} host=${MYSQL_HOST}:${MYSQL_PORT}"
log "scratch dir=${DATA_DIR}"
OUT="${DATA_DIR}/mysqldump-${MYSQL_DATABASE}-$(date '+%Y-%m-%d-%H-%M').sql"
export MYSQL_PWD="${MYSQL_PASSWORD}"
log "running mysqldump -> ${OUT}"
mysqldump --single-transaction --routines --triggers --events \
  --host="${MYSQL_HOST}" --port="${MYSQL_PORT}" --user="${MYSQL_USER}" \
  --result-file="${OUT}" "${MYSQL_DATABASE}"
log "mysqldump finished size=$(wc -c < "${OUT}") bytes"
touch "${DATA_DIR}/.step-dump-done"
log "wrote marker .step-dump-done; stage work done"
hold_until_job_done
