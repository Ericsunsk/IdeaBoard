package workspaceuploads

import (
	"encoding/json"
	"strings"
	"time"
)

// assetMetadata is the workspace binding envelope stored in
// media_assets.metadata (no new columns; see doc.go). Unknown keys from
// other writers are preserved on read-modify-write by the postgres
// sibling, which merges rather than replaces.
type assetMetadata struct {
	WorkspaceID string `json:"workspace_id"`
	ArchivedAt  string `json:"archived_at,omitempty"`
}

// EncodeAssetMetadata renders the workspace binding for new asset rows.
func EncodeAssetMetadata(workspaceID string) []byte {
	raw, err := json.Marshal(assetMetadata{WorkspaceID: strings.TrimSpace(workspaceID)})
	if err != nil {
		return []byte(`{}`)
	}
	return raw
}

// DecodeAssetWorkspace extracts the workspace binding. Empty means the
// row predates workspace uploads (legacy user-owned asset): callers
// treat it as invisible in the workspace scope (ErrNotFound), never as
// a match.
func DecodeAssetWorkspace(metadata []byte) string {
	var decoded assetMetadata
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return ""
	}
	return strings.TrimSpace(decoded.WorkspaceID)
}

// DecodeAssetArchivedAt extracts the archive lifecycle marker.
func DecodeAssetArchivedAt(metadata []byte) *time.Time {
	var decoded assetMetadata
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(decoded.ArchivedAt)
	if trimmed == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil
	}
	moment := parsed.UTC()
	return &moment
}

// FormatArchivedAt renders the archive marker (RFC3339 UTC).
func FormatArchivedAt(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
