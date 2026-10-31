package wiring

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	modelregistrydomain "ideaboard/api/internal/modelregistry/domain"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
	taskservice "ideaboard/api/internal/tasking/application/task"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type routeCatalogStub []modelregistrydomain.CatalogItem

func (s routeCatalogStub) ListCatalog(context.Context) ([]modelregistrydomain.CatalogItem, error) {
	return s, nil
}

func TestLookupRouteOverrideProviderModelIdentity(t *testing.T) {
	reader := aiGatewayModelRouteReader{repo: routeCatalogStub{
		{Provider: "openai", ID: "GPT-IMAGE-1", CustomPath: "openai/custom-generate", CustomParams: `{"quality":"high"}`},
		{Provider: "azureopenai", ID: "gpt-image-1", CustomPath: "azure/custom-generate", CustomParams: `{"quality":"medium"}`},
		{Provider: "openrouter", ID: "Google/Gemini-3-Flash", CustomPath: "openrouter/custom-generate", CustomParams: `{"temperature":0.5}`},
	}}
	cases := []struct {
		name, provider, model, wantPath, wantParams string
		wantFound                                   bool
	}{
		{name: "case-insensitive native model", provider: "openai", model: "gpt-image-1", wantPath: "openai/custom-generate", wantParams: `{"quality":"high"}`, wantFound: true},
		{name: "same-name provider isolation", provider: " AzureOpenAI ", model: " GPT-IMAGE-1 ", wantPath: "azure/custom-generate", wantParams: `{"quality":"medium"}`, wantFound: true},
		{name: "nested native model", provider: "OpenRouter", model: "google/gemini-3-flash", wantPath: "openrouter/custom-generate", wantParams: `{"temperature":0.5}`, wantFound: true},
		{name: "different provider", provider: "anthropic", model: "gpt-image-1"},
		{name: "empty native model", provider: "openai", model: " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, params, found, err := reader.LookupRouteOverride(context.Background(), tc.provider, tc.model)
			if err != nil || path != tc.wantPath || params != tc.wantParams || found != tc.wantFound {
				t.Fatalf("LookupRouteOverride(%q, %q) = (%q, %q, %v, %v), want (%q, %q, %v, nil)", tc.provider, tc.model, path, params, found, err, tc.wantPath, tc.wantParams, tc.wantFound)
			}
		})
	}
}

// newTestTaskStatusInstance builds one isolated publisher+hub pair against
// the shared NATS test server. Test-only helper duplicated from
// interfaces/events (same package cannot import _test helpers across
// packages); production wiring has no duplication. EventBusConfig carries
// only NATSURL, so tests needing isolated streams build the bus
// directly with the same handshake NewTaskStatusEventPublisher performs.
func newTestTaskStatusInstance(
	t *testing.T,
	ctx context.Context,
	natsURL, streamName, subjectPrefix, consumerName string,
) (events.TaskStatusPublisher, *taskservice.EventHub) {
	t.Helper()
	hub := taskservice.NewEventHub()
	localPublisher := eventinterfaces.TaskEventHubPublisher{Hub: hub}
	bus, err := eventnats.NewNATSJetStreamEventBus(ctx, eventnats.NATSJetStreamConfig{
		URL:            natsURL,
		StreamName:     streamName,
		SubjectPrefix:  subjectPrefix,
		ConsumerName:   consumerName,
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 5 * time.Second,
		DrainTimeout:   2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new Task status publisher %s failed: %v", consumerName, err)
	}
	remotePublisher := eventinterfaces.TaskEventHubPublisher{Hub: hub}
	if err := bus.SubscribeTaskStatus(ctx, remotePublisher.PublishTaskStatus); err != nil {
		_ = bus.Close()
		t.Fatalf("subscribe Task status %s failed: %v", consumerName, err)
	}
	publisher := eventinterfaces.CompositeTaskStatusPublisher{Local: localPublisher, Remote: bus}
	t.Cleanup(func() {
		_ = bus.Close()
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
