package httpapi

// Workspace-rooted boards + collaboration handlers (issue #802).
//
// Additive only: all routes mount under
// /api/v1/workspaces/{workspace_id}/boards via the existing
// ContractV1GroupPath + PublicContractChain pattern. Legacy
// /api/user|/api/admin board routes were removed in #809.
//
// Auth model: ResolvePrincipal + RequireWorkspaceCapability for the explicit
// workspace context, then the board ACL (roles owner|editor|viewer).
// The board creator/implicit owner row counts as owner; visible workspace
// members default to viewer (workspace owners to owner) and explicit board
// rows override the default. Outsiders (no workspace membership and no
// board role) converge to 404 board_not_found; visible-but-forbidden
// callers get 403 board_forbidden.
//
// Envelope: contract.RenderSuccess/RenderPaginated/RenderError; cursor
// pagination cursor/page_size; POST/PUT/DELETE require the Idempotency-Key
// header-only scope board:<op>; concurrency via ETag/If-Match over the
// revision counter (stale -> 409 board_revision_conflict).
//
// Production boards surface: the postgres workspace-board mapping
// (board_documents/board_revisions) is live since the #809 follow-up;
// ContractV1Deps.Boards is wired in composition/http_wiring.go.
// Tests inject boards.NewInMemoryWorkspaceBoardStoreForTests().

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/boards/previewscene"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// registerBoardRoutes mounts the workspace-rooted board surface on group.
// It is called from RegisterContractV1Routes (contract_v1.go).
func (h *contractV1Handler) registerBoardRoutes(group *gin.RouterGroup) {
	group.GET("/workspaces/:workspace_id/boards", h.listBoards)
	group.POST("/workspaces/:workspace_id/boards", h.createBoard)
	group.POST("/workspaces/:workspace_id/boards/join", h.joinBoardByToken)
	group.GET("/workspaces/:workspace_id/boards/:board_id", h.getBoard)
	group.PATCH("/workspaces/:workspace_id/boards/:board_id", h.renameBoard)
	group.POST("/workspaces/:workspace_id/boards/:board_id/archive", h.archiveBoard)
	group.POST("/workspaces/:workspace_id/boards/:board_id/restore", h.restoreBoard)
	group.GET("/workspaces/:workspace_id/boards/:board_id/document", h.getBoardDocument)
	group.PUT("/workspaces/:workspace_id/boards/:board_id/document", h.replaceBoardDocument)
	group.GET("/workspaces/:workspace_id/boards/:board_id/revisions", h.listBoardRevisions)
	group.GET("/workspaces/:workspace_id/boards/:board_id/revisions/:revision_id", h.getBoardRevision)
	group.GET("/workspaces/:workspace_id/boards/:board_id/members", h.listBoardMembers)
	group.POST("/workspaces/:workspace_id/boards/:board_id/members", h.addBoardMember)
	group.PATCH("/workspaces/:workspace_id/boards/:board_id/members/:member_id", h.updateBoardMember)
	group.DELETE("/workspaces/:workspace_id/boards/:board_id/members/:member_id", h.removeBoardMember)
	group.GET("/workspaces/:workspace_id/boards/:board_id/shares", h.listShareTokens)
	group.POST("/workspaces/:workspace_id/boards/:board_id/shares", h.createShareToken)
	group.DELETE("/workspaces/:workspace_id/boards/:board_id/shares/:token_id", h.revokeShareToken)
}

func (h *contractV1Handler) requireBoardsStore(c *gin.Context) bool {
	if h.deps.Boards == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 boards surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

// boardWorkspaceContext resolves the explicit workspace context and enforces
// workspace membership: hidden workspaces converge to 404
// workspace_not_found, non-members to the same 404. Visible members return
// their workspace role so board role resolution can apply the viewer/owner
// default without extra queries.
func (h *contractV1Handler) boardWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, string, bool) {
	workspace, role, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return nil, "", false
	}
	if !RequireWorkspaceCapability(c, visible, visible) {
		return nil, "", false
	}
	return workspace, role, true
}

// boardAccess resolves a board within a visible workspace and enforces the
// board ACL: malformed ids, unknown boards, and callers holding no
// effective board role (no explicit grant and no visible workspace default)
// converge to 404 board_not_found (Baseline 4).
func (h *contractV1Handler) boardAccess(c *gin.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64) (*boards.V1Board, boards.AccessRole, bool) {
	raw := strings.TrimSpace(c.Param("board_id"))
	if !contract.IsBoardID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
		c.Abort()
		return nil, "", false
	}
	ctx := c.Request.Context()
	board, err := h.deps.Boards.GetBoard(ctx, workspace.PublicID, raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
		c.Abort()
		return nil, "", false
	}
	role, ok, err := h.deps.Boards.GetBoardRole(ctx, workspace.PublicID, board.PublicID, userID, workspaceRole)
	if err != nil || !ok {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
		c.Abort()
		return nil, "", false
	}
	return board, role, true
}

