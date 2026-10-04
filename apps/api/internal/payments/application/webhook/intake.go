// Package webhook intake owns the Stripe webhook front door: raw-body
// signature verification, event validation, and idempotent inbox receipt.
//
// Intake never performs order settlement, refund convergence, or dispute
// handling. Recognized events are handed to dispatch.go after the inbox row
// is durable; unknown-but-signed events are recorded as ignored and
// acknowledged with no business state change.
package webhook

import (
	"context"
	"fmt"
	"strings"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/payments/application/credentials"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/database"
)

// ReceiveStripeWebhook verifies the raw-body signature against every retained
// credential generation, validates the event envelope, and writes the minimal
// inbox receipt. Settlement stays in dispatch: this method only calls
// processEvent after the receipt is durable.
func (s *Service) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (paymentports.StripeWebhookReceipt, error) {
	if s == nil || s.webhookCredentials == nil || s.inbox == nil || s.verifier == nil {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	if len(raw) == 0 || strings.TrimSpace(signature) == "" {
		return paymentports.StripeWebhookReceipt{}, paymentports.ErrMalformedWebhookPayload
	}
	resolvedCredentials, err := s.webhookCredentials(ctx)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("load Stripe webhook credentials: %w", err)
	}
	if len(resolvedCredentials) == 0 {
		return paymentports.StripeWebhookReceipt{}, paymentcontracts.ErrStripeConfigurationNotFound
	}
	now := s.now()
	stored, event, err := s.verifyWebhook(ctx, raw, signature, resolvedCredentials, now)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	if err := paymentports.ValidateStripeWebhookEvent(event); err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	existing := ""
	if current, err := s.inbox.GetStripeWebhookEvent(ctx, event.ID); err == nil {
		existing = current
		if existing == string(WebhookProcessingProcessed) || existing == string(WebhookProcessingIgnored) {
			return paymentports.StripeWebhookReceipt{EventID: event.ID, Duplicate: true, Ignored: existing == string(WebhookProcessingIgnored)}, nil
		}
	} else if !database.IsNotFound(err) {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("load Stripe webhook inbox state: %w", err)
	}
	if event.Livemode != (stored.Mode == paymentports.StripeModeLive) {
		return paymentports.StripeWebhookReceipt{}, paymentports.ErrMalformedWebhookPayload
	}
	recognized := paymentports.IsRecognizedStripeWebhookEvent(event.Type)
	result := string(WebhookProcessingReceived)
	var processedAt *time.Time
	if !recognized {
		result, processedAt = string(WebhookProcessingIgnored), &now
	}
	duplicate, err := s.inbox.InsertStripeWebhookEvent(ctx, event, result, processedAt)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	receipt := paymentports.StripeWebhookReceipt{EventID: event.ID, Duplicate: duplicate, Ignored: !recognized}
	if !recognized {
		return receipt, nil
	}
	claimed, err := s.inbox.ClaimStripeWebhookEvent(ctx, event.ID)
	if err != nil {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("claim Stripe webhook event: %w", err)
	}
	if !claimed {
		return paymentports.StripeWebhookReceipt{EventID: event.ID, Duplicate: true}, nil
	}
	if err := s.processEvent(ctx, event, stored, now); err != nil {
		return paymentports.StripeWebhookReceipt{}, err
	}
	return receipt, nil
}

// verifyWebhook probes every retained generation and returns the first whose
// webhook secret validates the signature. Secret recovery funnels through
// the credentials boundary; a corrupt generation fails closed as invalid
// configuration before any business parsing.
func (s *Service) verifyWebhook(ctx context.Context, raw []byte, signature string, resolvedCredentials []credentials.WebhookCredential, now time.Time) (paymentconfiguration.StoredStripeConfiguration, paymentports.StripeWebhookEvent, error) {
	var lastErr error
	for _, credential := range resolvedCredentials {
		event, verifyErr := s.verifier.Verify(ctx, raw, signature, credential.WebhookSecret, paymentports.StripeWebhookTolerance, now)
		if verifyErr == nil {
			return credential.Stored, event, nil
		}
		lastErr = verifyErr
	}
	return paymentconfiguration.StoredStripeConfiguration{}, paymentports.StripeWebhookEvent{}, lastErr
}
