set -eu
log() { echo "[fetch $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
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
  log "previous stage (${prev}) complete"
}
fallback_enabled() {
  case "${USE_LATEST_BACKUP_AS_FALLBACK:-}" in
    yes|YES|true|TRUE|1) return 0 ;;
    *) return 1 ;;
  esac
}
find_latest() {
  DUMP_PREFIX="${DUMP_PREFIX:-pg_dump}"
  case "${DUMP_PREFIX}" in
    mysqldump) S3_NAME='mysqldump-*.sql.gz.gpg' ;;
    redisdump) S3_NAME='redisdump-*.rdb.gz.gpg' ;;
    *)         S3_NAME='pg_dump-*.pgdump.gz.gpg' ;;
  esac
  log "finding latest ${S3_NAME} under ${PREFIX}/"
  LATEST="$(mc find "${PREFIX}/" \
    --name "${S3_NAME}" \
    | sort \
    | tail -n 1)"
  if [ -z "${LATEST}" ]; then
    log "ERROR: no ${S3_NAME} under ${PREFIX}/" >&2
    mark_failed
    exit 1
  fi
  log "selected latest backup: ${LATEST}"
  printf '%s' "${LATEST}"
}
wait_for "${WORKDIR}/.step-cleanup-done" "cleanup"
log "stage start: fetch bucket=${S3_BUCKET} path=${S3_PATH}"
log "scratch dir=${WORKDIR}"
log "clearing workdir markers/dumps under ${WORKDIR}"
rm -f "${WORKDIR}/.step-"* "${WORKDIR}/dump"*
log "configuring mc alias endpoint=${S3_ENDPOINT}"
mc alias set backup "${S3_ENDPOINT}" "${S3_ACCESS_KEY}" "${S3_SECRET_KEY}"
PREFIX="backup/${S3_BUCKET}/${S3_PATH}"
if [ -n "${BACKUP_FILE:-}" ]; then
  SRC="${PREFIX}/${BACKUP_FILE}"
  log "checking explicit BACKUP_FILE=${BACKUP_FILE}"
  if mc stat "${SRC}" >/dev/null 2>&1; then
    log "BACKUP_FILE exists: ${SRC}"
  else
    log "BACKUP_FILE not found: ${SRC}"
    if fallback_enabled; then
      log "USE_LATEST_BACKUP_AS_FALLBACK=${USE_LATEST_BACKUP_AS_FALLBACK}; falling back to latest"
      SRC="$(find_latest)"
    else
      log "ERROR: BACKUP_FILE missing and USE_LATEST_BACKUP_AS_FALLBACK is not yes" >&2
      mark_failed
      exit 1
    fi
  fi
else
  if fallback_enabled; then
    log "BACKUP_FILE empty; USE_LATEST_BACKUP_AS_FALLBACK=${USE_LATEST_BACKUP_AS_FALLBACK}"
    SRC="$(find_latest)"
  else
    log "ERROR: BACKUP_FILE is empty; refusing to pick latest without agreement" >&2
    log "ERROR: set BACKUP_FILE=<object> OR USE_LATEST_BACKUP_AS_FALLBACK=yes" >&2
    mark_failed
    exit 1
  fi
fi
log "fetching ${SRC}"
mc cp "${SRC}" "${WORKDIR}/dump.gz.gpg"
log "fetched size=$(wc -c < "${WORKDIR}/dump.gz.gpg") bytes"
touch "${WORKDIR}/.step-fetch-done"
log "wrote marker .step-fetch-done; stage work done"
hold_until_job_done
