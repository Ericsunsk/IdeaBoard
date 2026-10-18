package wiring

// End-to-end proof that the worker terminal-settlement bridge really puts
// task.status_changed on the wire.
//
// The hermetic bridge tests in task_wiring_events_test.go observe the
// events handed to the publisher. This one closes the loop: it wires the
// bridge to the REAL CompositeTaskStatusPublisher (local EventHub +
// NATS JetStream), drives a terminal settlement, and then reads the
// message back off the NATS subject AND off a second instance's local hub.
// A call-count assertion cannot pass this test, because nothing here
// inspects the publisher — only the bus and a second subscriber can.
//
// Skips without API_TEST_NATS_URL, like the existing NATS smoke test.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/testsupport/testenv"

	"github.com/nats-io/nats.go"
)

type wireTaskStatusEnvelope struct {
	OriginInstance string                 `json:"origin_instance"`
	Event          events.TaskStatusEvent `json:"event"`
}

// TestSettlerEventBridgePublishesOnNATS proves a worker terminal
// settlement lands on the task status subject exactly once, and that a
// second API instance's local SSE hub receives it.
func TestSettlerEventBridgePublishesOnNATS(t *testing.T) {
	natsURL := testenv.GetOrSkip(t, "API_TEST_NATS_URL", "set API_TEST_NATS_URL to run the NATS fanout test")

	suffix := time.Now().UnixNano()
	streamName := fmt.Sprintf("IDEABOARD_TASKFANOUT_TEST_%d", suffix)
	subjectPrefix := fmt.Sprintf("ibtest.fanout.%d", suffix)
	taskStatusSubject := subjectPrefix + ".task.status"

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Cleanup(func() { deleteNATSTestStream(t, natsURL, streamName) })

	// The publishing instance: the real composite publisher (local hub +
	// JetStream), exactly as task_wiring.go builds it.
	originPublisher, originHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("origin-%d", suffix))
	// A second instance proves the cross-process fanout, not a local loop.
	_, remoteHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("remote-%d", suffix))

	taskID := "task_fanout_completed"
	originEvents, unsubscribeOrigin, ok := originHub.Subscribe(taskID, 16)
	if !ok {
		t.Fatal("origin hub subscribe rejected unexpectedly")
	}
	defer unsubscribeOrigin()
	remoteEvents, unsubscribeRemote, ok := remoteHub.Subscribe(taskID, 16)
	if !ok {
		t.Fatal("remote hub subscribe rejected unexpectedly")
	}
	defer unsubscribeRemote()

	// Raw NATS subscription: the wire format itself, not a Go call.
	nc, err := nats.Connect(natsURL, nats.Name("ideaboard-test-observer"), nats.Timeout(3*time.Second))
	if err != nil {
		t.Fatalf("connect observer to nats: %v", err)
	}
	defer nc.Close()
	wire := make(chan []byte, 16)
	wireSub, err := nc.Subscribe(taskStatusSubject, func(msg *nats.Msg) {
		wire <- msg.Data
	})
	if err != nil {
		t.Fatalf("subscribe to %s: %v", taskStatusSubject, err)
	}
	defer wireSub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush observer subscription: %v", err)
	}

	stub := &bridgeStubSettler{}
	bridge := newSettlerEventBridge(stub, originPublisher, settlerEventDelivery{})
	if _, err := bridge.Complete(ctx, "ws_fanout", taskID, nil); err != nil {
		t.Fatalf("Complete through the bridge: %v", err)
	}

	// 1) The wire: one message on the task status subject.
	var envelope wireTaskStatusEnvelope
	select {
	case payload := <-wire:
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatalf("decode task status envelope %q: %v", string(payload), err)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for task.status_changed on the NATS subject")
	}
	if envelope.OriginInstance == "" {
		t.Fatal("envelope carried no origin_instance")
	}
	if envelope.Event.Type != events.EventTypeTaskStatusChanged {
		t.Fatalf("wire type = %q, want %q", envelope.Event.Type, events.EventTypeTaskStatusChanged)
	}
	if envelope.Event.TaskID != taskID || envelope.Event.Status != events.TaskStatusCompleted {
		t.Fatalf("wire event = %+v, want %s completed", envelope.Event, taskID)
	}
	if envelope.Event.OccurredAt.IsZero() {
		t.Fatal("wire event carried a zero occurred_at")
	}

	// 2) The local SSE hub of the publishing instance sees it once (the
	// Local leg of the composite publisher; the NATS echo of its own
	// message is filtered by origin_instance).
	assertTaskEvent(t, ctx, originEvents, taskID, tasks.StatusCompleted, "origin local hub")

	// 3) A second API instance's local SSE hub receives the same
	// transition, so the fanout is genuinely cross-instance.
	assertTaskEvent(t, ctx, remoteEvents, taskID, tasks.StatusCompleted, "remote instance")

	// 4) Exactly once on the wire and in both hubs.
	select {
	case payload := <-wire:
		t.Fatalf("duplicate task.status_changed on the wire: %s", string(payload))
	case <-time.After(500 * time.Millisecond):
	}
	assertNoTaskEvent(t, originEvents, "origin local hub")
	assertNoTaskEvent(t, remoteEvents, "remote instance")
}

