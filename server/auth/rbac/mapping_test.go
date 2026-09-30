package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeNamespacedRequest struct {
	Namespace string
	Name      string
}

func (r fakeNamespacedRequest) GetNamespace() string { return r.Namespace }
func (r fakeNamespacedRequest) GetName() string      { return r.Name }

func TestActionFor(t *testing.T) {
	a, ok := ActionFor("/workflow.WorkflowService/ResumeWorkflow")
	assert.True(t, ok)
	assert.Equal(t, Action{Resource: "workflows", Verb: "resume"}, a)

	a, ok = ActionFor("/workflow.WorkflowService/TerminateWorkflow")
	assert.True(t, ok)
	assert.Equal(t, Action{Resource: "workflows", Verb: "terminate"}, a)

	_, ok = ActionFor("/not.Registered/Method")
	assert.False(t, ok)
}

func TestObjectFor(t *testing.T) {
	assert.Equal(t, "team-a/my-wf", ObjectFor(fakeNamespacedRequest{Namespace: "team-a", Name: "my-wf"}))
	assert.Equal(t, "team-a/", ObjectFor(fakeNamespacedRequest{Namespace: "team-a"}))
	assert.Equal(t, "/", ObjectFor(nil))
}
