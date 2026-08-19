package resources

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/config"
	"github.com/mahdidarabi/Karkive/internal/pipeline"
	"github.com/mahdidarabi/Karkive/internal/ptr"
)

const (
	volumeDataDir = "datadir"
	volumeWorkdir = "workdir"
	volumeTmp     = "empty-dir"
	volumeGPG     = "gpg-credentials"

	mountDataDir = "/backup/data"
	mountTmp     = "/tmp"
	mountGPG     = "/run/secrets/gpg"
)

// MutateBackupCronJob writes the postgres backup pipeline CronJob.
// Stage order: cleanup → pgdump → compress → encrypt → s3-sync.
func MutateBackupCronJob(cj *batchv1.CronJob, backup *karkivev1alpha1.Backup, cfg config.Config) {
	engine := EffectiveEngine(backup.Spec.Engine)
	job := backup.Spec.Job
	if job == nil {
		job = &karkivev1alpha1.JobPolicy{}
	}

	pgImage, pgPull := postgresImage(backup, cfg)
	busyImg, busyPull := busyBoxImage(backup, cfg)
	gpgImg, gpgPull := gnuPGImage(backup, cfg)
	mcImg, mcPull := mcImage(backup, cfg)
	cleanupRes, dumpRes, compressRes, encryptRes, s3Res := backupStageResources(backup)
	secret := secretName(backup)
	owned := BackupOwnedName(backup)
	cmName := owned
	labels := BackupLabels(backup)

	concurrency := batchv1.ForbidConcurrent
	if job.ConcurrencyPolicy != "" {
		concurrency = batchv1.ConcurrencyPolicy(job.ConcurrencyPolicy)
	}
	restart := corev1.RestartPolicyOnFailure
	if job.RestartPolicy != "" {
		restart = job.RestartPolicy
	}

	podSpec := corev1.PodSpec{
		RestartPolicy:   restart,
		SecurityContext: PodSecurityContext(),
		Containers: []corev1.Container{
			newScriptContainer(scriptOpts{
				Name: "cleanup", Image: busyImg, Pull: busyPull,
				Script:    pipeline.MustBackupScript("cleanup.sh"),
				ConfigMap: cmName, Resources: cleanupRes,
				Security: ToolsSecurityContext(), TmpSubPath: "cleanup-tmp",
			}),
			dumpContainer(engine, pgImage, pgPull, dumpRes, cmName, secret),
			newScriptContainer(scriptOpts{
				Name: "compress", Image: busyImg, Pull: busyPull,
				Script:    pipeline.MustBackupScript("compress.sh"),
				ConfigMap: cmName, Resources: compressRes,
				Security: ToolsSecurityContext(), TmpSubPath: "compress-tmp-dir",
			}),
			encryptContainer(gpgImg, gpgPull, encryptRes, cmName),
			s3SyncContainer(mcImg, mcPull, s3Res, cmName, secret),
		},
		Volumes: backupVolumes(backup, secret),
	}

	cj.Labels = labels
	cj.Spec = batchv1.CronJobSpec{
		Schedule:                   backup.Spec.Schedule,
		ConcurrencyPolicy:          concurrency,
		FailedJobsHistoryLimit:     ptr.To(ptr.Deref(job.FailedJobsHistoryLimit, int32(3))),
		SuccessfulJobsHistoryLimit: ptr.To(ptr.Deref(job.SuccessfulJobsHistoryLimit, int32(3))),
		StartingDeadlineSeconds:    ptr.To(ptr.Deref(job.StartingDeadlineSeconds, int64(86400))),
		Suspend:                    backup.Spec.Suspend,
		JobTemplate: batchv1.JobTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: batchv1.JobSpec{
				BackoffLimit:            ptr.To(ptr.Deref(job.BackoffLimit, int32(3))),
				TTLSecondsAfterFinished: ptr.To(ptr.Deref(job.TTLSecondsAfterFinished, int32(86400))),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec:       podSpec,
				},
			},
		},
	}
	if job.TimeZone != "" {
		cj.Spec.TimeZone = ptr.To(job.TimeZone)
	}
}

