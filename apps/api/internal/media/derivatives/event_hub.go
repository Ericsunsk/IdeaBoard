package derivatives

import (
	"encoding/json"

	"ideaboard/api/internal/events"
)

const EventTypeUpdated = "media.derivative.updated"

type Event struct {
	Type         string `json:"type"`
	MediaAssetID int64  `json:"media_asset_id"`
	Tier         string `json:"tier"`
	Format       string `json:"format,omitempty"`
	Status       string `json:"status"`
}

type UserEventPublisher struct {
	userHub *events.UserEventHub
}

func NewUserEventPublisher(userHub *events.UserEventHub) *UserEventPublisher {
	return &UserEventPublisher{userHub: userHub}
}

func (p *UserEventPublisher) Publish(userID int64, event Event) {
	if p == nil || p.userHub == nil {
		return
	}
	if event.Type == "" {
		event.Type = EventTypeUpdated
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	userEvent := events.UserEvent{UserID: userID, Type: events.EventTypeMediaDerivativeUpdated, Data: payload}
	p.userHub.Publish(userEvent)
}
