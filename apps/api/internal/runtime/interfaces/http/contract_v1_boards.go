package httpapi

// Workspace-scoped Board routes share one selected-member authorization policy.

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
	"ideaboard/api/internal/media/workspacemedia"
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
	group.GET("/workspaces/:workspace_id/boards/:board_id", h.getBoard)
	group.PATCH("/workspaces/:workspace_id/boards/:board_id", h.renameBoard)
	group.POST("/workspaces/:workspace_id/boards/:board_id/archive", h.archiveBoard)
	group.POST("/workspaces/:workspace_id/boards/:board_id/restore", h.restoreBoard)
	group.GET("/workspaces/:workspace_id/boards/:board_id/document", h.getBoardDocument)
	group.PUT("/workspaces/:workspace_id/boards/:board_id/document", h.replaceBoardDocument)
	group.GET("/workspaces/:workspace_id/boards/:board_id/revisions", h.listBoardRevisions)
	group.GET("/workspaces/:workspace_id/boards/:board_id/revisions/:revision_id", h.getBoardRevision)
	group.GET("/workspaces/:workspace_id/boards/:board_id/access", h.getBoardAccess)
	group.PUT("/workspaces/:workspace_id/boards/:board_id/access", h.updateBoardAccess)
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

// boardRoles exposes the narrow board-access port (#934) consumed by this
// surface today and by Media / Tasking / Events in #936/#937/#940: role
// facts only, never Boards private storage and never an HTTP handler.
// h.deps.Boards always satisfies it (both stores assert
// boards.RoleReader); nil fails closed through requireStoreAndBoards
// before any caller reaches here.
func (h *contractV1Handler) boardRoles() boards.RoleReader {
	return h.deps.Boards
}

// authorizeBoard resolves a board within a visible workspace and enforces
// the unified Boards access policy (boards.ResolveRole +
// boards.CheckCapability, see internal/boards/policy.go): malformed ids,
// unknown boards, and callers holding no effective board role (no explicit
// grant and no visible workspace default) converge to 404 board_not_found
// (Baseline 4); visible callers lacking the operation capability get 403
// board_forbidden. It is the only board authorization helper on this
// surface: every board-scoped route (metadata, document, revisions,
// catalog-embedded Preview Scene, member management, rename,
// archive/restore, writes) enters through it with its capability.
func (h *contractV1Handler) authorizeBoard(c *gin.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64, cap boards.Capability) (*boards.V1Board, boards.AccessRole, bool) {
	return h.authorizeBoardByID(c, workspace, strings.TrimSpace(c.Param("board_id")), workspaceRole, userID, cap)
}

// authorizeBoardByID is the explicit-id core behind authorizeBoard (#936:
// upload-target binding verifies a body-supplied board id through the same
// 404/403 convergence without a :board_id route param).
func (h *contractV1Handler) authorizeBoardByID(c *gin.Context, workspace *tenancy.Workspace, boardID, workspaceRole string, userID int64, cap boards.Capability) (*boards.V1Board, boards.AccessRole, bool) {
	raw := strings.TrimSpace(boardID)
	if !contract.IsBoardID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
		c.Abort()
		return nil, "", false
	}
	if h.deps.Boards == nil {
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
	role, ok, err := h.boardRoles().GetBoardRole(ctx, workspace.PublicID, board.PublicID, userID, workspaceRole)
	if err != nil || boards.CheckCapability(role, ok, boards.CapRead) != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
		c.Abort()
		return nil, "", false
	}
	if err := boards.CheckCapability(role, ok, cap); err != nil {
		contract.RenderError(c, http.StatusForbidden, contract.CodeBoardForbidden, msgForbidden)
		c.Abort()
		return nil, "", false
	}
	return board, role, true
}

func renderBoardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspacemedia.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, msgMediaNotFound)
	case errors.Is(err, boards.ErrInvalidInput):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, msgUpdateBoardAccess)
	case errors.Is(err, boards.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardNotFound, msgBoardNotFound)
	case errors.Is(err, boards.ErrRevisionConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeBoardRevisionConflict, msgBoardRevisionConflict)
	case errors.Is(err, boards.ErrAccessConflict):
		contract.RenderError(c, http.StatusPreconditionFailed, contract.CodeBoardAccessConflict, msgBoardAccessConflict)
	case errors.Is(err, boards.ErrPreconditionNeeded):
		contract.RenderError(c, http.StatusPreconditionRequired, contract.CodeBoardPreconditionRequired, msgBoardPreconditionRequired)
	case errors.Is(err, boards.ErrPermissionDenied):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBoardForbidden, msgForbidden)
	case errors.Is(err, boards.ErrV1Archived):
		contract.RenderError(c, contract.StatusOf(contract.CodeBoardArchived), contract.CodeBoardArchived, msgBoardArchived)
	case errors.Is(err, boards.ErrMemberNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeBoardMemberNotFound, msgWorkspaceMemberNotFound)
	case errors.Is(err, boards.ErrIDConflict):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable46)
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

// logBoardAudit emits the operator/target/reason/outcome tuple for access and
// member mutations. No secrets are logged: access targets use the board id.
// #808 persists these as durable audit records.
func logBoardAudit(op string, operatorUserID int64, target, reason, outcome string) {
	slog.Info("board v1 audit",
		"op", op,
		"operator_user_id", operatorUserID,
		"target", target,
		"reason", reason,
		"outcome", outcome,
	)
}

type v1BoardCapabilitiesDTO struct {
	Read     bool `json:"read"`
	Write    bool `json:"write"`
	Manage   bool `json:"manage"`
	Generate bool `json:"generate"`
}

type v1BoardRelationshipDTO struct {
	IsCreator      bool `json:"is_creator"`
	HasDirectGrant bool `json:"has_direct_grant"`
}

func toBoardCapabilitiesDTO(caps boards.Capabilities) v1BoardCapabilitiesDTO {
	return v1BoardCapabilitiesDTO{
		Read:     caps.CanRead,
		Write:    caps.CanWrite,
		Manage:   caps.CanManage,
		Generate: caps.CanGenerate,
	}
}

