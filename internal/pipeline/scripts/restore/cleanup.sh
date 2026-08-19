set -eu
log() { echo "[cleanup $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
umask 0002
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
mkdir -p "${WORKDIR}"
mark_failed() {
  # Group-writable so uid 1000 (mc) and uid 26 (postgres) can both signal.
  touch "${WORKDIR}/.step-failed" 2>/dev/null || true
  chmod 666 "${WORKDIR}/.step-failed" 2>/dev/null || true
}
trap 'ec=$?; [ "$ec" -eq 0 ] || mark_failed' EXIT
hold_until_job_done() {
  log "holding until job complete (.step-job-done) so pod stays Running"
  i=0
  while [ ! -f "${WORKDIR}/.step-job-done" ]; do
    if [ -f "${WORKDIR}/.step-failed" ]; then
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
log "stage start: cleanup root=${WORKDIR_ROOT}"
# mc image has no find(1); this stage owns PVC process-dir pruning.
log "removing old restore process dirs under ${WORKDIR_ROOT}"
find "${WORKDIR_ROOT}" -mindepth 1 -maxdepth 1 -type d \
  ! -name "${HOSTNAME}" ! -name lost+found -print \
  | while IFS= read -r d; do
      log "removing old process dir ${d}"
      rm -rf "${d}"
    done || true
log "clearing workdir markers/dumps under ${WORKDIR}"
rm -f "${WORKDIR}/.step-"* "${WORKDIR}/dump"*
touch "${WORKDIR}/.step-cleanup-done"
log "wrote marker .step-cleanup-done; stage work done"
hold_until_job_done
