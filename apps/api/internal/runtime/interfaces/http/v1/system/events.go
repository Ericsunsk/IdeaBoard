package system

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	domainboards "ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	eventoutbox "ideaboard/api/internal/events/outbox"
	eventwebhook "ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/sse"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1library "ideaboard/api/internal/runtime/interfaces/http/v1/library"
	v1media "ideaboard/api/internal/runtime/interfaces/http/v1/media"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	v1tasks "ideaboard/api/internal/runtime/interfaces/http/v1/tasks"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/gin-gonic/gin"
)

// Workspace members may open SSE streams; subscription management requires
// Owner or Admin. Hidden workspaces and non-members converge to event_not_found.
// Live and replay envelopes use current recipient authorization before delivery.
// Workspace webhooks carry only independently Workspace-wide settlement events.
//
// NATS JetStream retains events for seven days. The process-local hot log keeps
// up to 1000 envelopes per Workspace for live replay. Last-Event-ID cursors are
// Workspace-scoped; an unknown or evicted cursor produces a gap notice.

const eventStreamHeartbeatInterval = 15 * time.Second

type Deps struct {
	Store                    tenancy.Store
	Users                    runtimehttp.V1UserDirectory
	Boards                   domainboards.WorkspaceBoardStore
	Media                    v1media.ContractV1MediaDeps
	Tasks                    v1tasks.ContractV1TaskDeps
	Library                  v1library.ContractV1LibraryDeps
	Invitations              *invitations.Service
	InvitationReader         invitations.Reader
	Search                   ContractV1SearchDeps
	Notifications            notifications.Store
	UserEvents               *events.UserEventHub
	Events                   *events.WorkspaceEventLog
	Webhooks                 *eventwebhook.WebhookStore
	EventBus                 *eventnats.NATSJetStreamEventBus
	BoardAccessOutbox        *eventoutbox.BoardAccessOutboxStore
	SupportChannels          settings.SupportChannelsSettingProvider
	Branding                 settings.BrandingSettingProvider
	PublicSearchHandler      *searchhttp.SearchHandler
	Idempotency              *contract.IdempotencyStore
	TaskEventVisibleToViewer func(ctx context.Context, workspacePublicID string, viewerID int64, viewerRole string, envelope events.V1Envelope) bool
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	deps.Search.normalize()
	if deps.TaskEventVisibleToViewer == nil && deps.Tasks.Tasks != nil {
		deps.TaskEventVisibleToViewer = func(ctx context.Context, workspacePublicID string, viewerID int64, viewerRole string, envelope events.V1Envelope) bool {
			taskHandler := v1tasks.NewHandler(v1tasks.Deps{
				Store:  deps.Store,
				Boards: deps.Boards,
				Tasks:  deps.Tasks,
			})
			return taskHandler.TaskEventVisibleToViewer(ctx, workspacePublicID, viewerID, viewerRole, envelope)
		}
	}
	return &Handler{deps: deps}
}

func (h *Handler) Mount(group *gin.RouterGroup) {
	// Search routes
	group.GET("/workspaces/:workspace_id/search", h.searchWorkspace)
	group.POST("/workspaces/:workspace_id/search/vision-queries", h.createVisionQuery)

	// Event routes
	group.GET("/workspaces/:workspace_id/events", h.streamWorkspaceEvents)
	group.GET("/workspaces/:workspace_id/event-subscriptions", h.listEventSubscriptions)
	group.POST("/workspaces/:workspace_id/event-subscriptions", h.createEventSubscription)
	group.DELETE("/workspaces/:workspace_id/event-subscriptions/:subscription_id", h.deleteEventSubscription)

	// User notification routes
	group.GET("/user/notifications", h.listUserNotifications)
	group.GET("/user/notifications/events", h.streamUserNotificationEvents)
	group.GET("/user/notifications/unread-count", h.getUserNotificationsUnreadCount)
	group.POST("/user/notifications/:id/read", h.markUserNotificationRead)
	group.POST("/user/notifications/read-all", h.markAllUserNotificationsRead)
	group.POST("/user/notifications/:id/archive", h.archiveUserNotification)
}

