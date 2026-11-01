package wiring

import (
	"ideaboard/api/internal/platform/errorrender"
	"strings"
	"testing"

	taskworker "ideaboard/api/internal/tasking/interfaces/worker"
)

// Base pairing test for workspace_worker_adapter.go.
// A nil executor must fail closed with the user-facing copy from
// errors.go, never with a nil panic or an unwrapped error.
func TestWorkspaceWorkerCallerAdapterNilFailsClosed(t *testing.T) {
	var adapter *workspaceWorkerCallerAdapter
	_, err := adapter.CallWorkerAPI(t.Context(), taskworker.WorkspaceWorkerCallInput{})
	if err == nil {
		t.Fatal("nil adapter call succeeded, want fail-closed error")
	}
	if !strings.Contains(err.Error(), errorrender.MsgRuntimeCompositionWiringCallWorkerAPI) {
		t.Fatalf("nil adapter error = %q, want message %q", err.Error(), errorrender.MsgRuntimeCompositionWiringCallWorkerAPI)
	}
}
