package informer

import (
	"testing"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiNamespaceInformerIndexer(t *testing.T) {
	informer := NewMultiNamespaceInformer([]string{"team-a", "team-b"}, func(string) cache.SharedIndexInformer {
		return cache.NewSharedIndexInformer(nil, &apiv1.ConfigMap{}, 0, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})
	})

	teamA := &apiv1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "settings"}}
	teamB := &apiv1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "team-b", Name: "settings"}}
	require.NoError(t, informer.GetIndexer().Add(teamA))
	require.NoError(t, informer.GetIndexer().Add(teamB))

	item, exists, err := informer.GetIndexer().GetByKey("team-a/settings")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Same(t, teamA, item)
	assert.Len(t, informer.GetStore().List(), 2)

	items, err := informer.GetIndexer().ByIndex(cache.NamespaceIndex, "team-b")
	require.NoError(t, err)
	assert.Equal(t, []any{teamB}, items)
	assert.ElementsMatch(t, []string{"team-a", "team-b"}, informer.GetIndexer().ListIndexFuncValues(cache.NamespaceIndex))
}

func TestMultiNamespaceInformerRejectsUnmanagedNamespace(t *testing.T) {
	informer := NewMultiNamespaceInformer([]string{"team-a"}, func(string) cache.SharedIndexInformer {
		return cache.NewSharedIndexInformer(nil, &apiv1.ConfigMap{}, 0, cache.Indexers{})
	})

	err := informer.GetStore().Add(&apiv1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "team-b", Name: "settings"}})
	require.ErrorContains(t, err, "namespace \"team-b\" is not managed")
}

func TestMultiNamespaceInformerAddIndexersIsAtomicOnConflict(t *testing.T) {
	informer := NewMultiNamespaceInformer([]string{"team-a", "team-b"}, func(namespace string) cache.SharedIndexInformer {
		indexers := cache.Indexers{}
		if namespace == "team-b" {
			indexers[cache.NamespaceIndex] = cache.MetaNamespaceIndexFunc
		}
		return cache.NewSharedIndexInformer(nil, &apiv1.ConfigMap{}, 0, indexers)
	})
	multiIndexer := informer.GetIndexer().(*multiNamespaceIndexer)

	err := informer.AddIndexers(cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	require.Error(t, err)
	assert.NotContains(t, multiIndexer.indexers[0].GetIndexers(), cache.NamespaceIndex)
}
