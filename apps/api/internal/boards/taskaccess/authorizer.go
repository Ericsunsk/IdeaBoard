package taskaccess

import (
	"context"
	"strings"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// Reader provides live Board capability facts to Tasking.
type Reader interface {
	GetBoardRole(context.Context, string, string, int64, string) (boards.AccessRole, bool, error)
	GetBoard(context.Context, string, string) (*boards.V1Board, error)
}

// Authorizer projects canonical Board capabilities onto Tasking's access port.
type Authorizer struct {
	Store Reader
}

var _ workspacetasks.BoardAuthorizer = (*Authorizer)(nil)

// ResolveBoardAccess implements workspacetasks.BoardAuthorizer.
func (a *Authorizer) ResolveBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (*workspacetasks.BoardAccess, error) {
	if a == nil || a.Store == nil {
		return nil, workspacetasks.ErrBoardNotFound
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	boardPublicID = strings.TrimSpace(boardPublicID)
	if !contract.IsBoardID(boardPublicID) {
		return nil, workspacetasks.ErrBoardNotFound
	}
	role, ok, err := a.Store.GetBoardRole(ctx, workspacePublicID, boardPublicID, userID, workspaceRole)
	if err != nil || !ok {
		return nil, workspacetasks.ErrBoardNotFound
	}
	board, err := a.Store.GetBoard(ctx, workspacePublicID, boardPublicID)
	if err != nil {
		return nil, workspacetasks.ErrBoardNotFound
	}
	archived := board.ArchivedAt != nil
	caps := boards.CapabilitiesFor(role, true, archived)
	return &workspacetasks.BoardAccess{
		BoardID: boardPublicID, IsOwner: role == boards.RoleOwner,
		CanRead: caps.CanRead, CanGenerate: caps.CanGenerate, CanWrite: caps.CanWrite,
		Archived: archived, AccessVersion: board.AccessVersion,
	}, nil
}
