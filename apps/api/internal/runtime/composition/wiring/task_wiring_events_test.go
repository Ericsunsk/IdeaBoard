package wiring

// Terminal-settlement event fanout (#807 dead-channel fix).
//
// tasks.Service STAGES a status event inside its settlement call
// and hands it to whoever drains it. The v1 HTTP submit and cancel edges
// drain their own per-request Service; the River worker had no drain point
// at all, so task.completed / task.failed / the requeue's task.pending were
// staged and dropped: PublishTaskStatus had zero production callers and
// task.status_changed never appeared on NATS. settlerEventBridge is that
// missing drain point, and these tests pin its three load-bearing
// properties:
//
//   - exactly one publish per settlement, carrying the settled status;
//   - the publish happens AFTER the settlement call returned (a rollback
//     that never committed must not be observable on the event channel);
//   - a failed settlement publishes nothing and leaves nothing behind for
//     the next drain to misattribute.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	tasks "ideaboard/api/internal/tasking/domain"
)

// settlementPhase is what the stub settler reports at the moment the bridge
// publishes. A publish observed while still "settling" would mean the
// bridge emitted from inside the settlement call (i.e. inside the
// transaction), which is exactly the bug the ordering contract forbids.
type settlementPhase int

const (
	phaseSettling settlementPhase = iota
	phaseSettled
)

type bridgeStubSettler struct {
	mu sync.Mutex
	// phase is phaseSettling while a settlement call is in flight.
	phase settlementPhase
	// staged mirrors the Service's per-call staged-event buffer.
	staged []tasks.TaskEvent
	// completeErr / failErr / retryErr inject settlement failures.
	completeErr, failErr, retryErr error
	calls                          int
}

func TestSettlerEventBridgePublishesValidatedWorkspaceEnvelopes(t *testing.T) {
	const workspaceID = "ws_abc123"
	cases := []struct {
		name      string
		wantType  string
		settle    func(*settlerEventBridge, context.Context, string, string) error
		checkData func(t *testing.T, envelope events.V1Envelope)
	}{
		{
			name:     "completed",
			wantType: events.EventV1TaskCompleted,
			settle: func(b *settlerEventBridge, ctx context.Context, ws, id string) error {
				_, err := b.Complete(ctx, ws, id, nil)
				return err
			},
			checkData: func(t *testing.T, envelope events.V1Envelope) {
				var data events.V1TaskCompletedData
				if err := json.Unmarshal(envelope.Data, &data); err != nil {
					t.Fatal(err)
				}
				want := "/api/v1/workspaces/ws_abc123/tasks/task_abc123/outputs"
				if data.OutputsURI != want {
					t.Fatalf("outputs_uri = %q, want %q", data.OutputsURI, want)
				}
			},
		},
		{
			name:     "failed",
			wantType: events.EventV1TaskFailed,
			settle: func(b *settlerEventBridge, ctx context.Context, ws, id string) error {
				_, err := b.Fail(ctx, ws, id, "provider exploded")
				return err
			},
			checkData: func(t *testing.T, envelope events.V1Envelope) {
				var data events.V1TaskFailedData
				if err := json.Unmarshal(envelope.Data, &data); err != nil {
					t.Fatal(err)
				}
				if data.ErrorCode != "task_failed" {
					t.Fatalf("error_code = %q, want task_failed", data.ErrorCode)
				}
			},
		},
		{
			name:     "retry does not reemit task.created",
			wantType: events.EventV1TaskCreated,
			settle: func(b *settlerEventBridge, ctx context.Context, ws, id string) error {
				_, err := b.Retry(ctx, ws, id, true)
				return err
			},
			checkData: func(t *testing.T, envelope events.V1Envelope) {},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &bridgeStubSettler{}
			log := events.NewWorkspaceEventLog()
			bridge := newSettlerEventBridge(stub, nil, settlerEventDelivery{log: log})
			t.Logf("delivery log nil=%v", bridge.delivery.log == nil)
			if err := tc.settle(bridge, t.Context(), workspaceID, "task_abc123"); err != nil {
				t.Fatalf("settlement: %v", err)
			}
			// An unknown cursor requests the retained replay window; an empty
			// cursor is the live-only SSE mode and intentionally returns no
			// historical envelopes.
			got, gap, found := log.ReplayAfter(workspaceID, "evt_missing", events.WorkspaceEventFilter{})
			if tc.name == "retry does not reemit task.created" {
				if len(got) != 0 {
					t.Fatalf("workspace events = %d, want none for requeue", len(got))
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("workspace events = %d, gap=%v found=%v, want one", len(got), gap, found)
			}
			envelope := got[0]
			if envelope.Type != tc.wantType {
				t.Fatalf("type = %q, want %q", envelope.Type, tc.wantType)
			}
			if err := events.ValidateV1Envelope(envelope); err != nil {
				t.Fatalf("worker envelope invalid: %v", err)
			}
			tc.checkData(t, envelope)
		})
	}
}

