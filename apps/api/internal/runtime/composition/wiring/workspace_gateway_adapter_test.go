package wiring

import (
	"ideaboard/api/internal/platform/errorrender"
	"strings"
	"testing"

	taskworker "ideaboard/api/internal/tasking/interfaces/worker"
)

// Base pairing test for workspace_gateway_adapter.go.
// A nil executor must fail closed with the user-facing copy from
// errors.go, never with a nil panic or an unwrapped error.
func TestWorkspaceGatewayCallerAdapterNilFailsClosed(t *testing.T) {
	var adapter *workspaceGatewayCallerAdapter
	_, err := adapter.CallGatewayAPI(t.Context(), taskworker.WorkspaceGatewayCallInput{})
	if err == nil {
		t.Fatal("nil adapter call succeeded, want fail-closed error")
	}
	if !strings.Contains(err.Error(), errorrender.MsgRuntimeCompositionWiringCallGatewayAPI) {
		t.Fatalf("nil adapter error = %q, want message %q", err.Error(), errorrender.MsgRuntimeCompositionWiringCallGatewayAPI)
	}
}
