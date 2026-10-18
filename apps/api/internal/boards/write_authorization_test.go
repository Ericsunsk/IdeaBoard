package boards

import (
	"context"
	"errors"
	"testing"
)

func exerciseWriteAuthorization(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	for _, state := range []struct {
		name   string
		revoke bool
		want   error
	}{
		{"demoted", false, ErrPermissionDenied},
		{"revoked", true, ErrNotFound},
	} {
		for _, operation := range []string{"document", "rename", "archive", "restore"} {
			t.Run(state.name+"/"+operation, func(t *testing.T) {
				id := mustMatrixBoard(t, ctx, store, workspaceID, "Write authorization", matrixCreator)
				if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccess(RoleEditor), []int64{matrixEditor}); err != nil {
					t.Fatal(err)
				}
				role, visible, err := store.GetBoardRole(ctx, workspaceID, id, matrixEditor, "member")
				if err != nil || CheckCapability(role, visible, CapWrite) != nil {
					t.Fatalf("preflight: %s %v %v", role, visible, err)
				}
				if state.revoke {
					err = func() error {
						_, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessNone, nil)
						return err
					}()
				} else {
					_, err = store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccess(RoleViewer), []int64{matrixEditor})
				}
				if err != nil {
					t.Fatal(err)
				}
				if operation == "restore" {
					if _, err := store.ArchiveBoard(ctx, workspaceID, id, matrixCreator); err != nil {
						t.Fatal(err)
					}
				}
				switch operation {
				case "document":
					_, err = store.ReplaceDocument(ctx, workspaceID, id, matrixEditor, 1, 1, V1InitialDocument())
				case "rename":
					_, err = store.RenameBoard(ctx, workspaceID, id, "Unauthorized", matrixEditor)
				case "archive":
					_, err = store.ArchiveBoard(ctx, workspaceID, id, matrixEditor)
				case "restore":
					_, err = store.RestoreBoard(ctx, workspaceID, id, matrixEditor)
				}
				if !errors.Is(err, state.want) {
					t.Fatalf("write error = %v, want %v", err, state.want)
				}
			})
		}
	}
}

func TestWriteAuthorizationInMemory(t *testing.T) {
	exerciseWriteAuthorization(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_write_authorization")
}

func exerciseCatalogRoleMerge(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	id := mustMatrixBoard(t, ctx, store, workspaceID, "Role merge", matrixCreator)
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccess(RoleViewer), []int64{matrixViewer}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessEditor, []int64{matrixViewer}); err != nil {
		t.Fatal(err)
	}
	catalog, err := store.ListBoardCatalog(ctx, workspaceID, false, matrixViewer, "member", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog.Entries {
		if entry.PublicID == id {
			if entry.EffectiveRole != RoleEditor || !entry.HasDirectGrant {
				t.Fatalf("role = %q, direct grant = %v; want editor, true", entry.EffectiveRole, entry.HasDirectGrant)
			}
			return
		}
	}
	t.Fatal("catalog omitted authorized board")
}

func TestCatalogRoleMergeInMemory(t *testing.T) {
	exerciseCatalogRoleMerge(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_catalog_role_merge")
}