func (s *bridgeStubSettler) Complete(_ context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	return s.settle(workspaceID, taskID, tasks.StatusCompleted, s.completeErr, outputs)
}

func (s *bridgeStubSettler) Fail(_ context.Context, workspaceID, taskID, _ string) (*tasks.WorkspaceTask, error) {
	return s.settle(workspaceID, taskID, tasks.StatusFailed, s.failErr, nil)
}

func (s *bridgeStubSettler) Retry(_ context.Context, workspaceID, taskID string, _ bool) (*tasks.WorkspaceTask, error) {
	return s.settle(workspaceID, taskID, tasks.StatusPending, s.retryErr, nil)
}

// settle models the production Service: it stages its event only on the
// post-commit line, so a failing settlement stages nothing.
func (s *bridgeStubSettler) settle(workspaceID, taskID string, status tasks.Status, err error, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	s.phase = phaseSettling
	s.calls++
	s.mu.Unlock()

	if err == nil {
		s.staged = append(s.staged, tasks.TaskEvent{
			TaskID: taskID, WorkspaceID: workspaceID, Status: status, OccurredAt: time.Now().UTC(),
			Outputs: tasks.BuildTaskCompletedOutputs(workspaceID, outputs),
		})
	}

	s.mu.Lock()
	s.phase = phaseSettled
	s.mu.Unlock()
	return &tasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, Status: status}, err
}

func (s *bridgeStubSettler) StagedEvents() []tasks.TaskEvent {
	staged := append([]tasks.TaskEvent(nil), s.staged...)
	s.staged = nil
	return staged
}

func (s *bridgeStubSettler) currentPhase() settlementPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

type bridgeRecordingPublisher struct {
	mu        sync.Mutex
	published []events.TaskStatusEvent
	// phases records the settlement phase observed at each publish, so a
	// test can prove the bridge never emits from inside the settlement.
	phases []settlementPhase
	// phaseFn reports the stub's current phase.
	phaseFn func() settlementPhase
	err     error
}

func (p *bridgeRecordingPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	phase := phaseSettled
	if p.phaseFn != nil {
		phase = p.phaseFn()
	}
	p.published = append(p.published, event)
	p.phases = append(p.phases, phase)
	return p.err
}

func (p *bridgeRecordingPublisher) snapshot() ([]events.TaskStatusEvent, []settlementPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]events.TaskStatusEvent(nil), p.published...), append([]settlementPhase(nil), p.phases...)
}

func newBridgeFixture() (*settlerEventBridge, *bridgeStubSettler, *bridgeRecordingPublisher) {
	stub := &bridgeStubSettler{}
	publisher := &bridgeRecordingPublisher{phaseFn: stub.currentPhase}
	return newSettlerEventBridge(stub, publisher, settlerEventDelivery{}), stub, publisher
}

func assertPublishes(t *testing.T, publisher *bridgeRecordingPublisher, wantStatus events.TaskStatus, wantTaskID string) {
	t.Helper()
	published, phases := publisher.snapshot()
	if len(published) != 1 {
		t.Fatalf("published %d events, want exactly 1", len(published))
	}
	if published[0].Type != events.EventTypeTaskStatusChanged {
		t.Fatalf("event type = %q, want %q", published[0].Type, events.EventTypeTaskStatusChanged)
	}
	if published[0].Status != wantStatus {
		t.Fatalf("event status = %q, want %q", published[0].Status, wantStatus)
	}
	if published[0].TaskID != wantTaskID {
		t.Fatalf("event task_id = %q, want %q", published[0].TaskID, wantTaskID)
	}
	if published[0].OccurredAt.IsZero() {
		t.Fatal("event carried a zero occurred_at")
	}
	if phases[0] != phaseSettled {
		t.Fatal("event was published while the settlement call was still in flight; the flush must run after the commit")
	}
}

