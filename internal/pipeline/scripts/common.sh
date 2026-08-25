# Shared pipeline helpers. Callers set STAGE and DATA_DIR (backup) or WORKDIR
# (restore), then pipeline_init.
log() { echo "[${STAGE:-pipeline} $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }

mark_failed() {
  # Group-writable so uid 1000 (mc), engine UIDs (postgres 26, mysql/redis 999),
  # and tools UID 65532 (busybox / gpg) can all signal.
  touch "${STEP_DIR}/.step-failed" 2>/dev/null || true
  chmod 666 "${STEP_DIR}/.step-failed" 2>/dev/null || true
}

pipeline_init() {
  if [ -n "${STEP_DIR:-}" ]; then
    :
  elif [ -n "${DATA_DIR:-}" ]; then
    STEP_DIR="${DATA_DIR}"
  elif [ -n "${WORKDIR:-}" ]; then
    STEP_DIR="${WORKDIR}"
  else
    log "ERROR: STEP_DIR, DATA_DIR, or WORKDIR required" >&2
    exit 1
  fi
  mkdir -p "${STEP_DIR}"
  trap 'ec=$?; [ "$ec" -eq 0 ] || mark_failed' EXIT
}

wait_for() {
  marker="$1"
  prev="$2"
  log "waiting for previous stage (${prev}) marker=${marker}"
  i=0
  while [ ! -f "${marker}" ]; do
    if [ -f "${STEP_DIR}/.step-failed" ]; then
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
  while [ ! -f "${STEP_DIR}/.step-job-done" ]; do
    if [ -f "${STEP_DIR}/.step-failed" ]; then
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
