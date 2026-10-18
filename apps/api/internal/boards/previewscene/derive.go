// Derivation of the persisted preview projection inputs.
//
// content_version and the scene projection are
// computed from the accepted persisted document bytes in the same
// transaction that accepts the document. This file owns the single pure
// rule both stores (postgres, in-memory), the backfill path, and the
// read-derivation fallback share:
//
//   - content_version is the lowercase hex SHA-256 of the CANONICAL form
//     of the accepted document: parsed once with number literals
//     preserved, then re-marshaled with object keys sorted. Canonical
//     (not wire) bytes are hashed because PostgreSQL JSONB does not
//     preserve input whitespace or key order: hashing wire bytes would
//     give a different version at accept time (client bytes) than on
//     read-back or backfill (normalized bytes) for the same document.
//     Same logical document always shares a version, so a no-op re-save
//     never forces a Gallery re-render; renames and archive transitions
//     never touch board_documents and therefore never change the version.
//   - The scene is derived only when the envelope carries the extraction
//     schema (SnapshotSchemaVersion). Any other schema (fresh empty
//     documents, future schemas) yields a nil scene without failing the
//     document write: the projection is a pure function of extractable
//     content, never a second source of truth, and document acceptance
//     never depends on preview derivation.
//   - Extraction input is rebuilt by splicing the verbatim payload object
//     under a "schemaVersion" key. Payload bytes are never re-marshaled,
//     so numeric identities cannot drift through float64 round-trips.
//
// Canonical contract: contract.md §7 (content version) and §10
// (determinism and failure posture).
package previewscene

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// envelopeHeader decodes only the stored envelope framing. Payload stays
// opaque here; snapshotForExtraction splices its raw bytes.
type envelopeHeader struct {
	SchemaVersion int32           `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

// canonicalJSON renders the canonical form of a JSON document: object
// keys sorted at every level (encoding/json marshals string-keyed maps
// in sorted order), array order and number literals preserved verbatim
// (json.Number never passes through float64). Same logical document
// always renders identical bytes regardless of input whitespace or key
// order, so it is the stable hash input across client bytes, stored
// bytes, and JSONB-normalized read-backs.
func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("previewscene: trailing payload")
	}
	return json.Marshal(value)
}

// ContentVersionOf returns the opaque content identity for accepted
// document bytes: lowercase hex SHA-256 over the canonical form.
// Unparseable or empty input yields an empty version (callers treat it
// as not derivable).
func ContentVersionOf(stored []byte) string {
	canonical, err := canonicalJSON(stored)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// snapshotForExtraction rebuilds the BoardSnapshot-shaped extraction input
// ({"schemaVersion":N,...payload fields...}) from stored envelope bytes.
// It reports false for any non-extractable input: malformed envelopes,
// non-object payloads, or schemas outside SnapshotSchemaVersion. It never
// guesses, migrates, or fills defaults.
func snapshotForExtraction(stored []byte) ([]byte, bool) {
	var header envelopeHeader
	if err := json.Unmarshal(stored, &header); err != nil {
		return nil, false
	}
	if header.SchemaVersion != SnapshotSchemaVersion {
		return nil, false
	}
	inner := bytes.TrimSpace(header.Payload)
	if len(inner) < 2 || inner[0] != '{' || inner[len(inner)-1] != '}' {
		return nil, false
	}
	out := make([]byte, 0, len(stored)+32)
	out = append(out, `{"schemaVersion":`...)
	out = append(out, fmt.Sprintf("%d", SnapshotSchemaVersion)...)
	if len(inner) > 2 {
		out = append(out, ',')
		out = append(out, inner[1:len(inner)-1]...)
	}
	out = append(out, '}')
	return out, true
}

// CatalogLimits bounds each persisted Gallery scene independently of document size.
// Truncated and omitted fields preserve the distinction from a complete layout.
// Values come from the CatalogMax* policy constants above.
func CatalogLimits() Limits {
	maxNodes := int64(CatalogMaxNodes)
	maxConnections := int64(CatalogMaxConnections)
	maxMediaRefs := int64(CatalogMaxMediaRefs)
	maxSceneBytes := int64(CatalogMaxSceneBytes)
	return Limits{
		MaxNodes:       &maxNodes,
		MaxConnections: &maxConnections,
		MaxMediaRefs:   &maxMediaRefs,
		MaxSceneBytes:  &maxSceneBytes,
	}
}

// DeriveCatalog derives the persisted Gallery catalog projection with
// server-side budget enforcement. Every persisted or served
// catalog scene passes through here (accept path, backfill, read
// fallback) so no unbounded scene is ever stored or served.
func DeriveCatalog(stored []byte) (contentVersion string, sceneJSON json.RawMessage, derivable bool) {
	contentVersion = ContentVersionOf(stored)
	if contentVersion == "" {
		return "", nil, false
	}
	snapshot, ok := snapshotForExtraction(stored)
	if !ok {
		return contentVersion, nil, false
	}
	scene, err := Extract(snapshot, contentVersion)
	if err != nil {
		return contentVersion, nil, false
	}
	limits := CatalogLimits()
	ApplyBudgets(scene, limits)
	raw, err := json.Marshal(scene)
	if err != nil || int64(len(raw)) > *limits.MaxSceneBytes {
		return contentVersion, nil, false
	}
	return contentVersion, json.RawMessage(raw), true
}