// requireBoardRole renders 403 board_forbidden unless the caller holds one of
// the allowed board roles.
func requireBoardRole(c *gin.Context, role boards.AccessRole, allowed ...boards.AccessRole) bool {
	for _, candidate := range allowed {
		if role == candidate {
			return true
		}
	}
	contract.RenderError(c, http.StatusForbidden, contract.CodeBoardForbidden, msgForbidden)
	c.Abort()
	return false
}

func renderBoardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, boards.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
	case errors.Is(err, boards.ErrRevisionConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeBoardRevisionConflict, msgBoardRevisionConflict)
	case errors.Is(err, boards.ErrPermissionDenied):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBoardForbidden, msgForbidden)
	case errors.Is(err, boards.ErrV1Archived):
		contract.RenderError(c, contract.StatusOf(contract.CodeBoardArchived), contract.CodeBoardArchived, msgBoardArchived)
	case errors.Is(err, boards.ErrMemberNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardMemberNotFound, msgWorkspaceMemberNotFound)
	case errors.Is(err, boards.ErrMemberInviteSelf):
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgRenderBoardError, contract.FieldErrors{Fields: map[string]string{"user_id": "invalid"}})
	case errors.Is(err, boards.ErrMemberInvalidRole):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			msgRenderBoardError2, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
	case errors.Is(err, boards.ErrIDConflict):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable46)
	case errors.Is(err, boards.ErrV1LastOwner):
		contract.RenderError(c, contract.StatusOf(contract.CodeMembershipTransitionForbidden),
			contract.CodeMembershipTransitionForbidden, msgMembershipTransitionForbidden2)
	case errors.Is(err, boards.ErrShareTokenNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardShareTokenNotFound, msgBoardShareTokenNotFound)
	case errors.Is(err, boards.ErrShareTokenRevoked),
		errors.Is(err, boards.ErrShareTokenExpired),
		errors.Is(err, boards.ErrV1ShareTokenExpired),
		errors.Is(err, boards.ErrShareTokenInvalid):
		contract.RenderError(c, contract.StatusOf(contract.CodeBoardShareTokenInvalid),
			contract.CodeBoardShareTokenInvalid, msgBoardShareTokenInvalid)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable46)
	}
	c.Abort()
}

// boardDo runs fn under the header-only Idempotency-Key scope board:<op>:
// missing keys render 400 idempotency_key_required, key reuse with a
// different request hash renders 409 board_idempotency_conflict (aborted),
// same key+hash replays the first result. Domain errors from fn are returned
// for the caller to map (they must not collapse to 503 here).
func (h *contractV1Handler) boardDo(c *gin.Context, userExternalID string, userID int64, op, key, hash string, fn func() (any, error)) (any, error, bool) {
	scope := contract.IdempotencyScope(userExternalID, "board:"+op)
	result, _, err := h.deps.Idempotency.Do(c.Request.Context(), scope, userID, key, hash, fn)
	if err != nil {
		if contract.IsIdempotencyConflict(err) {
			contract.RenderError(c, http.StatusConflict, contract.CodeBoardIdempotencyConflict, msgWorkspaceIdempotencyConflict)
			c.Abort()
			return nil, nil, false
		}
		return nil, err, true
	}
	return result, nil, true
}

// renderJoinError converges every join failure to 404
// board_share_token_not_found (unknown, revoked, expired, or
// foreign-workspace bearers never leak existence); only malformed input is
// 400.
func renderJoinError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, boards.ErrInvalidInput):
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgRenderJoinError, contract.FieldErrors{Fields: map[string]string{"token": "invalid"}})
	case errors.Is(err, boards.ErrShareTokenNotFound),
		errors.Is(err, boards.ErrShareTokenRevoked),
		errors.Is(err, boards.ErrShareTokenExpired),
		errors.Is(err, boards.ErrV1ShareTokenExpired),
		errors.Is(err, boards.ErrShareTokenInvalid),
		errors.Is(err, boards.ErrNotFound),
		errors.Is(err, boards.ErrV1Archived):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardShareTokenNotFound, msgBoardShareTokenNotFound)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable46)
	}
	c.Abort()
}

// parseBoardIfMatch requires the If-Match precondition for document writes:
// missing/empty/"*"/unparsable values render 400, stale values surfacing as
// 409 happens at ReplaceDocument time.
func parseBoardIfMatch(c *gin.Context) (int64, bool) {
	raw := contract.IfMatch(c)
	if strings.TrimSpace(raw) == "" {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgParseBoardIfMatch, contract.FieldErrors{Fields: map[string]string{"if_match": "required"}})
		c.Abort()
		return 0, false
	}
	revision, err := boards.ParseV1IfMatch(raw)
	if err != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgParseBoardIfMatch2, contract.FieldErrors{Fields: map[string]string{"if_match": "invalid"}})
		c.Abort()
		return 0, false
	}
	return revision, true
}

