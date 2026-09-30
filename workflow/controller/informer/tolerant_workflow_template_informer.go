package informer

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/argoproj/argo-workflows/v4/pkg/apis/workflow"
	extwfv1 "github.com/argoproj/argo-workflows/v4/pkg/client/informers/externalversions/workflow/v1alpha1"
	"github.com/argoproj/argo-workflows/v4/pkg/client/listers/workflow/v1alpha1"
	informerutil "github.com/argoproj/argo-workflows/v4/util/informer"
)

type tolerantWorkflowTemplateInformer struct {
	informer cache.SharedIndexInformer
}

// NewTolerantWorkflowTemplateInformer is a drop-in replacement for `extwfv1.WorkflowTemplateInformer` that ignores malformed resources.
// namespaces is the static set of namespaces to watch; an empty slice (or a slice containing
// "") watches every namespace.
func NewTolerantWorkflowTemplateInformer(dynamicInterface dynamic.Interface, defaultResync time.Duration, namespaces []string) extwfv1.WorkflowTemplateInformer {
	resource := schema.GroupVersionResource{Group: workflow.Group, Version: workflow.Version, Resource: workflow.WorkflowTemplatePlural}
	informer := informerutil.NewMultiNamespaceIndexInformer(namespaces, func(ns string) cache.SharedIndexInformer {
		delegate := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicInterface, defaultResync, ns, func(options *metav1.ListOptions) {
			// `ResourceVersion=0` does not honor the `limit` in API calls, which results in making significant List calls
			// without `limit`. For details, see https://github.com/argoproj/argo-workflows/pull/11343
			// Check if ResourceVersion is "0" and reset it to empty string to ensure proper pagination behavior
			if options.ResourceVersion == "0" {
				options.ResourceVersion = ""
			}
		}).ForResource(resource)
		//nolint:errcheck // the error only happens if the informer was already started, and it hasn't been
		delegate.Informer().SetTransform(informerutil.StripManagedFields)
		return delegate.Informer()
	})
	return &tolerantWorkflowTemplateInformer{informer: informer}
}

func (t *tolerantWorkflowTemplateInformer) Informer() cache.SharedIndexInformer {
	return t.informer
}

func (t *tolerantWorkflowTemplateInformer) Lister() v1alpha1.WorkflowTemplateLister {
	return &tolerantWorkflowTemplateLister{delegate: cache.NewGenericLister(t.informer.GetIndexer(), schema.GroupResource{Group: workflow.Group, Resource: workflow.WorkflowTemplatePlural})}
}
