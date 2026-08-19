# Karkive

Kubernetes operator that reads `Backup` / `Restore` CRs and runs the same
logical dump pipeline as the existing Helm CronJobs: dump → gzip → gpg → S3
(and the reverse).

**Phase 1:** PostgreSQL backup and restore. MariaDB and Redis land in follow-up work.
The CRDs already accept those engines so the API does not need a rename later.

## Layout

```text
api/v1alpha1/          Backup + Restore types (karkive.io)
cmd/                   Operator entrypoint
internal/
  controller/          Reconcile loops
  resources/           ConfigMap / PVC / CronJob builders
  pipeline/scripts/    Embedded shell stages for the dump pipeline
  config/              Operator-wide image / S3 defaults
charts/karkive/        Helm chart that deploys the operator
config/crd/bases/      Generated CRDs
config/samples/        Example Backup / Restore / Secret
```

A `Backup` named `app-postgres` is reconciled into:

| Resource | Name |
| --- | --- |
| ConfigMap | `karkive-app-postgres` |
| PVC | `karkive-app-postgres` (skip with `spec.persistence.enabled: false`) |
| CronJob | `karkive-app-postgres` |

Containers in the CronJob: `cleanup` → `pgdump` → `compress` → `encrypt` → `s3-sync`.

A `Restore` named `app-postgres` is reconciled into the same shape (`karkive-<restore-name>`).
Containers: `cleanup` → `fetch` → `decrypt` → `extract` → `pgrestore`.

Backup secret keys (same namespace as the CR): `username`, `password`,
`s3_access_key`, `s3_secret_key`, `gpg_passphrase`.

Restore `secretRef` keys: `s3_access_key`, `s3_secret_key`, `gpg_passphrase`.
Target DB credentials come from `spec.postgresSecret` (`username` / `password` by default).

## Develop

```bash
make generate manifests test
make run          # against the current kubeconfig
kubectl apply -f config/crd/bases
kubectl apply -f config/samples/backup-secret.yaml
kubectl apply -f config/samples/karkive_v1alpha1_backup.yaml
```

Deploy the operator:

```bash
helm install karkive ./charts/karkive -n karkive-system --create-namespace \
  --set image.tag=latest
```

Images are published to `ghcr.io/mahdidarabi/karkive` from GitHub Actions on
`main` (`latest`, `main`, `sha-<git-sha>`) and on tags `v*` (semver).

## Roadmap

1. Postgres backup (done)
2. Postgres restore (done)
3. MariaDB backup / restore
4. Redis backup / restore
