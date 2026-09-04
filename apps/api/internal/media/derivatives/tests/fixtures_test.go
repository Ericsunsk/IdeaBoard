package tests

import (
	"context"

	"ideaboard/api/internal/media/derivatives/application/reconciliation"
	"ideaboard/api/internal/media/derivatives/application/repair"
	"ideaboard/api/internal/media/derivatives/domain"
	eventsinfra "ideaboard/api/internal/media/derivatives/infrastructure/events"
	"ideaboard/api/internal/media/derivatives/infrastructure/rendering"
	"ideaboard/api/internal/media/derivatives/infrastructure/worker"
)

type (
	Event                      = domain.Event
	TierSpec                   = domain.TierSpec
	Result                     = reconciliation.Result
	ReconcilerConfig           = reconciliation.ReconcilerConfig
	RepairConfig               = repair.RepairConfig
	Repair                     = repair.Repair
	InputProfile               = domain.InputProfile
	DerivativeOrphanFinding    = reconciliation.DerivativeOrphanFinding
	DerivativeOrphanScanResult = reconciliation.DerivativeOrphanScanResult
	UserEventPublisher         = eventsinfra.UserEventPublisher
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
	TierProcessing           = domain.TierProcessing
	TierProcessingInput      = domain.TierProcessingInput
	FormatJPEG               = domain.FormatJPEG
	FormatPNG                = domain.FormatPNG
	FormatWebP               = domain.FormatWebP
	QueueName                = worker.QueueName
	ProfileProcessingInput   = domain.ProfileProcessingInput
)

var (
	TierSpecs                     = domain.TierSpecs
	ProcessingInputProfile        = domain.ProcessingInputProfile
	ErrRepairNotPossible          = domain.ErrRepairNotPossible
	ScanDerivativeOrphanObjects   = reconciliation.ScanDerivativeOrphanObjects
	OutcomeReady                  = reconciliation.OutcomeReady
	OutcomeMissing                = reconciliation.OutcomeMissing
	OutcomeInvalid                = reconciliation.OutcomeInvalid
	DefaultReconcilerProbeTimeout = reconciliation.DefaultReconcilerProbeTimeout
	DefaultReconcilerBatchSize    = reconciliation.DefaultReconcilerBatchSize
	DefaultReconcilerLease        = reconciliation.DefaultReconcilerLease
	DefaultRepairMaxAttempts      = repair.DefaultRepairMaxAttempts
	DefaultRepairLease            = repair.DefaultRepairLease
)

func render(data []byte, maxSide int) ([]byte, int, int, string, error) {
	return rendering.GoRenderer{}.Render(context.Background(), data, "", maxSide)
}
