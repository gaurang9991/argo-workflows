package informer

import (
	"context"
	"errors"
	"sync"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
)

// NewMultiNamespaceIndexInformer builds one cache.SharedIndexInformer per namespace
// (via build) and, if there is more than one, composes them behind a single
// cache.SharedIndexInformer facade so that callers (indexers, listers, event
// handlers) don't need to know they're watching more than one namespace.
//
// namespaces must be disjoint. An empty slice, or a slice containing "", means
// "all namespaces" (cluster scope) and is passed straight through to build without
// any wrapping, exactly as today.
func NewMultiNamespaceIndexInformer(namespaces []string, build func(namespace string) cache.SharedIndexInformer) cache.SharedIndexInformer {
	if len(namespaces) <= 1 {
		ns := ""
		if len(namespaces) == 1 {
			ns = namespaces[0]
		}
		return build(ns)
	}
	m := &multiNamespaceInformer{
		informers:   make([]cache.SharedIndexInformer, len(namespaces)),
		byNamespace: make(map[string]cache.SharedIndexInformer, len(namespaces)),
	}
	for i, ns := range namespaces {
		informer := build(ns)
		m.informers[i] = informer
		m.byNamespace[ns] = informer
	}
	return m
}

// multiNamespaceInformer fans SharedIndexInformer operations out across one
// real informer per namespace. Each child keeps its own Reflector, so List+Watch
// resource-version continuity is handled independently and correctly per namespace;
// only read (index/store) operations and event handler registration are merged.
type multiNamespaceInformer struct {
	informers   []cache.SharedIndexInformer
	byNamespace map[string]cache.SharedIndexInformer
}

// multiRegistration is returned from AddEventHandler* and is HasSynced only once
// every per-namespace registration has synced.
type multiRegistration []cache.ResourceEventHandlerRegistration

func (r multiRegistration) HasSynced() bool {
	for _, reg := range r {
		if reg == nil || !reg.HasSynced() {
			return false
		}
	}
	return true
}

func (m *multiNamespaceInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return m.addEventHandler(func(i cache.SharedIndexInformer) (cache.ResourceEventHandlerRegistration, error) {
		return i.AddEventHandler(handler)
	})
}

func (m *multiNamespaceInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return m.addEventHandler(func(i cache.SharedIndexInformer) (cache.ResourceEventHandlerRegistration, error) {
		return i.AddEventHandlerWithResyncPeriod(handler, resyncPeriod)
	})
}

func (m *multiNamespaceInformer) AddEventHandlerWithOptions(handler cache.ResourceEventHandler, options cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	return m.addEventHandler(func(i cache.SharedIndexInformer) (cache.ResourceEventHandlerRegistration, error) {
		return i.AddEventHandlerWithOptions(handler, options)
	})
}

func (m *multiNamespaceInformer) addEventHandler(add func(cache.SharedIndexInformer) (cache.ResourceEventHandlerRegistration, error)) (cache.ResourceEventHandlerRegistration, error) {
	regs := make(multiRegistration, 0, len(m.informers))
	for _, i := range m.informers {
		reg, err := add(i)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}
	return regs, nil
}

