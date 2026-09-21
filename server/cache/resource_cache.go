package cache

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	clientcache "k8s.io/client-go/tools/cache"

	informerutil "github.com/argoproj/argo-workflows/v4/util/informer"
)

type ResourceCache struct {
	cache  Interface
	client kubernetes.Interface
	v1.ServiceAccountLister
	informerFactory informers.SharedInformerFactory
	informer        clientcache.SharedIndexInformer
}

func NewResourceCacheWithTimeout(client kubernetes.Interface, namespace string, timeout time.Duration) *ResourceCache {
	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, time.Minute*20, informers.WithNamespace(namespace))
	cache := &ResourceCache{
		cache:                NewLRUTtlCache(timeout, 2000),
		client:               client,
		ServiceAccountLister: informerFactory.Core().V1().ServiceAccounts().Lister(),
		informerFactory:      informerFactory,
	}
	return cache
}

func NewResourceCache(client kubernetes.Interface, namespace string) *ResourceCache {
	return NewResourceCacheWithTimeout(client, namespace, time.Minute*1)
}

func NewResourceCacheForNamespaces(client kubernetes.Interface, namespace string, namespaces []string) *ResourceCache {
	if len(namespaces) == 0 {
		return NewResourceCache(client, namespace)
	}
	informer := informerutil.NewMultiNamespaceInformer(namespaces, func(namespace string) clientcache.SharedIndexInformer {
		return coreinformers.NewFilteredServiceAccountInformer(client, namespace, time.Minute*20, clientcache.Indexers{clientcache.NamespaceIndex: clientcache.MetaNamespaceIndexFunc}, nil)
	})
	return &ResourceCache{
		cache:                NewLRUTtlCache(time.Minute*1, 2000),
		client:               client,
		ServiceAccountLister: v1.NewServiceAccountLister(informer.GetIndexer()),
		informer:             informer,
	}
}

func (c *ResourceCache) Run(stopCh <-chan struct{}) {
	if c.informer != nil {
		go c.informer.Run(stopCh)
		clientcache.WaitForCacheSync(stopCh, c.informer.HasSynced)
		return
	}
	c.informerFactory.Start(stopCh)
	c.informerFactory.WaitForCacheSync(stopCh)
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
