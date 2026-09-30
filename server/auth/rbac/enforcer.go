// Package rbac implements fine-grained, Casbin-backed authorization for the Argo Server,
// layered on top of the existing Kubernetes-identity-based auth in server/auth. It is
// currently wired into SSO mode only: after Casbin allows a request, the role that allowed
// it is mapped to a linked Kubernetes ServiceAccount, whose token is used exactly the way a
// legacy workflows.argoproj.io/rbac-rule match is used today.
package rbac

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

//go:embed model.conf
var modelConf string

const (
	// PolicyKey is the ConfigMap data key holding the Casbin policy/grouping (g, p) lines.
	PolicyKey = "policy.csv"
	// LinkedServiceAccountsKey is the ConfigMap data key holding the YAML-encoded map of
	// policy subject (role) -> "namespace/name" of the ServiceAccount used to call the
	// Kubernetes API when that subject authorizes a request.
	LinkedServiceAccountsKey = "linkedServiceAccounts.yaml"
)

// Result is the outcome of a fine-grained authorization check.
type Result struct {
	Allowed bool
	// Subject is the policy subject (role) whose rule allowed the request. Only set when Allowed.
	Subject string
	// ServiceAccount is the "namespace/name" of the ServiceAccount linked to Subject. Only set when Allowed.
	ServiceAccount string
}

// Enforcer evaluates fine-grained Casbin policies loaded from a dedicated ConfigMap. It is
// safe for concurrent use; Load may be called repeatedly (e.g. from a ConfigMap informer) to
// hot-reload the policy.
type Enforcer struct {
	mu       sync.RWMutex
	enforcer *casbin.Enforcer
	linked   map[string]string
}

// NewEnforcer returns an Enforcer with no policy loaded. Enforce fails until Load succeeds.
func NewEnforcer() *Enforcer {
	return &Enforcer{}
}

// Enabled reports whether a policy has been successfully loaded at least once.
func (e *Enforcer) Enabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enforcer != nil
}

// Load (re)builds the enforcer from the dedicated RBAC policy ConfigMap's data. Every
// subject referenced by a "p" policy line must have a corresponding entry in
// LinkedServiceAccountsKey, or Load fails and the previously loaded policy (if any) is kept.
func (e *Enforcer) Load(cm *corev1.ConfigMap) error {
	m, err := model.NewModelFromString(modelConf)
	if err != nil {
		return fmt.Errorf("rbac: failed to parse casbin model: %w", err)
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return fmt.Errorf("rbac: failed to create casbin enforcer: %w", err)
	}
	for i, line := range strings.Split(cm.Data[PolicyKey], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if lineErr := persist.LoadPolicyLine(line, enforcer.GetModel()); lineErr != nil {
			return fmt.Errorf("rbac: invalid %s line %d (%q): %w", PolicyKey, i+1, line, lineErr)
		}
	}
	if buildErr := enforcer.BuildRoleLinks(); buildErr != nil {
		return fmt.Errorf("rbac: failed to build role links: %w", buildErr)
	}
	linked := map[string]string{}
	if raw := cm.Data[LinkedServiceAccountsKey]; raw != "" {
		if yamlErr := yaml.UnmarshalStrict([]byte(raw), &linked); yamlErr != nil {
			return fmt.Errorf("rbac: failed to parse %s: %w", LinkedServiceAccountsKey, yamlErr)
		}
	}
	policies, err := enforcer.GetPolicy()
	if err != nil {
		return fmt.Errorf("rbac: failed to read policy: %w", err)
	}
	for _, rule := range policies {
		if len(rule) == 0 {
			continue
		}
		subject := rule[0]
		if _, ok := linked[subject]; !ok {
			return fmt.Errorf("rbac: subject %q is granted permissions in %s but has no entry in %s", subject, PolicyKey, LinkedServiceAccountsKey)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.enforcer = enforcer
	e.linked = linked
	return nil
}

// Enforce decides whether sub may perform act on res/obj, returning the linked
// ServiceAccount to use for the downstream Kubernetes API call when allowed.
func (e *Enforcer) Enforce(sub, res, act, obj string) (Result, error) {
	e.mu.RLock()
	enforcer, linked := e.enforcer, e.linked
	e.mu.RUnlock()
	if enforcer == nil {
		return Result{}, fmt.Errorf("rbac: no policy loaded")
	}
	allowed, explain, err := enforcer.EnforceEx(sub, res, act, obj)
	if err != nil {
		return Result{}, fmt.Errorf("rbac: enforce failed: %w", err)
	}
	if !allowed {
		return Result{Allowed: false}, nil
	}
	if len(explain) == 0 {
		return Result{}, fmt.Errorf("rbac: policy allowed the request but returned no matching rule")
	}
	subject := explain[0]
	serviceAccount, ok := linked[subject]
	if !ok {
		return Result{}, fmt.Errorf("rbac: subject %q has no linked service account", subject)
	}
	return Result{Allowed: true, Subject: subject, ServiceAccount: serviceAccount}, nil
}