// logBoardAudit emits the operator/target/reason/outcome tuple for share and
// member mutations. Secrets (bearers) are never logged: share targets use
// the list-safe prefix. #808 persists these as durable audit records.
func logBoardAudit(op string, operatorUserID int64, target, reason, outcome string) {
	slog.Info("board v1 audit",
		"op", op,
		"operator_user_id", operatorUserID,
		"target", target,
		"reason", reason,
		"outcome", outcome,
	)
}

type v1BoardDTO struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	Name           string          `json:"name"`
	Revision       int64           `json:"revision"`
	SchemaVersion  int32           `json:"schema_version"`
	ContentVersion string          `json:"content_version,omitempty"`
	PreviewScene   json.RawMessage `json:"preview_scene,omitempty"`
	ArchivedAt     *string         `json:"archived_at,omitempty"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

func toBoardDTO(board *boards.V1Board) v1BoardDTO {
	dto := v1BoardDTO{
		ID:             board.PublicID,
		WorkspaceID:    board.WorkspacePublicID,
		Name:           board.Name,
		Revision:       board.Revision,
		SchemaVersion:  board.SchemaVersion,
		ContentVersion: board.ContentVersion,
		PreviewScene:   board.PreviewScene,
		CreatedAt:      v1FormatTime(board.CreatedAt),
		UpdatedAt:      v1FormatTime(board.UpdatedAt),
	}
	if board.ArchivedAt != nil {
		archived := v1FormatTime(*board.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	return dto
}

// toBoardCatalogDTO renders one Gallery catalog card: metadata plus the
// stored scene projection, never the full document.
func toBoardCatalogDTO(entry *boards.V1BoardCatalogEntry) v1BoardDTO {
	dto := v1BoardDTO{
		ID:             entry.PublicID,
		WorkspaceID:    entry.WorkspacePublicID,
		Name:           entry.Name,
		Revision:       entry.Revision,
		SchemaVersion:  entry.SchemaVersion,
		ContentVersion: entry.ContentVersion,
		PreviewScene:   entry.PreviewScene,
		CreatedAt:      v1FormatTime(entry.CreatedAt),
		UpdatedAt:      v1FormatTime(entry.UpdatedAt),
	}
	if entry.ArchivedAt != nil {
		archived := v1FormatTime(*entry.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	return dto
}

func boardPageRequest(c *gin.Context) (boards.V1BoardCatalogPageRequest, bool) {
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest31)
		c.Abort()
		return boards.V1BoardCatalogPageRequest{}, false
	}
	page := boards.V1BoardCatalogPageRequest{Limit: pageSize}
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
			renderBoardCursorInvalid(c)
			return boards.V1BoardCatalogPageRequest{}, false
		}
		page.Offset = cursor.Offset
	}
	return page, true
}

func renderBoardCursorInvalid(c *gin.Context) {
	contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid3)
	c.Abort()
}

func boardPageMeta(c *gin.Context, nextOffset int, hasMore bool) (contract.PageMeta, bool) {
	if !hasMore {
		return contract.NewPageMeta("", false), true
	}
	next, err := contract.EncodeCursor(v1ListCursor{Offset: nextOffset})
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable46)
		c.Abort()
		return contract.PageMeta{}, false
	}
	return contract.NewPageMeta(next, true), true
}

func boardPageSlice[T any](items []T, c *gin.Context) ([]T, contract.PageMeta, bool) {
	page, ok := boardPageRequest(c)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	if page.Offset > len(items) {
		renderBoardCursorInvalid(c)
		return nil, contract.PageMeta{}, false
	}
	end := min(page.Offset+page.Limit, len(items))
	meta, ok := boardPageMeta(c, end, end < len(items))
	return items[page.Offset:end], meta, ok
}

func (h *contractV1Handler) listBoards(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		request, ok := boardPageRequest(c)
		if !ok {
			return
		}
		page, err := h.deps.Boards.ListBoardCatalog(c.Request.Context(), wctx.Workspace.PublicID, false, wctx.Principal.UserID(), wctx.Role, request)
		if errors.Is(err, boards.ErrV1CatalogOffset) {
			renderBoardCursorInvalid(c)
			return
		}
		if err != nil {
			renderBoardError(c, err)
			return
		}
		meta, ok := boardPageMeta(c, request.Offset+len(page.Entries), page.HasMore)
		if !ok {
			return
		}
		data := make([]v1BoardDTO, 0, len(page.Entries))
		for _, entry := range page.Entries {
			data = append(data, toBoardCatalogDTO(entry))
		}
		contract.RenderPaginated(c, data, meta)
	})
}

type v1CreateBoardBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) createBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateBoardBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		name := strings.TrimSpace(body.Data.Name)
		if err := boards.ValidateV1BoardName(name); err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgCreateBoard, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
			c.Abort()
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:create", workspace.PublicID, name)
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "create", key, hash, func() (any, error) {
			publicID, mintErr := boards.NewV1BoardPublicID()
			if mintErr != nil {
				return nil, mintErr
			}
			return h.deps.Boards.CreateBoard(c.Request.Context(), workspace.PublicID, identityPrincipal.UserID(), name, publicID)
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		created, ok := result.(*boards.V1Board)
		if !ok || created == nil {
			renderBoardError(c, errors.New("boards: create type mismatch"))
			return
		}
		logBoardAudit("board_create", identityPrincipal.UserID(), workspace.PublicID+"/"+created.PublicID, "board_create", "created")
		contract.SetETag(c, boards.FormatV1ETag(created.Revision))
		contract.RenderCreated(c, toBoardDTO(created))
	})
}

func (h *contractV1Handler) getBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(board.Revision))
		contract.RenderSuccess(c, toBoardDTO(board))
	})
}

type v1RenameBoardBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) renameBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner, boards.RoleEditor) {
			return
		}
		var body v1RenameBoardBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		name := strings.TrimSpace(body.Data.Name)
		if err := boards.ValidateV1BoardName(name); err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgCreateBoard, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
			c.Abort()
			return
		}
		renamed, err := h.deps.Boards.RenameBoard(c.Request.Context(), workspace.PublicID, board.PublicID, name)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(renamed.Revision))
		contract.RenderSuccess(c, toBoardDTO(renamed))
	})
}

func (h *contractV1Handler) archiveBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner, boards.RoleEditor) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:archive", workspace.PublicID, board.PublicID)
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "archive", key, hash, func() (any, error) {
			return h.deps.Boards.ArchiveBoard(c.Request.Context(), workspace.PublicID, board.PublicID)
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		archived, ok := result.(*boards.V1Board)
		if !ok || archived == nil {
			renderBoardError(c, errors.New("boards: archive type mismatch"))
			return
		}
		logBoardAudit("board_archive", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID, "board_archive", "archived")
		contract.RenderSuccess(c, toBoardDTO(archived))
	})
}

func (h *contractV1Handler) restoreBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner, boards.RoleEditor) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:restore", workspace.PublicID, board.PublicID)
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "restore", key, hash, func() (any, error) {
			return h.deps.Boards.RestoreBoard(c.Request.Context(), workspace.PublicID, board.PublicID)
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		restored, ok := result.(*boards.V1Board)
		if !ok || restored == nil {
			renderBoardError(c, errors.New("boards: restore type mismatch"))
			return
		}
		logBoardAudit("board_restore", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID, "board_restore", "restored")
		contract.RenderSuccess(c, toBoardDTO(restored))
	})
}

type v1BoardDocumentDTO struct {
	BoardID        string          `json:"board_id"`
	Revision       int64           `json:"revision"`
	SchemaVersion  int32           `json:"schema_version"`
	ContentVersion string          `json:"content_version,omitempty"`
	PreviewScene   json.RawMessage `json:"preview_scene,omitempty"`
	Document       json.RawMessage `json:"document"`
	UpdatedAt      string          `json:"updated_at"`
}

func toBoardDocumentDTO(board *boards.V1Board) v1BoardDocumentDTO {
	document := board.Document
	if len(document) == 0 {
		document = boards.V1InitialDocument()
	}
	dto := v1BoardDocumentDTO{
		BoardID:        board.PublicID,
		Revision:       board.Revision,
		SchemaVersion:  board.SchemaVersion,
		ContentVersion: board.ContentVersion,
		PreviewScene:   board.PreviewScene,
		Document:       document,
		UpdatedAt:      v1FormatTime(board.UpdatedAt),
	}
	if len(dto.PreviewScene) == 0 || dto.ContentVersion == "" {
		// Rows without a stored projection derive it
		// deterministically from the canonical document on read. Pure
		// function of the stored bytes — nothing is persisted here and
		// the content identity never advances on a read.
		version, scene, _ := previewscene.DeriveCatalog(board.Document)
		if len(dto.PreviewScene) == 0 && len(scene) > 0 {
			dto.PreviewScene = scene
		}
		if dto.ContentVersion == "" && version != "" {
			dto.ContentVersion = version
		}
	}
	return dto
}

func (h *contractV1Handler) getBoardDocument(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(board.Revision))
		contract.RenderSuccess(c, toBoardDocumentDTO(board))
	})
}

type v1ReplaceDocumentBody struct {
	Data struct {
		SchemaVersion int32           `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	} `json:"data"`
}

