package config

// RBACConfig contains role-based access control configuration
type RBACConfig struct {
	// Enabled controls whether RBAC is enabled
	Enabled bool `json:"enabled,omitempty"`
	// PolicyConfigMap is the name of a dedicated ConfigMap, in the same namespace as the
	// Argo Server, that holds the fine-grained Casbin policy (see server/auth/rbac). It is
	// watched and hot-reloaded independently of this configmap. When unset, RBAC falls back
	// to the legacy behavior of matching the workflows.argoproj.io/rbac-rule annotation on
	// ServiceAccounts.
	PolicyConfigMap string `json:"policyConfigMap,omitempty"`
}

func (c *RBACConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}

// GetPolicyConfigMap returns the configured name of the dedicated RBAC policy ConfigMap, or
// "" if fine-grained policy-based RBAC is not configured.
func (c *RBACConfig) GetPolicyConfigMap() string {
	if c == nil {
		return ""
	}
	return c.PolicyConfigMap
}
