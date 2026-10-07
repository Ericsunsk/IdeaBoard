package httpapi

// Control task status announcements (admin cancel dead-channel fix).
//
// WHY A NEUTRAL PORT INSTEAD OF IMPORTING THE EVENT LAYER
// -------------------------------------------------------
// The Control Plane is an administration surface: capability auth, an
// operator reason, a mandatory audit record, and a settlement call. It
// has no business knowing that announcements travel over a message
// bus, are logged in a per-workspace hot window, or are signed and
// POSTed to subscriber URLs. Naming any of those here would couple the
// control handlers to event infrastructure, and TestControlNeverTouchesEventLayer
// exists precisely to keep that coupling from creeping back in.
//
// So the Control Plane declares WHAT happened in its own vocabulary (one
// committed task status transition) and composition decides HOW it is
// delivered. The dependency arrow points inward: composition (the sole
// assembly root) knows both the port and the delivery machinery, and the
// handlers know only the port. That is also what makes the seam testable
// without a bus: a fake port records announcements, so "admin cancel
// announced task.cancelled exactly once" is assertable with a counter.
//
// The port is deliberately WIDE ENOUGH to carry what the delivery side
// genuinely consumes (the workspace public id, the operator reason, and
// the acting operator's public id) rather than a task-id/status pair that
// the adapter would then have to re-derive. Narrowing it past that would
// lose context the downstream envelope validation requires, not remove
// coupling.

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
//   - Reason is the operator-supplied cancel reason recorded by the
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
