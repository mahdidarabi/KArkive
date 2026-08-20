package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/jobstatus"
	"github.com/mahdidarabi/Karkive/internal/resources"
)

var (
	backupReadyDesc = prometheus.NewDesc(
		"karkive_backup_ready",
		"1 if the Backup status phase is Ready.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	backupSuspendedDesc = prometheus.NewDesc(
		"karkive_backup_suspended",
		"1 if Backup.spec.suspend is true.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	backupLastScheduleDesc = prometheus.NewDesc(
		"karkive_backup_last_schedule_timestamp_seconds",
		"Unix time of the last CronJob schedule for this Backup, if known.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	backupLastSuccessDesc = prometheus.NewDesc(
		"karkive_backup_last_successful_timestamp_seconds",
		"Unix time of the last successful backup Job, if known.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	backupLastJobFailedDesc = prometheus.NewDesc(
		"karkive_backup_last_job_failed",
		"1 if the most recently completed Job for this Backup failed.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	backupLastJobDurationDesc = prometheus.NewDesc(
		"karkive_backup_last_job_duration_seconds",
		"Wall time in seconds of the most recently finished Job for this Backup, or 0 if none has finished.",
		[]string{"namespace", "name", "engine"},
		nil,
	)

	restoreReadyDesc = prometheus.NewDesc(
		"karkive_restore_ready",
		"1 if the Restore status phase is Ready.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	restoreSuspendedDesc = prometheus.NewDesc(
		"karkive_restore_suspended",
		"1 if Restore.spec.suspend is true.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	restoreLastScheduleDesc = prometheus.NewDesc(
		"karkive_restore_last_schedule_timestamp_seconds",
		"Unix time of the last CronJob schedule for this Restore, if known.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	restoreLastSuccessDesc = prometheus.NewDesc(
		"karkive_restore_last_successful_timestamp_seconds",
		"Unix time of the last successful restore Job, if known.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	restoreLastJobFailedDesc = prometheus.NewDesc(
		"karkive_restore_last_job_failed",
		"1 if the most recently completed Job for this Restore failed.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
	restoreLastJobDurationDesc = prometheus.NewDesc(
		"karkive_restore_last_job_duration_seconds",
		"Wall time in seconds of the most recently finished Job for this Restore, or 0 if none has finished.",
		[]string{"namespace", "name", "engine"},
		nil,
	)
)

// Collector exports Backup and Restore status plus last Job outcome.
//
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
type Collector struct {
	Client client.Client
}

// Register adds the collector to the controller-runtime metrics registry.
func Register(c client.Client) {
	crmetrics.Registry.MustRegister(&Collector{Client: c})
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- backupReadyDesc
	ch <- backupSuspendedDesc
	ch <- backupLastScheduleDesc
	ch <- backupLastSuccessDesc
	ch <- backupLastJobFailedDesc
	ch <- backupLastJobDurationDesc
	ch <- restoreReadyDesc
	ch <- restoreSuspendedDesc
	ch <- restoreLastScheduleDesc
	ch <- restoreLastSuccessDesc
	ch <- restoreLastJobFailedDesc
	ch <- restoreLastJobDurationDesc
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	jobs := &batchv1.JobList{}
	_ = c.Client.List(ctx, jobs, client.MatchingLabels{resources.LabelAppManagedBy: resources.ManagedBy})

	backups := &karkivev1alpha1.BackupList{}
	if err := c.Client.List(ctx, backups); err == nil {
		for i := range backups.Items {
			c.collectBackup(ch, &backups.Items[i], jobs.Items)
		}
	}

	restores := &karkivev1alpha1.RestoreList{}
	if err := c.Client.List(ctx, restores); err == nil {
		for i := range restores.Items {
			c.collectRestore(ch, &restores.Items[i], jobs.Items)
		}
	}
}

func (c *Collector) collectBackup(ch chan<- prometheus.Metric, backup *karkivev1alpha1.Backup, jobs []batchv1.Job) {
	engine := string(resources.EffectiveEngine(backup.Spec.Engine))
	labels := []string{backup.Namespace, backup.Name, engine}

	ch <- gauge(backupReadyDesc, boolValue(backup.Status.Phase == karkivev1alpha1.BackupPhaseReady), labels)
	ch <- gauge(backupSuspendedDesc, boolValue(backup.Spec.Suspend != nil && *backup.Spec.Suspend), labels)
	if t := unixTime(backup.Status.LastScheduleTime); t > 0 {
		ch <- gauge(backupLastScheduleDesc, t, labels)
	}
	if t := unixTime(backup.Status.LastSuccessfulTime); t > 0 {
		ch <- gauge(backupLastSuccessDesc, t, labels)
	}

	job := jobstatus.LastFinished(jobs, backup.Namespace, resources.LabelBackupName, backup.Name, resources.KindBackup)
	emitLastJob(ch, job, backupLastJobFailedDesc, backupLastJobDurationDesc, labels)
}

func (c *Collector) collectRestore(ch chan<- prometheus.Metric, restore *karkivev1alpha1.Restore, jobs []batchv1.Job) {
	engine := string(resources.EffectiveEngine(restore.Spec.Engine))
	labels := []string{restore.Namespace, restore.Name, engine}

	ch <- gauge(restoreReadyDesc, boolValue(restore.Status.Phase == karkivev1alpha1.RestorePhaseReady), labels)
	ch <- gauge(restoreSuspendedDesc, boolValue(restore.Spec.Suspend != nil && *restore.Spec.Suspend), labels)
	if t := unixTime(restore.Status.LastScheduleTime); t > 0 {
		ch <- gauge(restoreLastScheduleDesc, t, labels)
	}
	if t := unixTime(restore.Status.LastSuccessfulTime); t > 0 {
		ch <- gauge(restoreLastSuccessDesc, t, labels)
	}

	job := jobstatus.LastFinished(jobs, restore.Namespace, resources.LabelRestoreName, restore.Name, resources.KindRestore)
	emitLastJob(ch, job, restoreLastJobFailedDesc, restoreLastJobDurationDesc, labels)
}

func emitLastJob(ch chan<- prometheus.Metric, job *batchv1.Job, failedDesc, durationDesc *prometheus.Desc, labels []string) {
	if job == nil {
		ch <- gauge(failedDesc, 0, labels)
		ch <- gauge(durationDesc, 0, labels)
		return
	}
	ch <- gauge(failedDesc, boolValue(job.Status.Failed > 0 || jobstatus.Failed(job)), labels)
	d := 0.0
	if job.Status.StartTime != nil {
		if end := jobstatus.FinishedAt(job); end != nil {
			d = end.Sub(job.Status.StartTime.Time).Seconds()
			if d < 0 {
				d = 0
			}
		}
	}
	ch <- gauge(durationDesc, d, labels)
}

func unixTime(t *metav1.Time) float64 {
	if t == nil || t.IsZero() {
		return 0
	}
	return float64(t.Unix())
}

func gauge(desc *prometheus.Desc, v float64, labels []string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, labels...)
}

func boolValue(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
