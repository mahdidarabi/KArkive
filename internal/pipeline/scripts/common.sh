# Shared pipeline helpers. Callers set STAGE and DATA_DIR (backup) or WORKDIR
# (restore), then pipeline_init.
log_file_enabled() {
  case "${LOG_FILE_ENABLED:-false}" in
    1|yes|YES|true|TRUE) return 0 ;;
    *) return 1 ;;
  esac
}

log() {
  msg="[${STAGE:-pipeline} $(date '+%Y-%m-%dT%H:%M:%S%z')] $*"
  echo "$msg" >&2
  if [ -n "${LOG_FILE:-}" ]; then
    echo "$msg" >> "${LOG_FILE}" 2>/dev/null || true
  fi
}

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
  if log_file_enabled; then
    # Durable log file on the volume: DATA_ROOT/logs or WORKDIR_ROOT/logs.
    # World-writable: stage UIDs do not share a write group besides fsGroup 26.
    log_root="${DATA_ROOT:-${WORKDIR_ROOT:-}}"
    if [ -n "$log_root" ]; then
      LOG_DIR="${log_root}/logs"
      mkdir -p "${LOG_DIR}" 2>/dev/null || true
      chmod 777 "${LOG_DIR}" 2>/dev/null || true
      LOG_FILE="${LOG_DIR}/${HOSTNAME:-pipeline}.log"
      touch "${LOG_FILE}" 2>/dev/null || true
      chmod 666 "${LOG_FILE}" 2>/dev/null || true
    fi
  fi
  trap 'ec=$?; [ "$ec" -eq 0 ] || mark_failed' EXIT
}

prune_pipeline_logs() {
  log_file_enabled || return 0
  root="${1:-}"
  keep="${2:-${LOCAL_KEEP:-7}}"
  [ -n "$root" ] || return 0
  logs_dir="${root}/logs"
  mkdir -p "${logs_dir}"
  log "pruning pipeline logs under ${logs_dir} older than ${keep} day(s)"
  find "${logs_dir}" -type f -name '*.log' -mtime "+${keep}" -print \
    | while IFS= read -r f; do
        log "deleted expired log ${f}"
        rm -f "${f}"
      done || true
}

# If this container restarted after finishing, skip work. Cleanup must not
# run `rm -f .step-*` again (that wipes in-flight dump/compress markers).
already_done_hold() {
  marker="$1"
  name="$2"
  if [ -f "${marker}" ]; then
    log "already complete (${name}); skipping work"
    hold_until_job_done
    exit 0
  fi
}

already_done_exit() {
  marker="$1"
  name="$2"
  if [ -f "${marker}" ]; then
    log "already complete (${name}); exiting"
    exit 0
  fi
}

# Dump/fetch retry in the same pod: drop a prior .step-failed so sibling
# wait loops can proceed. Only call from the stage that is retrying work.
clear_step_failed() {
  if [ -f "${STEP_DIR}/.step-failed" ]; then
    log "clearing .step-failed from a previous in-pod attempt"
    rm -f "${STEP_DIR}/.step-failed"
  fi
}

# Copy a file into the pipeline log (tool stderr). Empty files are skipped.
log_file_lines() {
  prefix="$1"
  file="$2"
  [ -f "$file" ] && [ -s "$file" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    [ -n "$line" ] || continue
    log "${prefix}${line}"
  done < "$file"
}

# Background size/time lines while dump writes $1. Call dump_heartbeat_stop after.
dump_heartbeat_start() {
  out="$1"
  touch "${STEP_DIR}/.dump-running"
  (
    i=0
    while [ -f "${STEP_DIR}/.dump-running" ]; do
      sleep 30
      [ -f "${STEP_DIR}/.dump-running" ] || break
      i=$((i + 30))
      sz=0
      if [ -f "$out" ]; then
        sz=$(wc -c < "$out") || sz=0
      fi
      log "still dumping (~${i}s) size=${sz} bytes"
    done
  ) &
  DUMP_HB_PID=$!
}

dump_heartbeat_stop() {
  rm -f "${STEP_DIR}/.dump-running"
  if [ -n "${DUMP_HB_PID:-}" ]; then
    kill "${DUMP_HB_PID}" 2>/dev/null || true
    wait "${DUMP_HB_PID}" 2>/dev/null || true
    DUMP_HB_PID=""
  fi
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
