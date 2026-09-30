package rbac

import (
	"github.com/argoproj/argo-workflows/v4/server/types"
)

// Action describes the Argo-level resource and verb semantics of a gRPC method, used as the
// (res, act) pair in a fine-grained policy check. Unlike Kubernetes RBAC verbs, act is not
// limited to get/list/create/update/delete: distinct workflow operations that all map to the
// Kubernetes verb "update" (resume, retry, suspend, terminate, stop, resubmit) get their own
// action name so policies can distinguish them.
type Action struct {
	Resource string
	Verb     string
}

// methodActions maps a gRPC full method name ("/<proto package>.<Service>/<Method>") to its
// (resource, action) pair. Methods not present here are denied when RBAC is enabled; add new
// entries here rather than guessing a mapping at request time.
var methodActions = map[string]Action{
	"/workflow.WorkflowService/CreateWorkflow":    {"workflows", "create"},
	"/workflow.WorkflowService/GetWorkflow":       {"workflows", "get"},
	"/workflow.WorkflowService/ListWorkflows":     {"workflows", "list"},
	"/workflow.WorkflowService/WatchWorkflows":    {"workflows", "watch"},
	"/workflow.WorkflowService/WatchEvents":       {"events", "watch"},
	"/workflow.WorkflowService/DeleteWorkflow":    {"workflows", "delete"},
	"/workflow.WorkflowService/RetryWorkflow":     {"workflows", "retry"},
	"/workflow.WorkflowService/ResubmitWorkflow":  {"workflows", "resubmit"},
	"/workflow.WorkflowService/ResumeWorkflow":    {"workflows", "resume"},
	"/workflow.WorkflowService/SuspendWorkflow":   {"workflows", "suspend"},
	"/workflow.WorkflowService/TerminateWorkflow": {"workflows", "terminate"},
	"/workflow.WorkflowService/StopWorkflow":      {"workflows", "stop"},
	"/workflow.WorkflowService/SetWorkflow":       {"workflows", "update"},
	"/workflow.WorkflowService/LintWorkflow":      {"workflows", "create"},
	"/workflow.WorkflowService/PodLogs":           {"workflows", "logs"},
	"/workflow.WorkflowService/WorkflowLogs":      {"workflows", "logs"},
	"/workflow.WorkflowService/SubmitWorkflow":    {"workflows", "create"},

	"/cronworkflow.CronWorkflowService/LintCronWorkflow":    {"cronworkflows", "create"},
	"/cronworkflow.CronWorkflowService/CreateCronWorkflow":  {"cronworkflows", "create"},
	"/cronworkflow.CronWorkflowService/ListCronWorkflows":   {"cronworkflows", "list"},
	"/cronworkflow.CronWorkflowService/GetCronWorkflow":     {"cronworkflows", "get"},
	"/cronworkflow.CronWorkflowService/UpdateCronWorkflow":  {"cronworkflows", "update"},
	"/cronworkflow.CronWorkflowService/DeleteCronWorkflow":  {"cronworkflows", "delete"},
	"/cronworkflow.CronWorkflowService/ResumeCronWorkflow":  {"cronworkflows", "resume"},
	"/cronworkflow.CronWorkflowService/SuspendCronWorkflow": {"cronworkflows", "suspend"},

	"/workflowtemplate.WorkflowTemplateService/CreateWorkflowTemplate": {"workflowtemplates", "create"},
	"/workflowtemplate.WorkflowTemplateService/GetWorkflowTemplate":    {"workflowtemplates", "get"},
	"/workflowtemplate.WorkflowTemplateService/ListWorkflowTemplates":  {"workflowtemplates", "list"},
	"/workflowtemplate.WorkflowTemplateService/UpdateWorkflowTemplate": {"workflowtemplates", "update"},
	"/workflowtemplate.WorkflowTemplateService/DeleteWorkflowTemplate": {"workflowtemplates", "delete"},
	"/workflowtemplate.WorkflowTemplateService/LintWorkflowTemplate":   {"workflowtemplates", "create"},

	"/clusterworkflowtemplate.ClusterWorkflowTemplateService/CreateClusterWorkflowTemplate": {"clusterworkflowtemplates", "create"},
	"/clusterworkflowtemplate.ClusterWorkflowTemplateService/GetClusterWorkflowTemplate":    {"clusterworkflowtemplates", "get"},
	"/clusterworkflowtemplate.ClusterWorkflowTemplateService/ListClusterWorkflowTemplates":  {"clusterworkflowtemplates", "list"},
	"/clusterworkflowtemplate.ClusterWorkflowTemplateService/UpdateClusterWorkflowTemplate": {"clusterworkflowtemplates", "update"},
	"/clusterworkflowtemplate.ClusterWorkflowTemplateService/DeleteClusterWorkflowTemplate": {"clusterworkflowtemplates", "delete"},
	"/clusterworkflowtemplate.ClusterWorkflowTemplateService/LintClusterWorkflowTemplate":   {"clusterworkflowtemplates", "create"},

	"/workflowarchive.ArchivedWorkflowService/ListArchivedWorkflows":           {"workflows", "list"},
	"/workflowarchive.ArchivedWorkflowService/GetArchivedWorkflow":             {"workflows", "get"},
	"/workflowarchive.ArchivedWorkflowService/DeleteArchivedWorkflow":          {"workflows", "delete"},
	"/workflowarchive.ArchivedWorkflowService/ListArchivedWorkflowLabelKeys":   {"workflows", "list"},
	"/workflowarchive.ArchivedWorkflowService/ListArchivedWorkflowLabelValues": {"workflows", "list"},
	"/workflowarchive.ArchivedWorkflowService/RetryArchivedWorkflow":           {"workflows", "retry"},
	"/workflowarchive.ArchivedWorkflowService/ResubmitArchivedWorkflow":        {"workflows", "resubmit"},

	"/sync.SyncService/CreateSyncLimit": {"sync", "create"},
	"/sync.SyncService/GetSyncLimit":    {"sync", "get"},
	"/sync.SyncService/UpdateSyncLimit": {"sync", "update"},
	"/sync.SyncService/DeleteSyncLimit": {"sync", "delete"},
}

// ActionFor returns the (resource, action) pair registered for a gRPC full method name
// (as found on grpc.UnaryServerInfo.FullMethod / grpc.StreamServerInfo.FullMethod).
func ActionFor(fullMethod string) (Action, bool) {
	a, ok := methodActions[fullMethod]
	return a, ok
}

type namedRequest interface {
	GetName() string
}

// ObjectFor derives the "namespace/name" object string for a request, matching the
// {namespace}/{name} shape used in policy object patterns (e.g. "team-a/*"). Either
// component may be empty, e.g. for list/watch requests which have no name.
func ObjectFor(req any) string {
	namespace := ""
	if nr, ok := req.(types.NamespacedRequest); ok {
		namespace = nr.GetNamespace()
	}
	name := ""
	if nr, ok := req.(namedRequest); ok {
		name = nr.GetName()
	}
	return namespace + "/" + name
}
