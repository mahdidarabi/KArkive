set -eu
umask 0002
STAGE=extract
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${WORKDIR}/.step-decrypt-done" "decrypt"
log "stage start: extract (gunzip)"
test -s "${WORKDIR}/dump.gz"
log "gunzip dump.gz ($(wc -c < "${WORKDIR}/dump.gz") bytes)"
gzip -d "${WORKDIR}/dump.gz"
log "plain dump ready size=$(wc -c < "${WORKDIR}/dump") bytes"
touch "${WORKDIR}/.step-extract-done"
log "wrote marker .step-extract-done; stage work done"
hold_until_job_done
