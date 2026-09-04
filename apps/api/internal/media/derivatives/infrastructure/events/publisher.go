package events

import (
	"encoding/json"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
)

var _ ports.EventPublisher = (*UserEventPublisher)(nil)

type UserEventPublisher struct {
	userHub *events.UserEventHub
}

func NewUserEventPublisher(userHub *events.UserEventHub) *UserEventPublisher {
	return &UserEventPublisher{userHub: userHub}
}

func (p *UserEventPublisher) Publish(userID int64, event domain.Event) {
	if p == nil || p.userHub == nil {
		return
	}
	if event.Type == "" {
		event.Type = domain.EventTypeUpdated
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = domain.EventSchemaVersion
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	userEvent := events.UserEvent{UserID: userID, Type: events.EventTypeMediaDerivativeUpdated, Data: payload}
	p.userHub.Publish(userEvent)
}
