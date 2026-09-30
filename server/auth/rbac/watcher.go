package rbac

import (
	"context"
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/argoproj/argo-workflows/v4/util/logging"
)

// NewConfigMapInformer returns an informer that keeps enforcer's policy in sync with the
// dedicated RBAC policy ConfigMap named name in namespace, so that policy edits take effect
// without an Argo Server restart. Call Run on the returned informer to start it; load errors
// (e.g. an invalid policy) are logged and leave the previously loaded policy (if any) active.
func NewConfigMapInformer(ctx context.Context, kubeclientset kubernetes.Interface, namespace, name string, enforcer *Enforcer) cache.SharedIndexInformer {
	logger := logging.RequireLoggerFromContext(ctx).WithFields(logging.Fields{"component": "rbac_policy_watcher", "namespace": namespace, "name": name})
	informer := v1.NewFilteredConfigMapInformer(kubeclientset, namespace, 0, cache.Indexers{}, func(opts *metav1.ListOptions) {
		opts.FieldSelector = fmt.Sprintf("metadata.name=%s", name)
	})
	load := func(cm *apiv1.ConfigMap) {
		if err := enforcer.Load(cm); err != nil {
			logger.WithError(err).Error(ctx, "failed to load RBAC policy config map, keeping previous policy in effect")
			return
		}
		logger.Info(ctx, "loaded RBAC policy config map")
	}
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			load(obj.(*apiv1.ConfigMap))
		},
		UpdateFunc: func(_, newObj any) {
			load(newObj.(*apiv1.ConfigMap))
		},
		DeleteFunc: func(obj any) {
			logger.Error(ctx, "RBAC policy config map was deleted, keeping previous policy in effect")
		},
	})
	return informer
}
