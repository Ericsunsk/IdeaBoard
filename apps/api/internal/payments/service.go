package payments

import (
	"context"
	"fmt"

	platformevents "ideaboard/api/internal/events"
	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentwebhook "ideaboard/api/internal/payments/application/webhook"
)

// The five application services share the *Service type name, so each
// embedded field uses a distinct alias. Promoted method names do not collide:
// checkout owns the user top-up boundary, readmodel owns admin/history reads,
// reconciliation owns the River recovery sweep, refund owns refund intake and
// convergence, and webhook owns signed intake plus dispatch.
type checkoutApplicationService = paymentcheckout.Service
type paymentReadModelService = paymentreadmodel.Service
type paymentReconciliationService = paymentreconciliation.Service
type paymentRefundService = paymentrefund.Service
type paymentWebhookService = paymentwebhook.Service

// Service is the only supported Payments composition root. It directly embeds
// the five application services; Stripe configuration stays a named field so
// its summary-only boundary never promotes credential material.
type Service struct {
	*checkoutApplicationService
	*paymentReadModelService
	*paymentReconciliationService
	*paymentRefundService
	*paymentWebhookService
	configurationService *paymentconfiguration.Service
	runtime              *PaymentRuntime
}

// GetStripeConfiguration delegates to the configuration application service;
// the composition root remains the boundary consumed by HTTP adapters.
func (s *Service) GetStripeConfiguration(ctx context.Context) (paymentcontracts.StripeConfigurationSummary, error) {
	if s == nil || s.configurationService == nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	return s.configurationService.GetStripeConfiguration(ctx)
}

func (s *Service) ConfigureStripe(ctx context.Context, command paymentcontracts.ConfigureStripeCommand) (paymentcontracts.StripeConfigurationSummary, error) {
	if s == nil || s.configurationService == nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	return s.configurationService.ConfigureStripe(ctx, command)
}

// AttachUserEventHub configures the composition-owned user event transport.
func (s *Service) AttachUserEventHub(userHub *platformevents.UserEventHub) error {
	if s == nil || s.runtime == nil || s.runtime.events == nil {
		return fmt.Errorf("payments event publisher is unavailable")
	}
	if userHub == nil {
		return fmt.Errorf("payments user event hub is unavailable")
	}
	attachable, ok := s.runtime.events.(interface {
		SetUserEventHub(*platformevents.UserEventHub)
	})
	if !ok {
		return fmt.Errorf("payments event publisher does not support user event fanout")
	}
	attachable.SetUserEventHub(userHub)
	return nil
}
