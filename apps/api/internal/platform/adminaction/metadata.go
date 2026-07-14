// Package adminaction owns the shared invariants for audited Admin writes.
package adminaction

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const MaxReasonRunes = 1000

var (
	ErrInvalidActor  = errors.New("invalid Admin action actor")
	ErrInvalidReason = errors.New("invalid Admin action reason")
)

// Metadata is the validated actor and reason attached to an audited Admin write.
// Its fields stay private so business contexts cannot bypass the shared invariants.
type Metadata struct {
	actorID int64
	reason  string
}

func New(actorID int64, reason string) (Metadata, error) {
	if actorID <= 0 {
		return Metadata{}, ErrInvalidActor
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > MaxReasonRunes {
		return Metadata{}, ErrInvalidReason
	}
	return Metadata{actorID: actorID, reason: reason}, nil
}

func (m Metadata) ActorID() int64 { return m.actorID }

func (m Metadata) Reason() string { return m.reason }

func (m Metadata) IsValid() bool {
	return m.actorID > 0 && m.reason != "" && utf8.RuneCountInString(m.reason) <= MaxReasonRunes
}
