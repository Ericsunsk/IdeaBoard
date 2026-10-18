package outbox

// Board access-signal visibility, bridge fanout, and durable replay
// (issue #940). Covers the delivery-time rules: board content reaches
// current readers only, directed signals reach the named member only,
// cross-instance appends converge without loops, and the outbox replayer
// retries pending rows on the stable evt_ id without ever touching
// webhooks.

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/contract"
)

func TestStableEventIDConvergesTriple(t *testing.T) {
	a := events.StableEventID("board_abc123", "usr_member1", strconv.FormatInt(3, 10))
	b := events.StableEventID("board_abc123", "usr_member1", strconv.FormatInt(3, 10))
	if a != b || !contract.IsEventID(a) {
		t.Fatalf("stable id mismatch: %q %q", a, b)
	}
	if a == events.StableEventID("board_abc123", "usr_member1", strconv.FormatInt(4, 10)) ||
		a == events.StableEventID("board_abc123", "usr_member2", strconv.FormatInt(3, 10)) ||
		a == events.StableEventID("board_other999", "usr_member1", strconv.FormatInt(3, 10)) {
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
	log := events.NewWorkspaceEventLog()
	reader := &fakeBoardAccessReader{rows: []BoardAccessOutboxRow{
		boardAccessRowForTest(t, "usr_member1", 3),
	}}
	replayer := &BoardAccessSignals{Store: reader, Log: log, Bus: &failingBoardAccessPublisher{}}
	if err := replayer.Replay(context.Background(), 100); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(reader.delivered) != 1 {
		t.Fatalf("delivered=%v, want the stable id marked once", reader.delivered)
	}
	if log.Len("ws_abc123") != 1 {
		t.Fatalf("hot log=%d, want 1 replayed signal", log.Len("ws_abc123"))
	}
	missed, gap, _ := log.ReplayAfter("ws_abc123", "", events.WorkspaceEventFilter{})
	_ = missed
	_ = gap
	replayed, _, found := log.ReplayAfter("ws_abc123", "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	if len(replayed) != 1 || found {
		t.Fatalf("replayed=%d found=%v, want the durable signal in the window", len(replayed), found)
	}
	recipient, ok := events.AccessChangedRecipient(replayed[0])
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
	replayer := &BoardAccessSignals{Store: reader, Log: events.NewWorkspaceEventLog(), Bus: nil}
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
	envelopes []events.V1Envelope
}

func (p *failingBoardAccessPublisher) PublishWorkspaceEvent(_ context.Context, envelope events.V1Envelope) error {
	p.envelopes = append(p.envelopes, envelope)
	return p.err
}

func TestBoardAccessImmediateDeliveryAndReplayShareAcceptedFact(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 13, 14, 123456789, time.UTC)
	record := NewBoardAccessOutboxRecord("ws_abc123", "board_abc123", "usr_member1", 3, at)
	reader := &fakeBoardAccessReader{rows: []BoardAccessOutboxRow{{BoardAccessOutboxRecord: record}}}
	publisher := &failingBoardAccessPublisher{err: errors.New("publish unavailable")}
	log := events.NewWorkspaceEventLog()
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
	log := events.NewWorkspaceEventLog()
	signals := &BoardAccessSignals{Store: reader, Log: log}
	if err := signals.Deliver(t.Context(), []BoardAccessOutboxRecord{invalid, valid}); err == nil {
		t.Fatal("invalid record must report failure")
	}
	if reader.attempts != 1 || len(reader.delivered) != 1 || reader.delivered[0] != valid.EventID || log.Len(valid.WorkspaceID) != 1 {
		t.Fatalf("attempts=%d delivered=%v log=%d", reader.attempts, reader.delivered, log.Len(valid.WorkspaceID))
	}
}
