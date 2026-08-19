# Karkive

Kubernetes operator that reads `Backup` / `Restore` CRs and runs the same
logical dump pipeline as the existing Helm CronJobs: dump → gzip → gpg → S3
(and the reverse, later).

**Phase 1:** PostgreSQL backup. MariaDB, Redis, and restore land in follow-up work.
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

Secret keys (same namespace as the CR): `username`, `password`,
`s3_access_key`, `s3_secret_key`, `gpg_passphrase`.

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
helm install karkive ./charts/karkive -n karkive-system --create-namespace
```

## Roadmap

1. Postgres backup (this tree)
2. Postgres restore
3. MariaDB backup / restore
4. Redis backup / restore
