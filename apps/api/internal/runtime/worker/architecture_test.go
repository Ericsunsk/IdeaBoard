package worker

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

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
}

func TestWorkerCollaboratesThroughTaskingPortsOnly(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenWorkerTaskingImports)
}
