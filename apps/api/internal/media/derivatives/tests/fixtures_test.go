package tests

import (
	"context"

	"ideaboard/api/internal/media/derivatives/domain"
	eventsinfra "ideaboard/api/internal/media/derivatives/infrastructure/events"
	"ideaboard/api/internal/media/derivatives/infrastructure/rendering"
)

type (
	Event              = domain.Event
	TierSpec           = domain.TierSpec
	InputProfile       = domain.InputProfile
	UserEventPublisher = eventsinfra.UserEventPublisher
)

const (
	EventTypeReady           = domain.EventTypeReady
	EventTypeUpdated         = domain.EventTypeUpdated
	EventTypeInvalidated     = domain.EventTypeInvalidated
	EventTypeRepairRequested = domain.EventTypeRepairRequested
	EventTypeRepaired        = domain.EventTypeRepaired
	EventSchemaVersion       = domain.EventSchemaVersion
	TierThumb                = domain.TierThumb
	TierPreview              = domain.TierPreview
	TierProcessingInput      = domain.TierProcessingInput
	ProcessingInputMaxSide   = domain.ProcessingInputMaxSide
	FormatJPEG               = domain.FormatJPEG
	FormatPNG                = domain.FormatPNG
	FormatWebP               = domain.FormatWebP
	ProfileProcessingInput   = domain.ProfileProcessingInput
)

var (
	TierSpecs              = domain.TierSpecs
	ProcessingInputProfile = domain.ProcessingInputProfile
	ErrRepairNotPossible   = domain.ErrRepairNotPossible
)

func render(data []byte, maxSide int) ([]byte, int, int, string, error) {
	return rendering.GoRenderer{}.Render(context.Background(), data, "", maxSide)
}
