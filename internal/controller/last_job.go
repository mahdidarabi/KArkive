package controller

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/jobstatus"
	"github.com/mahdidarabi/Karkive/internal/resources"
)

func mapJobToOwner(kind, nameLabel string) handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		if obj.GetLabels()[resources.LabelKind] != kind {
			return nil
		}
		name := obj.GetLabels()[nameLabel]
		if name == "" {
			return nil
		}
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: name, Namespace: obj.GetNamespace()},
		}}
	}
}

func lastJobStatus(ctx context.Context, c client.Client, namespace, nameLabel, name, kind string) *karkivev1alpha1.LastJobStatus {
	jobs := &batchv1.JobList{}
	if err := c.List(ctx, jobs, client.InNamespace(namespace), client.MatchingLabels{
		resources.LabelAppManagedBy: resources.ManagedBy,
		resources.LabelKind:         kind,
		nameLabel:                   name,
	}); err != nil {
		return nil
	}
	return jobstatus.Summarize(jobstatus.LastFinished(jobs.Items, namespace, nameLabel, name, kind))
}
