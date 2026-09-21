# Multi-Namespace Mode Design

Status: proposed.

## Summary

Argo Workflows currently runs either cluster-wide or in one managed namespace.
This design adds a static list of managed namespaces to the Workflow Controller and makes that list available to the UI through Argo Server.
It allows one shared installation to serve several teams while using namespace-scoped Roles and RoleBindings for workflow resources.

The implementation should preserve the existing reconciliation and server code by treating multi-namespace support primarily as a controller informer and list concern.
Workflow execution already uses namespace-qualified keys and namespaced Kubernetes clients, so the core workflow operation logic should not need to change.

## Goals

* Allow one controller deployment to manage a fixed list of namespaces.
* Require no cluster-wide list or watch permission for namespaced workflow resources.
* Preserve cluster-wide, namespace-install, and single managed namespace behavior.
* Preserve high availability, archiving, offloading, synchronization, CronWorkflows, templates, artifacts, metrics, and health checks.
* Expose the configured namespace list through the Argo Server Info API for the UI namespace filter.
* Keep the change localized to startup configuration, controller informer construction, direct controller list operations, the Info API, and UI namespace selection.

## Non-Goals

* Dynamically adding or removing namespaces without restarting the controller and server is not part of the initial implementation.
* Selecting namespaces by label is not part of the initial implementation because it requires cluster-level access to Namespace resources.
* Multi-namespace mode does not provide a security boundary between teams that share the same controller service account.
* This design does not remove the cluster-scoped CRDs required to install Argo Workflows.
* This design does not make cluster-scoped features, such as ClusterWorkflowTemplates, available without their existing cluster permissions.
* Argo Server request authorization, persistence queries, workflow archive filtering, SSO behavior, and artifact resolution are not changed.
* Argo Server does not aggregate results across namespaces; the UI continues to issue requests for one selected namespace.

## Current Behavior

The `--namespaced` flag selects namespace-scoped operation.
The singular `--managed-namespace` flag selects either the installation namespace or one separate workflow namespace.
An empty managed namespace represents all namespaces in cluster-wide mode.

The controller passes that single namespace string to workflow, pod, CronWorkflow, WorkflowTemplate, task-result, task-set, artifact-GC, and ConfigMap informers.
Argo Server returns the managed namespace in the Info API, and the UI treats a non-empty value as a fixed namespace.
Other server APIs already accept a namespace in each request, and Kubernetes RBAC authorizes those requests.

The controller configuration field named `namespace` is also a single namespace fallback.
It is not a list of namespace names and should remain supported for compatibility.

## User-Facing Configuration

Add a new `--managed-namespaces` string-slice flag to both `workflow-controller` and `argo server`.
Keep `--managed-namespace` unchanged and supported.

For example:

```yaml
args:
  - --namespaced
  - --managed-namespaces=team-a,team-b,team-c
```

The startup rules should be:

| `--namespaced` | Namespace options | Effective scope |
|----------------|-------------------|-----------------|
| `false` | none | All namespaces, preserving cluster-wide mode |
| `false` | any managed namespace option | Ignore with the existing warning behavior |
| `true` | none | Installation namespace, preserving namespace-install mode |
| `true` | `--managed-namespace=x` | Namespace `x`, preserving managed-namespace mode |
| `true` | `--managed-namespaces=x,y` | Exactly namespaces `x` and `y` |

Specifying both managed namespace flags should fail startup with a clear error.
Names must be trimmed, validated as Kubernetes namespace names, de-duplicated, and stored in stable sorted order.
An explicitly empty multi-namespace list should fail rather than accidentally becoming cluster-wide.

A ConfigMap field can be added later if runtime configuration is required.
The first implementation should use startup flags because controller informer topology and RBAC cannot be changed safely by the existing ConfigMap reload path.

## Internal Representation

Keep the existing `managedNamespace string` for cluster-wide and single-namespace behavior.
Add a normalized `managedNamespaces []string` only for the new multi-namespace path.
Avoid replacing all existing namespace arguments with a new scope type because that would increase the size and risk of the change without improving the required behavior.

One startup helper should normalize the legacy and new flags into these two representations.
Existing call sites should continue to use `managedNamespace` unless they create a controller watch or perform a controller-wide list.
This keeps the established empty-string meaning for cluster-wide mode and avoids unrelated changes.

## Controller Design

### Informer Sets

Create one normal namespaced informer per configured namespace and group them in a small project-owned informer-set abstraction.
Do not combine namespace watches into a custom Kubernetes `ListWatch`.
A combined watch has difficult resource-version, relist, bookmark, and partial-failure semantics that Kubernetes client-go does not provide automatically.

The informer set should provide only the operations used by Argo Workflows:

