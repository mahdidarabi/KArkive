set -eu
log() { echo "[s3-sync $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
umask 0002
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
mkdir -p "${DATA_DIR}"
mark_failed() {
  # Group-writable so uid 1000 (mc) and uid 26 (postgres) can both signal.
  touch "${DATA_DIR}/.step-failed" 2>/dev/null || true
  chmod 666 "${DATA_DIR}/.step-failed" 2>/dev/null || true
}
trap 'ec=$?; [ "$ec" -eq 0 ] || mark_failed' EXIT
wait_for() {
  marker="$1"
  prev="$2"
  log "waiting for previous stage (${prev}) marker=${marker}"
  i=0
  while [ ! -f "${marker}" ]; do
    if [ -f "${DATA_DIR}/.step-failed" ]; then
      log "ERROR: peer stage failed (.step-failed); aborting wait for ${prev}" >&2
      exit 1
    fi
    sleep 2
    i=$(( i + 1 ))
    if [ $(( i % 15 )) -eq 0 ]; then
      log "still waiting for ${prev} (${i} checks, ~$(( i * 2 ))s)"
    fi
    if [ "$i" -gt 43200 ]; then
      mark_failed
      log "ERROR: timeout waiting for ${prev} after ~$(( i * 2 ))s" >&2
      exit 1
    fi
  done
  log "previous stage (${prev}) finished; marker present"
}
wait_for "${DATA_DIR}/.step-encrypt-done" "encrypt"
log "stage start: s3-sync bucket=${S3_BUCKET} path=${S3_PATH}"
log "configuring mc alias endpoint=${S3_ENDPOINT}"
mc alias set backup "${S3_ENDPOINT}" "${S3_ACCESS_KEY}" "${S3_SECRET_KEY}"
log "mirroring ${DATA_DIR}/ -> backup/${S3_BUCKET}/${S3_PATH}/"
mc mirror --exclude '.step-*' "${DATA_DIR}/" "backup/${S3_BUCKET}/${S3_PATH}/"
# Prefer S3_RETENTION_DAYS; fall back to S3_RETENTION_HOURS (legacy).
if [ -n "${S3_RETENTION_DAYS:-}" ]; then
  S3_OLDER_THAN="$((S3_RETENTION_DAYS * 24))h"
else
  S3_OLDER_THAN="${S3_RETENTION_HOURS:-48}h"
fi
DUMP_PREFIX="${DUMP_PREFIX:-pg_dump}"
case "${DUMP_PREFIX}" in
  mysqldump) S3_NAME='mysqldump-*.sql.gz.gpg' ;;
  redisdump) S3_NAME='redisdump-*.rdb.gz.gpg' ;;
  *)         S3_NAME='pg_dump-*.pgdump.gz.gpg' ;;
esac
log "mirror finished; pruning S3 objects older than ${S3_OLDER_THAN} name=${S3_NAME} (days=${S3_RETENTION_DAYS:-n/a} hours=${S3_RETENTION_HOURS:-n/a})"
mc find "backup/${S3_BUCKET}/${S3_PATH}/" \
  --name "${S3_NAME}" \
  --older-than "${S3_OLDER_THAN}" | while IFS= read -r obj; do
  if [ -n "$obj" ]; then
    log "removing expired ${obj}"
    mc rm "$obj"
  fi
done
touch "${DATA_DIR}/.step-job-done"
log "wrote marker .step-job-done; releasing sibling containers; stage done"