func (h *Handler) MountPublic(group *gin.RouterGroup) {
	group.GET("/support-channels", PublicSupportChannelsV1Handler(h.deps.SupportChannels))
	group.GET("/settings/branding", PublicBrandingSettingsV1Handler(h.deps.Branding))
	group.GET("/search", PublicSearchV1Handler(h.deps.PublicSearchHandler))
}

func (h *Handler) EmitWorkspaceEvent(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any) {
	h.emitWorkspaceEvent(ctx, identityPrincipal, workspaceID, typ, resource, data)
}

func (h *Handler) EmitWorkspaceEventWithID(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any, eventID string, occurredAt time.Time) {
	h.emitWorkspaceEventWithID(ctx, identityPrincipal, workspaceID, typ, resource, data, eventID, occurredAt)
}

func (h *Handler) EmitWorkspaceEventWithActor(ctx context.Context, actor events.V1Actor, workspaceID, typ string, resource events.V1Resource, data any, eventID string, occurredAt time.Time) {
	h.emitWorkspaceEventWithActor(ctx, actor, workspaceID, typ, resource, data, eventID, occurredAt)
}

func (h *Handler) EmitTaskBillingSettled(ctx context.Context, identityPrincipal principal.Principal, workspaceID, taskID, settlementState, holdTransactionID string) {
	h.emitTaskBillingSettled(ctx, identityPrincipal, workspaceID, taskID, settlementState, holdTransactionID)
}

func (h *Handler) withWorkspace(c *gin.Context, opts shared.WorkspaceOpts, fn func(shared.WorkspaceCtx)) {
	require := opts.Require
	if require == nil {
		require = h.requireStore
	}
	resolve := opts.Resolve
	if resolve == nil {
		resolve = h.eventWorkspaceContext
	}
	shared.WorkspacePrelude(c, require, resolve, opts.Allow, fn)
}

func (h *Handler) requireStoreAndEvents(c *gin.Context) bool {
	if h.deps.Store == nil || h.deps.Events == nil {
		contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, runtimehttp.MsgContractNotImplemented)
		c.Abort()
		return false
	}
	return true
}

func (h *Handler) requireStore(c *gin.Context) bool {
	if h.deps.Store == nil {
		contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, runtimehttp.MsgContractNotImplemented)
		c.Abort()
		return false
	}
	return true
}

// eventWorkspaceContext resolves the explicit workspace context for the
// events surface. Hidden workspaces (malformed ids, unknown ids, or
// caller holds no membership) converge to 404 event_not_found so the
// surface never leaks workspace existence.
func (h *Handler) eventWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, string, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeEventNotFound, runtimehttp.MsgEventNotFound)
		c.Abort()
		return nil, "", false
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeEventNotFound, runtimehttp.MsgEventNotFound)
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil || !member {
		contract.RenderError(c, http.StatusNotFound, contract.CodeEventNotFound, runtimehttp.MsgEventNotFound)
		c.Abort()
		return nil, "", false
	}
	return workspace, role, true
}

// requireEventSubscriptionAdmin gates webhook subscription management to
// workspace owners/admins: webhook URLs and secrets are sensitive, and
// member enumeration of delivery targets stays privileged.
func requireEventSubscriptionAdmin(c *gin.Context, role string) bool {
	if role != tenancy.RoleOwner && role != tenancy.RoleAdmin {
		contract.RenderError(c, http.StatusForbidden, contract.CodeEventForbidden, runtimehttp.MsgForbidden)
		c.Abort()
		return false
	}
	return true
}

func (h *Handler) requireEventSinks(c *gin.Context) bool {
	if h.deps.Events == nil || h.deps.Webhooks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable48)
		c.Abort()
		return false
	}
	return true
}