```go
type InformerSet interface {
    AddEventHandler(handler cache.ResourceEventHandler) error
    Run(stopCh <-chan struct{})
    HasSynced() bool
    GetByKey(key string) (item any, exists bool, err error)
    List() []any
    ByIndex(indexName, indexedValue string) ([]any, error)
    IndexKeys(indexName, indexedValue string) ([]string, error)
}
```

`GetByKey` should parse the existing `<namespace>/<name>` key and route directly to the correct child informer.
List and index operations should aggregate child results.
Event handlers should be registered on every child informer before any child starts.
`HasSynced` should return true only after every child informer has synced.
Cluster-wide and single-namespace modes should continue to instantiate the existing informer directly.
Only the new multi-namespace mode should use the informer set.
This ensures that the established modes retain their current code paths.

Typed informers need resource-specific adapters instead of changes to generated client code.
WorkflowTemplate lookups should route by the workflow namespace.
Task-set and artifact-GC handlers can fan out events into their existing namespace-qualified queues.
The shared controller work queues, key locks, throttler, synchronization manager, and workflow operation context can remain unchanged.

The following namespaced watches require conversion:

* Workflows and workflow-child watches used by the CronWorkflow controller.
* Pods.
* CronWorkflows.
* WorkflowTemplates.
* WorkflowTaskResults.
* WorkflowTaskSets.
* WorkflowArtifactGCTasks.
* Typed and semaphore ConfigMaps.

ClusterWorkflowTemplate and Namespace informers remain optional cluster-scoped informers with their current permission checks.

### Direct List Operations

Replace direct list calls using one managed namespace with a helper that lists each configured namespace and combines the results.
The helper should preserve the current label selector, pagination behavior, and error handling.
Startup should fail if a required namespace cannot be listed, because silently managing only part of the configured allowlist is unsafe.

This applies to initialization of running workflows, pod cleanup, offload garbage collection, and any other maintenance loop that currently calls `GetManagedNamespace()`.
Namespaced create, get, update, patch, and delete calls already use the workflow object's namespace and should not change.

### CronWorkflow Controller

Pass the normalized namespace list to the CronWorkflow controller while retaining its existing singular namespace parameter for compatibility.
Create a CronWorkflow informer and child Workflow informer for each namespace.
Keep the existing shared queue and scheduler facade because their keys already include namespaces.
The periodic `syncAll` operation should list across the informer set and preserve namespace-qualified grouping.

### Health, Metrics, and Readiness

Health checks should aggregate unreconciled workflows across all child informer indexers.
Metrics based on workflow or pod indexes should sum results from every child informer.
Startup readiness must wait for all required namespace caches.
Logs should include `managedNamespaces` and identify the namespace when an individual cache cannot sync.

### Synchronization and Parallelism

Local and database-backed synchronization keys already include workflow identity and should continue to work across managed namespaces.
Initialization must combine running workflows from every namespace before restoring lock holders and throttler state.

Per-namespace parallelism based on Namespace labels still requires permission to read Namespace objects.
When that optional permission is absent, the controller should retain its current degraded behavior and use configured defaults.

## Argo Server and UI Design

The Argo Server change should be limited to passing the configured namespace list through the existing Info API.
Add `repeated string managedNamespaces` to `InfoResponse` while retaining `managedNamespace` for backward compatibility.
For a single namespace, the server should continue to populate `managedNamespace` and may also populate the list.
For multiple namespaces, `managedNamespace` should remain empty and `managedNamespaces` should contain the normalized list.
The protobuf and generated clients must be regenerated using the normal code-generation workflow.

The UI should store the managed namespace list received during startup.
When the list contains one namespace, the namespace filter should preserve the current fixed-namespace display.
When the list contains multiple namespaces, the namespace filter should render a dropdown restricted to those values and select a stable default, preferably the current selection when valid and otherwise the first configured namespace.
Routes and API requests should continue to contain exactly one selected namespace.

No server-side aggregation or new multi-namespace list API is required.
No new server authorization interceptor is required; existing Kubernetes RBAC remains responsible for rejecting requests to namespaces for which the selected credentials have no access.
Existing Workflow, CronWorkflow, WorkflowTemplate, archive, artifact, SSO, and persistence request paths should remain unchanged.

## Persistence and Existing Services

Persistence should not be changed for this feature.
The UI continues to query one namespace at a time, so existing namespace-scoped archive queries remain valid.
The controller should invoke namespace-dependent maintenance operations once per configured namespace only where it currently performs a controller-wide operation using the singular managed namespace.
Offload and archive reads performed for an already selected workflow keep their existing behavior.

The implementation should not proactively refactor server caches, artifact repository resolution, SSO, synchronization storage, or database interfaces.
If a focused multi-namespace test identifies a concrete single-namespace assumption in one of those paths, only that blocking call site should be adjusted.

## RBAC and Installation

