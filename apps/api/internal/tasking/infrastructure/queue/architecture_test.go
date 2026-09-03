package queue

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// Taskqueue owns the River/capacity/lease adapter seam. It may depend on the
// tasks domain and platform vocabulary, but never on sibling application
// packages or the Runtime worker implementation.
var forbiddenTaskqueueImports = []string{
	"ideaboard/api/internal/runtime",
	"ideaboard/api/internal/tasking/application/task",
	"ideaboard/api/internal/tasking/infrastructure/postgres",
	"ideaboard/api/internal/tasking/application/runtime",
	"ideaboard/api/internal/tasking/application/statistics",
	"ideaboard/api/internal/tasking/application/submission",
	"ideaboard/api/internal/tasking/application/billing",
	"ideaboard/api/internal/tasking/ports/media",
}

func TestTaskqueueDoesNotDependOnSiblingsOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenTaskqueueImports)
}
