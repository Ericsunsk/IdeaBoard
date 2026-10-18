package boards

// PostgreSQL parity for the unified access matrix (issue #934, criteria
// 2+5): the same behavior matrix runs unchanged on the postgres store.
// Skips without API_TEST_DATABASE_DSN like every other PG integration
// test. Repository methods keep context.Context first and the
// dbWithContext pattern (see postgres_workspace.go).

import (
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"
)

func TestAccessMatrixPostgres(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	ws, err := tenancypg.NewStore(db).CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-access-matrix-pg",
		OwnerUserID: matrixCreator,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	seedBoardTestMembers(t, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.InternalID, matrixEditor, matrixViewer, matrixStranger, matrixMember, matrixWSOwner, matrixWSAdmin)
	exerciseAccessMatrix(t, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.PublicID)
}
