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
