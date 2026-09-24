package tenancy

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"ideaboard/api/internal/platform/contract"
)

// Public identifier prefixes. These alias the contract kernel so the two
// can never drift; the middleware seam validates authoritatively via
// contract.IsWorkspaceID etc.
const (
	UserPrefix         = contract.PrefixUser
	OrganizationPrefix = contract.PrefixOrganization
	WorkspacePrefix    = contract.PrefixWorkspace
	// BillingAccountPrefix aliases the contract kernel so the two can
	// never drift; billing_accounts.public_id carries ba_ per #804.
	BillingAccountPrefix = contract.PrefixBillingAccount
)

// FormatUserPublicID renders the external user identifier. The users table
// keeps its UUID internal key; only this translator exposes usr_<hex> and
// it must stay the single place that does so (do NOT change users table).
func FormatUserPublicID(id uuid.UUID) string {
	return UserPrefix + strings.ReplaceAll(id.String(), "-", "")
}

// ParseUserPublicID parses an external usr_<hex> identifier back to UUID.
// It accepts dashed or undashed hex after the prefix.
func ParseUserPublicID(external string) (uuid.UUID, error) {
	if !strings.HasPrefix(external, UserPrefix) {
		return uuid.Nil, fmt.Errorf("tenancy: not a user public id: %q", external)
	}
	raw := strings.TrimPrefix(external, UserPrefix)
	raw = strings.ReplaceAll(raw, "-", "")
	if len(raw) != 32 {
		return uuid.Nil, fmt.Errorf("tenancy: malformed user public id: %q", external)
	}
	dashed := raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
	id, err := uuid.Parse(dashed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenancy: malformed user public id: %q", external)
	}
	return id, nil
}

// NewWorkspacePublicID mints an opaque ws_ public identifier.
func NewWorkspacePublicID() (string, error) {
	return contract.MintPublicID(WorkspacePrefix, 12)
}

// NewOrganizationPublicID mints an opaque org_ public identifier.
func NewOrganizationPublicID() (string, error) {
	return contract.MintPublicID(OrganizationPrefix, 12)
}

// NewBillingAccountPublicID mints an opaque ba_ public identifier for the
// workspace-owned billing account (#804).
func NewBillingAccountPublicID() (string, error) {
	return contract.MintPublicID(BillingAccountPrefix, 12)
}
