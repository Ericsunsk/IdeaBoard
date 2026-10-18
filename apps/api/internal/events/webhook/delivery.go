package webhook

// Webhook delivery: single-attempt execution, retry accounting, and
// dead-letter parking (issue #807).
//
// Retry/backoff: one webhook_deliver River job per delivery with
// MaxAttempts (8); WebhookRetryDelay keeps the webhookBackoffBase
// doubling curve capped at webhookBackoffCap. Attempt history and dead-letter state are persisted by
// the injected WebhookPersistence adapter; the in-memory projection is
// only a cache for the HTTP surface. Durable writes run outside the
// mutex: the attempt fact is committed to the projection under lock,
// copied out, then persisted unlocked.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ideaboard/api/internal/events"
)

// WebhookRetryDelay returns the backoff before attempt (1-based):
// webhookBackoffBase * 2^(attempt-1) capped at webhookBackoffCap.
// Attempt 1 needs no delay.
func WebhookRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	delay := webhookBackoffBase
	for i := 2; i < attempt; i++ {
		delay *= 2
		if delay >= webhookBackoffCap {
			return webhookBackoffCap
		}
	}
	if delay > webhookBackoffCap {
		return webhookBackoffCap
	}
	return delay
}

// executeDeliveryAttempt performs ONE signed delivery attempt for the
// River worker. Nil means delivered or dropped without retry
// (unknown/deleted subscription, dead-lettered subscription, or a
// delivery already marked Delivered). A returned error asks River to
// retry; on budget exhaustion the live DLQ entry is parked first so
// the discarded job row and the read surface agree.
func (s *WebhookStore) executeDeliveryAttempt(ctx context.Context, subscriptionID string, envelope events.V1Envelope, attempt, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = webhookMaxAttempts
	}
	if attempt < 1 {
		attempt = 1
	}
	s.mu.Lock()
	sub, ok := s.subs[subscriptionID]
	persistence := s.persistence
	s.mu.Unlock()
	if !ok && persistence != nil {
		loaded, loadErr := persistence.LoadSubscription(ctx, subscriptionID)
		if loadErr != nil {
			return fmt.Errorf("events: load webhook subscription %s: %w", subscriptionID, loadErr)
		}
		if loaded != nil {
			s.mu.Lock()
			s.subs[subscriptionID] = loaded
			s.byWorkspace[loaded.WorkspaceID] = appendIfMissing(s.byWorkspace[loaded.WorkspaceID], subscriptionID)
			sub, ok = loaded, true
			s.mu.Unlock()
		}
	}
	s.mu.Lock()
	if !ok || sub.Status == WebhookStatusDeadLetter || sub.WorkspaceID != envelope.WorkspaceID {
		s.mu.Unlock()
		return nil
	}
	if _, dup := sub.Delivered[envelope.ID]; dup {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	// Pending protected deliveries cannot leave through a Workspace webhook.
	// Mark the drop terminally so durable reconciliation does not retry it.
	if !workspaceWebhookVisible(envelope) {
		s.mu.Lock()
		if live, stillThere := s.subs[subscriptionID]; stillThere {
			if live.Delivered == nil {
				live.Delivered = map[string]struct{}{}
			}
			live.Delivered[envelope.ID] = struct{}{}
		}
		s.mu.Unlock()
		if persistence != nil {
			if err := persistence.MarkDelivered(ctx, subscriptionID, envelope); err != nil {
				return fmt.Errorf("events: persist dropped webhook: %w", err)
			}
		}
		return nil
	}
	s.mu.Lock()
	body, err := json.Marshal(envelope)
	s.mu.Unlock()
	if err != nil {
		if recordErr := s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, 0, AttemptOutcomeFailed, err.Error()); recordErr != nil {
			return recordErr
		}
		return s.deliveryFailure(subscriptionID, envelope, attempt, maxAttempts, err.Error())
	}
	s.mu.Lock()
	secret := append([]byte(nil), sub.Secret...)
	destination := sub.URL
	s.mu.Unlock()
	headers := SignWebhook(secret, body, envelope.ID, s.clock())
	sender := s.sender
	if sender == nil {
		if recordErr := s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, 0, AttemptOutcomeFailed, "no sender configured"); recordErr != nil {
			return recordErr
		}
		return s.deliveryFailure(subscriptionID, envelope, attempt, maxAttempts, "no sender configured")
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), webhookSendTimeout)
	status, sendErr := sender.Send(sendCtx, destination, headers, body)
	cancel()
	if sendErr == nil && status >= 200 && status < 300 {
		if err := s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, status, AttemptOutcomeDelivered, ""); err != nil {
			return err
		}
		if persistence != nil {
			if err := persistence.MarkDelivered(ctx, subscriptionID, envelope); err != nil {
				return fmt.Errorf("events: persist delivered webhook: %w", err)
			}
		}
		s.mu.Lock()
		if current, ok := s.subs[subscriptionID]; ok {
			current.Delivered[envelope.ID] = struct{}{}
			if current.Status == WebhookStatusDegraded && failureCount(current) == 0 {
				current.Status = WebhookStatusActive
			}
			current.UpdatedAt = s.clock()
		}
		s.mu.Unlock()
		return nil
	}
	detail := ""
	if sendErr != nil {
		detail = sendErr.Error()
	} else {
		detail = fmt.Sprintf("unexpected status %d", status)
	}
	if err := s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, status, AttemptOutcomeFailed, detail); err != nil {
		return err
	}
	return s.deliveryFailure(subscriptionID, envelope, attempt, maxAttempts, detail)
}

