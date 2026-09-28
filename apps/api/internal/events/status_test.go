package events

import "testing"

// TestCanWebhookTransitionMirrorsGuards pins the process-local
// subscription transition table to the guards in webhook_delivery.go
// (failed attempt degrades, clean delivery restores, budget exhaustion
// parks) and postgres_store.go hydrateSubscription. dead_letter is
// terminal. Every true entry was already possible before typing.
//
// NOTE: this table documents the in-memory projection only. The
// durable event_subscriptions.status column stays 'active'
// (chk_event_subscriptions_status allows active/paused/disabled, but
// no Go writer emits paused/disabled and degraded/dead_letter are
// never persisted).
func TestCanWebhookTransitionMirrorsGuards(t *testing.T) {
	allowed := [][2]WebhookStatus{
		{WebhookStatusActive, WebhookStatusDegraded},
		{WebhookStatusActive, WebhookStatusDeadLetter},
		{WebhookStatusDegraded, WebhookStatusActive},
		{WebhookStatusDegraded, WebhookStatusDeadLetter},
	}
	for _, transition := range allowed {
		if !CanWebhookTransition(transition[0], transition[1]) {
			t.Errorf("CanWebhookTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	for _, to := range []WebhookStatus{WebhookStatusActive, WebhookStatusDegraded, WebhookStatusDeadLetter} {
		if CanWebhookTransition(WebhookStatusDeadLetter, to) {
			t.Errorf("CanWebhookTransition(dead_letter, %q) = true, want false (dead_letter is terminal)", to)
		}
	}
	if CanWebhookTransition(WebhookStatusActive, WebhookStatusActive) {
		t.Error("CanWebhookTransition(active, active) = true, want false (no self-transition exists)")
	}
	if CanWebhookTransition(WebhookStatusDegraded, WebhookStatusDegraded) {
		t.Error("CanWebhookTransition(degraded, degraded) = true, want false (no self-transition exists)")
	}
}

// TestCanDeliveryTransitionMirrorsGuards pins the durable delivery
// transition table to the writes in postgres_store.go (EnsureDelivery
// inserts pending; RecordAttempt flips to retrying/delivered/
// dead_letter; MarkDelivered flips to delivered) and
// chk_event_deliveries_status. 'failed' is legacy read-only: it has a
// word but no writer and no transitions.
func TestCanDeliveryTransitionMirrorsGuards(t *testing.T) {
	allowed := [][2]DeliveryStatus{
		{DeliveryStatusPending, DeliveryStatusRetrying},
		{DeliveryStatusPending, DeliveryStatusDelivered},
		{DeliveryStatusPending, DeliveryStatusDeadLetter},
		{DeliveryStatusRetrying, DeliveryStatusRetrying},
		{DeliveryStatusRetrying, DeliveryStatusDelivered},
		{DeliveryStatusRetrying, DeliveryStatusDeadLetter},
	}
	for _, transition := range allowed {
		if !CanDeliveryTransition(transition[0], transition[1]) {
			t.Errorf("CanDeliveryTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	for _, to := range []DeliveryStatus{
		DeliveryStatusPending, DeliveryStatusRetrying, DeliveryStatusDelivered,
		DeliveryStatusFailed, DeliveryStatusDeadLetter,
	} {
		if CanDeliveryTransition(DeliveryStatusDelivered, to) {
			t.Errorf("CanDeliveryTransition(delivered, %q) = true, want false (delivered is terminal)", to)
		}
		if CanDeliveryTransition(DeliveryStatusDeadLetter, to) {
			t.Errorf("CanDeliveryTransition(dead_letter, %q) = true, want false (dead_letter is terminal)", to)
		}
		if CanDeliveryTransition(DeliveryStatusFailed, to) {
			t.Errorf("CanDeliveryTransition(failed, %q) = true, want false (failed is legacy read-only)", to)
		}
	}
	if CanDeliveryTransition(DeliveryStatusPending, DeliveryStatusFailed) {
		t.Error("CanDeliveryTransition(pending, failed) = true, want false (failures stay retrying until delivered or dead_letter)")
	}
	if CanDeliveryTransition(DeliveryStatusPending, DeliveryStatusPending) {
		t.Error("CanDeliveryTransition(pending, pending) = true, want false (no self-transition exists)")
	}
}

// TestEventsStatusWireWords pins the wire/DB words: typing must not
// change the strings on the wire or in the database.
func TestEventsStatusWireWords(t *testing.T) {
	for status, want := range map[WebhookStatus]string{
		WebhookStatusActive: "active", WebhookStatusDegraded: "degraded",
		WebhookStatusDeadLetter: "dead_letter",
	} {
		if string(status) != want {
			t.Errorf("WebhookStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[DeliveryStatus]string{
		DeliveryStatusPending: "pending", DeliveryStatusRetrying: "retrying",
		DeliveryStatusDelivered: "delivered", DeliveryStatusFailed: "failed",
		DeliveryStatusDeadLetter: "dead_letter",
	} {
		if string(status) != want {
			t.Errorf("DeliveryStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[AttemptOutcome]string{
		AttemptOutcomeDelivered: "delivered", AttemptOutcomeFailed: "failed",
	} {
		if string(status) != want {
			t.Errorf("AttemptOutcome = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[TaskStatus]string{
		TaskStatusPending: "pending", TaskStatusRunning: "running",
		TaskStatusCompleted: "completed", TaskStatusFailed: "failed",
		TaskStatusCancelled: "cancelled",
	} {
		if string(status) != want {
			t.Errorf("TaskStatus = %q, want %q", string(status), want)
		}
	}
}
