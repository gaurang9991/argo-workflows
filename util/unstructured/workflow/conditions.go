package workflow

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	wfv1 "github.com/argoproj/argo-workflows/v4/pkg/apis/workflow/v1alpha1"
	"github.com/argoproj/argo-workflows/v4/util/compress"
)

// GetConditions returns the conditions, excluding the `message` field. obj is
// typically an *unstructured.Unstructured or a compressed informer cache
// entry (*compress.Object); it is transparently decompressed as needed.
func GetConditions(obj any) wfv1.Conditions {
	un, err := compress.ToUnstructured(obj)
	if err != nil || un == nil {
		return nil
	}
	items, _, _ := unstructured.NestedSlice(un.Object, "status", "conditions")
	var x wfv1.Conditions
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		_, ok = m["type"].(string)
		if !ok {
			return nil
		}
		_, ok = m["status"].(string)
		if !ok {
			return nil
		}
		x = append(x, wfv1.Condition{
			Type:   wfv1.ConditionType(m["type"].(string)),
			Status: metav1.ConditionStatus(m["status"].(string)),
		})
	}
	return x
}
