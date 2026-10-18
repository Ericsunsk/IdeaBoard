package events

// Board access-signal visibility, bridge fanout, and durable replay
// (issue #940). Covers the delivery-time rules: board content reaches
// current readers only, directed signals reach the named member only,
// cross-instance appends converge without loops, and the outbox replayer
// retries pending rows on the stable evt_ id without ever touching
// webhooks.

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/contract"
)

func mustMintBoardAccessChanged(t *testing.T, boardID, memberID string, version int64) V1Envelope {
	t.Helper()
	envelope, err := MintV1Envelope(EventV1BoardAccessChanged, "ws_abc123",
		V1Actor{Type: V1ActorService, ID: "board-access"},
		V1Resource{Type: V1ResourceBoard, ID: boardID},
		V1BoardAccessChangedData{BoardID: boardID, MemberID: memberID, AccessVersion: version})
	if err != nil {
		t.Fatalf("mint access_changed: %v", err)
	}
	return envelope
}

func TestAccessChangedPayloadCarriesIdentityAndVersionOnly(t *testing.T) {
	envelope := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 3 {
		t.Fatalf("access_changed data keys=%v, want exactly board_id+member_id+access_version", data)
	}
	if data["board_id"] != "board_abc123" || data["member_id"] != "usr_member1" || data["access_version"].(float64) != 3 {
		t.Fatalf("access_changed data=%v", data)
	}
}

func TestAccessChangedValidationRejectsSmuggledContent(t *testing.T) {
	base := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	for name, raw := range map[string]string{
		"role":      `{"board_id":"board_abc123","member_id":"usr_member1","access_version":3,"role":"viewer"}`,
		"prompt":    `{"board_id":"board_abc123","member_id":"usr_member1","access_version":3,"prompt":"secret"}`,
		"asset":     `{"board_id":"board_abc123","member_id":"usr_member1","access_version":3,"asset_id":"asset_x"}`,
		"zero":      `{"board_id":"board_abc123","member_id":"usr_member1","access_version":0}`,
		"badmember": `{"board_id":"board_abc123","member_id":"alice","access_version":3}`,
	} {
		candidate := base
		candidate.Data = json.RawMessage(raw)
		if err := ValidateV1Envelope(candidate); err == nil {
			t.Fatalf("access_changed with %s must fail closed", name)
		}
	}
}

