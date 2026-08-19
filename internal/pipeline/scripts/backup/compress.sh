set -eu
log() { echo "[compress $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
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
wait_for "${DATA_DIR}/.step-dump-done" "dump"
log "stage start: compress"
DUMP_PREFIX="${DUMP_PREFIX:-pg_dump}"
case "${DUMP_PREFIX}" in
  mysqldump) PLAIN_GLOB='mysqldump-*.sql' ;;
  redisdump) PLAIN_GLOB='redisdump-*.rdb' ;;
  *)         PLAIN_GLOB='pg_dump-*.pgdump' ;;
esac
ls "${DATA_DIR}"/${PLAIN_GLOB} >/dev/null 2>&1 || {
  log "ERROR: no ${PLAIN_GLOB} after dump marker" >&2
  mark_failed
  exit 1
}
found=0
for f in "${DATA_DIR}"/${PLAIN_GLOB}; do
  [ -f "$f" ] || continue
  found=1
  log "gzip ${f} ($(wc -c < "$f") bytes)"
  gzip "$f"
  log "gzip done -> ${f}.gz ($(wc -c < "${f}.gz") bytes)"
done
if [ "$found" -eq 0 ]; then
  log "ERROR: no ${PLAIN_GLOB} files to compress" >&2
  mark_failed
  exit 1
fi
touch "${DATA_DIR}/.step-compress-done"
log "wrote marker .step-compress-done; stage work done"
hold_until_job_done
