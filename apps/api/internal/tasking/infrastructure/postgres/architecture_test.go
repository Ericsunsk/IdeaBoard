package postgres

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// Taskstore is the persistence adapter. It may serve every consumer seam, but
// it must never depend back on application facades or the Runtime worker
// implementation.
var forbiddenTaskstoreImports = []string{
	"ideaboard/api/internal/runtime",
	"ideaboard/api/internal/tasking/application/task",
	"ideaboard/api/internal/tasking/application/billing",
}

func TestTaskstoreDoesNotDependOnApplicationOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenTaskstoreImports)
}
