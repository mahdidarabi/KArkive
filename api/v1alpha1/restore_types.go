package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestoreImages overrides container images for restore pipeline stages.
type RestoreImages struct {
	BusyBox  *ImageSpec `json:"busybox,omitempty"`
	GnuPG    *ImageSpec `json:"gnupg,omitempty"`
	Postgres *ImageSpec `json:"postgres,omitempty"`
	Mc       *ImageSpec `json:"mc,omitempty"`
	MariaDB  *ImageSpec `json:"mariadb,omitempty"`
	Redis    *ImageSpec `json:"redis,omitempty"`
}

// RestoreResources overrides CPU/memory/ephemeral-storage per pipeline stage.
type RestoreResources struct {
	Cleanup *corev1.ResourceRequirements `json:"cleanup,omitempty"`
	Fetch   *corev1.ResourceRequirements `json:"fetch,omitempty"`
	Decrypt *corev1.ResourceRequirements `json:"decrypt,omitempty"`
	Extract *corev1.ResourceRequirements `json:"extract,omitempty"`
	Restore *corev1.ResourceRequirements `json:"restore,omitempty"`
}

// RestoreSpec defines the desired state of Restore.
type RestoreSpec struct {
	// Engine of the restore target.
	// +kubebuilder:default=postgres
	Engine Engine `json:"engine,omitempty"`

	// Schedule is a standard Cron expression (required).
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// Suspend stops scheduled runs. The CronJob remains for manual Jobs.
	Suspend *bool `json:"suspend,omitempty"`

	// Database is the restore target (sandbox cluster).
	Database DatabaseSpec `json:"database"`

	// S3 source of encrypted dumps.
	S3 S3Spec `json:"s3"`

	// SecretRef holds s3_access_key, s3_secret_key, gpg_passphrase.
	// Target DB credentials come from postgresSecret / mariadbSecret / redisSecret.
	SecretRef corev1.LocalObjectReference `json:"secretRef"`

	// PostgresSecret is the sandbox Cluster Secret (PGUSER/PGPASSWORD).
	PostgresSecret *SecretKeySelector `json:"postgresSecret,omitempty"`

	// MariaDBSecret is the target MariaDB Secret (MYSQL_USER / MYSQL_PASSWORD).
	MariaDBSecret *SecretKeySelector `json:"mariadbSecret,omitempty"`

	// RedisSecret is the target Redis Secret (username optional ACL, password).
	RedisSecret *SecretKeySelector `json:"redisSecret,omitempty"`

	// Persistence for the restore workdir. Set enabled=false for lite (emptyDir) restores.
	Persistence *PersistenceSpec `json:"persistence,omitempty"`

	// Workdir is the volume mount path. Default /workdir.
	Workdir string `json:"workdir,omitempty"`

	// McConfigDir for the minio client. Default /tmp/mc-config.
	McConfigDir string `json:"mcConfigDir,omitempty"`

	// BackupFile is a specific S3 object name. Empty + UseLatestBackupAsFallback
	// downloads the newest dump matching the engine prefix.
	BackupFile string `json:"backupFile,omitempty"`

	// UseLatestBackupAsFallback picks the newest object when BackupFile is empty
	// or missing. Default true.
	UseLatestBackupAsFallback *bool `json:"useLatestBackupAsFallback,omitempty"`

	// DropDatabaseIfExists drops a non-empty target before restore. Default true.
	DropDatabaseIfExists *bool `json:"dropDatabaseIfExists,omitempty"`

	// StripPgAuditExtension removes pgAudit DDL from dumps. Default true.
	StripPgAuditExtension *bool `json:"stripPgAuditExtension,omitempty"`

	// Images overrides operator-wide default images.
	Images *RestoreImages `json:"images,omitempty"`

	// Resources overrides per-stage resource requests/limits.
	Resources *RestoreResources `json:"resources,omitempty"`

	// Job tunes CronJob/Job behaviour.
	Job *JobPolicy `json:"job,omitempty"`

	// Component is the app.kubernetes.io/component label. Defaults to metadata.name.
	Component string `json:"component,omitempty"`
}

const (
	RestorePhasePending        = "Pending"
	RestorePhaseReady          = "Ready"
	RestorePhaseError          = "Error"
	RestorePhaseUnsupported    = "Unsupported"
	RestorePhaseNotImplemented = "NotImplemented"
)

// RestoreStatus defines the observed state of Restore.
type RestoreStatus struct {
	Phase              string       `json:"phase,omitempty"`
	ObservedGeneration int64        `json:"observedGeneration,omitempty"`
	CronJobName        string       `json:"cronJobName,omitempty"`
	LastScheduleTime   *metav1.Time `json:"lastScheduleTime,omitempty"`
	LastSuccessfulTime *metav1.Time `json:"lastSuccessfulTime,omitempty"`
	// LastJob is the most recently finished Job, including failure reason.
	LastJob *LastJobStatus `json:"lastJob,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=krestore;res
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=".spec.engine"
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=".spec.schedule"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Last Job",type=string,JSONPath=".status.lastJob.outcome"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Restore describes a scheduled logical restore pipeline (S3 → decrypt → extract → restore).
type Restore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreSpec   `json:"spec,omitempty"`
	Status RestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RestoreList contains a list of Restore.
type RestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Restore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Restore{}, &RestoreList{})
}