type v1BoardDTO struct {
	ID              string                 `json:"id"`
	WorkspaceID     string                 `json:"workspace_id"`
	Name            string                 `json:"name"`
	Revision        int64                  `json:"revision"`
	SchemaVersion   int32                  `json:"schema_version"`
	ContentVersion  string                 `json:"content_version,omitempty"`
	PreviewScene    json.RawMessage        `json:"preview_scene,omitempty"`
	ArchivedAt      *string                `json:"archived_at,omitempty"`
	CreatorUserID   string                 `json:"creator_user_id"`
	WorkspaceAccess string                 `json:"workspace_access"`
	AccessVersion   int64                  `json:"access_version"`
	AccessState     string                 `json:"access_state"`
	Relationship    v1BoardRelationshipDTO `json:"relationship"`
	Capabilities    v1BoardCapabilitiesDTO `json:"capabilities"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
}

func boardCapabilitiesDTOFor(role boards.AccessRole, callerUserID int64, board *boards.V1Board) v1BoardCapabilitiesDTO {
	return toBoardCapabilitiesDTO(boards.CapabilitiesFor(role, true, board.ArchivedAt != nil))
}

// toBoardMemberDTO resolves the creator public identity for board payloads.
// Boards without a resolvable creator fail closed (503): creator_user_id is
// a required User public identity, never an internal id.
func (h *contractV1Handler) toBoardCreatorDTO(c *gin.Context, ownerUserID int64) (string, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 boards surface is not implemented yet")
		c.Abort()
		return "", false
	}
	publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), ownerUserID)
	if err != nil || publicID == uuid.Nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable47)
		c.Abort()
		return "", false
	}
	return tenancy.FormatUserPublicID(publicID), true
}

// toBoardDTOFull renders one board with its access facts: creator public
// identity, common sharing role, monotonic access version, derived posture,
// caller relationship, and capabilities. The catalog path stays down the
// single-query entry facts instead (toBoardCatalogDTO); single-board reads
// resolve the caller's grant from the roster scan.
func (h *contractV1Handler) toBoardDTOFull(c *gin.Context, board *boards.V1Board, callerUserID int64, role boards.AccessRole) (v1BoardDTO, bool) {
	creator, ok := h.toBoardCreatorDTO(c, board.OwnerUserID)
	if !ok {
		return v1BoardDTO{}, false
	}
	members, err := h.deps.Boards.ListBoardMembers(c.Request.Context(), board.WorkspacePublicID, board.PublicID)
	if err != nil {
		renderBoardError(c, err)
		return v1BoardDTO{}, false
	}
	var hasDirectGrant bool
	for _, member := range members {
		if member.UserID == callerUserID {
			hasDirectGrant = true
			break
		}
	}
	dto := v1BoardDTO{
		ID:              board.PublicID,
		WorkspaceID:     board.WorkspacePublicID,
		Name:            board.Name,
		Revision:        board.Revision,
		SchemaVersion:   board.SchemaVersion,
		ContentVersion:  board.ContentVersion,
		PreviewScene:    board.PreviewScene,
		CreatorUserID:   creator,
		WorkspaceAccess: string(board.WorkspaceAccess),
		AccessVersion:   board.AccessVersion,
		AccessState:     string(boards.DeriveAccessState(board.WorkspaceAccess, len(members) > 0)),
		Relationship: v1BoardRelationshipDTO{
			IsCreator:      callerUserID == board.OwnerUserID,
			HasDirectGrant: hasDirectGrant,
		},
		Capabilities: boardCapabilitiesDTOFor(role, callerUserID, board),
		CreatedAt:    v1FormatTime(board.CreatedAt),
		UpdatedAt:    v1FormatTime(board.UpdatedAt),
	}
	if board.ArchivedAt != nil {
		archived := v1FormatTime(*board.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	return dto, true
}

// toBoardCatalogDTO renders one Gallery catalog card: metadata plus the
// stored scene projection (never the full document or the full ACL), with
// the caller's effective role facts carried from the catalog query.
func (h *contractV1Handler) toBoardCatalogDTO(c *gin.Context, entry *boards.V1BoardCatalogEntry, callerUserID int64) (v1BoardDTO, bool) {
	creator, ok := h.toBoardCreatorDTO(c, entry.OwnerUserID)
	if !ok {
		return v1BoardDTO{}, false
	}
	dto := v1BoardDTO{
		ID:              entry.PublicID,
		WorkspaceID:     entry.WorkspacePublicID,
		Name:            entry.Name,
		Revision:        entry.Revision,
		SchemaVersion:   entry.SchemaVersion,
		ContentVersion:  entry.ContentVersion,
		PreviewScene:    entry.PreviewScene,
		CreatorUserID:   creator,
		WorkspaceAccess: string(entry.WorkspaceAccess),
		AccessVersion:   entry.AccessVersion,
		AccessState:     string(entry.AccessState),
		Relationship: v1BoardRelationshipDTO{
			IsCreator:      callerUserID == entry.OwnerUserID,
			HasDirectGrant: entry.HasDirectGrant,
		},
		Capabilities: toBoardCapabilitiesDTO(boards.CapabilitiesFor(entry.EffectiveRole, true, entry.ArchivedAt != nil)),
		CreatedAt:    v1FormatTime(entry.CreatedAt),
		UpdatedAt:    v1FormatTime(entry.UpdatedAt),
	}
	if entry.ArchivedAt != nil {
		archived := v1FormatTime(*entry.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	return dto, true
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
			dto, ok := h.toBoardCatalogDTO(c, entry, wctx.Principal.UserID())
			if !ok {
				return
			}
			data = append(data, dto)
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
		dto, ok := h.toBoardDTOFull(c, created, identityPrincipal.UserID(), boards.RoleOwner)
		if !ok {
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(created.Revision))
		contract.RenderCreated(c, dto)
	})
}

func (h *contractV1Handler) getBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapRead)
		if !ok {
			return
		}
		dto, ok := h.toBoardDTOFull(c, board, identityPrincipal.UserID(), role)
		if !ok {
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(board.Revision))
		contract.RenderSuccess(c, dto)
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
		board, role, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapWrite)
		if !ok {
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
		renamed, err := h.deps.Boards.RenameBoard(c.Request.Context(), workspace.PublicID, board.PublicID, name, identityPrincipal.UserID())
		if err != nil {
			renderBoardError(c, err)
			return
		}
		dto, ok := h.toBoardDTOFull(c, renamed, identityPrincipal.UserID(), role)
		if !ok {
			return
		}
		contract.SetETag(c, boards.FormatV1ETag(renamed.Revision))
		contract.RenderSuccess(c, dto)
	})
}

func (h *contractV1Handler) archiveBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapWrite)
		if !ok {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:archive", workspace.PublicID, board.PublicID)
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "archive", key, hash, func() (any, error) {
			return h.deps.Boards.ArchiveBoard(c.Request.Context(), workspace.PublicID, board.PublicID, identityPrincipal.UserID())
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
		dto, ok := h.toBoardDTOFull(c, archived, identityPrincipal.UserID(), role)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

func (h *contractV1Handler) restoreBoard(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapWrite)
		if !ok {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:restore", workspace.PublicID, board.PublicID)
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "restore", key, hash, func() (any, error) {
			return h.deps.Boards.RestoreBoard(c.Request.Context(), workspace.PublicID, board.PublicID, identityPrincipal.UserID())
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
		dto, ok := h.toBoardDTOFull(c, restored, identityPrincipal.UserID(), role)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
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
		board, _, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapRead)
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
		board, _, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapWrite)
		if !ok {
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
		board, _, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapRead)
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
		board, _, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapRead)
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

// v1BoardAccessDTO is the independent access-resource view of one board:
// the common sharing role, the monotonic version (served as ETag
// access-<version>), and the derived sharing posture. It carries no
// document bytes, revision, preview payload, or ACL rows.
type v1BoardAccessDTO struct {
	WorkspaceID     string   `json:"workspace_id"`
	BoardID         string   `json:"board_id"`
	WorkspaceAccess string   `json:"workspace_access"`
	AccessVersion   int64    `json:"access_version"`
	AccessState     string   `json:"access_state"`
	MemberUserIDs   []string `json:"member_user_ids"`
	ArchivedAt      *string  `json:"archived_at,omitempty"`
}

func (h *contractV1Handler) toBoardAccessDTO(c *gin.Context, access *boards.V1BoardAccess) (v1BoardAccessDTO, bool) {
	dto := v1BoardAccessDTO{
		WorkspaceID:     access.WorkspacePublicID,
		BoardID:         access.BoardPublicID,
		WorkspaceAccess: string(access.WorkspaceAccess),
		AccessVersion:   access.AccessVersion,
		AccessState:     string(access.AccessState),
		MemberUserIDs:   make([]string, 0, len(access.MemberUserIDs)),
	}
	if access.ArchivedAt != nil {
		archived := v1FormatTime(*access.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	for _, id := range access.MemberUserIDs {
		publicID, ok := h.toBoardCreatorDTO(c, id)
		if !ok {
			return v1BoardAccessDTO{}, false
		}
		dto.MemberUserIDs = append(dto.MemberUserIDs, publicID)
	}
	return dto, true
}

// parseBoardAccessIfMatch requires the access-version precondition for
// access-resource writes: a missing If-Match renders 428
// board_precondition_required (aborted), an unparsable value renders 400,
// and a stale version surfaces as 412 board_access_conflict at commit time.
func parseBoardAccessIfMatch(c *gin.Context) (int64, bool) {
	raw := strings.TrimSpace(contract.IfMatch(c))
	if raw == "" {
		renderBoardError(c, boards.ErrPreconditionNeeded)
		return 0, false
	}
	version, err := boards.ParseAccessIfMatch(raw)
	if err != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgParseBoardAccessIfMatch, contract.FieldErrors{Fields: map[string]string{"if_match": "invalid"}})
		c.Abort()
		return 0, false
	}
	return version, true
}

type v1UpdateBoardAccessBody struct {
	Data struct {
		WorkspaceAccess string   `json:"workspace_access"`
		MemberUserIDs   []string `json:"member_user_ids"`
	} `json:"data"`
}

// getBoardAccess reads the common role, selected members, the monotonic
// version as ETag access-<version>, and the
// derived posture. Management capability is required; every other caller
// already sees the version fact on board payloads.
func (h *contractV1Handler) getBoardAccess(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapManage)
		if !ok {
			return
		}
		access, err := h.deps.Boards.GetBoardAccess(c.Request.Context(), workspace.PublicID, board.PublicID)
		if err != nil {
			renderBoardError(c, err)
			return
		}
		contract.SetETag(c, boards.FormatAccessETag(access.AccessVersion))
		dto, ok := h.toBoardAccessDTO(c, access)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

// updateBoardAccess replaces the complete sharing configuration (none|viewer|editor).
// Only board owners may write; archived boards reject the write; a missing
// If-Match renders 428 and a stale access_version renders 412
// board_access_conflict. No-op replays keep the version; real changes bump
// it without touching the document revision, content_version, or Preview
// Scene. Ordinary renames stay on PATCH.
func (h *contractV1Handler) updateBoardAccess(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, _, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapManage)
		if !ok {
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		expectedVersion, ok := parseBoardAccessIfMatch(c)
		if !ok {
			return
		}
		var body v1UpdateBoardAccessBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		if body.Data.MemberUserIDs == nil {
			writeV1InputError(c)
			return
		}
		target := boards.WorkspaceAccess(strings.TrimSpace(body.Data.WorkspaceAccess))
		if !boards.ValidateWorkspaceAccess(target) {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				msgUpdateBoardAccess, contract.FieldErrors{Fields: map[string]string{"workspace_access": "invalid"}})
			c.Abort()
			return
		}
		selectedIDs := make([]int64, 0, len(body.Data.MemberUserIDs))
		if target != boards.WorkspaceAccessNone {
			for _, publicID := range body.Data.MemberUserIDs {
				id, ok := h.resolveBoardMemberUserID(c, publicID)
				if !ok {
					return
				}
				selectedIDs = append(selectedIDs, id)
			}
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("board:access-update", workspace.PublicID, board.PublicID,
			strconv.FormatInt(expectedVersion, 10), string(target), strings.Join(body.Data.MemberUserIDs, ","))
		result, fnErr, ok := h.boardDo(c, externalUser, identityPrincipal.UserID(), "access-update", key, hash, func() (any, error) {
			updated, err := h.deps.Boards.ReplaceBoardAccess(c.Request.Context(), workspace.PublicID, board.PublicID, identityPrincipal.UserID(), expectedVersion, target, selectedIDs)
			if err != nil {
				return nil, err
			}
			h.deliverBoardAccessSignals(c.Request.Context(), updated.AccessSignals)
			return updated, nil
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderBoardError(c, fnErr)
			return
		}
		updated, ok := result.(*boards.V1BoardAccess)
		if !ok || updated == nil {
			renderBoardError(c, errors.New("boards: access-update type mismatch"))
			return
		}
		logBoardAudit("board_access_update", identityPrincipal.UserID(), workspace.PublicID+"/"+board.PublicID, "access_update:"+string(target), "updated")
		contract.SetETag(c, boards.FormatAccessETag(updated.AccessVersion))
		dto, ok := h.toBoardAccessDTO(c, updated)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}
