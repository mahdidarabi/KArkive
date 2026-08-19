set -eu
log() { echo "[pgdump $(date '+%Y-%m-%dT%H:%M:%S%z')] $*" >&2; }
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
wait_for "${DATA_DIR}/.step-cleanup-done" "cleanup"
log "stage start: dump database=${PGDATABASE} host=${PGHOST}:${PGPORT}"
log "scratch dir=${DATA_DIR}"
OUT="${DATA_DIR}/pg_dump-${PGDATABASE}-$(date '+%Y-%m-%d-%H-%M').pgdump"
log "running pg_dump -> ${OUT}"
pg_dump --clean --if-exists --load-via-partition-root --quote-all-identifiers \
  --no-password --format=plain --file="${OUT}"
# PG18+ pg_dump may emit SETs (e.g. transaction_timeout) rejected by older majors.
sed -i -E '/^SET[[:space:]]+transaction_timeout[[:space:]]*=/d' "${OUT}"
log "pg_dump finished size=$(wc -c < "${OUT}") bytes"
log "database size summary"
psql -v ON_ERROR_STOP=1 --no-password -c "SELECT pg_size_pretty(pg_database_size(current_database()));"
log "table size summary"
psql -v ON_ERROR_STOP=1 --no-password -c "
  SELECT
      schemaname,
      relname AS table_name,
      n_live_tup AS estimated_rows,
      pg_size_pretty(pg_total_relation_size(relid)) AS total_size,
      pg_size_pretty(pg_relation_size(relid)) AS table_size,
      pg_size_pretty(
          pg_total_relation_size(relid) - pg_relation_size(relid)
      ) AS index_size
  FROM pg_stat_user_tables
  ORDER BY pg_total_relation_size(relid) DESC;
"
log "WARNING: live DB stats above may differ from dump contents (DB can change during/after backup)"
touch "${DATA_DIR}/.step-dump-done"
log "wrote marker .step-dump-done; stage work done"
hold_until_job_done