func (h *contractV1Handler) replaceBoardDocument(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner, boards.RoleEditor) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		expectedRevision, ok := parseBoardIfMatch(c)
		if !ok {
			return
		}
		var body v1ReplaceDocumentBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		// Re-encode through the strict discriminated-union validator so unknown
		// fields are rejected even though the envelope itself is strict-decoded.
		rawEnvelope, marshalErr := json.Marshal(map[string]any{
			"schema_version": body.Data.SchemaVersion,
			"payload":        body.Data.Payload,
		})
		if marshalErr != nil {
			contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
				msgReplaceBoardDocument, contract.FieldErrors{Fields: map[string]string{"document": "invalid"}})
			c.Abort()
			return
		}
		schemaVersion, _, validateErr := boards.ValidateV1DocumentStrict(rawEnvelope)
		if validateErr != nil {
			contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
				msgReplaceBoardDocument, contract.FieldErrors{Fields: map[string]string{"document": "invalid"}})
			c.Abort()
			return
		}
		// Canonical snapshot: re-marshal the validated envelope so stored bytes
		// are stable regardless of client whitespace.
		snapshot := json.RawMessage(rawEnvelope)
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:document-replace", workspace.PublicID, board.PublicID,
			strconv.FormatInt(expectedRevision, 10), string(rawEnvelope))
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "document-replace", key, hash, func() (any, error) {
			updated, replaceErr := h.deps.Boards.ReplaceDocument(c.Request.Context(), workspace.PublicID, board.PublicID,
				identityPrincipal.UserID(), expectedRevision, schemaVersion, snapshot)
			if replaceErr != nil {
				return nil, replaceErr
			}
			// #807: board.document_replaced AFTER the revision commit.
			h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, workspace.PublicID,
				events.EventV1BoardDocumentReplaced,
				events.V1Resource{Type: events.V1ResourceBoard, ID: board.PublicID},
				events.V1BoardDocumentReplacedData{BoardID: board.PublicID, Revision: updated.Revision})
			return updated, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		updated, ok := result.(*boards.V1Board)
		if !ok || updated == nil {
			renderBoardError(c, errors.New("boards: document type mismatch"))
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(updated.Revision))
		contract.RenderSuccess(c, toBoardDocumentDTO(updated))
	})
}

