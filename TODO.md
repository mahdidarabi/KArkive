# TODO

## Operator skeleton

- [x] Go module and project layout (`cmd`, `api`, `internal`, Helm)
- [x] `Backup` and `Restore` CRDs (`karkive.io/v1alpha1`)
- [x] Engine enum: `postgres` / `mariadb` / `redis` (API reserved)
- [x] Generate deepcopy and CRD/RBAC manifests (`make generate manifests`)
- [x] Owned resource prefix: `karkive-<cr-name>`
- [x] `app.kubernetes.io/component` from `spec.component` (defaults to CR name)
- [x] Operator does not create Secrets; it only reads `spec.secretRef`
- [x] Default image and S3 flags plus `WATCH_NAMESPACE`
- [x] Helm chart to deploy the operator
- [x] Dockerfile, Makefile, samples, unit tests

## Postgres backup

- [x] Pipeline scripts: `cleanup` → `pgdump` → `compress` → `encrypt` → `s3-sync`
- [x] Backup controller: ConfigMap + optional PVC + CronJob
- [x] Split images: busybox, `vladgh/gpg`, CNPG postgres, minio/mc
- [x] PVC or emptyDir via `spec.persistence.enabled`
- [x] Secret keys: `username`, `password`, `s3_access_key`, `s3_secret_key`, `gpg_passphrase`
- [x] Status: `Ready` / `Pending` / `Error` / `Unsupported`
- [x] Backup samples and examples

## Postgres restore

- [x] Pipeline scripts: `cleanup` → `fetch` → `decrypt` → `extract` → `pgrestore`
- [x] Restore controller: ConfigMap + optional PVC + CronJob
- [x] `secretRef` is S3+GPG only; target DB credentials come from `spec.postgresSecret`
- [x] Job defaults: `restartPolicy: Never`, `backoffLimit: 1`
- [x] `useLatestBackupAsFallback` / `dropDatabaseIfExists` / `stripPgAuditExtension`
- [x] Restore samples and examples

## Monitoring

- [x] Operator `/healthz` and `/readyz`
- [x] Helm liveness and readiness probes
- [x] controller-runtime metrics bind address (`:8080`)
- [ ] Metrics Service for the operator
- [ ] Prometheus `ServiceMonitor`
- [ ] Custom metrics per Backup/Restore (last success, duration, failures)
- [ ] Alerts for missed schedules and failed Jobs
- [ ] Grafana dashboard

## Later engines

- [ ] MariaDB backup (`mysqldump`)
- [ ] MariaDB restore
- [ ] Redis backup (`redis-cli --rdb`)
- [ ] Redis restore

## Later

- [ ] Validating webhooks for Backup/Restore
- [x] CI (tests + image build)
- [x] Publish the operator image to GHCR
- [ ] Publish the Helm chart
- [ ] Richer status from the last Job (success/failure, failure reason)
