package cache

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	informerutil "github.com/argoproj/argo-workflows/v4/util/informer"
)

type ResourceCache struct {
	cache  Interface
	client kubernetes.Interface
	v1.ServiceAccountLister
	informerFactories []informers.SharedInformerFactory
}

// NewResourceCacheWithTimeout builds a ResourceCache scoped to namespaces (nil/empty means
// every namespace).
func NewResourceCacheWithTimeout(client kubernetes.Interface, namespaces []string, timeout time.Duration) *ResourceCache {
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	factories := make([]informers.SharedInformerFactory, len(namespaces))
	informer := informerutil.NewMultiNamespaceIndexInformer(namespaces, func(ns string) cache.SharedIndexInformer {
		factory := informers.NewSharedInformerFactoryWithOptions(client, time.Minute*20, informers.WithNamespace(ns))
		for i, n := range namespaces {
			if n == ns {
				factories[i] = factory
			}
		}
		return factory.Core().V1().ServiceAccounts().Informer()
	})
	return &ResourceCache{
		cache:                NewLRUTtlCache(timeout, 2000),
		client:               client,
		ServiceAccountLister: v1.NewServiceAccountLister(informer.GetIndexer()),
		informerFactories:    factories,
	}
}

func NewResourceCache(client kubernetes.Interface, namespaces []string) *ResourceCache {
	return NewResourceCacheWithTimeout(client, namespaces, time.Minute*1)
}

func (c *ResourceCache) Run(stopCh <-chan struct{}) {
	for _, factory := range c.informerFactories {
		factory.Start(stopCh)
		factory.WaitForCacheSync(stopCh)
	}
}

func (c *ResourceCache) GetSecret(ctx context.Context, namespace string, secretName string) (*corev1.Secret, error) {
	cacheKey := c.getSecretCacheKey(namespace, secretName)
	if secret, ok := c.cache.Get(cacheKey); ok {
		if secret, ok := secret.(*corev1.Secret); ok {
			return secret, nil
		}
	}

	secret, err := c.getSecretFromServer(ctx, namespace, secretName)
	if err != nil {
		return nil, err
	}

	c.cache.Add(cacheKey, secret)
	return secret, nil
}

func (c *ResourceCache) getSecretFromServer(ctx context.Context, namespace string, secretName string) (*corev1.Secret, error) {
	return c.client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
}

func (c *ResourceCache) getSecretCacheKey(namespace string, secretName string) string {
	return namespace + ":secret:" + secretName
}
