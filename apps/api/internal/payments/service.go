package payments

import (
	"fmt"

	platformevents "ideaboard/api/internal/events"
)

type Service struct {
	*CheckoutWorkflow
	*StripeFactsWorkflow
	runtime *PaymentRuntime
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
