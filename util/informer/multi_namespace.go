package informer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
)

type multiNamespaceInformer struct {
	informers []cache.SharedIndexInformer
	indexer   cache.Indexer
}

type multiNamespaceIndexer struct {
	byNamespace map[string]cache.Indexer
	indexers    []cache.Indexer
}

type multiNamespaceRegistration struct {
	registrations []cache.ResourceEventHandlerRegistration
}

func NewMultiNamespaceInformer(namespaces []string, newInformer func(namespace string) cache.SharedIndexInformer) cache.SharedIndexInformer {
	informers := make([]cache.SharedIndexInformer, 0, len(namespaces))
	indexers := make(map[string]cache.Indexer, len(namespaces))
	orderedIndexers := make([]cache.Indexer, 0, len(namespaces))
	for _, namespace := range namespaces {
		informer := newInformer(namespace)
		informers = append(informers, informer)
		indexers[namespace] = informer.GetIndexer()
		orderedIndexers = append(orderedIndexers, informer.GetIndexer())
	}
	return &multiNamespaceInformer{
		informers: informers,
		indexer:   &multiNamespaceIndexer{byNamespace: indexers, indexers: orderedIndexers},
	}
}

func (m *multiNamespaceInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	registrations := make([]cache.ResourceEventHandlerRegistration, 0, len(m.informers))
	for _, informer := range m.informers {
		registration, err := informer.AddEventHandler(handler)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, registration)
	}
	return &multiNamespaceRegistration{registrations: registrations}, nil
}

func (m *multiNamespaceInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return m.AddEventHandlerWithOptions(handler, cache.HandlerOptions{ResyncPeriod: &resyncPeriod})
}

func (m *multiNamespaceInformer) AddEventHandlerWithOptions(handler cache.ResourceEventHandler, options cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	registrations := make([]cache.ResourceEventHandlerRegistration, 0, len(m.informers))
	for _, informer := range m.informers {
		registration, err := informer.AddEventHandlerWithOptions(handler, options)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, registration)
	}
	return &multiNamespaceRegistration{registrations: registrations}, nil
}

func (m *multiNamespaceInformer) RemoveEventHandler(handle cache.ResourceEventHandlerRegistration) error {
	registration, ok := handle.(*multiNamespaceRegistration)
	if !ok || len(registration.registrations) != len(m.informers) {
		return fmt.Errorf("invalid multi-namespace event handler registration")
	}
	for i, informer := range m.informers {
		if err := informer.RemoveEventHandler(registration.registrations[i]); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) GetStore() cache.Store {
	return m.indexer
}

func (m *multiNamespaceInformer) GetController() cache.Controller {
	return nil
}

func (m *multiNamespaceInformer) Run(stopCh <-chan struct{}) {
	var waitGroup sync.WaitGroup
	for _, informer := range m.informers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			informer.Run(stopCh)
		}()
	}
	waitGroup.Wait()
}

func (m *multiNamespaceInformer) RunWithContext(ctx context.Context) {
	m.Run(ctx.Done())
}

func (m *multiNamespaceInformer) HasSynced() bool {
	for _, informer := range m.informers {
		if !informer.HasSynced() {
			return false
		}
	}
	return true
}

func (m *multiNamespaceInformer) LastSyncResourceVersion() string {
	return ""
}

