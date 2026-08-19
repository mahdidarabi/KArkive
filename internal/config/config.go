package config

// Config is operator-wide defaults, typically set from flags / Helm values.
type Config struct {
	BusyBoxImage  string
	GnuPGImage    string
	PostgresImage string
	McImage       string
	MariaDBImage  string
	RedisImage    string

	DefaultS3Endpoint string
	DefaultS3Bucket   string
}

const (
	DefaultBusyBoxImage  = "docker.io/library/busybox:1.37"
	DefaultGnuPGImage    = "docker.io/instrumentisto/gnupg:2.4"
	DefaultPostgresImage = "docker.io/cloudnative-pg/postgresql:18.4"
	DefaultMcImage       = "docker.io/minio/mc:RELEASE.2025-08-13T08-35-41Z"
	DefaultMariaDBImage  = "docker.io/library/mariadb:10.6"
	DefaultRedisImage    = "docker.io/library/redis:7.4"

	DefaultDataDir     = "/backup/data"
	DefaultMcConfigDir = "/tmp/mc-config"
	DefaultWorkdir     = "/workdir"

	DefaultLocalRetentionDays int32 = 7
	DefaultS3RetentionDays    int32 = 14

	DefaultBackupSchedule  = "0 2 * * *"
	DefaultRestoreSchedule = "30 2 * * *"
)
