package composition

// Control Plane task event adapter (admin cancel dead-channel fix).
//
// This is the ONLY place where the Control Plane's neutral
// httpapi.ControlTaskEventPort meets the real event layer. The Control
// Plane declares WHAT happened (one committed task status transition)
// in its own vocabulary; this adapter decides HOW it is delivered, and
// only this file may know that delivery means "publish to the task
// status channel" and "mint a workspace envelope and fan it out".
//
// WHY THE ADAPTER LIVES IN COMPOSITION
// Composition is the sole assembly root: it is the one place that
// already holds the task status publisher (built beside the River
// worker), the NATS bus, the workspace event log, and the webhook
// store. Keeping the translation here means the control handlers stay
// free of event infrastructure entirely, which is what
// TestControlNeverTouchesEventLayer guards: the Control Plane must
// never reach for the event layer directly, because an operator action
// is an administration concern, not an infrastructure one. The port
// keeps the dependency arrow pointing inward (composition -> control
// handlers) instead of outward (control handlers -> events).

import (
	"context"
	"log/slog"
	"strings"

	"ideaboard/api/internal/events"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
)

// controlPlaneActorID is the fallback actor for a control-plane
// announcement whose operator could not be resolved to a usr_ opaque id.
// The envelope validator rejects an empty actor id, so an unresolvable
// operator degrades to the service actor rather than failing the
// announcement or fabricating a user identity.
const controlPlaneActorID = "control-plane"

// controlTaskEventAdapter fans one committed Control Plane task
// transition out to the two delivery channels the port exposes.
type controlTaskEventAdapter struct {
	status   events.TaskStatusPublisher
	log      *events.WorkspaceEventLog
	bus      *events.NATSJetStreamEventBus
	webhooks *events.WebhookStore
}

var _ httpapi.ControlTaskEventPort = (*controlTaskEventAdapter)(nil)

func newControlTaskEventAdapter(
	status events.TaskStatusPublisher,
	log *events.WorkspaceEventLog,
	bus *events.NATSJetStreamEventBus,
	webhooks *events.WebhookStore,
) *controlTaskEventAdapter {
	return &controlTaskEventAdapter{status: status, log: log, bus: bus, webhooks: webhooks}
}

// PublishTaskStatus reaches the cross-instance task status channel and
// the local task/user hubs. PublishTaskStatusBestEffort owns the
// timeout and the detached context, so a slow bus can never hold the
// operator's request open, and it tolerates a nil publisher (single
// process, bus disabled) instead of panicking.
func (a *controlTaskEventAdapter) PublishTaskStatus(ctx context.Context, event httpapi.ControlTaskStatusEvent) error {
	return events.PublishTaskStatusBestEffort(ctx, a.status, events.TaskStatusEvent{
		Type:       events.EventTypeTaskStatusChanged,
		EventID:    event.EventID,
		TaskID:     event.TaskID,
		Status:     events.TaskStatus(event.Status),
		OccurredAt: event.OccurredAt,
	})
}

