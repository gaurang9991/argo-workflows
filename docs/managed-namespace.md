# Managed Namespace

> v2.5 and after

You can install Argo in either namespace scoped or cluster scoped configurations.
The main difference is whether you install Roles or ClusterRoles, respectively.

In namespace scoped configuration, you must run both the Workflow Controller and Argo Server using `--namespaced`.
If you want to run workflows in a separate namespace, add `--managed-namespace` as well.
(In cluster scoped configuration, _don't_ include `--namespaced` or `--managed-namespace`.)

For example:

```yaml
      - args:
        - --configmap
        - workflow-controller-configmap
        - --executor-image
        - quay.io/argoproj/workflow-controller:v3.6.7
        - --namespaced
        - --managed-namespace
        - default
```

Please note that both cluster scoped and namespace scoped configurations require "admin" roles to install because Argo's Custom Resource Definitions (CRDs) must be created (CRDs are cluster scoped objects).

!!! Info "Example Use Case"
    You can use a managed namespace install if you want some users or services to run Workflows without granting them privileges in the namespace where Argo Workflows is installed.
    For example, if you only run CI/CD Workflows that are maintained by the same team that manages the Argo Workflows installation, you may want a namespace install.
    But if all the Workflows are run by a separate data science team, you may want to give them a "data-science-workflows" namespace and use a managed namespace install of Argo Workflows in another namespace.

## Multiple managed namespaces

> v3.7 and after

`--managed-namespace` accepts a comma-separated list, so a single Workflow Controller and Argo Server
instance can watch a static allowlist of several namespaces instead of just one, without requiring
cluster-scoped RBAC:

```yaml
      - args:
        - --namespaced
        - --managed-namespace
        - team-a,team-b,team-c
```

The same list can be set via the `ARGO_MANAGED_NAMESPACE` environment variable (see
[environment variables](environment-variables.md)), or, for the Workflow Controller only, via the
`managedNamespaces` field in the `workflow-controller-configmap`:

```yaml
# workflow-controller-configmap
managedNamespaces:
  - team-a
  - team-b
  - team-c
```

When both are set, the configmap's `managedNamespaces` takes precedence over `--managed-namespace`/`ARGO_MANAGED_NAMESPACE`.
As with `Namespace`/`--managed-namespace` today, changing the set of watched namespaces requires restarting the
Controller/Server pods to take effect.

RBAC (`Role`/`RoleBinding`) must be granted in every namespace in the list; see the
[security docs](security.md) for the permissions the Controller and Server need.

!!! Info "Example Use Case"
    A platform team can run one shared Argo Workflows installation for several application teams,
    each with their own namespace, without granting cluster-wide permissions and without the
    operational overhead of running (and upgrading) one Argo Workflows installation per namespace,
    or of using `instanceID` to partition a single cluster-scoped installation.
