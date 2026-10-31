package queue

// WorkspaceTaskArgs is the River job payload for workspace-scoped task
// execution. The workspace worker consumes this kind on the shared drawing queue.
// The payload carries only opaque identifiers (no bytes, no secrets).
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

// WorkspaceTaskJobArgs is an alias for WorkspaceTaskArgs.
type WorkspaceTaskJobArgs = WorkspaceTaskArgs
