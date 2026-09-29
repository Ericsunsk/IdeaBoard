package workspacetasks

// WorkspaceTaskArgs is the River job payload for workspace-scoped task
// execution. The workspace worker (#825, workspace_task_worker.go) consumes
// this kind on the shared drawing queue. The payload carries only opaque
// identifiers (no bytes, no secrets).
//
// It lives in this shared kernel (not in an infrastructure adapter) so the
// enqueue side and the worker side agree on one kind and one shape without
// either importing the other's package. The Kind string is the River
// contract: renaming it strands in-flight jobs.
type WorkspaceTaskArgs struct {
	TaskID         string `json:"task_id"`
	WorkspaceID    string `json:"workspace_id"`
	Service        string `json:"service"`
	ExecutionToken string `json:"execution_token"`
	TraceParent    string `json:"traceparent,omitempty"`
}

// Kind implements river.JobArgs (structural: the River client accepts any
// value with this method, so this package stays free of River imports).
func (WorkspaceTaskArgs) Kind() string { return "workspace_task_execute" }
