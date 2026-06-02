package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"api/internal/accounts"
	"api/internal/events"
	"api/internal/platform/config"
	"api/internal/tasks"

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

	hub := accounts.NewTaskEventHub()
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

	select {
	case event := <-taskEvents:
		if event.TaskID != taskID || event.Status != tasks.StatusRunning {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, taskID, tasks.StatusRunning)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for bridged task event: %v", ctx.Err())
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