type v1BoardRevisionDTO struct {
	ID             int64  `json:"id"`
	Revision       int64  `json:"revision"`
	SchemaVersion  int32  `json:"schema_version"`
	SourceRevision int64  `json:"source_revision"`
	CreatedBy      string `json:"created_by,omitempty"`
	CreatedAt      string `json:"created_at"`
}

func (h *contractV1Handler) toBoardRevisionDTO(c *gin.Context, revision *boards.V1Revision) v1BoardRevisionDTO {
	dto := v1BoardRevisionDTO{
		ID:             revision.ID,
		Revision:       revision.Revision,
		SchemaVersion:  revision.SchemaVersion,
		SourceRevision: revision.SourceRevision,
		CreatedAt:      v1FormatTime(revision.CreatedAt),
	}
	if h.deps.Users != nil && revision.CreatedBy > 0 {
		if publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), revision.CreatedBy); err == nil && publicID != uuid.Nil {
			dto.CreatedBy = tenancy.FormatUserPublicID(publicID)
		}
	}
	return dto
}

func (h *contractV1Handler) listBoardRevisions(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		revisions, err := h.deps.Boards.ListRevisions(c.Request.Context(), workspace.PublicID, board.PublicID)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		page := make([]v1BoardRevisionDTO, 0, len(revisions))
		for _, revision := range revisions {
			page = append(page, h.toBoardRevisionDTO(c, revision))
		}
		slice, meta, ok := boardPageSlice(page, c)
		if !ok {
			return
		}
		contract.RenderPaginated(c, slice, meta)
	})
}

type v1BoardRevisionDetailDTO struct {
	v1BoardRevisionDTO
	Snapshot json.RawMessage `json:"snapshot"`
}

func (h *contractV1Handler) getBoardRevision(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		// :revision_id is the board-scoped revision_number (1-based sequence
		// within one board), NOT the board_revisions internal_id — so the
		// numeric shape is a formal exception to the opaque-id policy, not a
		// DB-id leak. Lookups stay scoped to (workspace, board) and unknown
		// numbers converge to 404.
		revisionNumber, parseErr := strconv.ParseInt(strings.TrimSpace(c.Param("revision_id")), 10, 64)
		if parseErr != nil || revisionNumber <= 0 {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound2)
			c.Abort()
			return
		}
		revision, err := h.deps.Boards.GetRevision(c.Request.Context(), workspace.PublicID, board.PublicID, revisionNumber)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		contract.RenderSuccess(c, v1BoardRevisionDetailDTO{
			v1BoardRevisionDTO: h.toBoardRevisionDTO(c, revision),
			Snapshot:           revision.Snapshot,
		})
	})
}

