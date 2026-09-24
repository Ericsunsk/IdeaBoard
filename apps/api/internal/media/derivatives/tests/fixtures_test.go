package tests

import (
	"ideaboard/api/internal/media/derivatives/domain"
)

type (
	Event        = domain.Event
	TierSpec     = domain.TierSpec
	InputProfile = domain.InputProfile
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
