set -eu
umask 0002
STAGE=cleanup
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
log "stage start: cleanup root=${WORKDIR_ROOT}"
# mc image has no find(1); this stage owns PVC process-dir pruning.
log "removing old restore process dirs under ${WORKDIR_ROOT}"
find "${WORKDIR_ROOT}" -mindepth 1 -maxdepth 1 -type d \
  ! -name "${HOSTNAME}" ! -name lost+found -print \
  | while IFS= read -r d; do
      log "removing old process dir ${d}"
      rm -rf "${d}"
    done || true
log "clearing workdir markers/dumps under ${WORKDIR}"
rm -f "${WORKDIR}/.step-"* "${WORKDIR}/dump"*
touch "${WORKDIR}/.step-cleanup-done"
log "wrote marker .step-cleanup-done; stage work done"
hold_until_job_done
