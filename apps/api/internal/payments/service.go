package payments

import (
	"context"
	"fmt"

	platformevents "ideaboard/api/internal/events"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
)

type Service struct {
	*CheckoutWorkflow
	*StripeFactsWorkflow
	runtime *PaymentRuntime
}

func (s *StripeFactsWorkflow) CreatePaymentRefund(ctx context.Context, command paymentcontracts.CreatePaymentRefundCommand) (paymentsdomain.PaymentRefund, error) {
	if s == nil || s.refundService == nil {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundUnavailable
	}
	return s.refundService.CreatePaymentRefund(ctx, command)
}

type PaymentCheckoutShutdownEnqueuer interface {
	EnqueuePaymentCheckoutShutdown(context.Context, string) error
}

func (s *Service) eventHub() *paymentevents.Hub {
	if s == nil || s.runtime == nil {
		return nil
	}
	hub, _ := s.runtime.events.(*paymentevents.Hub)
	return hub
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
