package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/sse"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Workspace event streams + webhook delivery (issue #807).
//
// Surface:
//
//	GET    /api/v1/workspaces/{workspace_id}/events (SSE, any member)
//	GET    /api/v1/workspaces/{workspace_id}/event-subscriptions (owner|admin)
//	POST   /api/v1/workspaces/{workspace_id}/event-subscriptions (owner|admin, Idempotency-Key)
//	DELETE /api/v1/workspaces/{workspace_id}/event-subscriptions/{subscription_id} (owner|admin, Idempotency-Key)
//
// Visibility matrix: malformed ws_ shapes, unknown workspaces, and
// non-members converge to 404 event_not_found (Baseline 4
// anti-enumeration); visible members without owner|admin on
// subscription management get 403 event_forbidden; unknown
// subscription ids converge to 404 event_subscription_not_found.
//
// Exactly-once-from-domain-boundary producers (emit AFTER commit only;
// emission never fails the commit, sinks are best-effort):
//
//	task.created/cancelled  contract_v1_tasks.go create/cancel once-fns:
//	                        workspacetasks staged events drained after the
//	                        settlement commit (running/completed/failed flush
//	                        at the worker settle via the same staged seam;
//	                        wiring lands with the #809 worker flush).
//	billing.settled         same task once-fns only (held/free on create,
//	                        released on cancel). Refs only, no amounts.
//	                        Materialization never crossed billing.settled.
//	                        Stripe inbound webhooks stay distinct: they emit
//	                        internal user events, never workspace envelopes
//	                        (orders are user-scoped, no workspace mapping).
//	media.asset_ready       REMOVED with the workspace upload surface
//	                        (#809 phase 3b-03, issue #813): not emitted
//	                        while the replacement lands.
//	media.asset_archived    REMOVED with the workspace media surface
//	                        (same issue): not emitted while down.
//	board.document_replaced replaceBoardDocument once-fn after ReplaceDocument.
//	board.member_changed    member-add/member-remove once-fns after the
//	                        roster commit; member role update emits after its
//	                        direct store commit (no idempotency wrapper there).
//	board.share_revoked     share-revoke once-fn after RevokeShareToken.
//	library.materialization_* REMOVED with the HTTP materialization
//	                        surface (internal-only now: no HTTP emitter,
//	                        types dropped from the v1 registry).
//
// Retention/replay: NATS JetStream MaxAge (7 days per DESIGN) is the
// authoritative retention; the process-local hot log
// (events.WorkspaceEventLog, 1000 envelopes/workspace) is the live
// replay window. Cursors are Last-Event-ID evt_ ids bound to the
// workspace (+ principal membership + types filter): cross-workspace
// cursors cannot resume (workspace isolation), unknown/evicted cursors
// resume from the oldest retained envelope with a `gap` notice frame.

const eventStreamHeartbeatInterval = 15 * time.Second

func (h *contractV1Handler) registerEventRoutes(group *gin.RouterGroup) {
	group.GET("/workspaces/:workspace_id/events", h.streamWorkspaceEvents)
	group.GET("/workspaces/:workspace_id/event-subscriptions", h.listEventSubscriptions)
	group.POST("/workspaces/:workspace_id/event-subscriptions", h.createEventSubscription)
	group.DELETE("/workspaces/:workspace_id/event-subscriptions/:subscription_id", h.deleteEventSubscription)
}

// eventWorkspaceContext resolves the explicit workspace context for the
// events surface. Hidden workspaces (malformed ids, unknown ids, or
// caller holds no membership) converge to 404 event_not_found so the
// surface never leaks workspace existence.
func (h *contractV1Handler) eventWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, string, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeEventNotFound, msgEventNotFound)
		c.Abort()
		return nil, "", false
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeEventNotFound, msgEventNotFound)
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil || !member {
		contract.RenderError(c, http.StatusNotFound, contract.CodeEventNotFound, msgEventNotFound)
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
		contract.RenderError(c, http.StatusForbidden, contract.CodeEventForbidden, msgForbidden)
		c.Abort()
		return false
	}
	return true
}

func (h *contractV1Handler) requireEventSinks(c *gin.Context) bool {
	if h.deps.Events == nil || h.deps.Webhooks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable48)
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
			contract.CodeValidationFailed, msgValidationFailed6,
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid3)
		c.Abort()
		return "", false
	}
	return cursor, true
}

