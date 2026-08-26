package controller

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mahdidarabi/KArkive/internal/resources"
)

// deleteLegacyOwned removes the pre-0.0.4 ConfigMap and CronJob named
// karkive-<cr-name> when this CR still owns them. The PVC is left in place
// because Kubernetes cannot rename it; retained dumps must be copied by the
// operator if still needed.
func deleteLegacyOwned(ctx context.Context, c client.Client, owner client.Object, currentName string) error {
	legacy := resources.LegacyOwnedName(owner.GetName())
	if legacy == currentName {
		return nil
	}
	ns := owner.GetNamespace()
	logger := log.FromContext(ctx)

	cmDeleted, err := deleteIfController(ctx, c, owner, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: legacy, Namespace: ns},
	})
	if err != nil {
		return err
	}
	cjDeleted, err := deleteIfController(ctx, c, owner, &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: legacy, Namespace: ns},
	})
	if err != nil {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: legacy}, pvc); err == nil && metav1.IsControlledBy(pvc, owner) {
		logger.Info("legacy PVC kept (Kubernetes cannot rename it); copy retained dumps then delete", "name", legacy)
	}

	if cmDeleted || cjDeleted {
		logger.Info("deleted legacy owned ConfigMap/CronJob", "legacy", legacy, "current", currentName)
	}
	return nil
}

func deleteIfController(ctx context.Context, c client.Client, owner client.Object, obj client.Object) (bool, error) {
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if !metav1.IsControlledBy(obj, owner) {
		return false, nil
	}
	if err := c.Delete(ctx, obj); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return true, nil
}
