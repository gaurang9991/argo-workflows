package gccontroller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Heap is the interface for a GC heap (implements heap.Interface)
type Heap interface {
	Len() int
	Less(i, j int) bool
	Swap(i, j int)
	Push(x any)
	Pop() any
}

type gcHeap struct {
	heap  []metav1.Object
	dedup map[string]bool
}

func NewHeap() Heap {
	return &gcHeap{
		heap:  make([]metav1.Object, 0),
		dedup: make(map[string]bool),
	}
}

func (h *gcHeap) Len() int { return len(h.heap) }
func (h *gcHeap) Less(i, j int) bool {
	return h.heap[j].GetCreationTimestamp().After((h.heap[i].GetCreationTimestamp().Time))
}
func (h *gcHeap) Swap(i, j int) { h.heap[i], h.heap[j] = h.heap[j], h.heap[i] }

func (h *gcHeap) Push(x any) {
	m := x.(metav1.Object)
	if _, ok := h.dedup[m.GetName()]; ok {
		return
	}
	h.dedup[m.GetName()] = true
	h.heap = append(h.heap, m)
}

func (h *gcHeap) Pop() any {
	old := h.heap
	n := len(old)
	x := old[n-1]
	h.heap = old[0 : n-1]
	delete(h.dedup, x.GetName())
	return x
}
