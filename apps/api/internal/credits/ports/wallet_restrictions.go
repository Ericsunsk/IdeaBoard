package ports

import (
	"context"

	"ideaboard/api/internal/credits/domain"
)

type WalletRestrictionCommands interface {
	RestrictWallet(context.Context, RestrictWalletCommand) (WalletRestrictionChange, error)
	ReleaseWalletRestriction(context.Context, ReleaseWalletRestrictionCommand) (WalletRestrictionChange, error)
}

type RestrictWalletCommand struct {
	UserID        int64
	Type          string
	Source        string
	RelatedObject domain.BusinessReference
	Reason        string
	Actor         domain.Actor
}

type ReleaseWalletRestrictionCommand struct {
	RestrictionID string
	Reason        string
	Actor         domain.Actor
}

type WalletRestrictionChange struct {
	RestrictionID string `json:"restriction_id"`
	Replayed      bool   `json:"replayed"`
}