type v1BoardMemberDTO struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (h *contractV1Handler) toBoardMemberDTO(c *gin.Context, userID int64, role boards.AccessRole, createdAt string) (v1BoardMemberDTO, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 boards surface is not implemented yet")
		c.Abort()
		return v1BoardMemberDTO{}, false
	}
	publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), userID)
	if err != nil || publicID == uuid.Nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable47)
		c.Abort()
		return v1BoardMemberDTO{}, false
	}
	return v1BoardMemberDTO{UserID: tenancy.FormatUserPublicID(publicID), Role: string(role), CreatedAt: createdAt}, true
}

func (h *contractV1Handler) resolveBoardMemberUserID(c *gin.Context, raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if !contract.IsUserID(trimmed) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardMemberNotFound, msgWorkspaceMemberNotFound)
		c.Abort()
		return 0, false
	}
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 boards surface is not implemented yet")
		c.Abort()
		return 0, false
	}
	parsed, err := tenancy.ParseUserPublicID(trimmed)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardMemberNotFound, msgWorkspaceMemberNotFound)
		c.Abort()
		return 0, false
	}
	userID, err := h.deps.Users.ResolveUserID(c.Request.Context(), parsed)
	if err != nil || userID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardMemberNotFound, msgWorkspaceMemberNotFound)
		c.Abort()
		return 0, false
	}
	return userID, true
}

func (h *contractV1Handler) listBoardMembers(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		ownerDTO, ok := h.toBoardMemberDTO(c, board.OwnerUserID, boards.RoleOwner, v1FormatTime(board.CreatedAt))
		if !ok {
			return
		}
		members, err := h.deps.Boards.ListBoardMembers(c.Request.Context(), workspace.PublicID, board.PublicID)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		page := []v1BoardMemberDTO{ownerDTO}
		for _, member := range members {
			if member.UserID == board.OwnerUserID {
				continue
			}
			dto, ok := h.toBoardMemberDTO(c, member.UserID, member.Role, v1FormatTime(member.CreatedAt))
			if !ok {
				return
			}
			page = append(page, dto)
		}
		contract.RenderSuccess(c, page)
	})
}

type v1AddBoardMemberBody struct {
	Data struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) addBoardMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1AddBoardMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		targetRole := boards.AccessRole(strings.TrimSpace(body.Data.Role))
		if err := boards.ValidateV1BoardRole(targetRole); err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgRenderBoardError2, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		targetUserID, ok := h.resolveBoardMemberUserID(c, body.Data.UserID)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:member-add", workspace.PublicID, board.PublicID, body.Data.UserID, string(targetRole))
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "member-add", key, hash, func() (any, error) {
			added, addErr := h.deps.Boards.AddBoardMember(c.Request.Context(), workspace.PublicID, board.PublicID, identityPrincipal.UserID(), targetUserID, targetRole, workspaceRole)
			if addErr != nil {
				return nil, addErr
			}
			// #807: board.member_changed AFTER the roster commit.
			memberExternal := body.Data.UserID
			if h.deps.Users != nil {
				if publicID, lookupErr := h.deps.Users.GetUserPublicID(c.Request.Context(), targetUserID); lookupErr == nil {
					memberExternal = tenancy.FormatUserPublicID(publicID)
				}
			}
			h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, workspace.PublicID,
				events.EventV1BoardMemberChanged,
				events.V1Resource{Type: events.V1ResourceBoard, ID: board.PublicID},
				events.V1BoardMemberChangedData{BoardID: board.PublicID, MemberID: memberExternal})
			return added, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		added, ok := result.(*boards.V1BoardMember)
		if !ok || added == nil {
			renderBoardError(c, errors.New("boards: member-add type mismatch"))
			return
		}
		dto, ok := h.toBoardMemberDTO(c, added.UserID, added.Role, v1FormatTime(added.CreatedAt))
		if !ok {
			return
		}
		logBoardAudit("board_member_add", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID+"/"+dto.UserID, "member_add:"+string(targetRole), "added")
		contract.RenderCreated(c, dto)
	})
}

type v1UpdateBoardMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) updateBoardMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner) {
			return
		}
		var body v1UpdateBoardMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		newRole := boards.AccessRole(strings.TrimSpace(body.Data.Role))
		if err := boards.ValidateV1BoardRole(newRole); err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgRenderBoardError2, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		targetUserID, ok := h.resolveBoardMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		updated, err := h.deps.Boards.UpdateBoardMemberRole(c.Request.Context(), workspace.PublicID, board.PublicID, identityPrincipal.UserID(), targetUserID, newRole, workspaceRole)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		// #807: board.member_changed AFTER the role commit (no idempotency
		// wrapper on this route; the store commit is the boundary).
		memberExternal := strings.TrimSpace(c.Param("member_id"))
		if h.deps.Users != nil {
			if publicID, lookupErr := h.deps.Users.GetUserPublicID(c.Request.Context(), targetUserID); lookupErr == nil {
				memberExternal = tenancy.FormatUserPublicID(publicID)
			}
		}
		h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, workspace.PublicID,
			events.EventV1BoardMemberChanged,
			events.V1Resource{Type: events.V1ResourceBoard, ID: board.PublicID},
			events.V1BoardMemberChangedData{BoardID: board.PublicID, MemberID: memberExternal})
		dto, ok := h.toBoardMemberDTO(c, updated.UserID, updated.Role, v1FormatTime(updated.CreatedAt))
		if !ok {
			return
		}
		logBoardAudit("board_member_update", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID+"/"+dto.UserID, "member_update:"+string(newRole), "updated")
		contract.RenderSuccess(c, dto)
	})
}