// EmitWorkspaceTaskEvent mints one workspace envelope and fans it out
// best-effort: hot-log append (SSE replay + live), NATS publish
// (cross-instance), and signed webhook dispatch. This mirrors the
// workspace-scoped cancel edge exactly, so an operator cancel and a
// user cancel reach subscribers through the same envelope shape.
//
// Fan-out failures are warn-logged and swallowed: the domain fact is
// already committed and the envelope id is minted once, so a partial
// fan-out must not fail the announcement (or, upstream, the cancel).
func (a *controlTaskEventAdapter) EmitWorkspaceTaskEvent(ctx context.Context, event httpapi.ControlTaskStatusEvent) error {
	typ, ok := events.TaskStatusV1Type(events.TaskStatus(event.Status))
	if !ok {
		slog.Warn("control task transition has no workspace event type", "task_id", event.TaskID, "status", event.Status)
		return nil
	}
	data, ok := controlWorkspaceEventData(event)
	if !ok {
		// Fail closed rather than mint an envelope whose typed payload would
		// not validate: task.created needs a service, task.completed an
		// outputs URI, task.failed an error code, none of which the control
		// settlement port carries.
		slog.Warn("control task transition has no workspace payload", "task_id", event.TaskID, "type", typ)
		return nil
	}
	var envelope events.V1Envelope
	var err error
	if strings.TrimSpace(event.EventID) != "" {
		envelope, err = events.MintV1EnvelopeWithID(
			event.EventID,
			typ,
			event.WorkspaceID,
			events.V1Actor{Type: events.V1ActorService, ID: "tasking"},
			events.V1Resource{Type: events.V1ResourceTask, ID: event.TaskID},
			data,
		)
		envelope.OccurredAt = event.OccurredAt.UTC()
		if err == nil {
			err = events.ValidateV1Envelope(envelope)
		}
	} else {
		envelope, err = events.MintV1Envelope(
			typ,
			event.WorkspaceID,
			controlPlaneActor(event.ActorUserID),
			events.V1Resource{Type: events.V1ResourceTask, ID: event.TaskID},
			data,
		)
	}
	if err != nil {
		slog.Warn("mint control workspace event failed", "type", typ, "task_id", event.TaskID, "err", err)
		return nil
	}
	if a.log != nil {
		a.log.Append(envelope)
	}
	if a.bus != nil {
		if publishErr := a.bus.PublishWorkspaceEvent(ctx, envelope); publishErr != nil {
			slog.Warn("publish control workspace event failed", "id", envelope.ID, "type", typ, "err", publishErr)
		}
	}
	if a.webhooks != nil {
		a.webhooks.Dispatch(ctx, envelope)
	}
	if strings.TrimSpace(event.SettlementState) != "" {
		billingEnvelope, billingErr := events.MintV1Envelope(
			events.EventV1BillingSettled,
			event.WorkspaceID,
			controlPlaneActor(event.ActorUserID),
			events.V1Resource{Type: events.V1ResourceTask, ID: event.TaskID},
			events.V1BillingSettledData{
				ReferenceType: "task", ReferenceID: event.TaskID,
				SettlementState:   strings.TrimSpace(event.SettlementState),
				HoldTransactionID: strings.TrimSpace(event.HoldTransactionID),
			},
		)
		if billingErr != nil {
			slog.Warn("mint control billing settlement event failed", "task_id", event.TaskID, "err", billingErr)
			return nil
		}
		if a.log != nil {
			a.log.Append(billingEnvelope)
		}
		if a.bus != nil {
			if publishErr := a.bus.PublishWorkspaceEvent(ctx, billingEnvelope); publishErr != nil {
				slog.Warn("publish control billing settlement event failed", "id", billingEnvelope.ID, "err", publishErr)
			}
		}
		if a.webhooks != nil {
			a.webhooks.Dispatch(ctx, billingEnvelope)
		}
	}
	return nil
}

// controlPlaneActor prefers the operator's own usr_ opaque id so the
// envelope attributes the cancel to a real actor, and degrades to the
// Control Plane service actor when the operator is unknown.
func controlPlaneActor(actorUserID string) events.V1Actor {
	actorUserID = strings.TrimSpace(actorUserID)
	if actorUserID != "" {
		return events.V1Actor{Type: events.V1ActorUser, ID: actorUserID}
	}
	return events.V1Actor{Type: events.V1ActorService, ID: controlPlaneActorID}
}

// controlWorkspaceEventData builds the typed payload for the statuses the
// control settlement port can fully populate. Statuses whose v1 payload
// requires context the port does not carry report ok=false so the caller
// fails closed instead of emitting an envelope that cannot validate.
func controlWorkspaceEventData(event httpapi.ControlTaskStatusEvent) (any, bool) {
	switch events.TaskStatus(event.Status) {
	case events.TaskStatusCancelled:
		return events.V1TaskCancelledData{TaskID: event.TaskID, CancelReason: event.Reason}, true
	case events.TaskStatusRunning:
		return events.V1TaskRunningData{TaskID: event.TaskID}, true
	default:
		return nil, false
	}
}
