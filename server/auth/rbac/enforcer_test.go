package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func configMap(policy, linked string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "argo-rbac-cm", Namespace: "argo"},
		Data: map[string]string{
			PolicyKey:                policy,
			LinkedServiceAccountsKey: linked,
		},
	}
}

func TestEnforcer_NotLoaded(t *testing.T) {
	e := NewEnforcer()
	assert.False(t, e.Enabled())
	_, err := e.Enforce("alice", "workflows", "get", "team-a/my-wf")
	require.Error(t, err)
}

func TestEnforcer_LoadAndEnforce(t *testing.T) {
	policy := `
# comment lines and blank lines are ignored

g, team-a-admins, role:team-a-operator
g, everyone, role:read-only
p, role:team-a-operator, workflows, get, team-a/*, allow
p, role:team-a-operator, workflows, list, team-a/*, allow
p, role:team-a-operator, workflows, resume, team-a/*, allow
p, role:team-a-operator, workflows, retry, team-a/*, allow
p, role:team-a-operator, workflows, terminate, team-a/*, deny
p, role:team-a-operator, workflows, delete, team-a/*, deny
p, role:read-only, *, get, */*, allow
p, role:read-only, *, list, */*, allow
`
	linked := `
role:team-a-operator: team-a/team-a-operator-sa
role:read-only: argo/read-only-sa
`
	e := NewEnforcer()
	require.NoError(t, e.Load(configMap(policy, linked)))
	assert.True(t, e.Enabled())

	t.Run("allowed by specific role", func(t *testing.T) {
		result, err := e.Enforce("team-a-admins", "workflows", "resume", "team-a/my-wf")
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, "role:team-a-operator", result.Subject)
		assert.Equal(t, "team-a/team-a-operator-sa", result.ServiceAccount)
	})

	t.Run("explicit deny overrides allow", func(t *testing.T) {
		result, err := e.Enforce("team-a-admins", "workflows", "terminate", "team-a/my-wf")
		require.NoError(t, err)
		assert.False(t, result.Allowed)
	})

	t.Run("allowed by default/read-only role", func(t *testing.T) {
		result, err := e.Enforce("everyone", "workflows", "get", "team-b/my-wf")
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, "argo/read-only-sa", result.ServiceAccount)
	})

	t.Run("no matching policy", func(t *testing.T) {
		result, err := e.Enforce("nobody", "workflows", "get", "team-a/my-wf")
		require.NoError(t, err)
		assert.False(t, result.Allowed)
	})
}

func TestEnforcer_LoadFailsWithoutLinkedServiceAccount(t *testing.T) {
	policy := `
g, team-a-admins, role:team-a-operator
p, role:team-a-operator, workflows, get, team-a/*, allow
`
	e := NewEnforcer()
	err := e.Load(configMap(policy, ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role:team-a-operator")
	assert.False(t, e.Enabled())
}

func TestEnforcer_LoadFailsOnInvalidPolicyLine(t *testing.T) {
	e := NewEnforcer()
	err := e.Load(configMap("p, role:x", ""))
	require.Error(t, err)
}

func TestEnforcer_ReloadReplacesPolicy(t *testing.T) {
	e := NewEnforcer()
	require.NoError(t, e.Load(configMap(
		"g, alice, role:admin\np, role:admin, workflows, get, */*, allow",
		"role:admin: argo/admin-sa",
	)))
	result, err := e.Enforce("alice", "workflows", "get", "team-a/wf")
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// reload with a policy that no longer grants alice anything
	require.NoError(t, e.Load(configMap(
		"g, bob, role:admin\np, role:admin, workflows, get, */*, allow",
		"role:admin: argo/admin-sa",
	)))
	result, err = e.Enforce("alice", "workflows", "get", "team-a/wf")
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}
