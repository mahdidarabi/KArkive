set -eu
umask 0002
STAGE=pgdump
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${DATA_DIR}/.step-cleanup-done" "cleanup"
already_done_hold "${DATA_DIR}/.step-dump-done" "dump"
clear_step_failed
log "stage start: dump database=${PGDATABASE} host=${PGHOST}:${PGPORT}"
log "scratch dir=${DATA_DIR}"
# Cluster GUCs (often 180s) cancel a long pg_dump; 0 = no limit for this session.
export PGOPTIONS="${PGOPTIONS:+${PGOPTIONS} }-c statement_timeout=0 -c lock_timeout=0"
log "PGOPTIONS=${PGOPTIONS}"
OUT="${DATA_DIR}/pg_dump-${PGDATABASE}-$(date '+%Y-%m-%d-%H-%M').pgdump"
log "running pg_dump -> ${OUT}"
err="${DATA_DIR}/.pg_dump.stderr"
dump_heartbeat_start "${OUT}"
set +e
pg_dump --clean --if-exists --load-via-partition-root --quote-all-identifiers \
  --no-password --format=plain --file="${OUT}" 2>"${err}"
ec=$?
set -e
dump_heartbeat_stop
log_file_lines "pg_dump: " "${err}"
rm -f "${err}"
if [ "$ec" -ne 0 ]; then
  log "ERROR: pg_dump failed exit=${ec}" >&2
  mark_failed
  exit "$ec"
fi
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