// streamWorkspaceEvents serves the versioned SSE stream: Last-Event-ID
// replay first (with a `gap` notice when the cursor is unknown or
// evicted), then live envelopes. Every frame carries `id:` so native
// EventSource resumes automatically.
func (h *contractV1Handler) streamWorkspaceEvents(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireEventSinks(c) {
		return
	}
	workspace, _, _ := h.eventWorkspaceContext(c, identityPrincipal)
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

func (h *contractV1Handler) listEventSubscriptions(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndEvents, Resolve: h.eventWorkspaceContext}, func(wctx v1WorkspaceCtx) {
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

func (h *contractV1Handler) createEventSubscription(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndEvents, Resolve: h.eventWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		if !requireEventSubscriptionAdmin(c, wctx.Role) {
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, msgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CreateEventSubscriptionBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		destination := strings.TrimSpace(body.Data.URL)
		if validationErr := events.ValidateWebhookURL(destination); validationErr != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
				contract.CodeValidationFailed, msgValidationFailed7,
				contract.FieldErrors{Fields: map[string]string{"url": "invalid"}})
			c.Abort()
			return
		}
		for _, typ := range body.Data.EventTypes {
			if strings.TrimSpace(typ) != "" && !events.IsV1EventType(typ) {
				contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
					contract.CodeValidationFailed, msgValidationFailed6,
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
					contract.CodeValidationFailed, msgValidationFailed8,
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
					return map[string]any{"subscription": candidate, "secret": events.FormatWhsecSecret(minted)}, nil
				}
			}
			return map[string]any{"subscription": sub.ID}, nil
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeEventSubscriptionConflict, msgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
				contract.CodeValidationFailed, msgValidationFailed9,
				contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
			c.Abort()
			return
		}
		contract.RenderCreated(c, result)
	})
}

func (h *contractV1Handler) deleteEventSubscription(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndEvents, Resolve: h.eventWorkspaceContext}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if !requireEventSubscriptionAdmin(c, wctx.Role) {
			return
		}
		if _, err := contract.IdempotencyKeyFromHeader(c); err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, msgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		subscriptionID := strings.TrimSpace(c.Param("subscription_id"))
		if !events.IsWebhookSubscriptionID(subscriptionID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeEventSubscriptionNotFound, msgEventSubscriptionNotFound)
			c.Abort()
			return
		}
		deleted, err := h.deps.Webhooks.Delete(c.Request.Context(), workspace.PublicID, subscriptionID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable48)
			c.Abort()
			return
		}
		if !deleted {
			contract.RenderError(c, http.StatusNotFound, contract.CodeEventSubscriptionNotFound, msgEventSubscriptionNotFound)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, v1DeletedDTO{Deleted: true, ID: subscriptionID})
	})
}

// emitWorkspaceEvent mints one envelope and fans it out best-effort:
// hot-log append (SSE replay + live), NATS publish (cross-instance
// transport), webhook dispatch (signed deliveries). A mint failure is
// logged and dropped; emission never fails the domain commit it
// follows. Callers invoke it AFTER the store commit succeeds, inside
// the idempotency once-fn so replays never re-emit.
func (h *contractV1Handler) emitWorkspaceEvent(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any) {
	h.emitWorkspaceEventWithID(ctx, identityPrincipal, workspaceID, typ, resource, data, "", time.Time{})
}

// emitWorkspaceEventWithID is the durable-outbox bridge. When a task
// transition already has an evt_ identity, reuse it and preserve its commit
// timestamp so HTTP delivery and replay converge on the same envelope.
func (h *contractV1Handler) emitWorkspaceEventWithID(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any, eventID string, occurredAt time.Time) {
	if h == nil {
		return
	}
	actor := events.V1Actor{Type: events.V1ActorUser, ID: tenancy.FormatUserPublicID(identityPrincipal.PublicID())}
	if strings.TrimSpace(eventID) != "" {
		actor = events.V1Actor{Type: events.V1ActorService, ID: "tasking"}
	}
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
func (h *contractV1Handler) emitTaskBillingSettled(ctx context.Context, identityPrincipal principal.Principal, workspaceID, taskID, settlementState, holdTransactionID string) {
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
