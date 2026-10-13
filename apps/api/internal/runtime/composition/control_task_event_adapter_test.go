package composition

// Control Plane task event adapter (admin cancel dead-channel fix).
//
// The control-plane tests prove the Control Plane ANNOUNCES a committed
// transition exactly once. These prove the other half: that the adapter
// actually delivers that announcement to the real event layer, so the fix
// is not a silent no-op behind a counting fake.
//
// Both delivery channels are asserted with COUNTS against REAL event
// components (the workspace hot log and the webhook store), because the
// failure this guards is "the Control Plane called the port and nothing
// arrived". The NATS task status channel is covered by a fake publisher,
// which is the right tool there: it needs no broker, and the envelope
// shape it must carry is what matters.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/events"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	rivertype "github.com/riverqueue/river/rivertype"
)

// countingTaskStatusPublisher records what reached the task status channel.
type countingTaskStatusPublisher struct {
	events []events.TaskStatusEvent
}

func (p *countingTaskStatusPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.events = append(p.events, event)
	return nil
}

// countingWebhookEnqueuer records the webhook deliveries the adapter
// caused, i.e. the signed POSTs a subscriber would receive.
type countingWebhookEnqueuer struct {
	inserts []events.WebhookDeliveryArgs
}

func (e *countingWebhookEnqueuer) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	delivery, ok := args.(events.WebhookDeliveryArgs)
	if !ok {
		return nil, fmt.Errorf("unexpected webhook args %T", args)
	}
	e.inserts = append(e.inserts, delivery)
	return &rivertype.JobInsertResult{}, nil
}

func controlAdapterFixture(t *testing.T) (*countingTaskStatusPublisher, *events.WebhookStore, *countingWebhookEnqueuer, string) {
	t.Helper()
	status := &countingTaskStatusPublisher{}
	webhooks := events.NewWebhookStore(nil, nil)
	enqueuer := &countingWebhookEnqueuer{}
	webhooks.SetEnqueuer(enqueuer)

	// ws_ / task_ opaque public ids, built the way the platform mints them
	// so envelope validation accepts them.
	workspace := "ws_ctl_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	if _, _, _, err := webhooks.Create(
		context.Background(), workspace,
		"https://hooks.example.test/task-events",
		[]string{events.EventV1TaskCancelled},
		nil, // let the store mint the whsec_ secret
	); err != nil {
		t.Fatalf("create webhook subscription: %v", err)
	}
	return status, webhooks, enqueuer, workspace
}

func controlTaskPublicID() string {
	return "task_ctl_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
}

func controlTransition(workspace, taskID string, actorPublicID uuid.UUID) httpapi.ControlTaskStatusEvent {
	return httpapi.ControlTaskStatusEvent{
		WorkspaceID: workspace,
		TaskID:      taskID,
		Status:      "cancelled",
		Reason:      "cancelled",
		ActorUserID: tenancy.FormatUserPublicID(actorPublicID),
	}
}

