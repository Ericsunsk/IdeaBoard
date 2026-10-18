package webhook

import (
	"context"
	"testing"

	"ideaboard/api/internal/events"
)

func TestWorkspaceWebhooksRejectProtectedSubscriptionsAndDeliveries(t *testing.T) {
	for _, typ := range events.V1EventTypes() {
		t.Run(typ, func(t *testing.T) {
			sender := &fakeWebhookSender{status: 200}
			enqueuer := &fakeWebhookEnqueuer{}
			store := NewWebhookStore(sender, enqueuer)
			if !IsWorkspaceWebhookEventType(typ) {
				if _, _, _, err := store.Create(t.Context(), "ws_1", "https://hooks.example.test/events", []string{typ}, nil); err == nil {
					t.Fatal("protected event subscription must be rejected")
				}
			}
			sub, _, _, err := store.Create(t.Context(), "ws_1", "https://hooks.example.test/events", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(sub.EventTypes) != 1 || sub.EventTypes[0] != events.EventV1BillingSettled {
				t.Fatalf("default subscription types=%v", sub.EventTypes)
			}
			envelope := protectedWebhookEnvelope(t, typ)
			// Model an existing persisted subscription before enqueue.
			sub.EventTypes = []string{typ}
			if err := store.DispatchErr(t.Context(), envelope); err != nil {
				t.Fatal(err)
			}
			if len(enqueuer.inserts) != 0 {
				t.Fatal("protected event must not enqueue")
			}
			worker := NewWebhookDeliverWorker(store)
			args := WebhookDeliveryArgs{SubscriptionID: sub.ID, DeliveryID: envelope.ID, Envelope: envelope}
			for attempt := 1; attempt <= 2; attempt++ {
				if err := worker.Work(context.Background(), webhookJob(args, attempt, 8)); err != nil {
					t.Fatalf("protected queued event must drop terminally: %v", err)
				}
			}
			if sender.calls != 0 {
				t.Fatal("protected event must not send")
			}
			if _, ok := sub.Delivered[envelope.ID]; !ok {
				t.Fatal("protected delivery drop must be terminal")
			}
		})
	}
}

func protectedWebhookEnvelope(t *testing.T, typ string) events.V1Envelope {
	t.Helper()
	resource := events.V1Resource{Type: events.V1ResourceTask, ID: "task_1"}
	data := map[string]any{"task_id": "task_1", "service": "render", "error_code": "failed",
		"outputs_uri": "/api/v1/workspaces/ws_1/tasks/task_1/outputs"}
	switch typ {
	case events.EventV1BillingSettled:
		data = map[string]any{"reference_type": "task", "reference_id": "task_1", "settlement_state": "captured"}
	case events.EventV1MediaAssetReady, events.EventV1MediaAssetArchived:
		resource = events.V1Resource{Type: events.V1ResourceMediaAsset, ID: "asset_1"}
		data = map[string]any{"asset_id": "asset_1"}
	case events.EventV1BoardDocumentReplaced:
		resource = events.V1Resource{Type: events.V1ResourceBoard, ID: "board_1"}
		data = map[string]any{"board_id": "board_1", "revision": 1}
	case events.EventV1BoardAccessChanged:
		resource = events.V1Resource{Type: events.V1ResourceBoard, ID: "board_1"}
		data = map[string]any{"board_id": "board_1", "member_id": "usr_1", "access_version": 1}
	}
	envelope, err := events.MintV1Envelope(typ, "ws_1", events.V1Actor{Type: events.V1ActorSystem, ID: "system"}, resource, data)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestWorkspaceWebhookRejectsUnscopedSettlementContent(t *testing.T) {
	for name, raw := range map[string]string{
		"Task reference":             `{"reference_type":"task","reference_id":"task_1","settlement_state":"captured"}`,
		"mismatched identity":        `{"reference_type":"checkout_session","reference_id":"checkout_other","settlement_state":"captured"}`,
		"hidden protected reference": `{"reference_type":"checkout_session","reference_id":"checkout_1","settlement_state":"captured","task_id":"task_private"}`,
	} {
		t.Run(name, func(t *testing.T) {
			sender := &fakeWebhookSender{status: 200}
			enqueuer := &fakeWebhookEnqueuer{}
			store := NewWebhookStore(sender, enqueuer)
			sub, _, _, err := store.Create(t.Context(), "ws_1", "https://hooks.example.test/events", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			envelope := testEnvelope(t)
			envelope.Data = []byte(raw)
			if err := store.DispatchErr(t.Context(), envelope); err != nil {
				t.Fatal(err)
			}
			if len(enqueuer.inserts) != 0 {
				t.Fatal("unscoped settlement content must not enqueue")
			}
			worker := NewWebhookDeliverWorker(store)
			args := WebhookDeliveryArgs{SubscriptionID: sub.ID, DeliveryID: envelope.ID, Envelope: envelope}
			if err := worker.Work(t.Context(), webhookJob(args, 1, 8)); err != nil {
				t.Fatal(err)
			}
			if sender.calls != 0 {
				t.Fatal("unscoped settlement content must not send")
			}
		})
	}
}
