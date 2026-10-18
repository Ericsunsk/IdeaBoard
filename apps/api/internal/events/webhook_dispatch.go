package events

// Webhook dispatch: fan-out over matching subscriptions (issue #807).
//
// Dispatch enqueues one webhook_deliver job per matching subscription
// of the workspace. Per-subscription type filtering and duplicate
// envelope collapse (dedup) happen here; River owns every attempt
// (including the first) and its retries. ReconcilePending re-enqueues
// durable deliveries left behind by an enqueue failure or a process
// crash between commit and River insertion.

import (
	"context"
	"fmt"
	"log/slog"
)

// Dispatch enqueues one webhook_deliver job per matching subscription
// of the workspace. Per-subscription type filtering and duplicate
// envelope collapse (dedup) happen here; River owns every attempt
// (including the first) and its retries. Enqueue never fails the
// caller: failures are logged and dropped, mirroring the NATS publish
// seam (emission follows the domain commit).
func (s *WebhookStore) Dispatch(ctx context.Context, envelope V1Envelope) {
	if err := s.DispatchErr(ctx, envelope); err != nil {
		slog.Warn("webhook dispatch failed", "delivery_id", envelope.ID, "err", err)
	}
}

// DispatchErr is the durable outbox-aware variant. It returns the first
// persistence or enqueue failure so an upstream event outbox can retain the
// transition for replay. All matching subscriptions are still attempted.
func (s *WebhookStore) DispatchErr(ctx context.Context, envelope V1Envelope) error {
	if s == nil {
		return nil
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		return err
	}
	s.mu.Lock()
	targets := []string{}
	for _, id := range s.byWorkspace[envelope.WorkspaceID] {
		sub, ok := s.subs[id]
		if !ok || sub.Status == WebhookStatusDeadLetter {
			continue
		}
		if !sub.Matches(envelope.Type) {
			continue
		}
		if _, dup := sub.Delivered[envelope.ID]; dup {
			continue
		}
		targets = append(targets, sub.ID)
	}
	enqueuer := s.enqueuer
	persistence := s.persistence
	s.mu.Unlock()
	// Only events with a Workspace-wide audience can leave through webhooks.
	if !workspaceWebhookVisible(envelope) {
		return nil
	}
	var firstErr error
	for _, subscriptionID := range targets {
		if persistence != nil {
			if err := persistence.EnsureDelivery(ctx, envelope, subscriptionID); err != nil {
				slog.Warn("webhook durable delivery creation failed", "subscription", subscriptionID, "delivery_id", envelope.ID, "err", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		}
		if enqueuer == nil {
			slog.Warn("webhook dispatch has no enqueuer; durable delivery remains pending", "delivery_id", envelope.ID, "subscription", subscriptionID)
			if firstErr == nil {
				firstErr = fmt.Errorf("webhook dispatch enqueuer unavailable")
			}
			continue
		}
		args := WebhookDeliveryArgs{
			SubscriptionID: subscriptionID, DeliveryID: envelope.ID, Envelope: envelope,
		}
		if _, err := enqueuer.Insert(ctx, args, WebhookDeliveryInsertOpts(args)); err != nil {
			slog.Warn("webhook enqueue failed",
				"subscription", subscriptionID, "delivery_id", envelope.ID, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ReconcilePending re-enqueues durable deliveries left behind by an enqueue
// failure or a process crash between commit and River insertion.
func (s *WebhookStore) ReconcilePending(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	persistence, enqueuer := s.persistence, s.enqueuer
	s.mu.Unlock()
	if persistence == nil || enqueuer == nil {
		return nil
	}
	deliveries, err := persistence.PendingDeliveries(ctx)
	if err != nil {
		return err
	}
	for _, args := range deliveries {
		if _, err := enqueuer.Insert(ctx, args, WebhookDeliveryInsertOpts(args)); err != nil {
			slog.Warn("webhook pending reconciliation enqueue failed", "subscription", args.SubscriptionID, "delivery_id", args.DeliveryID, "err", err)
		}
	}
	return nil
}