// TestControlTaskEventAdapterDeliversBothChannelsOnce proves one
// announcement reaches the task status channel exactly once AND produces
// exactly one signed webhook delivery carrying a valid task.cancelled
// envelope scoped to the right workspace.
func TestControlTaskEventAdapterDeliversBothChannelsOnce(t *testing.T) {
	status, webhooks, enqueuer, workspace := controlAdapterFixture(t)
	log := events.NewWorkspaceEventLog()
	adapter := newControlTaskEventAdapter(status, log, nil, webhooks)

	taskID := controlTaskPublicID()
	actorPublicID := uuid.New()
	event := controlTransition(workspace, taskID, actorPublicID)

	ctx := context.Background()
	if err := adapter.PublishTaskStatus(ctx, event); err != nil {
		t.Fatalf("PublishTaskStatus: %v", err)
	}
	if err := adapter.EmitWorkspaceTaskEvent(ctx, event); err != nil {
		t.Fatalf("EmitWorkspaceTaskEvent: %v", err)
	}

	if len(status.events) != 1 {
		t.Fatalf("task status publishes = %d, want exactly 1: %+v", len(status.events), status.events)
	}
	published := status.events[0]
	if published.Type != events.EventTypeTaskStatusChanged {
		t.Fatalf("published type = %q, want %s", published.Type, events.EventTypeTaskStatusChanged)
	}
	if published.TaskID != taskID || published.Status != "cancelled" || published.OccurredAt.IsZero() {
		t.Fatalf("published event = %+v, want cancelled for %s", published, taskID)
	}

	if len(enqueuer.inserts) != 1 {
		t.Fatalf("webhook deliveries = %d, want exactly 1: %+v", len(enqueuer.inserts), enqueuer.inserts)
	}
	envelope := enqueuer.inserts[0].Envelope
	if envelope.Type != events.EventV1TaskCancelled {
		t.Fatalf("envelope type = %q, want %s", envelope.Type, events.EventV1TaskCancelled)
	}
	if envelope.WorkspaceID != workspace || envelope.Resource.ID != taskID {
		t.Fatalf("envelope scope = %s/%s, want %s/%s", envelope.WorkspaceID, envelope.Resource.ID, workspace, taskID)
	}
	if envelope.Actor.Type != events.V1ActorUser || envelope.Actor.ID != tenancy.FormatUserPublicID(actorPublicID) {
		t.Fatalf("envelope actor = %+v, want the operator usr_ id", envelope.Actor)
	}
	// The envelope must also land in the hot log SSE subscribers read, and
	// it must be the SAME envelope the webhook carries (one mint, one id).
	if got := log.Len(workspace); got != 1 {
		t.Fatalf("workspace event log length = %d, want exactly 1", got)
	}
}

// TestControlTaskEventAdapterEnvelopeCarriesCancelReason preserves the task
// cancellation cause in the workspace payload.
func TestControlTaskEventAdapterEnvelopeCarriesCancelReason(t *testing.T) {
	_, webhooks, enqueuer, workspace := controlAdapterFixture(t)
	adapter := newControlTaskEventAdapter(nil, events.NewWorkspaceEventLog(), nil, webhooks)

	taskID := controlTaskPublicID()
	if err := adapter.EmitWorkspaceTaskEvent(context.Background(),
		controlTransition(workspace, taskID, uuid.New())); err != nil {
		t.Fatalf("EmitWorkspaceTaskEvent: %v", err)
	}
	if len(enqueuer.inserts) != 1 {
		t.Fatalf("webhook deliveries = %d, want exactly 1", len(enqueuer.inserts))
	}
	envelope := enqueuer.inserts[0].Envelope
	if err := events.ValidateV1Envelope(envelope); err != nil {
		t.Fatalf("emitted envelope must validate: %v", err)
	}
	if got := string(envelope.Data); !containsAll(got, taskID, "cancelled") {
		t.Fatalf("envelope data = %s, want the task id and the cancellation cause", got)
	}
}

func TestControlTaskEventAdapterEmitsBillingSettledForCancel(t *testing.T) {
	_, webhooks, _, workspace := controlAdapterFixture(t)
	log := events.NewWorkspaceEventLog()
	adapter := newControlTaskEventAdapter(nil, log, nil, webhooks)
	event := controlTransition(workspace, controlTaskPublicID(), uuid.New())
	event.SettlementState = "released"
	event.HoldTransactionID = "ctx_hold_123"
	if err := adapter.EmitWorkspaceTaskEvent(context.Background(), event); err != nil {
		t.Fatalf("EmitWorkspaceTaskEvent: %v", err)
	}
	if got := log.Len(workspace); got != 2 {
		t.Fatalf("workspace event log length = %d, want task.cancelled + billing.settled", got)
	}
	missed, _, _ := log.ReplayAfter(workspace, "evt_missing", events.WorkspaceEventFilter{})
	if len(missed) != 2 || missed[1].Type != events.EventV1BillingSettled {
		t.Fatalf("workspace events = %+v, want billing.settled second", missed)
	}
	var data events.V1BillingSettledData
	if err := json.Unmarshal(missed[1].Data, &data); err != nil {
		t.Fatalf("decode billing event: %v", err)
	}
	if data.SettlementState != "released" || data.HoldTransactionID != "ctx_hold_123" {
		t.Fatalf("billing data = %+v", data)
	}
}

