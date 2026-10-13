// Package adminaction owns the shared invariants for audited Admin writes.
package adminaction

import "errors"

var ErrInvalidActor = errors.New("invalid Admin action actor")

// Metadata identifies the authenticated actor attached to an audited Admin write.
// Its field stays private so business contexts cannot bypass actor validation.
type Metadata struct {
	actorID int64
}

func New(actorID int64) (Metadata, error) {
	if actorID <= 0 {
		return Metadata{}, ErrInvalidActor
	}
	return Metadata{actorID: actorID}, nil
}

func (m Metadata) ActorID() int64 { return m.actorID }

func (m Metadata) IsValid() bool { return m.actorID > 0 }
