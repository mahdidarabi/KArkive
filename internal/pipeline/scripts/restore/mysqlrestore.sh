set -eu
umask 0002
STAGE=mysqlrestore
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${WORKDIR}/.step-extract-done" "extract"
log "stage start: mysqlrestore database=${MYSQL_DATABASE} host=${MYSQL_HOST}:${MYSQL_PORT}"
DUMP="${WORKDIR}/dump"
test -f "$DUMP"
log "dump file size=$(wc -c < "$DUMP") bytes"
export MYSQL_PWD="${MYSQL_PASSWORD}"

drop_allowed() {
  case "${DROP_DATABASE_IF_EXISTS:-}" in
    yes|YES|true|TRUE|1) return 0 ;;
    *) return 1 ;;
  esac
}

mysql_admin() {
  mysql --host="${MYSQL_HOST}" --port="${MYSQL_PORT}" --user="${MYSQL_USER}" --batch --skip-column-names "$@"
}

DB_EXISTS="$(mysql_admin -e "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '${MYSQL_DATABASE}';")"
if [ "${DB_EXISTS}" = "1" ]; then
  OBJ_COUNT="$(mysql_admin -e "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = '${MYSQL_DATABASE}';")"
  log "database ${MYSQL_DATABASE} exists; table count=${OBJ_COUNT}"
  if [ "${OBJ_COUNT}" -gt 0 ]; then
    if drop_allowed; then
      log "DROP_DATABASE_IF_EXISTS=${DROP_DATABASE_IF_EXISTS}; dropping and recreating ${MYSQL_DATABASE}"
      mysql_admin -e "DROP DATABASE \`${MYSQL_DATABASE}\`; CREATE DATABASE \`${MYSQL_DATABASE}\`;"
    else
      log "ERROR: database ${MYSQL_DATABASE} exists and is not empty (${OBJ_COUNT} tables)" >&2
      log "ERROR: set DROP_DATABASE_IF_EXISTS=yes to allow drop+recreate, or restore into an empty DB" >&2
      mark_failed
      exit 1
    fi
  else
    log "database ${MYSQL_DATABASE} exists but is empty"
  fi
else
  log "database ${MYSQL_DATABASE} does not exist; creating"
  mysql_admin -e "CREATE DATABASE \`${MYSQL_DATABASE}\`;"
fi

log "applying mysqldump SQL into ${MYSQL_DATABASE}"
mysql --host="${MYSQL_HOST}" --port="${MYSQL_PORT}" --user="${MYSQL_USER}" \
  "${MYSQL_DATABASE}" < "$DUMP"
log "mysql restore finished"

rm -f "$DUMP" "${WORKDIR}/dump"*
touch "${WORKDIR}/.step-job-done"
log "wrote marker .step-job-done; releasing sibling containers"
log "waiting for sibling containers to observe job-done before removing scratch"
sleep 20
log "removing restore scratch dir ${WORKDIR}"
rm -rf "${WORKDIR}"
log "stage done"
