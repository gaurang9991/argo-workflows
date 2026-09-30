package workflowtemplate

import (
	"context"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	wfextvv1alpha1 "github.com/argoproj/argo-workflows/v4/pkg/client/informers/externalversions/workflow/v1alpha1"
	"github.com/argoproj/argo-workflows/v4/server/types"
	"github.com/argoproj/argo-workflows/v4/util/logging"
	"github.com/argoproj/argo-workflows/v4/workflow/controller/informer"
	"github.com/argoproj/argo-workflows/v4/workflow/templateresolution"
)

const (
	workflowTemplateResyncPeriod = 20 * time.Minute
)

var _ types.WorkflowTemplateStore = &Informer{}

type Informer struct {
	managedNamespaces []string
	informer          wfextvv1alpha1.WorkflowTemplateInformer
}

func NewInformer(restConfig *rest.Config, managedNamespaces []string) (*Informer, error) {
	dynamicInterface, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	informer := informer.NewTolerantWorkflowTemplateInformer(
		dynamicInterface,
		workflowTemplateResyncPeriod,
		managedNamespaces)
	return &Informer{
		informer:          informer,
		managedNamespaces: managedNamespaces,
	}, nil
}

// Run starts the informer in a separate go-routine and blocks until cache sync.
func (wti *Informer) Run(ctx context.Context, stopCh <-chan struct{}) {
	go wti.informer.Informer().Run(stopCh)

	if !cache.WaitForCacheSync(
		stopCh,
		wti.informer.Informer().HasSynced,
	) {
		logging.RequireLoggerFromContext(ctx).WithFatal().Error(ctx, "Timed out waiting for caches to sync")
	}
}

// Getter returns a WorkflowTemplateNamespacedGetter. If namespace is empty, the Lister will use
// the first configured managed namespace (relevant when there's exactly one; with several
// managed namespaces callers are expected to always pass one explicitly).
func (wti *Informer) Getter(ctx context.Context, namespace string) templateresolution.WorkflowTemplateNamespacedGetter {
	if wti.informer == nil {
		logging.RequireLoggerFromContext(ctx).WithFatal().Error(ctx, "Template informer not started")
	}
	if namespace == "" && len(wti.managedNamespaces) > 0 {
		namespace = wti.managedNamespaces[0]
	}
	return templateresolution.WrapWorkflowTemplateLister(wti.informer.Lister().WorkflowTemplates(namespace))
}