func parseEventTypesFilter(c *gin.Context) (events.WorkspaceEventFilter, bool) {
	raw := strings.TrimSpace(c.Query("types"))
	var names []string
	if raw != "" {
		names = strings.Split(raw, ",")
	}
	filter, err := events.NewWorkspaceEventFilter(names)
	if err != nil {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
			contract.CodeValidationFailed, runtimehttp.MsgValidationFailed6,
			contract.FieldErrors{Fields: map[string]string{"types": "invalid"}})
		c.Abort()
		return events.WorkspaceEventFilter{}, false
	}
	return filter, true
}

func parseEventCursor(c *gin.Context) (string, bool) {
	cursor := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if cursor == "" {
		cursor = strings.TrimSpace(c.Query("last_event_id"))
	}
	if cursor != "" && !contract.IsEventID(cursor) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
		c.Abort()
		return "", false
	}
	return cursor, true
}

// streamWorkspaceEvents serves the versioned SSE stream: Last-Event-ID
// replay first (with a `gap` notice when the cursor is unknown or
// evicted), then live envelopes. Every frame carries `id:` so native
// EventSource resumes automatically.
//
// Replay and live frames pass
// eventVisibleToViewer for this connection's principal, evaluated against
// CURRENT board access per envelope (never emit-time, never
// frontend-filtered). Revoked viewers observe only their directed
// board.access_changed signals; ordinary replay never returns
// now-invisible history payloads.
func (h *Handler) streamWorkspaceEvents(c *gin.Context) {
	identityPrincipal, ok := shared.RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireEventSinks(c) {
		return
	}
	workspace, workspaceRole, _ := h.eventWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	filter, ok := parseEventTypesFilter(c)
	if !ok {
		return
	}
	cursor, ok := parseEventCursor(c)
	if !ok {
		return
	}
	viewer, _ := h.eventViewerFor(c.Request.Context(), identityPrincipal, workspaceRole)
	missed, gap, _ := h.deps.Events.ReplayAfter(workspace.PublicID, cursor, filter)
	if err := sse.Prepare(c); err != nil {
		slog.Warn("prepare workspace events SSE failed", "err", err)
	}
	c.Header("Cache-Control", "no-store")
	if _, err := c.Writer.Write([]byte(":connected\n\n")); err != nil {
		return
	}
	c.Writer.Flush()
	if cursor != "" && gap {
		if err := sse.WriteIDEvent(c, cursor, "gap", map[string]any{"truncated": true}); err != nil {
			return
		}
	}
	for _, envelope := range missed {
		if !h.eventVisibleToViewer(c.Request.Context(), workspace.PublicID, viewer, envelope) {
			continue
		}
		if err := sse.WriteIDEvent(c, envelope.ID, envelope.Type, envelope); err != nil {
			return
		}
	}
	stream, unsubscribe, accepted := h.deps.Events.Subscribe(workspace.PublicID, filter)
	if !accepted || stream == nil {
		if err := sse.WriteIDEvent(c, cursor, "error", map[string]any{"code": string(contract.CodeRateLimited)}); err != nil {
			return
		}
		return
	}
	defer unsubscribe()
	heartbeat := time.NewTicker(eventStreamHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case envelope, open := <-stream:
			if !open {
				return
			}
			if !h.eventVisibleToViewer(c.Request.Context(), workspace.PublicID, viewer, envelope) {
				continue
			}
			if err := sse.WriteIDEvent(c, envelope.ID, envelope.Type, envelope); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := c.Writer.Write([]byte(":keepalive\n\n")); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func (h *Handler) listEventSubscriptions(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Require: h.requireStoreAndEvents, Resolve: h.eventWorkspaceContext}, func(wctx shared.WorkspaceCtx) {
		workspace := wctx.Workspace
		if !requireEventSubscriptionAdmin(c, wctx.Role) {
			return
		}
		contract.RenderSuccess(c, map[string]any{"subscriptions": h.deps.Webhooks.List(workspace.PublicID)})
	})
}

type v1CreateEventSubscriptionBody struct {
	Data struct {
		URL        string   `json:"url"`
		Secret     *string  `json:"secret"`
		EventTypes []string `json:"event_types"`
	} `json:"data"`
}