func (m *multiNamespaceInformer) SetWatchErrorHandler(handler cache.WatchErrorHandler) error {
	for _, informer := range m.informers {
		if err := informer.SetWatchErrorHandler(handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	for _, informer := range m.informers {
		if err := informer.SetWatchErrorHandlerWithContext(handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) SetTransform(handler cache.TransformFunc) error {
	for _, informer := range m.informers {
		if err := informer.SetTransform(handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) IsStopped() bool {
	for _, informer := range m.informers {
		if !informer.IsStopped() {
			return false
		}
	}
	return true
}

func (m *multiNamespaceInformer) AddIndexers(indexers cache.Indexers) error {
	return m.indexer.AddIndexers(indexers)
}

func (m *multiNamespaceInformer) GetIndexer() cache.Indexer {
	return m.indexer
}

func (m *multiNamespaceRegistration) HasSynced() bool {
	for _, registration := range m.registrations {
		if !registration.HasSynced() {
			return false
		}
	}
	return true
}

func (m *multiNamespaceIndexer) indexerForObject(obj any) (cache.Indexer, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	indexer, ok := m.byNamespace[accessor.GetNamespace()]
	if !ok {
		return nil, fmt.Errorf("namespace %q is not managed", accessor.GetNamespace())
	}
	return indexer, nil
}

func (m *multiNamespaceIndexer) Add(obj any) error {
	indexer, err := m.indexerForObject(obj)
	if err != nil {
		return err
	}
	return indexer.Add(obj)
}

func (m *multiNamespaceIndexer) Update(obj any) error {
	indexer, err := m.indexerForObject(obj)
	if err != nil {
		return err
	}
	return indexer.Update(obj)
}

func (m *multiNamespaceIndexer) Delete(obj any) error {
	indexer, err := m.indexerForObject(obj)
	if err != nil {
		return err
	}
	return indexer.Delete(obj)
}

func (m *multiNamespaceIndexer) List() []any {
	var result []any
	for _, indexer := range m.indexers {
		result = append(result, indexer.List()...)
	}
	return result
}

func (m *multiNamespaceIndexer) ListKeys() []string {
	var result []string
	for _, indexer := range m.indexers {
		result = append(result, indexer.ListKeys()...)
	}
	return result
}

func (m *multiNamespaceIndexer) Get(obj any) (item any, exists bool, err error) {
	indexer, err := m.indexerForObject(obj)
	if err != nil {
		return nil, false, err
	}
	return indexer.Get(obj)
}

func (m *multiNamespaceIndexer) GetByKey(key string) (item any, exists bool, err error) {
	namespace, _, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, false, err
	}
	indexer, ok := m.byNamespace[namespace]
	if !ok {
		return nil, false, nil
	}
	return indexer.GetByKey(key)
}

func (m *multiNamespaceIndexer) Replace(list []any, resourceVersion string) error {
	byNamespace := make(map[string][]any, len(m.byNamespace))
	for _, obj := range list {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return err
		}
		byNamespace[accessor.GetNamespace()] = append(byNamespace[accessor.GetNamespace()], obj)
	}
	for namespace, indexer := range m.byNamespace {
		if err := indexer.Replace(byNamespace[namespace], resourceVersion); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceIndexer) Resync() error {
	for _, indexer := range m.indexers {
		if err := indexer.Resync(); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceIndexer) Index(indexName string, obj any) ([]any, error) {
	indexer, err := m.indexerForObject(obj)
	if err != nil {
		return nil, err
	}
	return indexer.Index(indexName, obj)
}

func (m *multiNamespaceIndexer) IndexKeys(indexName, indexedValue string) ([]string, error) {
	var result []string
	for _, indexer := range m.indexers {
		keys, err := indexer.IndexKeys(indexName, indexedValue)
		if err != nil {
			return nil, err
		}
		result = append(result, keys...)
	}
	return result, nil
}

func (m *multiNamespaceIndexer) ListIndexFuncValues(indexName string) []string {
	values := map[string]struct{}{}
	for _, indexer := range m.indexers {
		for _, value := range indexer.ListIndexFuncValues(indexName) {
			values[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

func (m *multiNamespaceIndexer) ByIndex(indexName, indexedValue string) ([]any, error) {
	var result []any
	for _, indexer := range m.indexers {
		items, err := indexer.ByIndex(indexName, indexedValue)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	return result, nil
}

func (m *multiNamespaceIndexer) GetIndexers() cache.Indexers {
	for _, indexer := range m.indexers {
		return indexer.GetIndexers()
	}
	return cache.Indexers{}
}

func (m *multiNamespaceIndexer) AddIndexers(indexers cache.Indexers) error {
	for _, indexer := range m.indexers {
		for name := range indexers {
			if _, exists := indexer.GetIndexers()[name]; exists {
				return fmt.Errorf("indexer conflict: %v", name)
			}
		}
	}
	for _, indexer := range m.indexers {
		if err := indexer.AddIndexers(indexers); err != nil {
			return err
		}
	}
	return nil
}
