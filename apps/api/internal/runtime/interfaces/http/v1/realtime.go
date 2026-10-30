package v1

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/boards/realtime"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// v1RealtimeTicketDTO is the presence ticket body: the opaque HMAC bearer
// plus its RFC3339 expiry. The ticket binds one board access version to
// one caller for 60 seconds; the Y.js server admits it via
// GET /internal/realtime/admit.
type v1RealtimeTicketDTO struct {
	Ticket    string `json:"ticket"`
	ExpiresAt string `json:"expires_at"`
}

// issueRealtimeTicket mints a short-lived presence ticket for the Y.js
// collaboration server (POST
// /workspaces/:workspace_id/boards/:board_id/realtime-ticket).
//
// Auth follows the board surface: workspace membership plus
// GetBoardRole/CheckCapability(CapRead), so viewers may mint while callers
// without board read converge to 404 board_not_found (anti-enumeration).
// Archived boards reject with 409 board_archived. A missing
// RT_TICKET_SECRET fails closed with 500 (no fallback default).
// No Idempotency-Key is required: issuance is a read-like POST with no
// state write (same class as quotes and batch reads).
func (h *contractV1Handler) issueRealtimeTicket(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndBoards, Resolve: h.boardWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		workspaceRole := wctx.Role
		board, role, ok := h.authorizeBoard(c, workspace, workspaceRole, identityPrincipal.UserID(), boards.CapRead)
		if !ok {
			return
		}
		if board.ArchivedAt != nil {
			contract.RenderError(c, contract.StatusOf(contract.CodeBoardArchived), contract.CodeBoardArchived, runtimehttp.MsgBoardArchived)
			c.Abort()
			return
		}
		ticketDTO, err := h.boardRealtimeTicket(board, role, identityPrincipal.PublicID())
		if err != nil {
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable46)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, ticketDTO)
	})
}

// boardRealtimeTicket builds a credential only after the caller has authorized
// a live Board. Both initial bundles and ticket refreshes use this issuance seam.
func (h *contractV1Handler) boardRealtimeTicket(board *boards.V1Board, role boards.AccessRole, userPublicID uuid.UUID) (v1RealtimeTicketDTO, error) {
	secret := strings.TrimSpace(h.deps.RealtimeTicketSecret)
	if secret == "" {
		return v1RealtimeTicketDTO{}, errors.New("realtime ticket secret is not configured")
	}
	caps := boards.CapabilitiesFor(role, true, false)
	ticket, exp, err := realtime.Issue(secret, board.PublicID, board.AccessVersion, realtime.CapFor(caps.CanWrite), tenancy.FormatUserPublicID(userPublicID), time.Now())
	if err != nil {
		return v1RealtimeTicketDTO{}, err
	}
	return v1RealtimeTicketDTO{Ticket: ticket, ExpiresAt: exp.UTC().Format(time.RFC3339)}, nil
}