func (h *contractV1Handler) removeBoardMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		targetUserID, ok := h.resolveBoardMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		// Owners manage the roster; anyone may leave by removing themselves
		// (last-owner protection still applies).
		if targetUserID != identityPrincipal.UserID() && !requireBoardRole(c, role, boards.RoleOwner) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		targetExternal, _ := h.deps.Users.GetUserPublicID(c.Request.Context(), targetUserID)
		hash := contract.HashIdempotencyRequest("board:member-remove", workspace.PublicID, board.PublicID, tenancy.FormatUserPublicID(targetExternal))
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "member-remove", key, hash, func() (any, error) {
			if removeErr := h.deps.Boards.RemoveBoardMember(c.Request.Context(), workspace.PublicID, board.PublicID, identityPrincipal.UserID(), targetUserID, workspaceRole); removeErr != nil {
				return nil, removeErr
			}
			// #807: board.member_changed AFTER the roster commit.
			memberExternal := tenancy.FormatUserPublicID(targetExternal)
			h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, workspace.PublicID,
				events.EventV1BoardMemberChanged,
				events.V1Resource{Type: events.V1ResourceBoard, ID: board.PublicID},
				events.V1BoardMemberChangedData{BoardID: board.PublicID, MemberID: memberExternal})
			return &v1DeletedDTO{Deleted: true}, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		deleted, ok := result.(*v1DeletedDTO)
		if !ok || deleted == nil {
			renderBoardError(c, errors.New("boards: member-remove type mismatch"))
			return
		}
		logBoardAudit("board_member_remove", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID, "member_remove", "removed")
		contract.RenderSuccess(c, *deleted)
	})
}

type v1BoardShareDTO struct {
	TokenID     string  `json:"token_id"`
	Token       string  `json:"token,omitempty"`
	TokenPrefix string  `json:"token_prefix"`
	Role        string  `json:"role"`
	Revoked     bool    `json:"revoked"`
	UsageCount  int64   `json:"usage_count"`
	CreatedAt   string  `json:"created_at"`
	RevokedAt   *string `json:"revoked_at,omitempty"`
}

func toBoardShareDTO(token *boards.V1ShareToken, withBearer bool) v1BoardShareDTO {
	dto := v1BoardShareDTO{
		TokenID:     token.TokenID,
		TokenPrefix: token.TokenPrefix,
		Role:        string(token.Role),
		Revoked:     token.Revoked,
		UsageCount:  token.UsageCount,
		CreatedAt:   v1FormatTime(token.CreatedAt),
	}
	if withBearer {
		dto.Token = token.Bearer
	}
	if token.RevokedAt != nil {
		revoked := v1FormatTime(*token.RevokedAt)
		dto.RevokedAt = &revoked
	}
	return dto
}

func (h *contractV1Handler) listShareTokens(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner) {
			return
		}
		tokens, err := h.deps.Boards.ListShareTokens(c.Request.Context(), workspace.PublicID, board.PublicID)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		page := make([]v1BoardShareDTO, 0, len(tokens))
		for _, token := range tokens {
			page = append(page, toBoardShareDTO(token, false))
		}
		contract.RenderSuccess(c, page)
	})
}

type v1CreateShareBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *contractV1Handler) createShareToken(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateShareBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		shareRole := boards.AccessRole(strings.TrimSpace(body.Data.Role))
		if shareRole != boards.RoleEditor && shareRole != boards.RoleViewer {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgCreateShareToken, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:share-create", workspace.PublicID, board.PublicID, string(shareRole))
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "share-create", key, hash, func() (any, error) {
			tokenID, mintErr := boards.NewV1ShareTokenID()
			if mintErr != nil {
				return nil, mintErr
			}
			created, createErr := h.deps.Boards.CreateShareToken(c.Request.Context(), workspace.PublicID, board.PublicID,
				identityPrincipal.UserID(), shareRole, tokenID, boards.V1ShareTokenHash(tokenID), tokenID)
			if createErr != nil {
				return nil, createErr
			}
			dto := toBoardShareDTO(created, true)
			return &dto, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		dto, ok := result.(*v1BoardShareDTO)
		if !ok || dto == nil {
			renderBoardError(c, errors.New("boards: share-create type mismatch"))
			return
		}
		logBoardAudit("board_share_create", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID+"/"+dto.TokenPrefix, "share_create:"+string(shareRole), "created")
		contract.RenderCreated(c, *dto)
	})
}

