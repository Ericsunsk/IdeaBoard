package derivatives

import (
	"encoding/json"

	"ideaboard/api/internal/events"
)

// Versioned media.derivative.* event types. The wire format is fixed at
// the outer UserEvent level (the events package owns the
// `media.derivative.updated` envelope); the inner Type field carries the
// versioned event identity, so subscribers decode the data payload and
// branch on data.type. The constants below are the single source of
// truth for both publishers and runbook references.
const (
	// EventTypeUpdated is the legacy event type. New subscribers
	// should branch on the inner Type field of the payload; this
	// constant remains so existing decoders do not break.
	EventTypeUpdated = "media.derivative.updated"

	// EventTypeReady signals a derivative completed its write +
	// integrity verification and is now usable by callers. The
	// publisher MUST emit this only after the DB transaction that
	// flipped status to ready has committed.
	EventTypeReady = "media.derivative.ready"

	// EventTypeInvalidated signals a previously-ready derivative
	// was found to be missing or invalid by the reconciler. The
	// publisher MUST emit this only after the DB transaction that
	// flipped status to missing/invalid has committed.
	EventTypeInvalidated = "media.derivative.invalidated"

	// EventTypeRepairRequested signals a bounded repair was
	// scheduled. The repair River job is the durable retry target;
	// this event is for observability only and MUST NOT be used
	// as a work trigger.
	EventTypeRepairRequested = "media.derivative.repair_requested"

	// EventTypeRepaired signals a previously-invalid derivative
	// was successfully rebuilt. The publisher MUST emit this only
	// after the new ready status has committed.
	EventTypeRepaired = "media.derivative.repaired"
)

// EventSchemaVersion is the version of the Event struct. Bump it when
// adding or removing fields; the publisher stamps it on every payload
// so subscribers can detect format drift.
const EventSchemaVersion = 2

type Event struct {
	SchemaVersion int    `json:"schema_version"`
	Type          string `json:"type"`
	MediaAssetID  int64  `json:"media_asset_id"`
	DerivativeID  string `json:"derivative_id,omitempty"`
	Tier          string `json:"tier"`
	Format        string `json:"format,omitempty"`
	Status        string `json:"status"`

	// Correlation facts. The presence of TaskID/ProviderAttempt/
	// CorrelationID lets the runbook pivot from a media event to
	// the originating task. The publisher fills these from the
	// Facts envelope the caller supplies.
	TaskID          string `json:"task_id,omitempty"`
	ProviderAttempt string `json:"provider_attempt_id,omitempty"`
	CorrelationID   string `json:"correlation_id,omitempty"`
	StorageProfile  string `json:"storage_profile_id,omitempty"`
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
	if event.SchemaVersion == 0 {
		event.SchemaVersion = EventSchemaVersion
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	userEvent := events.UserEvent{UserID: userID, Type: events.EventTypeMediaDerivativeUpdated, Data: payload}
	p.userHub.Publish(userEvent)
}