The controller service account still needs a Role in its installation namespace for leader-election leases and controller configuration.
Each managed namespace needs a Role containing the existing namespaced workflow-controller permissions and a RoleBinding to the controller service account.
The Argo Server service account needs equivalent RoleBindings according to the configured authentication modes because UI requests remain namespace scoped.

CRDs remain a cluster-scoped installation prerequisite but do not grant runtime permissions to the controller or server.
ClusterWorkflowTemplates and namespace-label parallelism continue to require their documented optional cluster permissions.

Kustomize examples should separate the installation-namespace Role from the repeatable managed-namespace RoleBinding pattern.
The community Helm chart should receive matching values after the core flags and manifests are stable.

## Compatibility and Failure Handling

Existing flags, environment-variable binding, manifests, and single-namespace behavior must remain unchanged.
The new flag is additive and does not reinterpret `--managed-namespace`.
Instance ID filtering remains available and orthogonal, but is no longer required merely to constrain a shared controller to selected namespaces.

The process should fail during startup when any required informer receives `Forbidden`, cannot complete its initial list, or cannot sync.
Continuing with a partial namespace set could leave workflows unmanaged and would make high-availability replicas disagree about ownership.
Optional cluster-scoped informers should retain their current graceful degradation.

All replicas must receive the same ordered namespace list.
The namespace list should be logged and exposed through the Info API to make configuration drift observable.

## Implementation Sequence

1. Add and test plural flag normalization, validation, and backward-compatible startup behavior without replacing the singular namespace representation.
2. Add reusable unstructured and typed informer-set helpers with unit tests for routing, aggregation, synchronization, and partial failure.
3. Convert the Workflow Controller's workflow and pod paths, then run the existing controller suite unchanged before converting secondary resources.
4. Convert CronWorkflow, WorkflowTemplate, task-result, task-set, artifact-GC, and ConfigMap informers.
5. Convert direct list operations, health checks, metrics, offload cleanup, and synchronization initialization.
6. Add `managedNamespaces` to the Info API and update the UI namespace filter.
7. Add namespace-scoped installation examples, security documentation, and end-to-end coverage.

Each step should preserve the one-namespace adapter path so existing tests exercise the new abstractions before multi-namespace mode is enabled.

## Testing Strategy

Unit tests should cover all, one, and multiple namespace configurations, duplicate normalization, conflicting flags, and empty-list rejection.
Informer-set tests should prove that events from two namespaces reach one queue, same-name resources do not collide, indexes aggregate correctly, and readiness waits for every child.
Controller tests should create workflows, CronWorkflows, templates, task results, synchronization records, and artifact-GC tasks in two allowed namespaces and one excluded namespace.
Server tests should verify that the Info API preserves the singular field and returns the plural field.
UI tests should verify fixed display for one namespace, dropdown selection for multiple namespaces, and one namespace per API request.
High-availability tests should verify that two replicas with the same allowlist elect one leader and recover all namespace caches after failover.
An end-to-end test should run the controller and server with only Roles and RoleBindings in two managed namespaces and no namespaced-resource ClusterRole.

## Magnitude of Change

This is a medium controller change with a small server and UI extension, and it should not require changes to workflow execution semantics, persistence, or CRDs.
The likely production impact is approximately 12 to 20 files across command startup, controller informer construction, CronWorkflow, the Info API, and UI namespace handling.
The likely test impact is approximately 8 to 15 files, with most additions covering informer aggregation and namespace selection.
Generated API and documentation outputs will add mechanical changes after the info response is extended.

The highest-risk work is informer lifecycle aggregation and ensuring every controller-wide list covers all configured namespaces.
The lowest-risk work is flag parsing, namespace-qualified queue processing, the Info API field, and namespaced resource mutations because those patterns already exist.
Keeping the namespace list static and reusing one standard informer per namespace avoids a much larger redesign of reconciliation, leader election, or generated clients.

## Alternatives Considered

### Continue Using Instance IDs

Instance IDs filter objects by label but still require a cluster-wide watch when the managed namespace is empty.
They do not satisfy the goal of removing cluster-wide permissions.

### Run One Argo Installation Per Namespace

This works today but duplicates controllers, servers, persistence configuration, upgrades, and operational overhead for teams that want one shared service.

### Use One Cluster-Wide Informer and Filter Events Locally

Local filtering still requires cluster-wide list and watch RBAC and therefore does not meet the security goal.

### Merge Namespace Watches Into One `ListWatch`

This appears to minimize call-site changes but introduces ambiguous resource-version and relist behavior across independent namespace requests.
Explicit informer sets are more code at the boundary but retain client-go's tested watch semantics and isolate failures by namespace.

### Dynamically Discover Namespaces by Label

Namespace discovery requires cluster-scoped permission to list and watch Namespace resources.
It can be considered as a separate future mode, but it does not meet the least-privilege requirement of this design.
