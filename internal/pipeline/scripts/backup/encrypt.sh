set -eu
log() { echo "[encrypt $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
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
wait_for "${DATA_DIR}/.step-compress-done" "compress"
log "stage start: encrypt"
DUMP_PREFIX="${DUMP_PREFIX:-pg_dump}"
case "${DUMP_PREFIX}" in
  mysqldump) GZ_GLOB='mysqldump-*.sql.gz' ;;
  redisdump) GZ_GLOB='redisdump-*.rdb.gz' ;;
  *)         GZ_GLOB='pg_dump-*.pgdump.gz' ;;
esac
ls "${DATA_DIR}"/${GZ_GLOB} >/dev/null 2>&1 || {
  log "ERROR: no ${GZ_GLOB} after compress marker" >&2
  mark_failed
  exit 1
}
export GNUPGHOME="/tmp/gpg-home"
mkdir -p "$GNUPGHOME" && chmod 700 "$GNUPGHOME"
found=0
for f in "${DATA_DIR}"/${GZ_GLOB}; do
  [ -f "$f" ] || continue
  found=1
  log "gpg encrypt ${f} ($(wc -c < "$f") bytes)"
  gpg --batch --yes --symmetric --cipher-algo AES256 \
      --passphrase-file /run/secrets/gpg/gpg_passphrase \
      --output "${f}.gpg" "$f"
  rm "$f"
  log "gpg done -> ${f}.gpg ($(wc -c < "${f}.gpg") bytes); removed plaintext gz"
  mkdir -p "${DATA_ROOT}/retained"
  cp -f "${f}.gpg" "${DATA_ROOT}/retained/"
  log "kept local copy ${DATA_ROOT}/retained/$(basename "${f}.gpg") (LOCAL_RETENTION_DAYS=${LOCAL_RETENTION_DAYS:-7})"
done
if [ "$found" -eq 0 ]; then
  log "ERROR: no ${GZ_GLOB} files to encrypt" >&2
  mark_failed
  exit 1
fi
touch "${DATA_DIR}/.step-encrypt-done"
log "wrote marker .step-encrypt-done; stage work done"
hold_until_job_done
