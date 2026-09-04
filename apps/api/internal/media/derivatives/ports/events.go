package ports

import "ideaboard/api/internal/media/derivatives/domain"

type EventPublisher interface {
	Publish(userID int64, event domain.Event)
}
