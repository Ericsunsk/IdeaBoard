package authidentity

import "time"

type UserIdentity struct {
	Email     string
	Providers []string
}

// AuthUserState is the Auth-side read model for the Identity admin
// reconciliation job (dry-run alerting only). It carries the fields the
// local users row mirrors (role, banned) plus the ban metadata the local
// schema cannot store (no ban_reason/ban_expires columns): metadata is
// alert-only and never auto-repaired.
type AuthUserState struct {
	AuthUserID   string
	Role         string
	Banned       bool
	BanReason    *string
	BanExpiresAt *time.Time
}
