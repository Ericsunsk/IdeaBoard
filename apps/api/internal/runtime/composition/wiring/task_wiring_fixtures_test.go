package wiring

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	"ideaboard/api/internal/platform/config"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	taskservice "ideaboard/api/internal/tasking/application/task"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func newTaskStatusEventPublisher(
	ctx context.Context,
	cfg config.EventBusConfig,
	hub *taskservice.EventHub,
) (events.TaskStatusPublisher, *eventnats.NATSJetStreamEventBus, error) {
	return eventinterfaces.NewTaskStatusEventPublisher(ctx, cfg, hub, nil)
}

// newTestTaskStatusInstance builds one isolated publisher+hub pair against
// the shared NATS test server. Test-only helper duplicated from
// interfaces/events (same package cannot import _test helpers across
// packages); production wiring has no duplication.
func newTestTaskStatusInstance(
	t *testing.T,
	ctx context.Context,
	natsURL, streamName, subjectPrefix, consumerName string,
) (events.TaskStatusPublisher, *taskservice.EventHub) {
	t.Helper()
	hub := taskservice.NewEventHub()
	publisher, eventBus, err := newTaskStatusEventPublisher(ctx, config.EventBusConfig{
		Enabled:            true,
		NATSURL:            natsURL,
		NATSStreamName:     streamName,
		NATSSubjectPrefix:  subjectPrefix,
		NATSConsumerName:   consumerName,
		NATSConnectTimeout: 5,
		NATSRequestTimeout: 5,
		NATSDrainTimeout:   2,
	}, hub)
	if err != nil {
		t.Fatalf("new Task status publisher %s failed: %v", consumerName, err)
	}
	t.Cleanup(func() {
		if eventBus != nil {
			_ = eventBus.Close()
		}
	})
	return publisher, hub
}

func assertTaskEvent(t *testing.T, ctx context.Context, taskEvents <-chan taskservice.TaskEvent, taskID, status, instance string) {
	t.Helper()
	select {
	case event := <-taskEvents:
		if event.TaskID != taskID || event.Status != status {
			t.Fatalf("%s event=(%s,%s), want=(%s,%s)", instance, event.TaskID, event.Status, taskID, status)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for %s task event: %v", instance, ctx.Err())
	}
}

func assertNoTaskEvent(t *testing.T, taskEvents <-chan taskservice.TaskEvent, instance string) {
	t.Helper()
	select {
	case event := <-taskEvents:
		t.Fatalf("%s received duplicate task event: (%s,%s)", instance, event.TaskID, event.Status)
	case <-time.After(250 * time.Millisecond):
	}
}

func deleteNATSTestStream(t *testing.T, natsURL, streamName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nc, err := nats.Connect(natsURL, nats.Name("ideaboard-test-cleanup"), nats.Timeout(2*time.Second))
	if err != nil {
		t.Logf("cleanup connect nats failed: %v", err)
		return
	}
	defer nc.Close()

	js, err := jetstream.New(nc, jetstream.WithDefaultTimeout(2*time.Second))
	if err != nil {
		t.Logf("cleanup create jetstream client failed: %v", err)
		return
	}
	if err := js.DeleteStream(ctx, streamName); err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Logf("cleanup delete stream %s failed: %v", streamName, err)
	}
}
