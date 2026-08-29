set -eu
umask 0002
STAGE=mysqldump
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
DATA_ROOT="${DATA_DIR:-${PGDUMP_DIR:?DATA_DIR or PGDUMP_DIR required}}"
DATA_DIR="${DATA_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${DATA_DIR}/.step-cleanup-done" "cleanup"
already_done_hold "${DATA_DIR}/.step-dump-done" "dump"
clear_step_failed
DB="${MYSQL_DATABASE:-${PGDATABASE:-}}"
HOST="${MYSQL_HOST:-${PGHOST:-}}"
PORT="${MYSQL_PORT:-${PGPORT:-3306}}"
USER="${MYSQL_USER:-${PGUSER:-}}"
: "${DB:?MYSQL_DATABASE/PGDATABASE required}"
: "${HOST:?MYSQL_HOST/PGHOST required}"
: "${USER:?MYSQL_USER/PGUSER required}"
export MYSQL_PWD="${MYSQL_PWD:-${MYSQL_PASSWORD:?MYSQL_PWD or MYSQL_PASSWORD required}}"
log "stage start: dump database=${DB} host=${HOST}:${PORT}"
log "scratch dir=${DATA_DIR}"
OUT="${DATA_DIR}/mysqldump-${DB}-$(date '+%Y-%m-%d-%H-%M').sql"
log "running mysqldump -> ${OUT}"
mysqldump \
  --host="${HOST}" \
  --port="${PORT}" \
  --user="${USER}" \
  --single-transaction \
  --routines \
  --triggers \
  --events \
  --hex-blob \
  --default-character-set=utf8mb4 \
  --databases "${DB}" \
  > "${OUT}"
log "mysqldump finished size=$(wc -c < "${OUT}") bytes"
log "database size summary"
mariadb --host="${HOST}" --port="${PORT}" --user="${USER}" -N -e "
  SELECT CONCAT(
    table_schema, ' ',
    ROUND(SUM(data_length + index_length) / 1024 / 1024, 2), ' MiB'
  )
  FROM information_schema.tables
  WHERE table_schema = '${DB}'
  GROUP BY table_schema;
" || true
log "WARNING: live DB stats above may differ from dump contents (DB can change during/after backup)"
touch "${DATA_DIR}/.step-dump-done"
log "wrote marker .step-dump-done; stage work done"
hold_until_job_done
