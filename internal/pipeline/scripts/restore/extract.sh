set -eu
log() { echo "[extract $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
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
wait_for() {
  marker="$1"
  prev="$2"
  log "waiting for previous stage (${prev}) marker=${marker}"
  i=0
  while [ ! -f "${marker}" ]; do
    if [ -f "${WORKDIR}/.step-failed" ]; then
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
wait_for "${WORKDIR}/.step-decrypt-done" "decrypt"
log "stage start: extract (gunzip)"
test -s "${WORKDIR}/dump.gz"
log "gunzip dump.gz ($(wc -c < "${WORKDIR}/dump.gz") bytes)"
gzip -d "${WORKDIR}/dump.gz"
log "plain dump ready size=$(wc -c < "${WORKDIR}/dump") bytes"
touch "${WORKDIR}/.step-extract-done"
log "wrote marker .step-extract-done; stage work done"
hold_until_job_done
