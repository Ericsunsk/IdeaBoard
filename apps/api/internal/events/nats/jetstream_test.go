package nats

import (
	"encoding/json"
	"testing"

	"ideaboard/api/internal/events"
)

func TestNormalizeNATSJetStreamConfig(t *testing.T) {
	cfg := normalizeNATSJetStreamConfig(NATSJetStreamConfig{
		URL:           " nats://127.0.0.1:4222 ",
		StreamName:    "IdeaBoard.Events",
		SubjectPrefix: " idea.board.* ",
		ConsumerName:  "api.instance/1",
	})

	if cfg.URL != "nats://127.0.0.1:4222" {
		t.Fatalf("URL=%q, want trimmed URL", cfg.URL)
	}
	if cfg.StreamName != "IdeaBoard_Events" {
		t.Fatalf("StreamName=%q, want IdeaBoard_Events", cfg.StreamName)
	}
	if cfg.SubjectPrefix != "idea.board" {
		t.Fatalf("SubjectPrefix=%q, want idea.board", cfg.SubjectPrefix)
	}
	if cfg.ConsumerName != "api_instance_1" {
		t.Fatalf("ConsumerName=%q, want api_instance_1", cfg.ConsumerName)
	}
}

func TestTaskStatusSubject(t *testing.T) {
	if got := taskStatusSubject(" idea.board "); got != "idea.board.task.status" {
		t.Fatalf("taskStatusSubject=%q, want idea.board.task.status", got)
	}
}

func TestDecodeUserEventForInstanceSkipsLocalOriginAndDeliversRemote(t *testing.T) {
	event := events.UserEvent{UserID: 42, Type: events.EventTypeTaskUpdated, Data: []byte(`{"task_id":"task-1"}`)}
	payload, err := json.Marshal(userEventEnvelope{OriginInstance: "api-instance-1", Event: event})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if _, shouldDeliver, err := decodeUserEventForInstance(payload, "api-instance-1"); err != nil {
		t.Fatalf("decode local-origin user event failed: %v", err)
	} else if shouldDeliver {
		t.Fatal("local-origin user event should not be delivered through NATS")
	}
	got, shouldDeliver, err := decodeUserEventForInstance(payload, "api-instance-2")
	if err != nil {
		t.Fatalf("decode remote user event failed: %v", err)
	}
	if !shouldDeliver || got.UserID != event.UserID || got.Type != event.Type || string(got.Data) != string(event.Data) {
		t.Fatalf("event=%+v shouldDeliver=%v, want remote event %+v", got, shouldDeliver, event)
	}
}

func TestDecodeUserEventForInstanceRejectsMalformedPayload(t *testing.T) {
	if _, _, err := decodeUserEventForInstance([]byte(`{"event":{"user_id":42,"type":"task.updated"}}`), "api-instance-1"); err == nil {
		t.Fatal("payload without transport origin should be rejected")
	}
	if _, _, err := decodeUserEventForInstance([]byte(`{"origin_instance":"api-instance-1","event":{"user_id":0,"type":"task.updated"}}`), "api-instance-1"); err == nil {
		t.Fatal("payload without a valid user id should be rejected")
	}
}

func TestTaskStatusEnvelopeCarriesTransportOriginSeparatelyFromDomainEvent(t *testing.T) {
	event := events.NewTaskStatusEvent("task-1", "running")
	payload, err := json.Marshal(taskStatusEnvelope{
		OriginInstance: "api-instance-1",
		Event:          event,
	})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var envelope taskStatusEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if envelope.OriginInstance != "api-instance-1" {
		t.Fatalf("OriginInstance=%q, want api-instance-1", envelope.OriginInstance)
	}
	if envelope.Event.TaskID != event.TaskID || envelope.Event.Status != event.Status {
		t.Fatalf("Event=(%s,%s), want=(%s,%s)", envelope.Event.TaskID, envelope.Event.Status, event.TaskID, event.Status)
	}
}

func TestDecodeTaskStatusForInstanceSkipsLocalOriginAndDeliversRemote(t *testing.T) {
	payload, err := json.Marshal(taskStatusEnvelope{
		OriginInstance: "api-instance-1",
		Event:          events.NewTaskStatusEvent("task-1", "running"),
	})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if _, shouldDeliver, err := decodeTaskStatusForInstance(payload, "api-instance-1"); err != nil {
		t.Fatalf("decode local-origin event failed: %v", err)
	} else if shouldDeliver {
		t.Fatal("local-origin event should not be delivered through NATS")
	}

	event, shouldDeliver, err := decodeTaskStatusForInstance(payload, "api-instance-2")
	if err != nil {
		t.Fatalf("decode remote event failed: %v", err)
	}
	if !shouldDeliver {
		t.Fatal("remote event should be delivered")
	}
	if event.TaskID != "task-1" || event.Status != "running" {
		t.Fatalf("Event=(%s,%s), want=(task-1,running)", event.TaskID, event.Status)
	}
}

func TestDecodeTaskStatusForInstanceRejectsPayloadWithoutTransportOrigin(t *testing.T) {
	payload, err := json.Marshal(events.NewTaskStatusEvent("task-1", "running"))
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if _, _, err := decodeTaskStatusForInstance(payload, "api-instance-1"); err == nil {
		t.Fatal("payload without transport origin should be rejected")
	}
}