func (h *Handler) createEventSubscription(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Require: h.requireStoreAndEvents, Resolve: h.eventWorkspaceContext}, func(wctx shared.WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		if !requireEventSubscriptionAdmin(c, wctx.Role) {
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CreateEventSubscriptionBody
		if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
			shared.WriteInputError(c)
			return
		}
		destination := strings.TrimSpace(body.Data.URL)
		if validationErr := eventwebhook.ValidateWebhookURL(destination); validationErr != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
				contract.CodeValidationFailed, runtimehttp.MsgValidationFailed7,
				contract.FieldErrors{Fields: map[string]string{"url": "invalid"}})
			c.Abort()
			return
		}
		for _, typ := range body.Data.EventTypes {
			if strings.TrimSpace(typ) != "" && !eventwebhook.IsWorkspaceWebhookEventType(typ) {
				contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
					contract.CodeValidationFailed, runtimehttp.MsgValidationFailed6,
					contract.FieldErrors{Fields: map[string]string{"event_types": "invalid"}})
				c.Abort()
				return
			}
		}
		var secret []byte
		if body.Data.Secret != nil {
			secret = []byte(*body.Data.Secret)
			if len(secret) > 0 && len(secret) < 16 {
				contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
					contract.CodeValidationFailed, runtimehttp.MsgValidationFailed8,
					contract.FieldErrors{Fields: map[string]string{"secret": "too_short"}})
				c.Abort()
				return
			}
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		scope := contract.IdempotencyScope(externalUser, "event-subscription:create")
		hash := contract.HashIdempotencyRequest("event-subscription:create", workspace.PublicID, destination, strings.Join(body.Data.EventTypes, ","))
		result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
			sub, minted, _, createErr := h.deps.Webhooks.Create(c.Request.Context(), workspace.PublicID, destination, body.Data.EventTypes, secret)
			if createErr != nil {
				return nil, createErr
			}
			view := h.deps.Webhooks.List(workspace.PublicID)
			for _, candidate := range view {
				if candidate.ID == sub.ID {
					if len(minted) == 0 {
						return map[string]any{"subscription": candidate}, nil
					}
					// Minted secrets are whsec_-encoded for transport and
					// returned exactly once; reads never carry them.
					return map[string]any{"subscription": candidate, "secret": eventwebhook.FormatWhsecSecret(minted)}, nil
				}
			}
			return map[string]any{"subscription": sub.ID}, nil
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeEventSubscriptionConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
				contract.CodeValidationFailed, runtimehttp.MsgValidationFailed9,
				contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
			c.Abort()
			return
		}
		contract.RenderCreated(c, result)
	})
}

func (h *Handler) deleteEventSubscription(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Require: h.requireStoreAndEvents, Resolve: h.eventWorkspaceContext}, func(wctx shared.WorkspaceCtx) {
		workspace := wctx.Workspace
		if !requireEventSubscriptionAdmin(c, wctx.Role) {
			return
		}
		if _, err := contract.IdempotencyKeyFromHeader(c); err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		subscriptionID := strings.TrimSpace(c.Param("subscription_id"))
		if !eventwebhook.IsWebhookSubscriptionID(subscriptionID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeEventSubscriptionNotFound, runtimehttp.MsgEventSubscriptionNotFound)
			c.Abort()
			return
		}
		deleted, err := h.deps.Webhooks.Delete(c.Request.Context(), workspace.PublicID, subscriptionID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable48)
			c.Abort()
			return
		}
		if !deleted {
			contract.RenderError(c, http.StatusNotFound, contract.CodeEventSubscriptionNotFound, runtimehttp.MsgEventSubscriptionNotFound)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, shared.DeletedDTO{Deleted: true, ID: subscriptionID})
	})
}

