package notifications

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	"ideaboard/api/internal/platform/contract"
)

// EventPublisher adapts committed notification facts to the existing refs-only
// workspace and recipient streams. Missing workspace homes remain poll-only.
type EventPublisher struct {
	Log *events.WorkspaceEventLog
	Bus *eventnats.NATSJetStreamEventBus
	Hub *events.UserEventHub
}

func (p *EventPublisher) Publish(ctx context.Context, actor events.V1Actor, typ string, fact *Notification) {
	if p == nil || fact == nil || fact.UserID <= 0 || fact.PublicID == "" {
		return
	}
	if typ != events.EventV1NotificationCreated && typ != events.EventV1NotificationUpdated {
		return
	}
	workspaceID, _ := fact.ActionPayload[PayloadKeyWorkspaceID].(string)
	workspaceID = strings.TrimSpace(workspaceID)
	if !contract.IsWorkspaceID(workspaceID) {
		return
	}
	inviteID, _ := fact.ActionPayload[PayloadKeyInviteID].(string)
	if inviteID == "" {
		inviteID, _ = fact.ActionPayload[PayloadKeyTransferInviteID].(string)
	}
	if inviteID == "" {
		return
	}
	// A row's identity separates recipients; persisted state separates resolution,
	// read and archive transitions, even when PostgreSQL gives them one timestamp.
	version := ""
	if typ == events.EventV1NotificationUpdated {
		payload, _ := json.Marshal(fact.ActionPayload)
		version = string(fact.Status) + ":" + fact.UpdatedAt.UTC().Format(time.RFC3339Nano) + ":" + string(payload)
	}
	envelope, err := events.MintV1EnvelopeWithID(events.StableEventID("notification", typ, fact.PublicID, version), typ, workspaceID, actor,
		events.V1Resource{Type: events.V1ResourceInvite, ID: inviteID},
		events.V1NotificationData{InviteID: inviteID, NotificationType: string(fact.Type)})
	if err != nil {
		slog.Warn("mint notification hint failed", "notification", fact.PublicID, "err", err)
		return
	}
	if p.Log != nil {
		p.Log.Append(envelope)
	}
	if p.Bus != nil {
		if err := p.Bus.PublishWorkspaceEvent(ctx, envelope); err != nil {
			slog.Warn("publish notification hint failed", "id", envelope.ID, "err", err)
		}
	}
	if p.Hub != nil {
		payload, err := json.Marshal(envelope)
		if err == nil {
			p.Hub.Publish(events.UserEvent{UserID: fact.UserID, Type: typ, Data: payload, OccurredAt: envelope.OccurredAt})
		}
	}
}