// TestSettlerEventBridgePublishesEachTerminalTransitionOnce proves the
// worker path now emits exactly one task.status_changed per settlement:
// completed on Complete, failed on Fail, pending on the requeue.
func TestSettlerEventBridgePublishesEachTerminalTransitionOnce(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus events.TaskStatus
		settle     func(*settlerEventBridge, context.Context, string, string) error
	}{
		{
			name:       "complete publishes completed",
			wantStatus: events.TaskStatusCompleted,
			settle: func(b *settlerEventBridge, ctx context.Context, ws, id string) error {
				_, err := b.Complete(ctx, ws, id, nil)
				return err
			},
		},
		{
			name:       "fail publishes failed",
			wantStatus: events.TaskStatusFailed,
			settle: func(b *settlerEventBridge, ctx context.Context, ws, id string) error {
				_, err := b.Fail(ctx, ws, id, "provider exploded")
				return err
			},
		},
		{
			name:       "retry publishes pending requeue",
			wantStatus: events.TaskStatusPending,
			settle: func(b *settlerEventBridge, ctx context.Context, ws, id string) error {
				_, err := b.Retry(ctx, ws, id, true)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bridge, _, publisher := newBridgeFixture()
			taskID := "task_bridge_" + tc.name[:3]
			if err := tc.settle(bridge, t.Context(), "ws_bridge", taskID); err != nil {
				t.Fatalf("settlement: %v", err)
			}
			assertPublishes(t, publisher, tc.wantStatus, taskID)
		})
	}
}

// TestSettlerEventBridgeStaysSilentOnRolledBackSettlement proves the
// rollback leg: a settlement that failed publishes nothing at all, so no
// subscriber can observe a transition the database rolled back. It also
// proves the failed call leaves no residue: the next successful settlement
// publishes exactly its own event, not the rolled-back one.
func TestSettlerEventBridgeStaysSilentOnRolledBackSettlement(t *testing.T) {
	rollback := errors.New("settlement rolled back")
	stub := &bridgeStubSettler{}
	publisher := &bridgeRecordingPublisher{phaseFn: stub.currentPhase}
	log := events.NewWorkspaceEventLog()
	bridge := newSettlerEventBridge(stub, publisher, settlerEventDelivery{log: log})
	stub.completeErr = rollback
	stub.failErr = rollback
	stub.retryErr = rollback

	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_a", nil); !errors.Is(err, rollback) {
		t.Fatalf("Complete err = %v, want %v", err, rollback)
	}
	if _, err := bridge.Fail(t.Context(), "ws_bridge", "task_bridge_a", "boom"); !errors.Is(err, rollback) {
		t.Fatalf("Fail err = %v, want %v", err, rollback)
	}
	if _, err := bridge.Retry(t.Context(), "ws_bridge", "task_bridge_a", true); !errors.Is(err, rollback) {
		t.Fatalf("Retry err = %v, want %v", err, rollback)
	}
	if published, _ := publisher.snapshot(); len(published) != 0 {
		t.Fatalf("published %d events for rolled-back settlements, want 0: %+v", len(published), published)
	}
	if got, _, _ := log.ReplayAfter("ws_bridge", "evt_missing", events.WorkspaceEventFilter{}); len(got) != 0 {
		t.Fatalf("workspace events = %d, want none after rollback", len(got))
	}

	// The next committed settlement must publish only its own transition.
	stub.completeErr, stub.failErr, stub.retryErr = nil, nil, nil
	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_a", nil); err != nil {
		t.Fatalf("Complete after rollback: %v", err)
	}
	assertPublishes(t, publisher, events.TaskStatusCompleted, "task_bridge_a")
	if got, _, _ := log.ReplayAfter("ws_bridge", "evt_missing", events.WorkspaceEventFilter{}); len(got) != 1 {
		t.Fatalf("workspace events after commit = %d, want 1", len(got))
	}
}

