package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/boards"
	boardrealtime "ideaboard/api/internal/boards/realtime"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const maxRealtimeHydrationBytes = 8 << 20

type internalYDocumentDTO struct {
	Snapshot json.RawMessage `json:"snapshot"`
	Updates  []string        `json:"updates"`
}

// LoadYDocument returns a bounded persisted snapshot and its ordered update
// tail only after validating the live ticket's current read capability.
func (h *internalRealtimeHandler) LoadYDocument(c *gin.Context) {
	if !h.requireInternalKey(c) {
		return
	}
	if !h.requireDependencies(c) {
		return
	}
	ticket := strings.TrimSpace(c.Query("ticket"))
	payload, err := boardrealtime.Verify(h.ticketSecret, ticket, time.Now())
	if err != nil || !h.ticketAdmit(c.Request.Context(), payload) {
		errorrender.Render(c, http.StatusForbidden, string(contract.CodeBoardForbidden), "forbidden")
		return
	}
	board, err := h.boards.GetBoardByPublicID(c.Request.Context(), payload.BID)
	if err != nil || board == nil || board.ArchivedAt != nil {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeBoardNotFound), "board not found")
		return
	}
	updates, err := h.boards.ListYUpdates(c.Request.Context(), board.WorkspacePublicID, board.PublicID, boards.MaxYUpdatesListLimit)
	if err != nil {
		renderYDocStoreError(c, err)
		return
	}
	if len(updates) >= int(boards.MaxYUpdatesListLimit) {
		errorrender.Render(c, http.StatusRequestEntityTooLarge, string(contract.CodeBadRequest), "document recovery exceeds update limit")
		return
	}
	result := internalYDocumentDTO{Snapshot: board.Document, Updates: make([]string, 0, len(updates))}
	used := len(result.Snapshot)
	for _, update := range updates {
		used += base64.StdEncoding.EncodedLen(len(update.Update))
		if used > maxRealtimeHydrationBytes {
			errorrender.Render(c, http.StatusRequestEntityTooLarge, string(contract.CodeBadRequest), "document recovery exceeds size limit")
			return
		}
		result.Updates = append(result.Updates, base64.StdEncoding.EncodeToString(update.Update))
	}
	response.SuccessDTO(c, result)
}

// Y.Doc collaboration internal surface (ADR-0020). The realtime sidecar calls
// these endpoints with X-Internal-Key (same guard as Admit):
//
//	POST /internal/realtime/y-updates    persist one opaque Y update
//	POST /internal/realtime/compactions  land a sanitized Y snapshot
//
// Updates are never decoded here; compaction snapshots pass the same
// ValidateV1DocumentStrict rules as the public path inside the store.
// 4xx outcomes are permanent (the sidecar must drop, not retry); 5xx and
// transport failures are retryable.

// internalYUpdateBody is one opaque Y update for a board.
type internalYUpdateBody struct {
	BoardID   string `json:"board_id"`
	Update    string `json:"update"`
	AuthorUID string `json:"author_uid"`
}

// internalYUpdateDTO is the persist acknowledgement body.
type internalYUpdateDTO struct {
	Persisted bool  `json:"persisted"`
	Pending   int64 `json:"pending"`
}

// internalYCompactBody is one Y-derived snapshot compaction request.
type internalYCompactBody struct {
	BoardID   string          `json:"board_id"`
	Snapshot  json.RawMessage `json:"snapshot"`
	AuthorUID string          `json:"author_uid"`
}

// internalYCompactDTO is the compaction acknowledgement body.
type internalYCompactDTO struct {
	Compacted bool  `json:"compacted"`
	Revision  int64 `json:"revision"`
}

// PersistYUpdate implements POST /internal/realtime/y-updates.
func (h *internalRealtimeHandler) PersistYUpdate(c *gin.Context) {
	if !h.requireInternalKey(c) {
		return
	}
	if !h.requireDependencies(c) {
		return
	}
	var body internalYUpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "invalid request")
		return
	}
	if !contract.IsBoardID(strings.TrimSpace(body.BoardID)) || !contract.IsUserID(strings.TrimSpace(body.AuthorUID)) {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "invalid request")
		return
	}
	update, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body.Update))
	if err != nil || len(update) == 0 {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "invalid request")
		return
	}
	board, err := h.boards.GetBoardByPublicID(c.Request.Context(), strings.TrimSpace(body.BoardID))
	if err != nil || board == nil {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeBoardNotFound), "board not found")
		return
	}
	authorID, ok := h.resolveUID(c.Request.Context(), strings.TrimSpace(body.AuthorUID))
	if !ok {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeBoardNotFound), "board not found")
		return
	}
	pending, err := h.boards.AppendYUpdate(c.Request.Context(), board.WorkspacePublicID, board.PublicID, authorID, update)
	if err != nil {
		renderYDocStoreError(c, err)
		return
	}
	response.SuccessDTO(c, internalYUpdateDTO{Persisted: true, Pending: pending})
}

// CompactYDocument implements POST /internal/realtime/compactions.
func (h *internalRealtimeHandler) CompactYDocument(c *gin.Context) {
	if !h.requireInternalKey(c) {
		return
	}
	if !h.requireDependencies(c) {
		return
	}
	var body internalYCompactBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "invalid request")
		return
	}
	if !contract.IsBoardID(strings.TrimSpace(body.BoardID)) || !contract.IsUserID(strings.TrimSpace(body.AuthorUID)) || len(body.Snapshot) == 0 {
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "invalid request")
		return
	}
	board, err := h.boards.GetBoardByPublicID(c.Request.Context(), strings.TrimSpace(body.BoardID))
	if err != nil || board == nil {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeBoardNotFound), "board not found")
		return
	}
	authorID, ok := h.resolveUID(c.Request.Context(), strings.TrimSpace(body.AuthorUID))
	if !ok {
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeBoardNotFound), "board not found")
		return
	}
	updated, err := h.boards.CompactYDocument(c.Request.Context(), board.WorkspacePublicID, board.PublicID, authorID, body.Snapshot)
	if err != nil {
		renderYDocStoreError(c, err)
		return
	}
	response.SuccessDTO(c, internalYCompactDTO{Compacted: true, Revision: updated.Revision})
}

// renderYDocStoreError maps Y seam failures to the internal status contract:
// capability and validation failures are permanent (4xx, the sidecar drops),
// storage failures are retryable (503, the sidecar retries). Collaborator
// writes never report 409: the store compacts without an expected revision.
func renderYDocStoreError(c *gin.Context, err error) {
	switch err {
	case boards.ErrInvalidInput:
		errorrender.Render(c, http.StatusBadRequest, string(contract.CodeBadRequest), "invalid request")
	case boards.ErrNotFound:
		errorrender.Render(c, http.StatusNotFound, string(contract.CodeBoardNotFound), "board not found")
	case boards.ErrPermissionDenied:
		errorrender.Render(c, http.StatusForbidden, string(contract.CodeBoardForbidden), "forbidden")
	case boards.ErrV1Archived:
		errorrender.Render(c, http.StatusConflict, string(contract.CodeBoardArchived), "board is archived")
	case boards.ErrRevisionConflict:
		response.InternalError(c, http.StatusServiceUnavailable, "board revision anomaly", err)
	default:
		response.InternalError(c, http.StatusServiceUnavailable, "board y-doc store unavailable", err)
	}
}
