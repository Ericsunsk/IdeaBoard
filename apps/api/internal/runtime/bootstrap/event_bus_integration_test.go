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
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskservice"

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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	t.Cleanup(func() { deleteNATSTestStream(t, natsURL, streamName) })

	originPublisher, originHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("origin-%d", suffix))
	_, firstRemoteHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("remote-a-%d", suffix))
	_, secondRemoteHub := newTestTaskStatusInstance(t, ctx, natsURL, streamName, subjectPrefix, fmt.Sprintf("remote-b-%d", suffix))

	taskID := "task-nats-smoke"
	originEvents, unsubscribeOrigin, ok := originHub.Subscribe(taskID, 99)
	if !ok {
		t.Fatal("origin hub subscribe rejected unexpectedly")
	}
	defer unsubscribeOrigin()
	firstRemoteEvents, unsubscribeFirstRemote, ok := firstRemoteHub.Subscribe(taskID, 100)
	if !ok {
		t.Fatal("first remote hub subscribe rejected unexpectedly")
	}
	defer unsubscribeFirstRemote()
	secondRemoteEvents, unsubscribeSecondRemote, ok := secondRemoteHub.Subscribe(taskID, 101)
	if !ok {
		t.Fatal("second remote hub subscribe rejected unexpectedly")
	}
	defer unsubscribeSecondRemote()

	if err := originPublisher.PublishTaskStatus(ctx, events.NewTaskStatusEvent(taskID, tasks.StatusRunning)); err != nil {
		t.Fatalf("PublishTaskStatus failed: %v", err)
	}

	assertTaskEvent(t, ctx, originEvents, taskID, tasks.StatusRunning, "origin")
	assertTaskEvent(t, ctx, firstRemoteEvents, taskID, tasks.StatusRunning, "first remote")
	assertTaskEvent(t, ctx, secondRemoteEvents, taskID, tasks.StatusRunning, "second remote")
	assertNoTaskEvent(t, originEvents, "origin")
	assertNoTaskEvent(t, firstRemoteEvents, "first remote")
	assertNoTaskEvent(t, secondRemoteEvents, "second remote")
}

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
	assertNoTaskEvent(t, taskEvents, "origin after remote publish failure")
}

func TestTaskEventHubPublisherProjectsToUserHubWithoutSecondRemotePublish(t *testing.T) {
	userHub := events.NewUserEventHub()
	remotePublishes := 0
	userHub.SetRemotePublisher(func(events.UserEvent) { remotePublishes++ })
	userEvents, unsubscribe, accepted := userHub.Subscribe(42)
	if !accepted {
		t.Fatal("user event subscription rejected unexpectedly")
	}
	defer unsubscribe()

	publisher := taskEventHubPublisher{
		hub:      taskservice.NewEventHub(),
		userHub:  userHub,
		resolver: fixedTaskResolver{task: &tasks.Task{UserID: 42}},
	}
	if err := publisher.PublishTaskStatus(t.Context(), events.NewTaskStatusEvent("task-1", tasks.StatusRunning)); err != nil {
		t.Fatalf("publish task status: %v", err)
	}

	select {
	case event := <-userEvents:
		if event.Type != events.EventTypeTaskUpdated {
			t.Fatalf("event type=%q, want %q", event.Type, events.EventTypeTaskUpdated)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for user task event")
	}
	if remotePublishes != 0 {
		t.Fatalf("task projection published %d duplicate user events", remotePublishes)
	}
}

type fixedTaskResolver struct {
	task *tasks.Task
}

func (r fixedTaskResolver) GetByTaskID(context.Context, string) (*tasks.Task, error) {
	return r.task, nil
}

type failingTaskStatusPublisher struct {
	err error
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