// TestSettlerEventBridgeNeverReEmitsCreateFromTerminalPaths proves the
// bridge cannot double-send the submit event: it exposes no Create seam, so
// the only pending it can ever publish is the retry requeue. The v1 submit
// edge owns task.created and drains its own per-request Service.
func TestSettlerEventBridgeNeverReEmitsCreateFromTerminalPaths(t *testing.T) {
	bridge, _, publisher := newBridgeFixture()
	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_b", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := bridge.Fail(t.Context(), "ws_bridge", "task_bridge_c", "boom"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	published, _ := publisher.snapshot()
	if len(published) != 2 {
		t.Fatalf("published %d events, want 2", len(published))
	}
	for i, event := range published {
		if event.Status == events.TaskStatusPending {
			t.Fatalf("event %d = %q; only the retry requeue may publish pending from the worker path", i, event.Status)
		}
	}

	// Retry is the single worker source of a pending event, and it emits it
	// once.
	if _, err := bridge.Retry(t.Context(), "ws_bridge", "task_bridge_c", true); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	published, _ = publisher.snapshot()
	if len(published) != 3 {
		t.Fatalf("published %d events, want 3", len(published))
	}
	if published[2].Status != events.TaskStatusPending || published[2].TaskID != "task_bridge_c" {
		t.Fatalf("requeue event = %+v, want one pending for task_bridge_c", published[2])
	}
}

// TestSettlerEventBridgeKeepsSettlementResultWhenPublishFails proves the
// fanout is strictly best-effort downstream of a committed fact: a dead
// NATS must not fail a completed job (which would make River retry an
// already-terminal task).
func TestSettlerEventBridgeKeepsSettlementResultWhenPublishFails(t *testing.T) {
	bridge, _, publisher := newBridgeFixture()
	publisher.err = errors.New("nats unavailable")

	completed, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_d", nil)
	if err != nil {
		t.Fatalf("Complete must not fail when the event channel is down: %v", err)
	}
	if completed == nil || completed.Status != tasks.StatusCompleted {
		t.Fatalf("completed = %+v, want a completed task", completed)
	}
	if published, _ := publisher.snapshot(); len(published) != 1 {
		t.Fatalf("published %d events, want the one attempt", len(published))
	}
}

// TestSettlerEventBridgeSerializesConcurrentSettlements proves the drain
// stays attributable when River runs several jobs against ONE Service: each
// task's event reaches the channel paired with its own task id, exactly
// once, and never as a duplicate.
func TestSettlerEventBridgeSerializesConcurrentSettlements(t *testing.T) {
	bridge, _, publisher := newBridgeFixture()
	const jobs = 8

	var wg sync.WaitGroup
	for i := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			taskID := fmt.Sprintf("task_bridge_race_%02d", i)
			if _, err := bridge.Complete(t.Context(), "ws_bridge", taskID, nil); err != nil {
				t.Errorf("Complete %s: %v", taskID, err)
			}
		}()
	}
	wg.Wait()

	published, phases := publisher.snapshot()
	if len(published) != jobs {
		t.Fatalf("published %d events for %d settlements, want %d", len(published), jobs, jobs)
	}
	seen := make(map[string]int, jobs)
	for i, event := range published {
		seen[event.TaskID]++
		if phases[i] != phaseSettled {
			t.Fatalf("event %d published inside the settlement call", i)
		}
	}
	for taskID, count := range seen {
		if count != 1 {
			t.Fatalf("task %s published %d times, want exactly 1", taskID, count)
		}
	}
}

// TestSettlerEventBridgeToleratesNilPublisher proves the read-only/unit
// runtimes (events disabled) settle without a nil-pointer on the fanout.
func TestSettlerEventBridgeToleratesNilPublisher(t *testing.T) {
	stub := &bridgeStubSettler{}
	bridge := newSettlerEventBridge(stub, nil, settlerEventDelivery{})
	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_nil", nil); err != nil {
		t.Fatalf("Complete with a nil publisher: %v", err)
	}
	if len(stub.StagedEvents()) != 0 {
		t.Fatal("staged events must be drained even with a nil publisher")
	}
}

