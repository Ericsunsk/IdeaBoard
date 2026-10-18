package nats

// Board access-signal bridge fanout (issue #940). Covers the
// cross-instance path: remote-origin envelopes decode and append into the
// local hot log without loops, and invalid envelopes are dropped.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"ideaboard/api/internal/events"
)

func mustMintBoardAccessChanged(t *testing.T, boardID, memberID string, version int64) events.V1Envelope {
	t.Helper()
	envelope, err := events.MintV1Envelope(events.EventV1BoardAccessChanged, "ws_abc123",
		events.V1Actor{Type: events.V1ActorService, ID: "board-access"},
		events.V1Resource{Type: events.V1ResourceBoard, ID: boardID},
		events.V1BoardAccessChangedData{BoardID: boardID, MemberID: memberID, AccessVersion: version})
	if err != nil {
		t.Fatalf("mint access_changed: %v", err)
	}
	return envelope
}

func TestWorkspaceEventBridgeDecodesRemoteOnly(t *testing.T) {
	signal := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	payload, err := json.Marshal(workspaceV1Envelope{OriginInstance: "api-a", Envelope: signal})
	if err != nil {
		t.Fatal(err)
	}
	if _, deliver, err := decodeWorkspaceEventForInstance(payload, "api-a"); err != nil || deliver {
		t.Fatalf("local-origin must ack without delivery (deliver=%v err=%v)", deliver, err)
	}
	remote, deliver, err := decodeWorkspaceEventForInstance(payload, "api-b")
	if err != nil || !deliver {
		t.Fatalf("remote-origin must deliver (deliver=%v err=%v)", deliver, err)
	}
	if remote.ID != signal.ID || remote.Type != events.EventV1BoardAccessChanged {
		t.Fatalf("bridged envelope=%+v, want the access_changed signal", remote)
	}
	if _, _, err := decodeWorkspaceEventForInstance([]byte(`{"envelope":{}}`), "api-b"); err == nil {
		t.Fatal("payload without origin must be rejected")
	}
}

func TestWorkspaceEventBridgeHandlerAppendsWithoutLoops(t *testing.T) {
	logA := events.NewWorkspaceEventLog()
	logB := events.NewWorkspaceEventLog()
	signal := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	logA.Append(signal)
	handler := events.NewWorkspaceEventBridgeHandler(logB)
	if err := handler(context.Background(), signal); err != nil {
		t.Fatalf("bridge handler: %v", err)
	}
	if logB.Len("ws_abc123") != 1 {
		t.Fatalf("instance B log=%d, want 1 bridged envelope", logB.Len("ws_abc123"))
	}
	// Redelivery converges: the duplicate append dedups without refanout.
	live, unsubscribe, ok := logB.Subscribe("ws_abc123", events.WorkspaceEventFilter{})
	if !ok {
		t.Fatal("subscribe instance B log")
	}
	defer unsubscribe()
	if err := handler(context.Background(), signal); err != nil {
		t.Fatalf("bridge redelivery: %v", err)
	}
	select {
	case envelope := <-live:
		t.Fatalf("duplicate bridge append must not refanout (got %s)", envelope.ID)
	case <-time.After(50 * time.Millisecond):
	}
	// Invalid envelopes never enter the hot log.
	if err := handler(context.Background(), events.V1Envelope{}); err != nil {
		t.Fatalf("bridge handler must drop invalid envelopes without error: %v", err)
	}
	if logB.Len("ws_abc123") != 1 {
		t.Fatalf("instance B log=%d, want still 1", logB.Len("ws_abc123"))
	}
	if err := events.NewWorkspaceEventBridgeHandler(nil)(context.Background(), signal); err != nil {
		t.Fatalf("nil-log bridge must no-op: %v", err)
	}
}
