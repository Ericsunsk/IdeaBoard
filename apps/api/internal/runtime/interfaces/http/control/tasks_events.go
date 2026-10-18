package control

// Control task status announcements use a transport-neutral port.
// Handlers report committed transitions with workspace identity, cancellation
// cause, and acting operator. Composition owns delivery infrastructure.

import (
	"context"
	"time"
)

// ControlTaskStatusEvent is one committed workspace task status
// transition, expressed in Control Plane vocabulary. Every field is
// something the delivery side must know to route and validate the
// announcement; nothing here describes a transport.
//
// Field semantics:
//
//   - WorkspaceID is the ws_ public id the task belongs to. It is what
//     scopes the per-workspace event log and the webhook subscription
//     lookup, so it is required (the adapter drops an announcement
//     without it rather than emitting an unscoped envelope).
//   - TaskID is the task_ public id that transitioned.
//   - Status is the workspacetasks lifecycle word ("cancelled").
//   - Reason is the system cancellation cause recorded by the
//     settlement. It rides the task.cancelled payload.
//   - ActorUserID is the acting operator's usr_ opaque public id, empty
//     when the operator could not be resolved (the adapter then records
//     the Control Plane itself as a service actor rather than inventing a
//     user identity).
//   - OccurredAt is when the transition committed (stamped by the
//     settlement Service after its transaction, never before).
//   - SettlementState and HoldTransactionID carry the post-cancel billing
//     projection so the adapter can emit billing.settled without exposing
//     amounts. They are empty for transitions without a settlement fact.
type ControlTaskStatusEvent struct {
	EventID           string
	WorkspaceID       string
	TaskID            string
	Status            string
	Reason            string
	ActorUserID       string
	OccurredAt        time.Time
	SettlementState   string
	HoldTransactionID string
}

// ControlTaskEventPort is the Control Plane's narrow, transport-agnostic
// announcement seam. Production wires it to a composition adapter that
// fans each transition out to the task status channel and to the
// workspace event envelope; tests wire a recording fake.
//
// The two methods are TWO DISTINCT DELIVERY CHANNELS, not two copies of
// one event, and a caller must invoke both exactly once per transition:
//
//   - PublishTaskStatus reaches the cross-instance task status channel
//     and the local task/user hubs (the same channel the River worker
//     settler bridge uses for its terminal transitions).
//   - EmitWorkspaceTaskEvent reaches the workspace event envelope, i.e.
//     the SSE hot window, the workspace bus subject, and signed webhook
//     dispatch (the same channel the workspace-scoped cancel handler
//     uses).
//
// Announcements are strictly DOWNSTREAM of a committed domain fact and
// never fail it: callers pass an already-settled transition and treat an
// error as a warn-level condition, never as a settlement failure.
type ControlTaskEventPort interface {
	PublishTaskStatus(ctx context.Context, event ControlTaskStatusEvent) error
	EmitWorkspaceTaskEvent(ctx context.Context, event ControlTaskStatusEvent) error
}
