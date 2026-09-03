package domain

type WalletStatus string

const (
	WalletStatusActive WalletStatus = "active"
	WalletStatusFrozen WalletStatus = "frozen"
)

// WalletDetails is the current administrative read model. CommandResult keeps
// WalletBalance only, preserving its transaction-time balance snapshot.
type WalletDetails struct {
	WalletBalance
	Status             WalletStatus        `json:"status"`
	ActiveRestrictions []WalletRestriction `json:"active_restrictions"`
}
