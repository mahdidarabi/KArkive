package resources

import (
	corev1 "k8s.io/api/core/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/ptr"
)

// PostgresUID is the CNPG / postgres image user (also the shared PVC fsGroup).
const PostgresUID int64 = 26

// MariaDBUID is the official mariadb image user.
const MariaDBUID int64 = 999

// RedisUID is the official redis image user.
const RedisUID int64 = 999

// McUID is the minio/mc image user.
const McUID int64 = 1000

// ToolsUID is used for busybox / gnupg stages (numeric; images have no dedicated user).
const ToolsUID int64 = 65532

func PodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		FSGroup:             ptr.To(PostgresUID),
		FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch),
	}
}

func PostgresSecurityContext() *corev1.SecurityContext {
	return unixUserSecurityContext(PostgresUID)
}

func MariaDBSecurityContext() *corev1.SecurityContext {
	return unixUserSecurityContext(MariaDBUID)
}

func RedisSecurityContext() *corev1.SecurityContext {
	return unixUserSecurityContext(RedisUID)
}

func McSecurityContext() *corev1.SecurityContext {
	return unixUserSecurityContext(McUID)
}

func ToolsSecurityContext() *corev1.SecurityContext {
	return unixUserSecurityContext(ToolsUID)
}

func unixUserSecurityContext(uid int64) *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		Privileged:             ptr.To(false),
		ReadOnlyRootFilesystem: ptr.To(true),
		RunAsGroup:             ptr.To(uid),
		RunAsNonRoot:           ptr.To(true),
		RunAsUser:              ptr.To(uid),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func DumpSecurityContext(engine karkivev1alpha1.Engine) *corev1.SecurityContext {
	switch EffectiveEngine(engine) {
	case karkivev1alpha1.EngineMariaDB:
		return MariaDBSecurityContext()
	case karkivev1alpha1.EngineRedis:
		return RedisSecurityContext()
	default:
		return PostgresSecurityContext()
	}
}
