// Package hints owns the audience policy for committed Payments hints.
package hints

import (
	"context"

	"ideaboard/api/internal/payments/ports"
)

// BillingAccountAudience reads current financial managers, not the membership
// at checkout time. The event's payer account remains fixed by payment facts.
type BillingAccountAudience interface {
	ListBillingAccountManagerIDs(context.Context, int64) ([]int64, error)
}

type Publisher struct {
	events   ports.PaymentEventPublisher
	audience BillingAccountAudience
}

func NewPublisher(events ports.PaymentEventPublisher, audience BillingAccountAudience) *Publisher {
	return &Publisher{events: events, audience: audience}
}

// Publish resolves recipients at delivery time. Missing or unavailable audience
// falls back only to the operator; it never resolves a different payer.
func (p *Publisher) Publish(ctx context.Context, operatorID int64, event ports.PaymentEvent) {
	if p == nil || p.events == nil {
		return
	}
	if p.audience != nil && event.BillingAccountID > 0 {
		managers, err := p.audience.ListBillingAccountManagerIDs(ctx, event.BillingAccountID)
		if err == nil {
			seen := make(map[int64]struct{}, len(managers))
			for _, managerID := range managers {
				if managerID <= 0 {
					continue
				}
				if _, exists := seen[managerID]; exists {
					continue
				}
				seen[managerID] = struct{}{}
				p.events.Publish(managerID, event)
			}
			if len(seen) > 0 {
				return
			}
		}
	}
	if operatorID > 0 {
		p.events.Publish(operatorID, event)
	}
}

var _ ports.PaymentHintPublisher = (*Publisher)(nil)