// deliveryFailure parks the live DLQ entry once the budget is
// exhausted and always returns the failure for River (retry, then
// discard on the final attempt).
func (s *WebhookStore) deliveryFailure(subscriptionID string, envelope events.V1Envelope, attempt, maxAttempts int, detail string) error {
	if attempt >= maxAttempts {
		s.parkDeadLetterOnce(subscriptionID, envelope, attempt, detail)
	}
	return fmt.Errorf("events: webhook delivery %s attempt %d/%d failed: %s",
		envelope.ID, attempt, maxAttempts, detail)
}

// parkDeadLetterOnce moves the subscription to dead_letter and parks
// the envelope exactly once per delivery id, so worker redelivery
// after a restart never double-parks.
func (s *WebhookStore) parkDeadLetterOnce(subscriptionID string, envelope events.V1Envelope, attempts int, lastError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[subscriptionID]
	if !ok {
		return
	}
	for _, parked := range sub.DeadLetter {
		if parked.DeliveryID == envelope.ID {
			sub.Status = WebhookStatusDeadLetter
			sub.UpdatedAt = s.clock()
			return
		}
	}
	sub.Status = WebhookStatusDeadLetter
	sub.DeadLetter = append(sub.DeadLetter, WebhookDeadLetter{
		DeliveryID: envelope.ID, EventType: envelope.Type,
		Attempts: attempts, LastError: lastError, At: s.clock(),
	})
	for len(sub.DeadLetter) > webhookDeadLetterCap {
		sub.DeadLetter = append([]WebhookDeadLetter(nil), sub.DeadLetter[1:]...)
	}
	sub.UpdatedAt = s.clock()
}

func (s *WebhookStore) recordAttempt(ctx context.Context, subscriptionID string, envelope events.V1Envelope, attempt, maxAttempts, status int, outcome AttemptOutcome, detail string) error {
	s.mu.Lock()
	sub, ok := s.subs[subscriptionID]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	fact := WebhookAttempt{
		DeliveryID: envelope.ID, Attempt: attempt, StatusCode: status,
		Outcome: outcome, Error: detail, At: s.clock(),
	}
	sub.Attempts = append(sub.Attempts, fact)
	for len(sub.Attempts) > webhookAttemptLogCap {
		sub.Attempts = append([]WebhookAttempt(nil), sub.Attempts[1:]...)
	}
	if outcome != AttemptOutcomeDelivered && sub.Status == WebhookStatusActive {
		sub.Status = WebhookStatusDegraded
	}
	sub.UpdatedAt = s.clock()
	persistence := s.persistence
	exhausted := attempt >= maxAttempts
	s.mu.Unlock()
	if persistence != nil {
		if err := persistence.RecordAttempt(ctx, subscriptionID, envelope, fact, exhausted); err != nil {
			return fmt.Errorf("events: persist webhook attempt: %w", err)
		}
	}
	return nil
}