func TestSettlerEventBridgeEmitsMediaAssetReadyPerAssetOutput(t *testing.T) {
	stub := &bridgeStubSettler{}
	log := events.NewWorkspaceEventLog()
	bridge := newSettlerEventBridge(stub, nil, settlerEventDelivery{log: log})
	outputs := []tasks.OutputRef{
		{Kind: tasks.OutputKindAsset, AssetID: "asset_readytest01", Role: "result", ResultIndex: 0},
		{Kind: tasks.OutputKindAsset, AssetID: "asset_readytest02", Role: "result", ResultIndex: 1},
		{Kind: tasks.OutputKindRemote, URL: "https://cdn.example.com/o.png", ChecksumSHA256: "ab", ResultIndex: 2},
		{Kind: tasks.OutputKindAsset, AssetID: "asset_readytest01", Role: "result", ResultIndex: 3},
		{Kind: tasks.OutputKindAsset, AssetID: "  ", Role: "result", ResultIndex: 4},
	}
	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_media", outputs); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, _, _ := log.ReplayAfter("ws_bridge", "evt_missing", events.WorkspaceEventFilter{})
	if len(got) != 3 {
		t.Fatalf("workspace events = %d, want task.completed + 2 media.asset_ready", len(got))
	}
	if got[0].Type != events.EventV1TaskCompleted {
		t.Fatalf("first envelope type = %q, want task.completed before the asset hints", got[0].Type)
	}
	var completedData events.V1TaskCompletedData
	if err := json.Unmarshal(got[0].Data, &completedData); err != nil {
		t.Fatalf("decode completed data: %v", err)
	}
	if len(completedData.Outputs) != len(outputs) {
		t.Fatalf("completed outputs = %d, want %d", len(completedData.Outputs), len(outputs))
	}
	if completedData.Outputs[0].AssetID != "asset_readytest01" || !completedData.Outputs[0].PreviewReady {
		t.Fatalf("completed output 0 = %+v", completedData.Outputs[0])
	}
	seen := make(map[string]bool, 2)
	for _, envelope := range got[1:] {
		if envelope.Type != events.EventV1MediaAssetReady {
			t.Fatalf("envelope type = %q, want media.asset_ready", envelope.Type)
		}
		if err := events.ValidateV1Envelope(envelope); err != nil {
			t.Fatalf("media envelope invalid: %v", err)
		}
		if envelope.Resource.Type != events.V1ResourceMediaAsset {
			t.Fatalf("resource type = %q, want media_asset", envelope.Resource.Type)
		}
		var data events.V1MediaAssetReadyData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data.AssetID == "" || data.AssetID != envelope.Resource.ID {
			t.Fatalf("asset_id = %q, resource id = %q, want agreement", data.AssetID, envelope.Resource.ID)
		}
		if envelope.Actor.Type != events.V1ActorService {
			t.Fatalf("actor type = %q, want the worker service actor", envelope.Actor.Type)
		}
		if envelope.WorkspaceID != "ws_bridge" {
			t.Fatalf("workspace_id = %q, want the settled workspace", envelope.WorkspaceID)
		}
		wantID := events.StableEventID(data.AssetID, events.EventV1MediaAssetReady)
		if envelope.ID != wantID {
			t.Fatalf("envelope id = %q, want stable %q", envelope.ID, wantID)
		}
		if seen[data.AssetID] {
			t.Fatalf("duplicate media.asset_ready for %q", data.AssetID)
		}
		seen[data.AssetID] = true
	}
	if !seen["asset_readytest01"] || !seen["asset_readytest02"] {
		t.Fatalf("missing asset hints: %v", seen)
	}
}

func TestSettlerEventBridgeEmitsNoMediaAssetReadyOnRollback(t *testing.T) {
	rollback := errors.New("settlement rolled back")
	stub := &bridgeStubSettler{completeErr: rollback}
	log := events.NewWorkspaceEventLog()
	bridge := newSettlerEventBridge(stub, nil, settlerEventDelivery{log: log})
	outputs := []tasks.OutputRef{
		{Kind: tasks.OutputKindAsset, AssetID: "asset_readytest01", Role: "result", ResultIndex: 0},
	}
	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_rb", outputs); !errors.Is(err, rollback) {
		t.Fatalf("Complete err = %v, want %v", err, rollback)
	}
	if got, _, _ := log.ReplayAfter("ws_bridge", "evt_missing", events.WorkspaceEventFilter{}); len(got) != 0 {
		t.Fatalf("workspace events = %d, want none after rollback", len(got))
	}
	stub.completeErr = nil
	if _, err := bridge.Complete(t.Context(), "ws_bridge", "task_bridge_rb", outputs); err != nil {
		t.Fatalf("Complete after rollback: %v", err)
	}
	got, _, _ := log.ReplayAfter("ws_bridge", "evt_missing", events.WorkspaceEventFilter{})
	if len(got) != 2 {
		t.Fatalf("workspace events = %d, want task.completed + 1 media.asset_ready", len(got))
	}
}