func (h *contractV1Handler) revokeShareToken(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.boardAccess(c, workspace, workspaceRole, identityPrincipal.UserID())
		if !ok {
			return
		}
		if !requireBoardRole(c, role, boards.RoleOwner) {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		// {token_id} stays an opaque string: legacy integer ids simply miss the
		// lookup and read as 404 (tolerated read-only, never a distinct status).
		tokenID := strings.TrimSpace(c.Param("token_id"))
		if tokenID == "" {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBoardShareTokenNotFound, msgBoardShareTokenNotFound)
			c.Abort()
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:share-revoke", workspace.PublicID, board.PublicID, tokenID)
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "share-revoke", key, hash, func() (any, error) {
			if revokeErr := h.deps.Boards.RevokeShareToken(c.Request.Context(), workspace.PublicID, board.PublicID, tokenID, identityPrincipal.UserID()); revokeErr != nil {
				return nil, revokeErr
			}
			// #807: board.share_revoked AFTER the revocation commit (token
			// id only; the bearer never leaves the server).
			h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, workspace.PublicID,
				events.EventV1BoardShareRevoked,
				events.V1Resource{Type: events.V1ResourceBoard, ID: board.PublicID},
				events.V1BoardShareRevokedData{BoardID: board.PublicID, ShareTokenID: tokenID})
			return &v1DeletedDTO{Deleted: true}, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		deleted, ok := result.(*v1DeletedDTO)
		if !ok || deleted == nil {
			renderBoardError(c, errors.New("boards: share-revoke type mismatch"))
			return
		}
		prefix := tokenID
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		logBoardAudit("board_share_revoke", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID+"/"+prefix, "share_revoke", "revoked")
		contract.RenderSuccess(c, *deleted)
	})
}

type v1JoinBoardBody struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

func (h *contractV1Handler) joinBoardByToken(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireBoardsStore(c) {
		return
	}
	// The bearer is the capability: the workspace must exist (unknown ids
	// stay 404 workspace_not_found) but the joiner needs no workspace
	// membership yet. A successful join grants workspace membership
	// (member) alongside the board grant, so the collaborator can reach
	// the workspace-rooted board surface afterwards. Workspace members
	// without an explicit board grant default to viewer (owners to owner);
	// explicit board rows still override.
	rawWorkspace := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(rawWorkspace) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, msgListControlBillingAccounts)
		c.Abort()
		return
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), rawWorkspace)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, msgListControlBillingAccounts)
		c.Abort()
		return
	}
	key, ok := requireV1IdempotencyKey(c)
	if !ok {
		return
	}
	var body v1JoinBoardBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		writeV1InputError(c)
		return
	}
	bearer := strings.TrimSpace(body.Data.Token)
	if bearer == "" {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgRenderJoinError, contract.FieldErrors{Fields: map[string]string{"token": "required"}})
		c.Abort()
		return
	}
	// Join failures (unknown/revoked bearer, foreign-workspace token) all
	// converge to 404 board_share_token_not_found: existence is not leaked.
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	hash := contract.HashIdempotencyRequest("board:join", workspace.PublicID, bearer)
	result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "join", key, hash, func() (any, error) {
		board, _, joinErr := h.deps.Boards.JoinByToken(c.Request.Context(), workspace.PublicID, bearer, identityPrincipal.UserID())
		if joinErr != nil {
			return nil, joinErr
		}
		// Membership is granted only once the bearer has redeemed, never
		// before: JoinByToken cannot be gated on it because the board store
		// never reads workspace membership (it resolves the joiner's role
		// from the board row it just wrote), so an up-front grant authorizes
		// nothing and only leaves a member row behind on rejected joins.
		// A member row is not board-scoped — it reaches billing, credits,
		// tasks, media and library — so a rejected bearer must leave no
		// trace at all. Keeping the grant inside the idempotency scope also
		// means a replayed success can never outlive its own grant.
		if _, memberErr := h.deps.Store.AddWorkspaceMember(c.Request.Context(), workspace.PublicID, identityPrincipal.UserID(), tenancy.RoleMember); memberErr != nil {
			return nil, memberErr
		}
		dto := toBoardDTO(board)
		return &dto, nil
	})
	if !ok {
		return
	}
	if fnErr != nil {
		renderJoinError(c, fnErr)
		return
	}
	dto, ok := result.(*v1BoardDTO)
	if !ok || dto == nil {
		renderBoardError(c, errors.New("boards: join type mismatch"))
		return
	}
	prefix := bearer
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	logBoardAudit("board_join", identityPrincipal.UserID(), workspace.PublicID+"/"+dto.ID+"/"+prefix, "share_join", "joined")
	contract.RenderSuccess(c, *dto)
}
