set -eu
umask 0002
STAGE=pgdump
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
pipeline_init
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
