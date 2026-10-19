package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/boards"
	boardrealtime "ideaboard/api/internal/boards/realtime"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RealtimeBoardResolver provides board admission and Y.Doc persistence from
// one store. Both WorkspaceBoardStore implementations satisfy it.
type RealtimeBoardResolver interface {
	boards.BoardYUpdateStore
	GetBoardByPublicID(ctx context.Context, boardPublicID string) (*boards.V1Board, error)
	GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (boards.AccessRole, bool, error)
}

// RealtimeWorkspaceResolver reads current workspace membership for admit.
type RealtimeWorkspaceResolver interface {
	GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error)
}

// RealtimeUserResolver maps the ticket usr_ identity to the internal user id.
type RealtimeUserResolver interface {
	ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error)
}

// internalRealtimeAdmitDTO is the admit boolean body (internal
// machine surface, not the public contract envelope).
type internalRealtimeAdmitDTO struct {
	Admit bool `json:"admit"`
}

// internalRealtimeHandler serves GET /internal/realtime/admit for the Y.js
// presence server. Guarded by X-Internal-Key (constant-time, same shape as
// the identity internal routes and Auth trusted-identity/internalRoutes:
// missing config fails 500, wrong key fails 401).
//
// Primary contract is ?ticket=<ticket> (the full issuance bearer, which
// encodes bid/av/uid): signature + expiry are verified via the shared
// boards/realtime package (same Issue/Verify/Admit core as issuance), then
// gated against current board state (ticket av == current access_version,
// board live, caller still holds board read). Expired/tampered/stale/
// revoked/archived tickets read as admit:false (200), never 4xx, so the
// realtime server branches on one boolean. Only ticket is recognized:
// any extra bid/av/uid query params are ignored, never validated.
// Admission always requires a signed ticket.
type internalRealtimeHandler struct {
	boards       RealtimeBoardResolver
	workspaces   RealtimeWorkspaceResolver
	users        RealtimeUserResolver
	ticketSecret string
	internalKey  string
}

// NewInternalRealtimeHandler builds the admit handler. Empty secrets fail
// closed at request time (500), never with a fallback default.
func NewInternalRealtimeHandler(boardStore RealtimeBoardResolver, workspaces RealtimeWorkspaceResolver, users RealtimeUserResolver, ticketSecret, internalKey string) InternalRealtimeHandler {
	return &internalRealtimeHandler{
		boards:       boardStore,
		workspaces:   workspaces,
		users:        users,
		ticketSecret: strings.TrimSpace(ticketSecret),
		internalKey:  strings.TrimSpace(internalKey),
	}
}

func (h *internalRealtimeHandler) requireInternalKey(c *gin.Context) bool {
	if h == nil || strings.TrimSpace(h.internalKey) == "" {
		errorrender.Render(c, http.StatusInternalServerError, string(contract.CodeServiceUnavailable), "internal key is not configured")
		return false
	}
	got := strings.TrimSpace(c.GetHeader("X-Internal-Key"))
	want := h.internalKey
	if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		errorrender.Render(c, http.StatusUnauthorized, string(contract.CodeUnauthorized), "unauthorized")
		return false
	}
	return true
}

var errRealtimeDependenciesMissing = errors.New("realtime dependencies are not configured")

func (h *internalRealtimeHandler) requireDependencies(c *gin.Context) bool {
	if h.boards == nil || h.workspaces == nil || h.users == nil {
		response.InternalError(c, http.StatusInternalServerError, "realtime dependencies are not configured", errRealtimeDependenciesMissing)
		return false
	}
	return true
}

// Admit implements GET /internal/realtime/admit.
func (h *internalRealtimeHandler) Admit(c *gin.Context) {
	if !h.requireInternalKey(c) {
		return
	}
	if strings.TrimSpace(h.ticketSecret) == "" {
		response.InternalError(c, http.StatusInternalServerError, "realtime ticket secret is not configured", boardrealtime.ErrTicketSecretMissing)
		return
	}
	if !h.requireDependencies(c) {
		return
	}
	ticket := strings.TrimSpace(c.Query("ticket"))

	if ticket == "" {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "ticket is required")
		return
	}

	payload, err := boardrealtime.Verify(h.ticketSecret, ticket, time.Now())
	if err != nil {
		// Expired/tampered tickets deny with a boolean, not a status:
		// the realtime server branches on admit only.
		response.SuccessDTO(c, internalRealtimeAdmitDTO{Admit: false})
		return
	}
	admit := h.ticketAdmit(c.Request.Context(), payload)
	response.SuccessDTO(c, internalRealtimeAdmitDTO{Admit: admit})
}

// ticketAdmit gates a verified ticket against current board state.
func (h *internalRealtimeHandler) ticketAdmit(ctx context.Context, payload boardrealtime.Payload) bool {
	board, err := h.boards.GetBoardByPublicID(ctx, payload.BID)
	if err != nil || board == nil {
		return false
	}
	if board.ArchivedAt != nil || payload.AV != board.AccessVersion {
		return false
	}
	userID, ok := h.resolveUID(ctx, payload.UID)
	if !ok {
		return false
	}
	workspaceRole, member, err := h.workspaces.GetWorkspaceRole(ctx, board.WorkspacePublicID, userID)
	if err != nil || !member {
		return false
	}
	role, visible, err := h.boards.GetBoardRole(ctx, board.WorkspacePublicID, board.PublicID, userID, workspaceRole)
	if err != nil {
		return false
	}
	// payload.Cap has no decision point in Go: admit gates read only.
	// Write gating lives in the sidecar (setWriter by ticket cap) plus
	// per-author real-time re-checks in AppendYUpdate/CompactYDocument.
	return boards.CheckCapability(role, visible, boards.CapRead) == nil
}

func (h *internalRealtimeHandler) resolveUID(ctx context.Context, uid string) (int64, bool) {
	parsed, err := tenancy.ParseUserPublicID(strings.TrimSpace(uid))
	if err != nil {
		return 0, false
	}
	userID, err := h.users.ResolveUserID(ctx, parsed)
	if err != nil || userID <= 0 {
		return 0, false
	}
	return userID, true
}
