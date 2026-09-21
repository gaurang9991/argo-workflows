package managednamespace

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

func Resolve(namespaced bool, installationNamespace, managedNamespace string, managedNamespaces []string) (string, []string, error) {
	if !namespaced {
		return "", nil, nil
	}
	if managedNamespace != "" && len(managedNamespaces) > 0 {
		return "", nil, fmt.Errorf("--managed-namespace and --managed-namespaces cannot be used together")
	}
	if len(managedNamespaces) == 0 {
		if managedNamespace == "" {
			managedNamespace = installationNamespace
		}
		return managedNamespace, nil, nil
	}

	seen := make(map[string]struct{}, len(managedNamespaces))
	resolved := make([]string, 0, len(managedNamespaces))
	for _, namespace := range managedNamespaces {
		namespace = strings.TrimSpace(namespace)
		if errors := validation.IsDNS1123Label(namespace); len(errors) > 0 {
			return "", nil, fmt.Errorf("invalid managed namespace %q: %s", namespace, strings.Join(errors, ", "))
		}
		if _, ok := seen[namespace]; ok {
			continue
		}
		seen[namespace] = struct{}{}
		resolved = append(resolved, namespace)
	}
	slices.Sort(resolved)
	if len(resolved) == 1 {
		return resolved[0], nil, nil
	}
	return "", resolved, nil
}
