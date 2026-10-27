package v1

// Notification realtime hints (P1 notification domain).
//
// Browser push for the user notification center reuses the versioned
// workspace event infrastructure: notification.created/updated are frozen
// v1 registry members (see internal/events/v1_envelope.go) carrying
// refs only (invite_id + notification_type, never titles/bodies/emails).
// Frames are a "refetch now" prompt: clients invalidate and GET, payloads
// never write the cache.
//
// Two browser-reachable outlets share one envelope shape and one frontend
// transport implementation:
//
//   - workspace SSE (`GET /workspaces/{id}/events`, membership-gated like
//     billing.settled) for workspace-homed notification writes;
//   - the per-user stream below (`GET /user/notifications/events`,
//     recipient-addressed, no membership required) so pending invitees and
//     other non-members converge without polling.
//
// Notifications owns fact-derived post-commit emission with deterministic
// evt_ ids, so replays and redeliveries
// collapse in the hot log and the NATS duplicate window. Inbox hints never
// ride webhooks: they are principal-scoped, and the webhook allowlist
// already admits billing.settled only. Organization and billing
// notifications have no ws_ home for a v1 envelope, so they stay on the
// 30s poll fallback in this cut.

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/sse"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

const userNotificationStreamHeartbeatInterval = 15 * time.Second

// streamUserNotificationEvents serves the recipient-addressed notification
// hint stream: every frame is a validated notification.created/updated v1
// envelope for the authenticated user. Unlike the workspace stream there is
// no per-user replay window, so a cursor can never be honored: any
// last_event_id resumes with a `gap` notice and the client takes the
// mandated full pull (the same invalidate path the push uses). The
// recipient match IS the visibility rule (hub events are keyed by user id),
// so no membership check applies: pending invitees converge here.
func (h *contractV1Handler) streamUserNotificationEvents(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	hub := h.deps.UserEvents
	if hub == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable48)
		c.Abort()
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
	if err := sse.Prepare(c); err != nil {
		slog.Warn("prepare user notification events SSE failed", "err", err)
	}
	c.Header("Cache-Control", "no-store")
	if _, err := c.Writer.Write([]byte(":connected\n\n")); err != nil {
		return
	}
	c.Writer.Flush()
	if cursor != "" {
		if err := sse.WriteIDEvent(c, cursor, "gap", map[string]any{"truncated": true}); err != nil {
			return
		}
	}
	stream, unsubscribe, accepted := hub.Subscribe(identityPrincipal.UserID())
	if !accepted || stream == nil {
		if err := sse.WriteIDEvent(c, cursor, "error", map[string]any{"code": string(contract.CodeRateLimited)}); err != nil {
			return
		}
		return
	}
	defer unsubscribe()
	heartbeat := time.NewTicker(userNotificationStreamHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-stream:
			if !open {
				return
			}
			envelope, ok := userNotificationEnvelopeFor(event, filter)
			if !ok {
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

// userNotificationEnvelopeFor projects one user-hub event to its v1 hint
// envelope. Non-notification hub traffic (task/payment hints) and anything
// failing envelope validation fail closed: dropped, never delivered.
func userNotificationEnvelopeFor(event events.UserEvent, filter events.WorkspaceEventFilter) (events.V1Envelope, bool) {
	typ := strings.TrimSpace(event.Type)
	if typ != events.EventV1NotificationCreated && typ != events.EventV1NotificationUpdated {
		return events.V1Envelope{}, false
	}
	if !filter.Matches(typ) {
		return events.V1Envelope{}, false
	}
	var envelope events.V1Envelope
	if err := json.Unmarshal(event.Data, &envelope); err != nil {
		return events.V1Envelope{}, false
	}
	if strings.TrimSpace(envelope.Type) != typ {
		return events.V1Envelope{}, false
	}
	if err := events.ValidateV1Envelope(envelope); err != nil {
		return events.V1Envelope{}, false
	}
	return envelope, true
}
