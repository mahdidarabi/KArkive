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

// MutateRestoreCronJob writes the postgres restore pipeline CronJob.
// Stage order: cleanup → fetch → decrypt → extract → pgrestore.
func MutateRestoreCronJob(cj *batchv1.CronJob, restore *karkivev1alpha1.Restore, cfg config.Config) {
	engine := EffectiveEngine(restore.Spec.Engine)
	job := restore.Spec.Job
	if job == nil {
		job = &karkivev1alpha1.JobPolicy{}
	}

	pgImage, pgPull := restorePostgresImage(restore, cfg)
	busyImg, busyPull := restoreBusyBoxImage(restore, cfg)
	gpgImg, gpgPull := restoreGnuPGImage(restore, cfg)
	mcImg, mcPull := restoreMcImage(restore, cfg)
	cleanupRes, fetchRes, decryptRes, extractRes, restoreRes := restoreStageResources(restore)
	secret := restoreSecretName(restore)
	pgSecret, pgUserKey, pgPassKey := RestorePostgresSecret(restore)
	owned := RestoreOwnedName(restore)
	cmName := owned
	labels := RestoreLabels(restore)
	workdir := restoreWorkdir(restore)

	concurrency := batchv1.ForbidConcurrent
	if job.ConcurrencyPolicy != "" {
		concurrency = batchv1.ConcurrencyPolicy(job.ConcurrencyPolicy)
	}
	restart := corev1.RestartPolicyNever
	if job.RestartPolicy != "" {
		restart = job.RestartPolicy
	}

	podSpec := corev1.PodSpec{
		RestartPolicy:   restart,
		SecurityContext: PodSecurityContext(),
		Containers: []corev1.Container{
			newScriptContainer(scriptOpts{
				Name: "cleanup", Image: busyImg, Pull: busyPull,
				Script:    pipeline.MustRestoreScript("cleanup.sh"),
				ConfigMap: cmName, Resources: cleanupRes,
				Security: ToolsSecurityContext(), TmpSubPath: "cleanup-tmp",
				Volume: volumeWorkdir, MountPath: workdir,
			}),
			restoreFetchContainer(mcImg, mcPull, fetchRes, cmName, secret, workdir),
			restoreDecryptContainer(gpgImg, gpgPull, decryptRes, cmName, workdir),
			newScriptContainer(scriptOpts{
				Name: "extract", Image: busyImg, Pull: busyPull,
				Script:    pipeline.MustRestoreScript("extract.sh"),
				ConfigMap: cmName, Resources: extractRes,
				Security: ToolsSecurityContext(), TmpSubPath: "extract-tmp",
				Volume: volumeWorkdir, MountPath: workdir,
			}),
			pgrestoreContainer(engine, pgImage, pgPull, restoreRes, cmName, pgSecret, pgUserKey, pgPassKey, workdir),
		},
		Volumes: restoreVolumes(restore, secret),
	}

	cj.Labels = labels
	cj.Spec = batchv1.CronJobSpec{
		Schedule:                   restore.Spec.Schedule,
		ConcurrencyPolicy:          concurrency,
		FailedJobsHistoryLimit:     ptr.To(ptr.Deref(job.FailedJobsHistoryLimit, int32(3))),
		SuccessfulJobsHistoryLimit: ptr.To(ptr.Deref(job.SuccessfulJobsHistoryLimit, int32(3))),
		StartingDeadlineSeconds:    ptr.To(ptr.Deref(job.StartingDeadlineSeconds, int64(86400))),
		Suspend:                    restore.Spec.Suspend,
		JobTemplate: batchv1.JobTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: batchv1.JobSpec{
				BackoffLimit:            ptr.To(ptr.Deref(job.BackoffLimit, int32(1))),
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

func restoreFetchContainer(image string, pull corev1.PullPolicy, res corev1.ResourceRequirements, cmName, secret, workdir string) corev1.Container {
	c := newScriptContainer(scriptOpts{
		Name: "fetch", Image: image, Pull: pull,
		Script:    pipeline.MustRestoreScript("fetch.sh"),
		ConfigMap: cmName, Resources: res,
		Security: McSecurityContext(), TmpSubPath: "mc-tmp",
		Volume: volumeWorkdir, MountPath: workdir,
	})
	c.Env = append(c.Env,
		secretEnv("S3_ACCESS_KEY", secret, "s3_access_key"),
		secretEnv("S3_SECRET_KEY", secret, "s3_secret_key"),
	)
	return c
}

func restoreDecryptContainer(image string, pull corev1.PullPolicy, res corev1.ResourceRequirements, cmName, workdir string) corev1.Container {
	c := newScriptContainer(scriptOpts{
		Name: "decrypt", Image: image, Pull: pull,
		Script:    pipeline.MustRestoreScript("decrypt.sh"),
		ConfigMap: cmName, Resources: res,
		Security: ToolsSecurityContext(), TmpSubPath: "gpg-tmp",
		Volume: volumeWorkdir, MountPath: workdir,
	})
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name:      volumeGPG,
		MountPath: mountGPG,
		ReadOnly:  true,
	})
	return c
}

func pgrestoreContainer(
	engine karkivev1alpha1.Engine,
	pgImage string,
	pgPull corev1.PullPolicy,
	res corev1.ResourceRequirements,
	cmName, secret, userKey, passKey, workdir string,
) corev1.Container {
	_ = engine
	c := newScriptContainer(scriptOpts{
		Name: "pgrestore", Image: pgImage, Pull: pgPull,
		Script:    pipeline.MustRestoreScript("pgrestore.sh"),
		ConfigMap: cmName, Resources: res,
		Security: PostgresSecurityContext(), TmpSubPath: "pgrestore-tmp",
		Volume: volumeWorkdir, MountPath: workdir,
	})
	c.Env = append(c.Env,
		secretEnv("PGUSER", secret, userKey),
		secretEnv("PGPASSWORD", secret, passKey),
	)
	return c
}

func restoreVolumes(restore *karkivev1alpha1.Restore, secret string) []corev1.Volume {
	var workdir corev1.VolumeSource
	if persistenceEnabled(restore.Spec.Persistence) {
		workdir = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: RestoreOwnedName(restore),
			},
		}
	} else {
		size := persistenceSize(restore.Spec.Persistence)
		workdir = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &size},
		}
	}

	return []corev1.Volume{
		{Name: volumeWorkdir, VolumeSource: workdir},
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
