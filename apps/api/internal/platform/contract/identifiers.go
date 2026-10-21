package contract

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Opaque prefixed public identifiers (Baseline 5.8).
//
// Dual-key policy (Baseline 5.8, DESIGN 6): tables keep an internal
// numeric key (internal_id bigint) for efficient joins and a unique
// public identifier (public_id text) for all API exposure. Database IDs
// must not appear in URLs, event payloads, generated clients, or public
// logs. New v1 references must use public_id; see #801 for the
// workspace/organization/principal lifecycle that mints them.
const (
	PrefixUser         = "usr_"
	PrefixOrganization = "org_"
	PrefixWorkspace    = "ws_"
	PrefixBoard        = "board_"
	PrefixTask         = "task_"
	PrefixAsset        = "asset_"
	PrefixUpload       = "upload_"
	PrefixEvent        = "evt_"
	// Workspace billing account prefix (#804/#809). It matches the
	// billing_accounts public_id CHECK (ba_%), so the postgres ensure
	// path passes minted ids through opaquely.
	PrefixBillingAccount = "ba_"
	// Workspace library durable prefixes (#809). They match the
	// library_folders / library_assets / library_materializations
	// public_id CHECKs (lfolder_% / lasset_% / lmat_%), so the
	// postgres store passes minted ids through opaquely. The compact
	// libf_/liba_/libm_ shapes from the #806 additive phase are retired.
	PrefixLibraryFolder          = "lfolder_"
	PrefixLibraryAsset           = "lasset_"
	PrefixLibraryMaterialization = "lmat_"
	// Rewards + reconciliation opaque prefixes (parent #809 residue).
	// Reserved for the migration that moves reward codes, reward
	// redemptions, and reconciliation runs off numeric DB ids (see the
	// exception notes on the current numeric parse sites). Mint only
	// after the public_id columns land; until then these prefixes MUST
	// NOT appear on the wire.
	PrefixRewardCode        = "rwd_"
	PrefixRewardRedemption  = "rdm_"
	PrefixReconciliationRun = "rec_"
	// Billing transfer invite prefix (#948). It matches the
	// billing_account_transfer_invites public_id CHECK (bti_%), so the
	// postgres handover path passes minted ids through opaquely.
	// Invites carry the invitation for studio handover (owner initiate,
	// recipient accept/reject, initiator cancel) with idempotent replay.
	PrefixBillingTransferInvite = "bti_"
)

// knownPrefixes lists every kernel prefix for PrefixOf.
var knownPrefixes = []string{
	PrefixUser,
	PrefixOrganization,
	PrefixWorkspace,
	PrefixBoard,
	PrefixTask,
	PrefixAsset,
	PrefixUpload,
	PrefixEvent,
	PrefixBillingAccount,
	PrefixLibraryFolder,
	PrefixLibraryAsset,
	PrefixLibraryMaterialization,
	PrefixRewardCode,
	PrefixRewardRedemption,
	PrefixReconciliationRun,
	PrefixBillingTransferInvite,
}

// IsValidPublicID reports whether id carries prefix with a non-empty
// suffix of [A-Za-z0-9_-]. It validates shape only; existence and
// visibility are enforced by lookup + capability checks (hidden resources
// converge to 404 per Baseline 4).
func IsValidPublicID(prefix, id string) bool {
	if prefix == "" || !strings.HasPrefix(id, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(id, prefix)
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// IsUserID reports whether id is a well-formed usr_ identifier.
func IsUserID(id string) bool { return IsValidPublicID(PrefixUser, id) }

// IsOrganizationID reports whether id is a well-formed org_ identifier.
func IsOrganizationID(id string) bool { return IsValidPublicID(PrefixOrganization, id) }

// IsWorkspaceID reports whether id is a well-formed ws_ identifier.
func IsWorkspaceID(id string) bool { return IsValidPublicID(PrefixWorkspace, id) }

// IsBoardID reports whether id is a well-formed board_ identifier.
func IsBoardID(id string) bool { return IsValidPublicID(PrefixBoard, id) }

// IsTaskID reports whether id is a well-formed task_ identifier.
func IsTaskID(id string) bool { return IsValidPublicID(PrefixTask, id) }

// IsAssetID reports whether id is a well-formed asset_ identifier.
func IsAssetID(id string) bool { return IsValidPublicID(PrefixAsset, id) }

// IsUploadID reports whether id is a well-formed upload_ identifier.
func IsUploadID(id string) bool { return IsValidPublicID(PrefixUpload, id) }

// IsEventID reports whether id is a well-formed evt_ identifier.
func IsEventID(id string) bool { return IsValidPublicID(PrefixEvent, id) }

// IsBillingAccountID reports whether id is a well-formed ba_ identifier.
func IsBillingAccountID(id string) bool { return IsValidPublicID(PrefixBillingAccount, id) }

// IsLibraryAssetID reports whether id is a well-formed lasset_ identifier.
func IsLibraryAssetID(id string) bool { return IsValidPublicID(PrefixLibraryAsset, id) }

// IsRewardCodeID reports whether id is a well-formed rwd_ identifier.
// Reserved for the rewards opaque-id migration; the current numeric
// :id parse sites still resolve bigint ids until the public_id
// columns land (see requireAdminRewardCode).
func IsRewardCodeID(id string) bool { return IsValidPublicID(PrefixRewardCode, id) }

// IsRewardRedemptionID reports whether id is a well-formed rdm_ identifier.
// Reserved for the rewards opaque-id migration (see GetAdminRedemption).
func IsRewardRedemptionID(id string) bool { return IsValidPublicID(PrefixRewardRedemption, id) }

// IsReconciliationRunID reports whether id is a well-formed rec_ identifier.
// Reserved for the reconciliation opaque-id migration (see
// getControlReconciliationRun); the current :run_id stays numeric by
// formal exception until the public_id column lands.
func IsReconciliationRunID(id string) bool { return IsValidPublicID(PrefixReconciliationRun, id) }

// IsBillingTransferInviteID reports whether id is a well-formed bti_ identifier.
func IsBillingTransferInviteID(id string) bool {
	return IsValidPublicID(PrefixBillingTransferInvite, id)
}

// MintPublicID mints an opaque public identifier with the given prefix and
// nbytes of crypto randomness (hex-encoded). It is the single mint behind
// all v1 public ids; domain packages must delegate to it instead of rolling
// their own rand.Read loops.
func MintPublicID(prefix string, nbytes int) (string, error) {
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("contract: mint public id: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}

// PrefixOf reports the known kernel prefix carried by id. It reports
// shape only; use the Is* validators when the suffix must be well-formed.
func PrefixOf(id string) (string, bool) {
	for _, prefix := range knownPrefixes {
		if strings.HasPrefix(id, prefix) {
			return prefix, true
		}
	}
	return "", false
}
