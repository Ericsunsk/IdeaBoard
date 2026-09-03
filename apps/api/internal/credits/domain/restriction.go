package domain

import (
	"time"

	"github.com/google/uuid"
)

type WalletRestriction struct {
	RestrictionID string            `json:"restriction_id"`
	UserID        int64             `json:"-"`
	PublicID      uuid.UUID         `json:"public_id"`
	DisplayName   string            `json:"display_name"`
	Type          string            `json:"type"`
	Source        string            `json:"source"`
	RelatedObject BusinessReference `json:"related_object"`
	Reason        string            `json:"reason"`
	CreatedBy     Actor             `json:"created_by"`
	CreatedAt     time.Time         `json:"created_at"`
}
