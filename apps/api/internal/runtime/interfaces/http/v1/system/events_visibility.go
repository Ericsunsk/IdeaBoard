package system

// Workspace event delivery visibility.
//
// Servers filter every user-visible event channel by CURRENT permission at
// DELIVERY time; workspaces are never broadcast-then-frontend-filtered:
// board ids, roster membership, prompts, and input/output asset identifiers
// are protected content. The rules live in events.IsBoardScopedEventType /
// IsDirectedAccessSignal (single home); this file binds them to the
// request principal through the narrow boards.RoleReader port
// (h.boardRoles()). No second role computation exists anywhere: callers
// resolve through GetBoardRole and render 404/403 from CheckCapability
// outcomes exactly like the boards HTTP surface.
//
//   - board.document_replaced: delivered only while
//     the viewer holds current board read access. Emit-time permission is
//     irrelevant: an envelope emitted before a revocation but delivered
//     after it is dropped, which closes the send-vs-revoke race and the
//     pre-revocation backlog (ordinary replay never returns now-invisible
//     history payloads).
//   - board.access_changed: directed control signal, delivered ONLY to the
//     named member (recipient derives from the prior grant row). Payload is
//     the known board identity + access version, never content. Never
//     dispatched to webhooks because Workspace webhooks have no member identity.
//   - task.* and task-linked billing.settled use current Task readability.
//   - media.* use canonical caller-aware Media readability.

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	eventoutbox "ideaboard/api/internal/events/outbox"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/workspacemedia"
	v1tasks "ideaboard/api/internal/runtime/interfaces/http/v1/tasks"
	"ideaboard/api/internal/tenancy"
)

// WorkspaceEventViewer is the delivery-time identity for one SSE
// connection: the internal user, their usr_ opaque id, and their current
// initial Workspace role. Delivery refreshes membership and role before
// ordinary frames. Board role facts resolve per envelope through
// h.boardRoles() so a mid-connection revocation takes effect on the very
// next frame.
type WorkspaceEventViewer struct {
	UserID        int64
	UserPublicID  string
	WorkspaceRole string
}

// EventViewerFor builds the delivery identity for the stream principal.
// ok=false on directory failure; callers then fail closed on ordinary
// content (the stream entry gate already rejected unauthenticated and
// non-member callers before this runs).
func (h *Handler) EventViewerFor(ctx context.Context, identityPrincipal principal.Principal, workspaceRole string) (WorkspaceEventViewer, bool) {
	viewer := WorkspaceEventViewer{UserID: identityPrincipal.UserID(), WorkspaceRole: strings.TrimSpace(workspaceRole)}
	if viewer.UserID <= 0 {
		return WorkspaceEventViewer{}, false
	}
	if h.deps.Users == nil {
		return WorkspaceEventViewer{}, false
	}
	publicID, err := h.deps.Users.GetUserPublicID(ctx, viewer.UserID)
	if err != nil {
		return WorkspaceEventViewer{}, false
	}
	viewer.UserPublicID = tenancy.FormatUserPublicID(publicID)
	if strings.TrimSpace(viewer.UserPublicID) == "" {
		return WorkspaceEventViewer{}, false
	}
	return viewer, true
}

func (h *Handler) eventViewerFor(ctx context.Context, identityPrincipal principal.Principal, workspaceRole string) (WorkspaceEventViewer, bool) {
	return h.EventViewerFor(ctx, identityPrincipal, workspaceRole)
}

// EventVisibleToViewer reports whether one envelope may be written to the
// viewer's SSE connection right now.
func (h *Handler) EventVisibleToViewer(ctx context.Context, workspacePublicID string, viewer WorkspaceEventViewer, envelope events.V1Envelope) bool {
	if events.IsDirectedAccessSignal(envelope) {
		recipient, ok := events.AccessChangedRecipient(envelope)
		if !ok {
			return false
		}
		return recipient == viewer.UserPublicID
	}
	if h.deps.Store == nil || viewer.UserID <= 0 {
		return false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(ctx, workspacePublicID, viewer.UserID)
	if err != nil || !member {
		return false
	}
	viewer.WorkspaceRole = role
	// Task content envelopes: task
	// lifecycle progress and task-referencing billing settlement reach
	// only the submitter or current Board readers, resolved live at
	// delivery so a mid-connection revocation takes effect on the very
	// next frame (same send-vs-revoke closure as board content).
	if v1tasks.IsTaskContentEvent(envelope) {
		return h.taskEventVisibleToViewer(ctx, workspacePublicID, viewer, envelope)
	}
	if envelope.Type == events.EventV1MediaAssetReady || envelope.Type == events.EventV1MediaAssetArchived {
		return h.mediaEventVisibleToViewer(ctx, workspacePublicID, viewer, envelope)
	}
	if !events.IsBoardScopedEventType(envelope.Type) {
		return true
	}
	boardID, ok := events.BoardIDOf(envelope)
	if !ok {
		return false
	}
	roles := h.boardRoles()
	if roles == nil {
		return false
	}
	_, hasRole, err := roles.GetBoardRole(ctx, workspacePublicID, boardID, viewer.UserID, viewer.WorkspaceRole)
	if err != nil {
		return false
	}
	return hasRole
}

func (h *Handler) eventVisibleToViewer(ctx context.Context, workspacePublicID string, viewer WorkspaceEventViewer, envelope events.V1Envelope) bool {
	return h.EventVisibleToViewer(ctx, workspacePublicID, viewer, envelope)
}

func (h *Handler) taskEventVisibleToViewer(ctx context.Context, workspacePublicID string, viewer WorkspaceEventViewer, envelope events.V1Envelope) bool {
	if h.deps.TaskEventVisibleToViewer != nil {
		return h.deps.TaskEventVisibleToViewer(ctx, workspacePublicID, viewer.UserID, viewer.WorkspaceRole, envelope)
	}
	return false
}

func (h *Handler) boardRoles() boards.RoleReader {
	return h.deps.Boards
}

func (h *Handler) mediaBoards() workspacemedia.BoardAccess {
	return h.deps.Boards
}

func (h *Handler) mediaEventVisibleToViewer(ctx context.Context, workspaceID string, viewer WorkspaceEventViewer, envelope events.V1Envelope) bool {
	if h.deps.Media.Media == nil || viewer.UserID <= 0 || envelope.Resource.Type != events.V1ResourceMediaAsset {
		return false
	}
	var data struct {
		AssetID string `json:"asset_id"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil || data.AssetID != envelope.Resource.ID {
		return false
	}
	asset, err := h.deps.Media.Media.AuthorizeAsset(ctx, workspaceID, data.AssetID, viewer.UserID, viewer.WorkspaceRole, h.mediaBoards())
	return err == nil && asset != nil
}

func (h *Handler) DeliverBoardAccessSignals(ctx context.Context, records []eventoutbox.BoardAccessOutboxRecord) {
	delivery := eventoutbox.BoardAccessSignals{Log: h.deps.Events, Bus: h.deps.EventBus}
	if h.deps.BoardAccessOutbox != nil {
		delivery.Store = h.deps.BoardAccessOutbox
	}
	if err := delivery.Deliver(ctx, records); err != nil {
		slog.Warn("deliver board access signals failed", "err", err)
	}
}
