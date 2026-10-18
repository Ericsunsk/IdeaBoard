package boards

import (
	"context"
	"fmt"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
)

type testWorkspaceMembers struct{}

func (testWorkspaceMembers) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("board-test-user-%d", userID))), nil
}

func (testWorkspaceMembers) WithWorkspaceMembers(ctx context.Context, _ string, action func(func(int64) string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return action(func(userID int64) string {
		if userID <= 0 {
			return ""
		}
		return "member"
	})
}

func testDocumentAssetValidator(context.Context, *dbsqlc.Queries, string, int64, DocumentMediaAccess, []string) error {
	return nil
}

func seedBoardTestMembers(t interface {
	Helper()
	Fatal(...any)
}, store *PostgresWorkspaceBoardStore, workspaceID int64, users ...int64) {
	t.Helper()
	for _, userID := range users {
		if err := store.q.UpsertWorkspaceMembership(context.Background(), dbsqlc.UpsertWorkspaceMembershipParams{WorkspaceID: workspaceID, UserID: userID, Role: "member"}); err != nil {
			t.Fatal(err)
		}
	}
}

func cleanupMemberGrantsForTests(ctx context.Context, store WorkspaceBoardStore, workspaceID string, userID int64) ([]tenancy.MemberGrantCleanup, error) {
	if postgres, ok := store.(*PostgresWorkspaceBoardStore); ok {
		tx, err := postgres.db.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(context.Background())
		result, err := store.RemoveWorkspaceMemberGrants(dbutil.ContextWithAmbientTx(ctx, tx), workspaceID, userID)
		if err != nil {
			return nil, err
		}
		return result, tx.Commit(ctx)
	}
	return store.RemoveWorkspaceMemberGrants(ctx, workspaceID, userID)
}
