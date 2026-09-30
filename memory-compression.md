# Memory compression for unstructured informer caches

## Problem

`workflow-controller` (and, to a lesser degree, the cron controller and
argo-server) keep Workflow, WorkflowTemplate, ClusterWorkflowTemplate and
CronWorkflow objects in `cache.SharedIndexInformer` caches as
`*unstructured.Unstructured`. Internally that's a deeply nested
`map[string]any` tree — every key, string and nested map is its own heap
allocation, and for a Workflow this is dominated by `status.nodes`, which can
be huge for workflows with many steps. Kept for the lifetime of the
controller, across every Workflow in the cluster, this is a large chunk of
controller memory.

## Approach

Added `util/compress`, a drop-in replacement for the cached
`*unstructured.Unstructured` value:

- `compress.Object` keeps a plain `metav1.TypeMeta` / `metav1.ObjectMeta`
  (small: namespace, name, labels, annotations, resourceVersion, ...) plus a
  single `[]byte` holding the zstd-compressed JSON of the *entire* manifest
  (metadata included).
- Because it embeds `metav1.ObjectMeta`, it satisfies `metav1.Object` for
  free, so `meta.Accessor`, `cache.MetaNamespaceKeyFunc`,
  `cache.DeletionHandlingMetaNamespaceKeyFunc`, and every existing
  metadata-only `IndexFunc`/event handler keep working **without
  decompressing anything**.
- `compress.ToUnstructured(obj)` transparently decompresses back to a full
  `*unstructured.Unstructured` for the code paths that need spec/status (e.g.
  converting to a typed `Workflow`/`WorkflowTemplate`). It also passes a plain
  `*unstructured.Unstructured` straight through, so unit tests that construct
  one directly don't need to change.
- `compress.Transform` is a `cache.TransformFunc` wired onto each informer
  via `SharedIndexInformer.SetTransform`, the same mechanism already used in
  this codebase to strip `managedFields` before objects enter the cache.

Used `encoding/json` (not `encoding/json/v2`) + `github.com/klauspost/compress/zstd`
for the actual compression: `encoding/json/v2` is still gated behind
`GOEXPERIMENT=jsonv2` in the available Go toolchain, which would require
changing the Makefile/Dockerfiles/CI for every binary this repo builds just
to use it. zstd was already a direct, vendored dependency, so no `go.mod`/
vendor changes were needed for it.

## Files changed and why

### New

- **`util/compress/compressed.go`** — the `compress.Object` type, `Compress`/
  `Decompress`, `ToUnstructured`, and the `Transform` cache hook described
  above.
- **`util/compress/compressed_test.go`** — round-trip, passthrough, transform,
  deep-copy and nil-handling tests for the new package.

### Core wiring (informers)

- **`util/informer/transform.go`** — added `Chain(fns ...cache.TransformFunc)`
  so an informer can run `StripManagedFields` and `compress.Transform` in
  sequence via a single `SetTransform` call.
- **`workflow/util/util.go`** — `NewWorkflowInformer` now chains
  `compress.Transform` after `StripManagedFields`; `workflowLister.List()`
  decompresses each cached entry before converting it to a typed `Workflow`.
  This is the single informer constructor shared by the workflow-controller,
  the cron controller and the GC controller, so one change covers all three.
- **`workflow/controller/informer/tolerant_workflow_template_informer.go`**
  and **`tolerant_cluster_workflow_template_informer.go`** — chained
  `compress.Transform` onto the WorkflowTemplate/ClusterWorkflowTemplate
  dynamic informers the same way.
- **`workflow/cron/controller.go`** — chained `compress.Transform` onto the
  CronWorkflow informer; updated `processNextCronItem` and `syncAll` to
  decompress via `compress.ToUnstructured` before converting to a typed
  `CronWorkflow` (they need the full spec); updated the `FilterFunc` in
  `addCronWorkflowInformerHandler` to use `meta.Accessor` since it only reads
  labels.

### Conversion / consumer sites

These previously did an unchecked or `ok`-guarded type assertion to
`*unstructured.Unstructured`. Since the cache now stores `*compress.Object`,
every one of these had to either switch to `meta.Accessor` (when only
metadata is needed) or `compress.ToUnstructured` (when the full spec/status
is needed) — otherwise the assertion always fails/panics and the code path
silently breaks.

- **`workflow/controller/informer/workflow_template_convert.go`** and
  **`cluster_workflow_template_convert.go`** — `interfaceToWorkflowTemplate`/
  `interfaceToClusterWorkflowTemplate` (the single choke point every
  WorkflowTemplate/ClusterWorkflowTemplate lister call goes through, server
  and controller side) now decompress via `compress.ToUnstructured` before
  unmarshalling into the typed object.
- **`workflow/controller/informer/workflow_template_convert_test.go`** and
  **`cluster_workflow_template_convert_test.go`** — updated the expected
  error string for the "not an unstructured" case to mention
  `*compress.Object` as an accepted type too.
