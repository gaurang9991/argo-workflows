// Package compress provides a memory-efficient, drop-in replacement for the
// *unstructured.Unstructured objects held in informer caches.
//
// Workflow (and, to a lesser extent, WorkflowTemplate/ClusterWorkflowTemplate/
// CronWorkflow) objects are kept in memory for the lifetime of the controller
// as a deeply nested map[string]any tree (*unstructured.Unstructured.Object).
// For a Workflow this tree is dominated by status.nodes, which can be huge for
// workflows with thousands of steps, and every key, string and nested map in
// that tree is its own heap allocation.
//
// Object instead keeps only the small, frequently-accessed metadata
// (namespace, name, labels, annotations, resourceVersion, ...) as a plain
// metav1.ObjectMeta - so that cache keying (cache.MetaNamespaceKeyFunc),
// informer indexers and most event handlers, all of which only ever look at
// metadata, keep working without touching the bulk of the object - and
// compresses everything else (the full manifest, spec and status included)
// into a single []byte, replacing the nested map tree with one heap
// allocation. Callers that need the full object call ToUnstructured, which
// transparently decompresses it back into an *unstructured.Unstructured.
package compress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Object is a compressed, in-memory stand-in for an *unstructured.Unstructured.
// It implements runtime.Object (so it can live in a cache.SharedIndexInformer)
// and metav1.Object (so meta.Accessor, cache.MetaNamespaceKeyFunc and the
// existing metadata-only IndexFuncs/EventHandlers work unmodified).
type Object struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	data []byte // zstd-compressed JSON of the full unstructured object, metadata included
}

var _ runtime.Object = &Object{}

var (
	codecOnce sync.Once
	encoder   *zstd.Encoder
	decoder   *zstd.Decoder
)

// codecs lazily initializes the shared, goroutine-safe zstd encoder/decoder.
// EncodeAll/DecodeAll are stateless one-shot calls, so a single pair can
// safely be reused across every Compress/Decompress call.
func codecs() (*zstd.Encoder, *zstd.Decoder) {
	codecOnce.Do(func() {
		encoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		decoder, _ = zstd.NewReader(nil)
	})
	return encoder, decoder
}

// Compress serializes un to JSON and zstd-compresses it into a new Object.
// A plain copy of its metadata is kept alongside the compressed bytes.
func Compress(un *unstructured.Unstructured) (*Object, error) {
	if un == nil {
		return nil, nil
	}
	var om metav1.ObjectMeta
	if metaMap, found, _ := unstructured.NestedMap(un.Object, "metadata"); found {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(metaMap, &om); err != nil {
			return nil, fmt.Errorf("compress: decode metadata: %w", err)
		}
	}
	raw, err := json.Marshal(un.Object)
	if err != nil {
		return nil, fmt.Errorf("compress: marshal: %w", err)
	}
	enc, _ := codecs()
	return &Object{
		TypeMeta:   metav1.TypeMeta{Kind: un.GetKind(), APIVersion: un.GetAPIVersion()},
		ObjectMeta: om,
		data:       enc.EncodeAll(raw, make([]byte, 0, len(raw)/4)),
	}, nil
}

// Decompress reconstructs the full *unstructured.Unstructured that was passed to Compress.
func (o *Object) Decompress() (*unstructured.Unstructured, error) {
	if o == nil {
		return nil, nil
	}
	_, dec := codecs()
	raw, err := dec.DecodeAll(o.data, nil)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decompress: unmarshal: %w", err)
	}
	return &unstructured.Unstructured{Object: m}, nil
}

// ToUnstructured returns the full unstructured representation of a cached
// object, transparently decompressing it if it is a compressed Object.
// It also accepts a plain *unstructured.Unstructured so call sites (and
// tests) that don't go through a compressing informer keep working.
func ToUnstructured(obj any) (*unstructured.Unstructured, error) {
	switch v := obj.(type) {
	case *unstructured.Unstructured:
		return v, nil
	case *Object:
		return v.Decompress()
	default:
		return nil, fmt.Errorf("compress: expected *unstructured.Unstructured or *compress.Object, got %T", obj)
	}
}

// Transform is a cache.TransformFunc that compresses the unstructured objects
// an informer's reflector produces before they enter the cache. Wire it in
// with informer.SetTransform, typically chained after StripManagedFields.
func Transform(i any) (any, error) {
	un, ok := i.(*unstructured.Unstructured)
	if !ok {
		// Tombstones (cache.DeletedFinalStateUnknown) and anything else pass through untouched.
		return i, nil
	}
	return Compress(un)
}

func (o *Object) GetObjectKind() schema.ObjectKind {
	return &o.TypeMeta
}

func (o *Object) DeepCopyObject() runtime.Object {
	return o.DeepCopy()
}

// DeepCopy returns a deep copy of o.
func (o *Object) DeepCopy() *Object {
	if o == nil {
		return nil
	}
	out := &Object{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.ObjectMeta.DeepCopy(),
	}
	if o.data != nil {
		out.data = bytes.Clone(o.data)
	}
	return out
}
