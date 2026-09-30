package compress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testWorkflow() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Workflow",
		"metadata": map[string]any{
			"name":            "my-wf",
			"namespace":       "argo",
			"resourceVersion": "123",
			"labels":          map[string]any{"workflows.argoproj.io/phase": "Running"},
		},
		"spec": map[string]any{"entrypoint": "main"},
		"status": map[string]any{
			"phase": "Running",
			"nodes": map[string]any{
				"my-wf": map[string]any{"id": "my-wf", "phase": "Running"},
			},
		},
	}}
}

func TestCompressDecompressRoundTrip(t *testing.T) {
	un := testWorkflow()

	obj, err := Compress(un)
	require.NoError(t, err)
	require.NotNil(t, obj)

	// metadata is readable without decompression
	assert.Equal(t, "my-wf", obj.GetName())
	assert.Equal(t, "argo", obj.GetNamespace())
	assert.Equal(t, "123", obj.GetResourceVersion())
	assert.Equal(t, "Running", obj.GetLabels()["workflows.argoproj.io/phase"])

	out, err := obj.Decompress()
	require.NoError(t, err)
	assert.Equal(t, un.Object, out.Object)
}

func TestToUnstructured(t *testing.T) {
	un := testWorkflow()

	t.Run("passthrough", func(t *testing.T) {
		out, err := ToUnstructured(un)
		require.NoError(t, err)
		assert.Same(t, un, out)
	})

	t.Run("compressed", func(t *testing.T) {
		obj, err := Compress(un)
		require.NoError(t, err)

		out, err := ToUnstructured(obj)
		require.NoError(t, err)
		assert.Equal(t, un.Object, out.Object)
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := ToUnstructured("not an object")
		assert.Error(t, err)
	})
}

func TestTransform(t *testing.T) {
	un := testWorkflow()

	out, err := Transform(un)
	require.NoError(t, err)
	obj, ok := out.(*Object)
	require.True(t, ok)
	assert.Equal(t, "my-wf", obj.GetName())

	// non-unstructured input (e.g. tombstones) passes through untouched
	out, err = Transform("tombstone")
	require.NoError(t, err)
	assert.Equal(t, "tombstone", out)
}

func TestDeepCopy(t *testing.T) {
	un := testWorkflow()
	obj, err := Compress(un)
	require.NoError(t, err)

	cp := obj.DeepCopy()
	require.NotNil(t, cp)
	assert.Equal(t, obj.data, cp.data)
	cp.SetName("mutated")
	assert.Equal(t, "my-wf", obj.GetName(), "mutating the copy must not affect the original")

	out, err := cp.Decompress()
	require.NoError(t, err)
	assert.Equal(t, un.Object["spec"], out.Object["spec"])
}

func TestCompressNil(t *testing.T) {
	obj, err := Compress(nil)
	require.NoError(t, err)
	assert.Nil(t, obj)

	var nilObj *Object
	out, err := nilObj.Decompress()
	require.NoError(t, err)
	assert.Nil(t, out)
}
