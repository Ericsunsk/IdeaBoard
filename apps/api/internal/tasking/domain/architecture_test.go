package domain

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// Tasks owns domain facts only: Task, payload, status, identity, manifest and
// validation invariants. It must not depend on adapters (SQL, River, HTTP,
// provider APIs) or platform/runtime infrastructure.
var forbiddenTasksImports = []string{
	"ideaboard/api/internal/platform",
	"ideaboard/api/internal/db",
	"ideaboard/api/internal/runtime",
	"ideaboard/api/internal/tasking/",
	"github.com/riverqueue",
	"github.com/jackc/pgx",
	"github.com/gin-gonic",
}

func TestTasksPackageHasNoAdapterImports(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenTasksImports)
}
