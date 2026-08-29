set -eu
umask 0002
STAGE=mysqlrestore
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${WORKDIR}/.step-extract-done" "extract"
already_done_exit "${WORKDIR}/.step-job-done" "mysqlrestore"

DB="${MYSQL_DATABASE:-}"
HOST="${MYSQL_HOST:-}"
PORT="${MYSQL_PORT:-3306}"
USER="${MYSQL_USER:-}"
: "${DB:?MYSQL_DATABASE required}"
: "${HOST:?MYSQL_HOST required}"
: "${USER:?MYSQL_USER required}"
export MYSQL_PWD="${MYSQL_PWD:-${MYSQL_PASSWORD:?MYSQL_PWD or MYSQL_PASSWORD required}}"

log "stage start: restore database=${DB} host=${HOST}:${PORT}"
# Scratch artifact is dump (engine-neutral) for all engines (fetch/decrypt/extract stay shared).
DUMP="${WORKDIR}/dump"
test -f "$DUMP"
log "dump file size=$(wc -c < "$DUMP") bytes"

drop_allowed() {
  case "${DROP_DATABASE_IF_EXISTS:-}" in
    yes|YES|true|TRUE|1) return 0 ;;
    *) return 1 ;;
  esac
}

mysql_admin() {
  mariadb --host="${HOST}" --port="${PORT}" --user="${USER}" "$@"
}

DB_EXISTS="$(mysql_admin -N -e \
  "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '${DB}';" \
  | head -n 1 || true)"
if [ "${DB_EXISTS}" = "${DB}" ]; then
  OBJ_COUNT="$(mysql_admin -N -e \
    "SELECT COUNT(*) FROM information_schema.tables
     WHERE table_schema = '${DB}'
       AND table_type IN ('BASE TABLE', 'VIEW');" || echo 0)"
  log "database ${DB} exists; table/view count=${OBJ_COUNT}"
  if [ "${OBJ_COUNT}" -gt 0 ]; then
    if drop_allowed; then
      log "DROP_DATABASE_IF_EXISTS=${DROP_DATABASE_IF_EXISTS}; dropping ${DB}"
      mysql_admin -e "DROP DATABASE \`${DB}\`;"
      log "creating empty database ${DB}"
      mysql_admin -e "CREATE DATABASE \`${DB}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
    else
      log "ERROR: database ${DB} exists and is not empty (${OBJ_COUNT} objects)" >&2
      log "ERROR: set DROP_DATABASE_IF_EXISTS=yes to allow drop+recreate, or restore into an empty DB" >&2
      mark_failed
      exit 1
    fi
  else
    log "database ${DB} exists but is empty; proceeding"
  fi
else
  log "database ${DB} does not exist; creating"
  mysql_admin -e "CREATE DATABASE \`${DB}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
fi

log "applying mysqldump SQL into ${DB}"
# Strip mysqldump GTID / DEFINER noise that breaks restore across hosts.
FILTERED="${WORKDIR}/dump.filtered.sql"
{
  # Session-only restore speed knobs (not valid as mariadbd my.cnf options).
  echo "SET SESSION unique_checks=0;"
  echo "SET SESSION foreign_key_checks=0;"
  echo "SET SESSION autocommit=0;"
  sed -E \
    -e '/^SET[[:space:]]+@@GLOBAL\.GTID_PURGED/d' \
    -e '/^SET[[:space:]]+@@SESSION\.SQL_LOG_BIN/d' \
    -e 's/DEFINER=`[^`]*`@`[^`]*`/DEFINER=CURRENT_USER/g' \
    "$DUMP"
  echo "COMMIT;"
  echo "SET SESSION unique_checks=1;"
  echo "SET SESSION foreign_key_checks=1;"
  echo "SET SESSION autocommit=1;"
} > "$FILTERED"
rm -f "$DUMP"
DUMP="$FILTERED"

mysql_admin --database="${DB}" < "$DUMP"
log "mariadb restore finished"
log "database size summary"
mysql_admin -N -e "
  SELECT CONCAT(
    table_schema, ' ',
    ROUND(SUM(data_length + index_length) / 1024 / 1024, 2), ' MiB'
  )
  FROM information_schema.tables
  WHERE table_schema = '${DB}'
  GROUP BY table_schema;
" || true

rm -f "$DUMP" "${WORKDIR}/dump"*
touch "${WORKDIR}/.step-job-done"
log "wrote marker .step-job-done; releasing sibling containers"
log "waiting for sibling containers to observe job-done before removing scratch"
sleep 20
log "removing restore scratch dir ${WORKDIR}"
rm -rf "${WORKDIR}"
log "stage done"
