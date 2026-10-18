package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/events"
)

type fakeOutboxReader struct {
	rows      []TaskStatusOutboxRow
	delivered []string
	attempts  int
}

func (f *fakeOutboxReader) Pending(_ context.Context, _ int) ([]TaskStatusOutboxRow, error) {
	return append([]TaskStatusOutboxRow(nil), f.rows...), nil
}
func (f *fakeOutboxReader) MarkDelivered(_ context.Context, id string) error {
	f.delivered = append(f.delivered, id)
	return nil
}
func (f *fakeOutboxReader) MarkAttempt(_ context.Context, _ string, _ error) error {
	f.attempts++
	return nil
}

type fakeStatusPublisher struct {
	err    error
	events []events.TaskStatusEvent
}

func (p *fakeStatusPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.events = append(p.events, event)
	return p.err
}

func TestStableEventIDIsDeterministicAndTransitionScoped(t *testing.T) {
	a := events.StableEventID("task_abc123", string(events.TaskStatusCompleted), "exec_one")
	b := events.StableEventID("task_abc123", string(events.TaskStatusCompleted), "exec_one")
	if a != b || !events.IsV1EventType(events.EventV1TaskCompleted) {
		t.Fatalf("stable id mismatch: %q %q", a, b)
	}
	if a == events.StableEventID("task_abc123", string(events.TaskStatusFailed), "exec_one") || a == events.StableEventID("task_abc123", string(events.TaskStatusCompleted), "exec_two") {
		t.Fatal("different transitions must not share an event id")
	}
}

func TestTaskStatusOutboxWorkspaceEnvelopeRetainsIdentityAndTimestamp(t *testing.T) {
	at := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	raw, _ := json.Marshal(events.V1TaskFailedData{TaskID: "task_abc123", ErrorCode: "task_failed"})
	row := TaskStatusOutboxRow{TaskStatusOutboxRecord: TaskStatusOutboxRecord{
		EventID: events.StableEventID("task_abc123", string(events.TaskStatusFailed), "exec_one"), WorkspaceID: "ws_abc123", TaskID: "task_abc123", Status: events.TaskStatusFailed,
		EventType: events.EventV1TaskFailed, EventVersion: 1, ActorType: events.V1ActorService, ActorID: "tasking", ResourceType: events.V1ResourceTask, ResourceID: "task_abc123", EventData: raw, OccurredAt: at,
	}}
	envelope, ok, err := row.WorkspaceEnvelope()
	if err != nil || !ok {
		t.Fatalf("WorkspaceEnvelope = %+v, %v", envelope, err)
	}
	if envelope.ID != row.EventID || !envelope.OccurredAt.Equal(at) {
		t.Fatalf("envelope identity/time = %q/%s, want %q/%s", envelope.ID, envelope.OccurredAt, row.EventID, at)
	}
}

func TestTaskStatusOutboxWorkspaceEnvelopeSkipsInternalRetry(t *testing.T) {
	row := TaskStatusOutboxRow{TaskStatusOutboxRecord: TaskStatusOutboxRecord{EventID: events.StableEventID("task_abc123", string(events.TaskStatusPending), "exec_one"), WorkspaceID: "ws_abc123", TaskID: "task_abc123", Status: events.TaskStatusPending, OccurredAt: time.Now().UTC()}}
	if _, ok, err := row.WorkspaceEnvelope(); err != nil || ok {
		t.Fatalf("internal retry envelope = ok:%v err:%v, want no workspace envelope", ok, err)
	}
}

func TestTaskStatusOutboxReplayerRetriesPublishFailure(t *testing.T) {
	row := TaskStatusOutboxRow{TaskStatusOutboxRecord: TaskStatusOutboxRecord{EventID: events.StableEventID("task_abc123", string(events.TaskStatusCompleted), "exec_one"), WorkspaceID: "ws_abc123", TaskID: "task_abc123", Status: events.TaskStatusCompleted, OccurredAt: time.Now().UTC()}}
	store := &fakeOutboxReader{rows: []TaskStatusOutboxRow{row}}
	publisher := &fakeStatusPublisher{err: errors.New("nats down")}
	if err := (&TaskStatusOutboxReplayer{Store: store, Status: publisher}).Replay(context.Background(), 10); err != nil {
		t.Fatalf("Replay should isolate publish failure: %v", err)
	}
	if store.attempts != 1 || len(store.delivered) != 0 {
		t.Fatalf("failure bookkeeping attempts=%d delivered=%v", store.attempts, store.delivered)
	}
}

func TestTaskStatusOutboxDuplicateReplayKeepsStableIdentity(t *testing.T) {
	row := TaskStatusOutboxRow{TaskStatusOutboxRecord: TaskStatusOutboxRecord{EventID: events.StableEventID("task_abc123", string(events.TaskStatusCompleted), "exec_one"), WorkspaceID: "ws_abc123", TaskID: "task_abc123", Status: events.TaskStatusCompleted, EventType: events.EventV1TaskCompleted, EventVersion: 1, ActorType: events.V1ActorService, ActorID: "tasking", ResourceType: events.V1ResourceTask, ResourceID: "task_abc123", EventData: mustJSON(events.V1TaskCompletedData{TaskID: "task_abc123", OutputsURI: "/api/v1/workspaces/ws_abc123/tasks/task_abc123/outputs"}), OccurredAt: time.Now().UTC()}}
	store := &fakeOutboxReader{rows: []TaskStatusOutboxRow{row}}
	publisher := &fakeStatusPublisher{}
	log := events.NewWorkspaceEventLog()
	if _, _, err := row.WorkspaceEnvelope(); err != nil {
		t.Fatalf("row envelope: %v", err)
	}
	replayer := &TaskStatusOutboxReplayer{Store: store, Status: publisher, Workspace: log}
	if err := replayer.Replay(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if err := replayer.Replay(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(publisher.events) != 2 || publisher.events[0].EventID != publisher.events[1].EventID {
		t.Fatalf("replayed identities = %+v", publisher.events)
	}
	if log.Len("ws_abc123") != 1 {
		t.Fatalf("workspace log length = %d, want deduped 1", log.Len("ws_abc123"))
	}
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
