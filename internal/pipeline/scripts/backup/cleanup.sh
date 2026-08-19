set -eu
log() { echo "[cleanup $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
umask 0002
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
# DATA_DIR is the PVC mount (engine-neutral). Legacy PGDUMP_DIR still accepted.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
mkdir -p "${DATA_DIR}"
mark_failed() {
  # Group-writable so uid 1000 (mc) and uid 26 (postgres) can both signal.
  touch "${DATA_DIR}/.step-failed" 2>/dev/null || true
  chmod 666 "${DATA_DIR}/.step-failed" 2>/dev/null || true
}
trap 'ec=$?; [ "$ec" -eq 0 ] || mark_failed' EXIT
hold_until_job_done() {
  log "holding until job complete (.step-job-done) so pod stays Running"
  i=0
  while [ ! -f "${DATA_DIR}/.step-job-done" ]; do
    if [ -f "${DATA_DIR}/.step-failed" ]; then
      log "ERROR: peer stage failed (.step-failed); aborting hold" >&2
      exit 1
    fi
    sleep 5
    i=$(( i + 1 ))
    if [ $(( i % 12 )) -eq 0 ]; then
      log "still holding for job completion (~$(( i * 5 ))s)"
    fi
    if [ "$i" -gt 17280 ]; then
      mark_failed
      log "ERROR: timeout holding for job completion" >&2
      exit 1
    fi
  done
  log "job complete marker seen; exiting 0"
}
log "stage start: cleanup root=${DATA_ROOT}"
log "scratch dir=${DATA_DIR}"
log "clearing step markers under ${DATA_DIR}"
rm -f "${DATA_DIR}/.step-"*
LOCAL_KEEP="${LOCAL_RETENTION_DAYS:-7}"
RETAINED_DIR="${DATA_ROOT}/retained"
mkdir -p "${RETAINED_DIR}"
log "local retention=${LOCAL_KEEP} day(s); pruning retained dumps under ${RETAINED_DIR}"
DUMP_PREFIX="${DUMP_PREFIX:-pg_dump}"
case "${DUMP_PREFIX}" in
  mysqldump) RETAINED_GLOB='mysqldump-*.sql.gz.gpg' ;;
  redisdump) RETAINED_GLOB='redisdump-*.rdb.gz.gpg' ;;
  *)         RETAINED_GLOB='pg_dump-*.pgdump.gz.gpg' ;;
esac
find "${RETAINED_DIR}" -type f -name "${RETAINED_GLOB}" -mtime "+${LOCAL_KEEP}" -print \
  | while IFS= read -r f; do
      log "deleted expired local ${f}"
      rm -f "${f}"
    done || true
# Legacy / stray encrypted dumps outside retained/
# -delete implies -depth, which disables -prune; delete via rm instead.
find "${DATA_ROOT}" \( -path "${RETAINED_DIR}" -o -path "${DATA_DIR}" \) -prune -o \
  -type f \( -name 'pg_dump-*.pgdump.gz.gpg' -o -name 'mysqldump-*.sql.gz.gpg' -o -name 'redisdump-*.rdb.gz.gpg' \) \
  -mtime "+${LOCAL_KEEP}" -print \
  | while IFS= read -r f; do
      log "deleted expired local ${f}"
      rm -f "${f}"
    done || true
# Job scratch dirs ($HOSTNAME) are ephemeral once .gpg is in retained/.
# Drop every prior process dir immediately (keep retained/ + lost+found).
log "removing old process/scratch dirs (keep retained/ and current ${HOSTNAME})"
find "${DATA_ROOT}" -mindepth 1 -maxdepth 1 -type d \
  ! -name "${HOSTNAME}" ! -name retained ! -name lost+found -print \
  | while IFS= read -r d; do
      log "removing old process dir ${d}"
      rm -rf "${d}"
    done || true
touch "${DATA_DIR}/.step-cleanup-done"
log "wrote marker .step-cleanup-done; stage work done"
hold_until_job_done