func (m *multiNamespaceInformer) RemoveEventHandler(handle cache.ResourceEventHandlerRegistration) error {
	regs, ok := handle.(multiRegistration)
	if !ok {
		return errors.New("multinamespace: RemoveEventHandler given a registration not returned by AddEventHandler")
	}
	var errs []error
	for i, reg := range regs {
		if err := m.informers[i].RemoveEventHandler(reg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *multiNamespaceInformer) GetStore() cache.Store { return m.GetIndexer() }

func (m *multiNamespaceInformer) GetController() cache.Controller { return nil }

func (m *multiNamespaceInformer) Run(stopCh <-chan struct{}) {
	var wg sync.WaitGroup
	for _, i := range m.informers {
		wg.Go(func() { i.Run(stopCh) })
	}
	wg.Wait()
}

func (m *multiNamespaceInformer) RunWithContext(ctx context.Context) {
	var wg sync.WaitGroup
	for _, i := range m.informers {
		wg.Go(func() { i.RunWithContext(ctx) })
	}
	wg.Wait()
}

func (m *multiNamespaceInformer) HasSynced() bool {
	for _, i := range m.informers {
		if !i.HasSynced() {
			return false
		}
	}
	return true
}

func (m *multiNamespaceInformer) LastSyncResourceVersion() string {
	if len(m.informers) == 0 {
		return ""
	}
	return m.informers[0].LastSyncResourceVersion()
}

func (m *multiNamespaceInformer) SetWatchErrorHandler(handler cache.WatchErrorHandler) error {
	for _, i := range m.informers {
		if err := i.SetWatchErrorHandler(handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	for _, i := range m.informers {
		if err := i.SetWatchErrorHandlerWithContext(handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) SetTransform(handler cache.TransformFunc) error {
	for _, i := range m.informers {
		if err := i.SetTransform(handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) IsStopped() bool {
	for _, i := range m.informers {
		if !i.IsStopped() {
			return false
		}
	}
	return true
}

func (m *multiNamespaceInformer) AddIndexers(indexers cache.Indexers) error {
	for _, i := range m.informers {
		if err := i.AddIndexers(indexers); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiNamespaceInformer) GetIndexer() cache.Indexer {
	byNamespace := make(map[string]cache.Indexer, len(m.byNamespace))
	indexers := make([]cache.Indexer, len(m.informers))
	for ns, i := range m.byNamespace {
		byNamespace[ns] = i.GetIndexer()
	}
	for idx, i := range m.informers {
		indexers[idx] = i.GetIndexer()
	}
	return &multiIndexer{byNamespace: byNamespace, all: indexers}
}

// multiIndexer merges read access to one cache.Indexer per namespace. Keyed
// lookups/mutations are routed directly to the owning namespace's indexer;
// listing and index queries fan out across all of them.
type multiIndexer struct {
	byNamespace map[string]cache.Indexer
	all         []cache.Indexer
}

func (m *multiIndexer) indexerForKey(key string) cache.Indexer {
	ns, _, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil
	}
	return m.byNamespace[ns]
}

func (m *multiIndexer) indexerForObj(obj any) cache.Indexer {
	accessor, err := apimeta.Accessor(obj)
	if err != nil {
		return nil
	}
	return m.byNamespace[accessor.GetNamespace()]
}

func (m *multiIndexer) Add(obj any) error {
	if idx := m.indexerForObj(obj); idx != nil {
		return idx.Add(obj)
	}
	return nil
}

func (m *multiIndexer) Update(obj any) error {
	if idx := m.indexerForObj(obj); idx != nil {
		return idx.Update(obj)
	}
	return nil
}

func (m *multiIndexer) Delete(obj any) error {
	if idx := m.indexerForObj(obj); idx != nil {
		return idx.Delete(obj)
	}
	return nil
}

func (m *multiIndexer) List() []any {
	var out []any
	for _, idx := range m.all {
		out = append(out, idx.List()...)
	}
	return out
}

func (m *multiIndexer) ListKeys() []string {
	var out []string
	for _, idx := range m.all {
		out = append(out, idx.ListKeys()...)
	}
	return out
}

func (m *multiIndexer) Get(obj any) (item any, exists bool, err error) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return nil, false, err
	}
	return m.GetByKey(key)
}

func (m *multiIndexer) GetByKey(key string) (item any, exists bool, err error) {
	idx := m.indexerForKey(key)
	if idx == nil {
		return nil, false, nil
	}
	return idx.GetByKey(key)
}

func (m *multiIndexer) Replace(items []any, resourceVersion string) error {
	byNamespace := make(map[string][]any, len(m.byNamespace))
	for _, obj := range items {
		accessor, err := apimeta.Accessor(obj)
		if err != nil {
			return err
		}
		byNamespace[accessor.GetNamespace()] = append(byNamespace[accessor.GetNamespace()], obj)
	}
	for ns, idx := range m.byNamespace {
		if err := idx.Replace(byNamespace[ns], resourceVersion); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiIndexer) Resync() error {
	var errs []error
	for _, idx := range m.all {
		if err := idx.Resync(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *multiIndexer) Index(indexName string, obj any) ([]any, error) {
	var out []any
	for _, idx := range m.all {
		items, err := idx.Index(indexName, obj)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (m *multiIndexer) IndexKeys(indexName, indexedValue string) ([]string, error) {
	var out []string
	for _, idx := range m.all {
		keys, err := idx.IndexKeys(indexName, indexedValue)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
	}
	return out, nil
}

func (m *multiIndexer) ListIndexFuncValues(indexName string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, idx := range m.all {
		for _, v := range idx.ListIndexFuncValues(indexName) {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	return out
}

func (m *multiIndexer) ByIndex(indexName, indexedValue string) ([]any, error) {
	var out []any
	for _, idx := range m.all {
		items, err := idx.ByIndex(indexName, indexedValue)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (m *multiIndexer) GetIndexers() cache.Indexers {
	if len(m.all) == 0 {
		return cache.Indexers{}
	}
	return m.all[0].GetIndexers()
}

func (m *multiIndexer) AddIndexers(newIndexers cache.Indexers) error {
	for _, idx := range m.all {
		if err := idx.AddIndexers(newIndexers); err != nil {
			return err
		}
	}
	return nil
}