// TestSettlerEventBridgeRollbackSendsNothingOnNATS proves the wire stays
// silent when the settlement rolled back: nothing is staged, so nothing is
// published, and a subscriber cannot observe a transition the database
// discarded.
func TestSettlerEventBridgeRollbackSendsNothingOnNATS(t *testing.T) {
	natsURL := testenv.GetOrSkip(t, "API_TEST_NATS_URL", "set API_TEST_NATS_URL to run the NATS fanout test")

	suffix := time.Now().UnixNano()
	streamName := fmt.Sprintf("IDEABOARD_TASKFANOUT_ROLLBACK_TEST_%d", suffix)
	subjectPrefix := fmt.Sprintf("ibtest.rollback.%d", suffix)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Cleanup(func() { deleteNATSTestStream(t, natsURL, streamName) })

	originPublisher, originHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("origin-%d", suffix))
	taskID := "task_fanout_rollback"
	originEvents, unsubscribe, ok := originHub.Subscribe(taskID, 8)
	if !ok {
		t.Fatal("origin hub subscribe rejected unexpectedly")
	}
	defer unsubscribe()

	nc, err := nats.Connect(natsURL, nats.Name("ideaboard-test-observer"), nats.Timeout(3*time.Second))
	if err != nil {
		t.Fatalf("connect observer to nats: %v", err)
	}
	defer nc.Close()
	wire := make(chan []byte, 8)
	wireSub, err := nc.Subscribe(subjectPrefix+".task.status", func(msg *nats.Msg) { wire <- msg.Data })
	if err != nil {
		t.Fatalf("subscribe to the task status subject: %v", err)
	}
	defer wireSub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush observer subscription: %v", err)
	}

	// A settlement that rolled back stages nothing, exactly like the real
	// Service does when its transaction is discarded.
	stub := &bridgeStubSettler{completeErr: fmt.Errorf("settlement rolled back")}
	bridge := newSettlerEventBridge(stub, originPublisher, settlerEventDelivery{})
	if _, err := bridge.Complete(ctx, "ws_fanout", taskID, nil); err == nil {
		t.Fatal("Complete must surface the settlement error")
	}

	select {
	case payload := <-wire:
		t.Fatalf("rolled-back settlement published %s", string(payload))
	case <-time.After(750 * time.Millisecond):
	}
	assertNoTaskEvent(t, originEvents, "origin after a rolled-back settlement")
	if len(stub.StagedEvents()) != 0 {
		t.Fatal("a failed settlement left staged events behind for the next drain")
	}
}

// TestSettlerEventBridgeRetryPublishesPendingOnNATS proves the requeue
// transition reaches the wire with the pending status.
func TestSettlerEventBridgeRetryPublishesPendingOnNATS(t *testing.T) {
	natsURL := testenv.GetOrSkip(t, "API_TEST_NATS_URL", "set API_TEST_NATS_URL to run the NATS fanout test")

	suffix := time.Now().UnixNano()
	streamName := fmt.Sprintf("IDEABOARD_TASKFANOUT_RETRY_TEST_%d", suffix)
	subjectPrefix := fmt.Sprintf("ibtest.retry.%d", suffix)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Cleanup(func() { deleteNATSTestStream(t, natsURL, streamName) })

	originPublisher, originHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("origin-%d", suffix))
	taskID := "task_fanout_retry"
	originEvents, unsubscribe, ok := originHub.Subscribe(taskID, 8)
	if !ok {
		t.Fatal("origin hub subscribe rejected unexpectedly")
	}
	defer unsubscribe()

	stub := &bridgeStubSettler{}
	bridge := newSettlerEventBridge(stub, originPublisher, settlerEventDelivery{})
	if _, err := bridge.Retry(ctx, "ws_fanout", taskID, true); err != nil {
		t.Fatalf("Retry through the bridge: %v", err)
	}

	assertTaskEvent(t, ctx, originEvents, taskID, tasks.StatusPending, "origin")
	assertNoTaskEvent(t, originEvents, "origin after the requeue")
}
