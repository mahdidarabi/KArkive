# Karkive

Kubernetes operator that reads `Backup` / `Restore` CRs and runs a logical
dump pipeline: dump → gzip → gpg → S3 (and the reverse).

Engines: PostgreSQL, MariaDB, and Redis.

## Layout

```text
api/v1alpha1/          Backup + Restore types (karkive.io)
cmd/                   Operator entrypoint
internal/
  controller/          Reconcile loops
  resources/           ConfigMap / PVC / CronJob builders
  pipeline/scripts/    Embedded shell stages for the dump pipeline
  metrics/             Prometheus collector for Backup / Restore status
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

Containers in the CronJob depend on `spec.engine`:

| Engine | Dump | Restore |
| --- | --- | --- |
| postgres | `pgdump` | `pgrestore` |
| mariadb | `mysqldump` | `mysqlrestore` |
| redis | `redisdump` (`redis-cli --rdb`) | `redisrestore` |

Shared stages: `cleanup` → dump → `compress` → `encrypt` → `s3-sync` (backup)
and `cleanup` → `fetch` → `decrypt` → `extract` → restore.

Backup secret keys (same namespace as the CR): `username`, `password`,
`s3_access_key`, `s3_secret_key`, `gpg_passphrase`. For Redis, `username` is
the ACL user (`default` if unused).

Restore `secretRef` keys: `s3_access_key`, `s3_secret_key`, `gpg_passphrase`.
Target credentials come from `spec.postgresSecret`, `spec.mariadbSecret`, or
`spec.redisSecret` (`username` / `password` by default). Redis restore
`FLUSHALL`s the target when `spec.dropDatabaseIfExists` is true (default).

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

Helm charts are pushed to GHCR on tags `v*` (`Chart.yaml` `version` and
`appVersion` must match the tag without the `v` prefix):

```bash
helm show chart oci://ghcr.io/mahdidarabi/charts/karkive --version 0.0.1-alpha.8
helm install karkive oci://ghcr.io/mahdidarabi/charts/karkive \
  --version 0.0.1-alpha.8 \
  -n karkive-system --create-namespace
```

The chart exposes `/metrics` on a ClusterIP Service. Prometheus Operator
scrape, alerting rules, and a Grafana dashboard ConfigMap are off by default:

```bash
helm install karkive ./charts/karkive -n karkive-system --create-namespace \
  --set image.tag=latest \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.prometheusRule.enabled=true \
  --set metrics.grafanaDashboard.enabled=true
```

## Roadmap

1. Postgres backup (done)
2. Postgres restore (done)
3. MariaDB backup / restore (done)
4. Redis backup / restore (done)