// TestControlTaskEventAdapterFallsBackToServiceActor proves an
// unresolvable operator degrades to the Control Plane service actor
// instead of emitting an invalid (empty actor) envelope or failing.
func TestControlTaskEventAdapterFallsBackToServiceActor(t *testing.T) {
	_, webhooks, enqueuer, workspace := controlAdapterFixture(t)
	adapter := newControlTaskEventAdapter(nil, events.NewWorkspaceEventLog(), nil, webhooks)

	event := controlTransition(workspace, controlTaskPublicID(), uuid.New())
	event.ActorUserID = ""
	if err := adapter.EmitWorkspaceTaskEvent(context.Background(), event); err != nil {
		t.Fatalf("EmitWorkspaceTaskEvent: %v", err)
	}
	if len(enqueuer.inserts) != 1 {
		t.Fatalf("webhook deliveries = %d, want exactly 1", len(enqueuer.inserts))
	}
	envelope := enqueuer.inserts[0].Envelope
	if envelope.Actor.Type != events.V1ActorService || envelope.Actor.ID != controlPlaneActorID {
		t.Fatalf("envelope actor = %+v, want service/%s", envelope.Actor, controlPlaneActorID)
	}
	if err := events.ValidateV1Envelope(envelope); err != nil {
		t.Fatalf("fallback envelope must still validate: %v", err)
	}
}

// TestControlTaskEventAdapterFailsClosedOnUnrepresentableStatus proves the
// adapter never mints an envelope it cannot populate: task.created needs a
// service and task.completed an outputs URI, neither of which the control
// settlement port carries, so those statuses are dropped rather than
// emitted as an invalid envelope.
func TestControlTaskEventAdapterFailsClosedOnUnrepresentableStatus(t *testing.T) {
	_, webhooks, enqueuer, workspace := controlAdapterFixture(t)
	adapter := newControlTaskEventAdapter(nil, events.NewWorkspaceEventLog(), nil, webhooks)

	for _, status := range []string{"pending", "completed", "failed"} {
		event := controlTransition(workspace, controlTaskPublicID(), uuid.New())
		event.Status = status
		if err := adapter.EmitWorkspaceTaskEvent(context.Background(), event); err != nil {
			t.Fatalf("EmitWorkspaceTaskEvent(%s): %v", status, err)
		}
	}
	if len(enqueuer.inserts) != 0 {
		t.Fatalf("unrepresentable statuses produced %d webhook deliveries, want 0: %+v", len(enqueuer.inserts), enqueuer.inserts)
	}
}

// TestControlTaskEventAdapterToleratesNilSinks proves every dependency is
// optional in the safe direction: with no publisher, no log, no bus and no
// webhook store the adapter is a silent no-op rather than a panic. This is
// what lets the control seam stay fail-safe when the event layer is not
// configured.
func TestControlTaskEventAdapterToleratesNilSinks(t *testing.T) {
	adapter := newControlTaskEventAdapter(nil, nil, nil, nil)
	event := controlTransition("ws_ctl_nilsink0", controlTaskPublicID(), uuid.New())
	ctx := context.Background()
	if err := adapter.PublishTaskStatus(ctx, event); err != nil {
		t.Fatalf("PublishTaskStatus with nil sinks: %v", err)
	}
	if err := adapter.EmitWorkspaceTaskEvent(ctx, event); err != nil {
		t.Fatalf("EmitWorkspaceTaskEvent with nil sinks: %v", err)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
