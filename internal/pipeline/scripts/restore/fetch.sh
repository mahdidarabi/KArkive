set -eu
umask 0002
STAGE=fetch
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
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
