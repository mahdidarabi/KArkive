set -eu
umask 0002
STAGE=decrypt
# Shared PVC is reused across Jobs; scope scratch to this pod
# so peers never see stale .step-* markers from a prior run.
WORKDIR_ROOT="${WORKDIR}"
WORKDIR="${WORKDIR_ROOT}/${HOSTNAME}"
pipeline_init
wait_for "${WORKDIR}/.step-fetch-done" "fetch"
already_done_hold "${WORKDIR}/.step-decrypt-done" "decrypt"
log "stage start: decrypt"
test -s "${WORKDIR}/dump.gz.gpg"
export GNUPGHOME="/tmp/gpg-home"
mkdir -p "$GNUPGHOME" && chmod 700 "$GNUPGHOME"
log "gpg decrypt dump.gz.gpg ($(wc -c < "${WORKDIR}/dump.gz.gpg") bytes)"
gpg --batch --yes --decrypt \
  --passphrase-file /run/secrets/gpg/gpg_passphrase \
  --output "${WORKDIR}/dump.gz" \
  "${WORKDIR}/dump.gz.gpg"
rm -f "${WORKDIR}/dump.gz.gpg"
log "decrypt done size=$(wc -c < "${WORKDIR}/dump.gz") bytes"
touch "${WORKDIR}/.step-decrypt-done"
log "wrote marker .step-decrypt-done; stage work done"
hold_until_job_done
