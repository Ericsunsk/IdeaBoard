package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/taskservice"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestNATSJetStreamTaskEventBridgeSmoke(t *testing.T) {
	natsURL := strings.TrimSpace(os.Getenv("API_TEST_NATS_URL"))
	if natsURL == "" {
		t.Skip("set API_TEST_NATS_URL to run NATS JetStream smoke test")
	}

	suffix := time.Now().UnixNano()
	streamName := fmt.Sprintf("IDEABOARD_EVENTS_TEST_%d", suffix)
	subjectPrefix := fmt.Sprintf("ideaboard.test.%d", suffix)
	consumerName := fmt.Sprintf("ideaboard-api-events-test-%d", suffix)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hub := taskservice.NewEventHub()
	publisher, eventBus, err := newTaskStatusEventPublisher(ctx, config.EventBusConfig{
		Enabled:            true,
		NATSURL:            natsURL,
		NATSStreamName:     streamName,
		NATSSubjectPrefix:  subjectPrefix,
		NATSConsumerName:   consumerName,
		NATSRetentionDays:  1,
		NATSConnectTimeout: 5,
		NATSRequestTimeout: 5,
		NATSDrainTimeout:   2,
	}, hub)
	if err != nil {
		t.Fatalf("newTaskStatusEventPublisher failed: %v", err)
	}
	t.Cleanup(func() {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		deleteNATSTestStream(t, natsURL, streamName)
	})

	taskID := "task-nats-smoke"
	taskEvents, unsubscribe, ok := hub.Subscribe(taskID, 99)
	if !ok {
		t.Fatal("hub subscribe rejected unexpectedly")
	}
	defer unsubscribe()

	if err := publisher.PublishTaskStatus(ctx, events.NewTaskStatusEvent(taskID, tasks.StatusRunning)); err != nil {
		t.Fatalf("PublishTaskStatus failed: %v", err)
	}

	for received := 0; received < 2; received++ {
		select {
		case event := <-taskEvents:
			if event.TaskID != taskID || event.Status != tasks.StatusRunning {
				t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, taskID, tasks.StatusRunning)
			}
		case <-ctx.Done():
			t.Fatalf("timeout waiting for local and bridged task events after %d events: %v", received, ctx.Err())
		}
	}
}

func TestCompositeTaskStatusPublisherKeepsLocalHubOnRemoteFailure(t *testing.T) {
	ctx := context.Background()
	hub := taskservice.NewEventHub()
	taskID := "task-local-fallback"
	taskEvents, unsubscribe, ok := hub.Subscribe(taskID, 99)
	if !ok {
		t.Fatal("hub subscribe rejected unexpectedly")
	}
	defer unsubscribe()

	remoteErr := errors.New("nats unavailable")
	publisher := compositeTaskStatusPublisher{
		local:  taskEventHubPublisher{hub: hub},
		remote: failingTaskStatusPublisher{err: remoteErr},
	}

	err := publisher.PublishTaskStatus(ctx, events.NewTaskStatusEvent(taskID, tasks.StatusRunning))
	if !errors.Is(err, remoteErr) {
		t.Fatalf("err = %v, want remote error", err)
	}

	select {
	case event := <-taskEvents:
		if event.TaskID != taskID || event.Status != tasks.StatusRunning {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, taskID, tasks.StatusRunning)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for local fallback event")
	}
}

type failingTaskStatusPublisher struct {
	err error
}

func (p failingTaskStatusPublisher) PublishTaskStatus(context.Context, events.TaskStatusEvent) error {
	return p.err
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
