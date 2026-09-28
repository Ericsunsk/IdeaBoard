package domain

type WalletStatus string

const (
	WalletStatusActive WalletStatus = "active"
	WalletStatusFrozen WalletStatus = "frozen"
)

// allowedWalletTransitions mirrors the durable guards in
// set_credit_wallet_status
// (internal/db/schema/fragments/credits.sql): wallets move freely
// between active and frozen (freeze on restriction/anomaly, unfreeze
// on release). The type only documents the allowed transitions at
// compile time (see CanWalletTransition); SQL literals stay
// byte-identical.
var allowedWalletTransitions = map[WalletStatus]map[WalletStatus]struct{}{
	WalletStatusActive: {
		WalletStatusFrozen: {},
	},
	WalletStatusFrozen: {
		WalletStatusActive: {},
	},
}

// CanWalletTransition reports whether a from -> to flip is durably
// allowed. It is documentation, not a gate: every transition it
// reports true for was already possible through the SQL guards, and no
// previously possible transition reports false.
func CanWalletTransition(from, to WalletStatus) bool {
	nexts, ok := allowedWalletTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

// WalletDetails is the current administrative read model. CommandResult keeps
// WalletBalance only, preserving its transaction-time balance snapshot.
type WalletDetails struct {
	WalletBalance
	Status             WalletStatus        `json:"status"`
	ActiveRestrictions []WalletRestriction `json:"active_restrictions"`
}
