package managednamespace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name                  string
		namespaced            bool
		installationNamespace string
		managedNamespace      string
		managedNamespaces     []string
		expectedNamespace     string
		expectedNamespaces    []string
		expectedError         string
	}{
		{name: "cluster scoped", installationNamespace: "argo", managedNamespace: "ignored", managedNamespaces: []string{"also-ignored"}},
		{name: "installation namespace", namespaced: true, installationNamespace: "argo", expectedNamespace: "argo"},
		{name: "legacy managed namespace", namespaced: true, installationNamespace: "argo", managedNamespace: "team-a", expectedNamespace: "team-a"},
		{name: "single plural namespace uses legacy path", namespaced: true, installationNamespace: "argo", managedNamespaces: []string{"team-a"}, expectedNamespace: "team-a"},
		{name: "multiple namespaces", namespaced: true, installationNamespace: "argo", managedNamespaces: []string{" team-b ", "team-a", "team-b"}, expectedNamespaces: []string{"team-a", "team-b"}},
		{name: "conflicting flags", namespaced: true, managedNamespace: "team-a", managedNamespaces: []string{"team-b"}, expectedError: "cannot be used together"},
		{name: "invalid namespace", namespaced: true, managedNamespaces: []string{"Team A"}, expectedError: "invalid managed namespace"},
		{name: "empty namespace", namespaced: true, managedNamespaces: []string{""}, expectedError: "invalid managed namespace"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			managedNamespace, managedNamespaces, err := Resolve(test.namespaced, test.installationNamespace, test.managedNamespace, test.managedNamespaces)
			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expectedNamespace, managedNamespace)
			assert.Equal(t, test.expectedNamespaces, managedNamespaces)
		})
	}
}