- **`workflow/controller/controller.go`** — updated every place that read
  from `wfc.wfInformer`: `notifySemaphoreConfigUpdate`,
  `deleteOffloadedNodesForWorkflow`, `processNextItem`,
  `processNextArchiveItem`, `getWfPriority`, the `FilterFunc`/`AddFunc`/
  `UpdateFunc`/`DeleteFunc` handlers registered in
  `addWorkflowInformerHandlers` (three separate `AddEventHandler` calls),
  `archiveWorkflowAux`, and `releaseAllWorkflowLocks`. Metadata-only paths
  (resource-version comparisons, label checks, name/namespace lookups, UID
  for metrics cleanup) now use `meta.Accessor`; paths that need
  `spec`/`status` (priority lookup, workflow hydration/archiving, lock
  release) decompress via `compress.ToUnstructured`.
- **`workflow/controller/estimation/estimator_factory.go`** — the "find the
  newest succeeded sibling workflow" scan now uses `meta.Accessor` for the
  label/creation-timestamp comparison across all candidates, and only
  decompresses the single winning object — avoiding decompressing every
  candidate just to pick one.
- **`workflow/controller/healthz.go`** — the unreconciled-workflow scan
  decompresses each cached entry via `compress.ToUnstructured` before
  converting to a typed `Workflow`.
- **`workflow/controller/pod/controller.go`** — `podOrphaned` decompresses
  the owning workflow looked up from the informer before checking
  `DeletionTimestamp`. This one was initially missed and only surfaced as a
  test failure (`workflow is not an unstructured` warning breaking pod
  orphan detection), fixed after running the test suite.
- **`workflow/controller/indexes/conditions_index.go`** — `ConditionsIndexFunc`
  no longer does an unchecked `obj.(*unstructured.Unstructured)` (which would
  now panic on every call, since the cache holds `*compress.Object`); it
  delegates to the updated `GetConditions`, which handles decompression.
- **`util/unstructured/workflow/conditions.go`** — `GetConditions` now takes
  `any` instead of `*unstructured.Unstructured` and decompresses via
  `compress.ToUnstructured` internally, since it needs `status.conditions`.
- **`workflow/controller/indexes/workflow_index.go`** —
  `WorkflowSemaphoreKeysIndexFunc` checks the `completed` label via
  `meta.Accessor` first (cheap, runs on every index update) and only
  decompresses (`compress.ToUnstructured` + `util.FromUnstructured`) for
  workflows that aren't already completed.
- **`workflow/common/util.go`** — `IsDone` changed from taking
  `*unstructured.Unstructured` to the `metav1.Object` interface. It only ever
  reads `DeletionTimestamp`/labels, so this lets every caller pass either a
  raw unstructured object or a `meta.Accessor` result without decompressing.
- **`workflow/common/common.go`** — `UnstructuredHasCompletedLabel` switched
  from a type assertion to `meta.Accessor` for the same reason (metadata-only
  check); this function turned out to be unused outside its own test, but was
  updated for consistency with `IsDone`.
- **`workflow/gccontroller/gc_controller.go`** — the TTL and retention event
  handlers' `FilterFunc`s, `retentionEnqueue`, and `deleteWorkflow`'s
  "still completed?" check now use `meta.Accessor` (all metadata-only); only
  `enqueueWF` decompresses, since it needs `status.phase`/TTL fields to
  compute the deletion time.
- **`workflow/gccontroller/heap.go`** — the GC priority heap's element type
  changed from `*unstructured.Unstructured` to `metav1.Object`. It only ever
  reads `GetCreationTimestamp()`/`GetName()`, so with `retentionEnqueue`
  above now passing it a `meta.Accessor` result, the entire GC retention path
  never has to decompress a workflow at all.
- **`workflow/gccontroller/heap_test.go`** — updated the two direct `gcHeap{}`
  struct literals to use `[]metav1.Object` instead of
  `[]*unstructured.Unstructured` to match the field type change (the stored
  concrete type is still `*unstructured.Unstructured` in these tests, so the
  existing `.( *unstructured.Unstructured)` assertions on `Pop()` results
  still work).

## Validation

The repo has a pre-existing, unrelated `go.mod`/`vendor/modules.txt`
inconsistency that blocks the default `go build`/`test`. All changes were
validated with `GOFLAGS=-mod=mod go build|vet|test ./...` (confirmed via
`git status` that this does not modify `go.mod`/`go.sum`/`vendor`):

- `go build ./...` and `go vet ./...` — clean, no errors.
- Full test suite for every touched package (`util/compress`,
  `util/informer`, `util/unstructured/...`, `workflow/common`,
  `workflow/util`, `workflow/controller/...`, `workflow/gccontroller`,
  `workflow/cron`, `server/workflowtemplate`, `server/clusterworkflowtemplate`)
  passes. The only failures encountered (`TestArtifactPluginSidecar` and
  related sidecar tests) are pre-existing/environmental — they fail trying
  to look up a container image's entrypoint from a registry index that isn't
  reachable in this sandbox, unrelated to this change.
