package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Engine identifies the datastore being backed up or restored.
// MariaDB and Redis are reserved for later phases; only postgres is implemented today.
// +kubebuilder:validation:Enum=postgres;mariadb;redis
type Engine string

const (
	EnginePostgres Engine = "postgres"
	EngineMariaDB  Engine = "mariadb"
	EngineRedis    Engine = "redis"
)

// DatabaseSpec is the connection target for dump or restore.
type DatabaseSpec struct {
	// Host is the DNS name or IP of the database (required).
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port of the database. Defaults: postgres 5432, mariadb 3306, redis 6379.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Name is the database name (postgres/mariadb) or a logical label (redis).
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// OwnerRole is created/used on postgres restore (defaults to Name).
	OwnerRole string `json:"ownerRole,omitempty"`
}

// S3Spec is the object-storage destination (or source, for restore).
type S3Spec struct {
	// Endpoint is the S3 API URL, e.g. https://s3.example.com.
	// Falls back to the operator --default-s3-endpoint flag when empty.
	Endpoint string `json:"endpoint,omitempty"`

	// Bucket is the S3 bucket name.
	// Falls back to the operator --default-s3-bucket flag when empty.
	Bucket string `json:"bucket,omitempty"`

	// Path is the key prefix inside the bucket (required), e.g. app/pgdump.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// RetentionDays is how long encrypted dumps are kept in S3 (backup only). Default 14.
	// +kubebuilder:validation:Minimum=1
	RetentionDays *int32 `json:"retentionDays,omitempty"`
}

// PersistenceSpec controls scratch storage for the pipeline.
// Backup defaults to a PVC so retained/ dumps survive across Jobs.
// Lite jobs may set enabled=false to use emptyDir instead.
type PersistenceSpec struct {
	// Enabled creates and mounts a PVC when true (default true for Backup).
	Enabled *bool `json:"enabled,omitempty"`

	// StorageClassName of the PVC. Empty uses the cluster default.
	StorageClassName string `json:"storageClassName,omitempty"`

	// AccessModes of the PVC. Default [ReadWriteOnce].
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// Size of the PVC, or emptyDir.sizeLimit when Enabled=false. Default 1Gi.
	Size resource.Quantity `json:"size,omitempty"`

	// Annotations copied onto the PVC.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ImageSpec is a full image reference plus pull policy.
type ImageSpec struct {
	// Image is registry/repository:tag.
	Image string `json:"image,omitempty"`

	// PullPolicy for the container. Default IfNotPresent.
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// SecretKeySelector points at a Secret and optional key names.
type SecretKeySelector struct {
	// Name of the Secret in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// UsernameKey in the Secret. Default "username".
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey in the Secret. Default "password" (postgres) or engine-specific.
	PasswordKey string `json:"passwordKey,omitempty"`
}

// JobPolicy maps onto CronJob / Job fields.
type JobPolicy struct {
	// ConcurrencyPolicy of the CronJob. Default Forbid.
	ConcurrencyPolicy string `json:"concurrencyPolicy,omitempty"`

	// FailedJobsHistoryLimit. Default 3.
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`

	// SuccessfulJobsHistoryLimit. Default 3.
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`

	// StartingDeadlineSeconds. Default 86400.
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`

	// BackoffLimit on the Job. Default 3 for backup, 1 for restore.
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	// TTLSecondsAfterFinished. Default 86400.
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// RestartPolicy of the Pod. Default OnFailure for backup, Never for restore.
	RestartPolicy corev1.RestartPolicy `json:"restartPolicy,omitempty"`

	// TimeZone for the CronJob schedule (Kubernetes 1.27+).
	TimeZone string `json:"timeZone,omitempty"`
}