// NewBoardDocumentPublisher shares the public document mutation event pipeline
// with internal Y.Doc compactions. Both call it only after a snapshot commits.
func NewBoardDocumentPublisher(log *events.WorkspaceEventLog, bus *eventnats.NATSJetStreamEventBus, webhooks *eventwebhook.WebhookStore) runtimehttp.BoardDocumentPublisher {
	h := &Handler{deps: Deps{Events: log, EventBus: bus, Webhooks: webhooks}}
	return func(ctx context.Context, workspaceID, boardID, authorUID string, revision int64) {
		h.emitWorkspaceEventWithActor(ctx, events.V1Actor{Type: events.V1ActorUser, ID: authorUID},
			workspaceID, events.EventV1BoardDocumentReplaced,
			events.V1Resource{Type: events.V1ResourceBoard, ID: boardID},
			events.V1BoardDocumentReplacedData{BoardID: boardID, Revision: revision}, "", time.Time{})
	}
}

// emitWorkspaceEvent mints one envelope and fans it out best-effort:
// hot-log append (SSE replay + live), NATS publish (cross-instance
// transport), webhook dispatch (signed deliveries). A mint failure is
// logged and dropped; emission never fails the domain commit it
// follows. Callers invoke it AFTER the store commit succeeds, inside
// the idempotency once-fn so replays never re-emit.
func (h *Handler) emitWorkspaceEvent(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any) {
	h.emitWorkspaceEventWithID(ctx, identityPrincipal, workspaceID, typ, resource, data, "", time.Time{})
}

// emitWorkspaceEventWithID is the durable-outbox bridge. When a task
// transition already has an evt_ identity, reuse it and preserve its commit
// timestamp so HTTP delivery and replay converge on the same envelope.
func (h *Handler) emitWorkspaceEventWithID(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any, eventID string, occurredAt time.Time) {
	if h == nil {
		return
	}
	actor := events.V1Actor{Type: events.V1ActorUser, ID: tenancy.FormatUserPublicID(identityPrincipal.PublicID())}
	if strings.TrimSpace(eventID) != "" {
		actor = events.V1Actor{Type: events.V1ActorService, ID: "tasking"}
	}
	h.emitWorkspaceEventWithActor(ctx, actor, workspaceID, typ, resource, data, eventID, occurredAt)
}

func (h *Handler) emitWorkspaceEventWithActor(ctx context.Context, actor events.V1Actor, workspaceID, typ string, resource events.V1Resource, data any, eventID string, occurredAt time.Time) {
	var envelope events.V1Envelope
	var err error
	if strings.TrimSpace(eventID) != "" {
		envelope, err = events.MintV1EnvelopeWithID(eventID, typ, workspaceID, actor, resource, data)
		if !occurredAt.IsZero() {
			envelope.OccurredAt = occurredAt.UTC()
			err = events.ValidateV1Envelope(envelope)
		}
	} else {
		envelope, err = events.MintV1Envelope(typ, workspaceID, actor, resource, data)
	}
	if err != nil {
		slog.Warn("mint workspace event failed", "type", typ, "workspace", workspaceID, "err", err)
		return
	}
	if h.deps.Events != nil {
		h.deps.Events.Append(envelope)
	}
	if h.deps.EventBus != nil {
		if publishErr := h.deps.EventBus.PublishWorkspaceEvent(ctx, envelope); publishErr != nil {
			slog.Warn("publish workspace event failed", "id", envelope.ID, "type", typ, "err", publishErr)
		}
	}
	if h.deps.Webhooks != nil {
		h.deps.Webhooks.Dispatch(ctx, envelope)
	}
}

// emitTaskBillingSettled emits billing.settled with settlement refs only
// (no amounts): the hold/quote/capture/release reference plus state.
func (h *Handler) emitTaskBillingSettled(ctx context.Context, identityPrincipal principal.Principal, workspaceID, taskID, settlementState, holdTransactionID string) {
	state := strings.TrimSpace(settlementState)
	if state == "" {
		return
	}
	h.emitWorkspaceEvent(ctx, identityPrincipal, workspaceID, events.EventV1BillingSettled,
		events.V1Resource{Type: events.V1ResourceTask, ID: taskID},
		events.V1BillingSettledData{
			ReferenceType: events.V1ResourceTask, ReferenceID: taskID,
			SettlementState: state, HoldTransactionID: strings.TrimSpace(holdTransactionID),
		})
}
