package worker

import (
	"ideaboard/api/internal/archtest"
	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	runtimeports "ideaboard/api/internal/runtime/ports"
	"testing"
)

func TestProviderExecutorImplementsRuntimePort(t *testing.T) {
	var _ runtimeports.GatewayCaller = (*providers.Executor)(nil)
}

// The worker implementation collaborates with Tasking exclusively through
// task, execution, queue and media contracts (tasks, taskexecution, taskqueue, taskmedia).
// It must never reach into persistence, facades or sibling application
// packages; those seams belong to Tasking and Runtime wiring.
var forbiddenWorkerTaskingImports = []string{
	"ideaboard/api/internal/tasking/infrastructure/postgres",
	"ideaboard/api/internal/tasking/application/task",
	"ideaboard/api/internal/tasking/application/submission",
	"ideaboard/api/internal/tasking/application/billing",
	"ideaboard/api/internal/tasking/application/statistics",
	"ideaboard/api/internal/tasking/application/retention",
}

func TestWorkerCollaboratesThroughTaskingPortsOnly(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenWorkerTaskingImports)
}

func TestWorkerSubpackagesIsolation(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, "providers", []string{
		"ideaboard/api/internal/runtime/interfaces/worker/river",
		"ideaboard/api/internal/runtime/interfaces/worker/billing",
	})
	archtest.AssertNoForbiddenImports(t, "billing", []string{
		"ideaboard/api/internal/runtime/interfaces/worker/providers",
		"ideaboard/api/internal/runtime/interfaces/worker/river",
		"ideaboard/api/internal/runtime/interfaces/worker/retention",
	})
	archtest.AssertNoForbiddenImports(t, "retention", []string{
		"ideaboard/api/internal/runtime/interfaces/worker/providers",
		"ideaboard/api/internal/runtime/interfaces/worker/river",
		"ideaboard/api/internal/runtime/interfaces/worker/billing",
	})
}
