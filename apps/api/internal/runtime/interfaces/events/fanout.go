package events

import (
	"context"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/payments"
)

// ConfigureUserEventFanout subscribes to remote distributed events and attaches local payment listeners.
func ConfigureUserEventFanout(
	ctx context.Context,
	bus *events.NATSJetStreamEventBus,
	userHub *events.UserEventHub,
	paymentService *payments.Service,
) error {
	if userHub == nil || paymentService == nil {
		return fmt.Errorf("user event fanout requires user hub and payment service")
	}
	if err := paymentService.AttachUserEventHub(userHub); err != nil {
		return err
	}
	if bus != nil {
		userHub.SetRemotePublisher(func(event events.UserEvent) {
			publishCtx, cancel := context.WithTimeout(context.Background(), events.DefaultPublishTimeout)
			defer cancel()
			if err := bus.PublishUserEvent(publishCtx, event); err != nil {
				slog.Warn("publish user event failed", "type", event.Type, "user_id", event.UserID, "err", err)
			}
		})
	}
	if bus == nil {
		return nil
	}
	return bus.SubscribeUserEvents(ctx, func(_ context.Context, event events.UserEvent) error {
		userHub.PublishRemote(event)
		return nil
	})
}