func dumpContainer(
	engine karkivev1alpha1.Engine,
	pgImage string,
	pgPull corev1.PullPolicy,
	res corev1.ResourceRequirements,
	cmName, secret string,
) corev1.Container {
	// Phase 1: postgres only. MariaDB/Redis dump containers land here later.
	_ = engine
	c := newScriptContainer(scriptOpts{
		Name: "pgdump", Image: pgImage, Pull: pgPull,
		Script:    pipeline.MustBackupScript("pgdump.sh"),
		ConfigMap: cmName, Resources: res,
		Security: PostgresSecurityContext(), TmpSubPath: "tmp-dir",
	})
	c.Env = append(c.Env,
		secretEnv("PGUSER", secret, "username"),
		secretEnv("PGPASSWORD", secret, "password"),
	)
	return c
}

func encryptContainer(image string, pull corev1.PullPolicy, res corev1.ResourceRequirements, cmName string) corev1.Container {
	c := newScriptContainer(scriptOpts{
		Name: "encrypt", Image: image, Pull: pull,
		Script:    pipeline.MustBackupScript("encrypt.sh"),
		ConfigMap: cmName, Resources: res,
		Security: ToolsSecurityContext(), TmpSubPath: "encrypt-tmp-dir",
	})
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name:      volumeGPG,
		MountPath: mountGPG,
		ReadOnly:  true,
	})
	return c
}

func s3SyncContainer(image string, pull corev1.PullPolicy, res corev1.ResourceRequirements, cmName, secret string) corev1.Container {
	c := newScriptContainer(scriptOpts{
		Name: "s3-sync", Image: image, Pull: pull,
		Script:    pipeline.MustBackupScript("s3-sync.sh"),
		ConfigMap: cmName, Resources: res,
		Security: McSecurityContext(), TmpSubPath: "mc-tmp-dir",
	})
	c.Env = append(c.Env,
		secretEnv("S3_ACCESS_KEY", secret, "s3_access_key"),
		secretEnv("S3_SECRET_KEY", secret, "s3_secret_key"),
	)
	return c
}

type scriptOpts struct {
	Name       string
	Image      string
	Pull       corev1.PullPolicy
	Script     string
	ConfigMap  string
	Resources  corev1.ResourceRequirements
	Security   *corev1.SecurityContext
	TmpSubPath string
	Volume     string
	MountPath  string
}

func newScriptContainer(opts scriptOpts) corev1.Container {
	vol := opts.Volume
	if vol == "" {
		vol = volumeDataDir
	}
	mount := opts.MountPath
	if mount == "" {
		mount = mountDataDir
	}
	return corev1.Container{
		Name:            opts.Name,
		Image:           opts.Image,
		ImagePullPolicy: opts.Pull,
		Command:         []string{"/bin/sh", "-c", opts.Script},
		EnvFrom: []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: opts.ConfigMap},
			},
		}},
		Resources:       opts.Resources,
		SecurityContext: opts.Security,
		VolumeMounts: []corev1.VolumeMount{
			{Name: vol, MountPath: mount},
			{Name: volumeTmp, MountPath: mountTmp, SubPath: opts.TmpSubPath},
		},
	}
}

func secretEnv(envName, secret, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secret},
				Key:                  key,
			},
		},
	}
}

func backupVolumes(backup *karkivev1alpha1.Backup, secret string) []corev1.Volume {
	var datadir corev1.VolumeSource
	if persistenceEnabled(backup.Spec.Persistence) {
		datadir = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: BackupOwnedName(backup),
			},
		}
	} else {
		size := persistenceSize(backup.Spec.Persistence)
		datadir = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &size},
		}
	}

	return []corev1.Volume{
		{Name: volumeDataDir, VolumeSource: datadir},
		{Name: volumeTmp, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{
			Name: volumeGPG,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secret,
					Items: []corev1.KeyToPath{{
						Key:  "gpg_passphrase",
						Path: "gpg_passphrase",
					}},
				},
			},
		},
	}
}