func TestBoardScopeHelpers(t *testing.T) {
	doc, err := MintV1Envelope(EventV1BoardDocumentReplaced, "ws_abc123", testActor(),
		V1Resource{Type: V1ResourceBoard, ID: "board_abc123"},
		V1BoardDocumentReplacedData{BoardID: "board_abc123", Revision: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !IsBoardScopedEventType(doc.Type) || IsDirectedAccessSignal(doc) {
		t.Fatal("document_replaced must be board-scoped but not directed")
	}
	if boardID, ok := BoardIDOf(doc); !ok || boardID != "board_abc123" {
		t.Fatalf("BoardIDOf=%q %v", boardID, ok)
	}
	task, err := MintV1Envelope(EventV1TaskCreated, "ws_abc123", testActor(),
		V1Resource{Type: V1ResourceTask, ID: "task_abc123"},
		V1TaskCreatedData{TaskID: "task_abc123", Service: "render"})
	if err != nil {
		t.Fatal(err)
	}
	if IsBoardScopedEventType(task.Type) || IsDirectedAccessSignal(task) {
		t.Fatal("task.created must not be board-scoped")
	}
	if _, ok := BoardIDOf(task); ok {
		t.Fatal("task.created must carry no board scope")
	}
	signal := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	if !IsDirectedAccessSignal(signal) {
		t.Fatal("access_changed must be directed")
	}
	if recipient, ok := AccessChangedRecipient(signal); !ok || recipient != "usr_member1" {
		t.Fatalf("recipient=%q %v", recipient, ok)
	}
	if version, ok := AccessChangedVersion(signal); !ok || version != 3 {
		t.Fatalf("version=%d %v", version, ok)
	}
	if _, ok := AccessChangedRecipient(doc); ok {
		t.Fatal("document_replaced must have no directed recipient")
	}
}

func TestStableBoardAccessEventIDConvergesTriple(t *testing.T) {
	a := StableBoardAccessEventID("board_abc123", "usr_member1", 3)
	b := StableBoardAccessEventID("board_abc123", "usr_member1", 3)
	if a != b || !contract.IsEventID(a) {
		t.Fatalf("stable id mismatch: %q %q", a, b)
	}
	if a == StableBoardAccessEventID("board_abc123", "usr_member1", 4) ||
		a == StableBoardAccessEventID("board_abc123", "usr_member2", 3) ||
		a == StableBoardAccessEventID("board_other999", "usr_member1", 3) {
		t.Fatal("different (board, member, version) triples must not share an event id")
	}
	record := NewBoardAccessOutboxRecord("ws_abc123", "board_abc123", "usr_member1", 3, time.Now().UTC())
	if record.EventID != a {
		t.Fatalf("record id=%q, want stable %q", record.EventID, a)
	}
	envelope, err := record.AccessChangedEnvelope()
	if err != nil {
		t.Fatalf("record envelope: %v", err)
	}
	if envelope.ID != a {
		t.Fatalf("envelope id=%q, want stable %q", envelope.ID, a)
	}
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
	if remote.ID != signal.ID || remote.Type != EventV1BoardAccessChanged {
		t.Fatalf("bridged envelope=%+v, want the access_changed signal", remote)
	}
	if _, _, err := decodeWorkspaceEventForInstance([]byte(`{"envelope":{}}`), "api-b"); err == nil {
		t.Fatal("payload without origin must be rejected")
	}
}

func TestWorkspaceEventBridgeHandlerAppendsWithoutLoops(t *testing.T) {
	logA := NewWorkspaceEventLog()
	logB := NewWorkspaceEventLog()
	signal := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	logA.Append(signal)
	handler := NewWorkspaceEventBridgeHandler(logB)
	if err := handler(context.Background(), signal); err != nil {
		t.Fatalf("bridge handler: %v", err)
	}
	if logB.Len("ws_abc123") != 1 {
		t.Fatalf("instance B log=%d, want 1 bridged envelope", logB.Len("ws_abc123"))
	}
	// Redelivery converges: the duplicate append dedups without refanout.
	live, unsubscribe, ok := logB.Subscribe("ws_abc123", WorkspaceEventFilter{})
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
	if err := handler(context.Background(), V1Envelope{}); err != nil {
		t.Fatalf("bridge handler must drop invalid envelopes without error: %v", err)
	}
	if logB.Len("ws_abc123") != 1 {
		t.Fatalf("instance B log=%d, want still 1", logB.Len("ws_abc123"))
	}
	if err := NewWorkspaceEventBridgeHandler(nil)(context.Background(), signal); err != nil {
		t.Fatalf("nil-log bridge must no-op: %v", err)
	}
}

type fakeBoardAccessReader struct {
	rows      []BoardAccessOutboxRow
	delivered []string
	attempts  int
}

func (f *fakeBoardAccessReader) Pending(_ context.Context, _ int) ([]BoardAccessOutboxRow, error) {
	return append([]BoardAccessOutboxRow(nil), f.rows...), nil
}

func (f *fakeBoardAccessReader) MarkDelivered(_ context.Context, id string) error {
	f.delivered = append(f.delivered, id)
	return nil
}

func (f *fakeBoardAccessReader) MarkAttempt(_ context.Context, _ string, _ error) error {
	f.attempts++
	return nil
}

func boardAccessRowForTest(t *testing.T, member string, version int64) BoardAccessOutboxRow {
	t.Helper()
	record := NewBoardAccessOutboxRecord("ws_abc123", "board_abc123", member, version, time.Now().UTC())
	return BoardAccessOutboxRow{BoardAccessOutboxRecord: record}
}

func TestBoardAccessOutboxReplayDeliversToLogAndBus(t *testing.T) {
	log := NewWorkspaceEventLog()
	reader := &fakeBoardAccessReader{rows: []BoardAccessOutboxRow{
		boardAccessRowForTest(t, "usr_member1", 3),
	}}
	replayer := &BoardAccessSignals{Store: reader, Log: log, Bus: &NATSJetStreamEventBus{}}
	if err := replayer.Replay(context.Background(), 100); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(reader.delivered) != 1 {
		t.Fatalf("delivered=%v, want the stable id marked once", reader.delivered)
	}
	if log.Len("ws_abc123") != 1 {
		t.Fatalf("hot log=%d, want 1 replayed signal", log.Len("ws_abc123"))
	}
	missed, gap, _ := log.ReplayAfter("ws_abc123", "", WorkspaceEventFilter{})
	_ = missed
	_ = gap
	replayed, _, found := log.ReplayAfter("ws_abc123", "evt_doesnotexist0000", WorkspaceEventFilter{})
	if len(replayed) != 1 || found {
		t.Fatalf("replayed=%d found=%v, want the durable signal in the window", len(replayed), found)
	}
	recipient, ok := AccessChangedRecipient(replayed[0])
	if !ok || recipient != "usr_member1" {
		t.Fatalf("replayed recipient=%q %v", recipient, ok)
	}
	// Second sweep converges: delivered rows leave Pending (fake models
	// the store flip by dropping them), so no duplicate lands.
	reader.rows = nil
	if err := replayer.Replay(context.Background(), 100); err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if log.Len("ws_abc123") != 1 {
		t.Fatalf("hot log=%d after second sweep, want still 1", log.Len("ws_abc123"))
	}
}

func TestBoardAccessOutboxReplayRetriesWithoutWebhooks(t *testing.T) {
	reader := &fakeBoardAccessReader{rows: []BoardAccessOutboxRow{
		{BoardAccessOutboxRecord: BoardAccessOutboxRecord{
			EventID: "not-an-event-id", WorkspaceID: "ws_abc123", BoardID: "board_abc123",
			MemberPublicID: "usr_member1", AccessVersion: 3, OccurredAt: time.Now().UTC(),
		}},
	}}
	// The replayer has no webhook sink by construction: an invalid row
	// retries through MarkAttempt and is never marked delivered.
	replayer := &BoardAccessSignals{Store: reader, Log: NewWorkspaceEventLog(), Bus: nil}
	if err := replayer.deliver(context.Background(), reader.rows[0].BoardAccessOutboxRecord); err == nil {
		t.Fatal("invalid row must fail delivery (unknown evt_ id cannot mint)")
	}
	if err := replayer.Replay(context.Background(), 100); err == nil {
		t.Fatal("invalid record delivery must report failure")
	}
	if reader.attempts != 1 || len(reader.delivered) != 0 {
		t.Fatalf("attempts=%d delivered=%v, want 1 attempt and no delivery", reader.attempts, reader.delivered)
	}
	// The replayer struct owns Log + Bus only: directed signals have no
	// webhook sink by construction (see BoardAccessSignals).
	valid := boardAccessRowForTest(t, "usr_member1", 3)
	sinkless := &BoardAccessSignals{Store: reader}
	if err := sinkless.deliver(context.Background(), valid.BoardAccessOutboxRecord); err != nil {
		t.Fatalf("sinkless deliver must still mint without error: %v", err)
	}
}

func TestBoardAccessOutboxRecordValidation(t *testing.T) {
	good := NewBoardAccessOutboxRecord("ws_abc123", "board_abc123", "usr_member1", 3, time.Now().UTC())
	if err := good.valid(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	bad := good
	bad.MemberPublicID = "alice"
	if err := bad.valid(); err == nil {
		t.Fatal("non-usr_ recipient must fail closed")
	}
	bad = good
	bad.AccessVersion = 0
	if err := bad.valid(); err == nil {
		t.Fatal("non-positive version must fail closed")
	}
	if strings.TrimSpace(good.EventID) == "" {
		t.Fatal("record must carry the deterministic evt_ id")
	}
}

type failingBoardAccessPublisher struct {
	err       error
	envelopes []V1Envelope
}

func (p *failingBoardAccessPublisher) PublishWorkspaceEvent(_ context.Context, envelope V1Envelope) error {
	p.envelopes = append(p.envelopes, envelope)
	return p.err
}

func TestBoardAccessImmediateDeliveryAndReplayShareAcceptedFact(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 13, 14, 123456789, time.UTC)
	record := NewBoardAccessOutboxRecord("ws_abc123", "board_abc123", "usr_member1", 3, at)
	reader := &fakeBoardAccessReader{rows: []BoardAccessOutboxRow{{BoardAccessOutboxRecord: record}}}
	publisher := &failingBoardAccessPublisher{err: errors.New("publish unavailable")}
	log := NewWorkspaceEventLog()
	signals := &BoardAccessSignals{Store: reader, Log: log, Bus: publisher}
	if err := signals.Deliver(t.Context(), []BoardAccessOutboxRecord{record}); err == nil {
		t.Fatal("failed immediate delivery must remain retryable")
	}
	if len(reader.delivered) != 0 || reader.attempts != 1 || log.Len(record.WorkspaceID) != 1 {
		t.Fatalf("delivered=%v attempts=%d log=%d", reader.delivered, reader.attempts, log.Len(record.WorkspaceID))
	}
	publisher.err = nil
	if err := signals.Replay(t.Context(), 100); err != nil {
		t.Fatal(err)
	}
	if len(reader.delivered) != 1 || log.Len(record.WorkspaceID) != 1 || len(publisher.envelopes) != 2 {
		t.Fatalf("delivered=%v log=%d publications=%d", reader.delivered, log.Len(record.WorkspaceID), len(publisher.envelopes))
	}
	if !reflect.DeepEqual(publisher.envelopes[0], publisher.envelopes[1]) {
		t.Fatal("immediate and replay deliveries must preserve the entire accepted fact")
	}
	if !publisher.envelopes[0].OccurredAt.Equal(at.Truncate(time.Microsecond)) {
		t.Fatal("accepted occurrence must retain PostgreSQL precision across delivery paths")
	}
}

func TestBoardAccessDeliveryDoesNotBlockOtherRecipients(t *testing.T) {
	valid := boardAccessRowForTest(t, "usr_member1", 3).BoardAccessOutboxRecord
	invalid := valid
	invalid.EventID = "invalid"
	reader := &fakeBoardAccessReader{}
	log := NewWorkspaceEventLog()
	signals := &BoardAccessSignals{Store: reader, Log: log}
	if err := signals.Deliver(t.Context(), []BoardAccessOutboxRecord{invalid, valid}); err == nil {
		t.Fatal("invalid record must report failure")
	}
	if reader.attempts != 1 || len(reader.delivered) != 1 || reader.delivered[0] != valid.EventID || log.Len(valid.WorkspaceID) != 1 {
		t.Fatalf("attempts=%d delivered=%v log=%d", reader.attempts, reader.delivered, log.Len(valid.WorkspaceID))
	}
}
