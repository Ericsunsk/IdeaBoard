package previewscene

import (
	"encoding/json"
	"sort"
	"strings"

	"ideaboard/api/internal/platform/contract"
)

// ExtractMediaAssetIDs returns the durable workspace media identities
// (asset_ public ids) accepted into a board snapshot, deduplicated and
// stably sorted. Unlike Extract (the strict Preview Scene projection, gated
// on SnapshotSchemaVersion), this read is intentionally version-tolerant:
// it unwraps the persisted document envelope ({schema_version, payload})
// and decodes only the known node collections, ignoring unknown fields,
// so accepted documents from any snapshot schema version reconcile the
// same reference fact. Only contract-valid asset_ strings are returned —
// free-text URLs, prompt text, numeric legacy ids, and malformed values
// never become references (ids and URLs are not credentials, and text
// mentions must not grant reads).
func ExtractMediaAssetIDs(snapshot []byte) []string {
	doc, ok := decodeSnapshotDoc(snapshot)
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	collect := func(ref *snapshotMediaRef) {
		if ref == nil {
			return
		}
		id := strings.TrimSpace(ref.AssetID)
		if id == "" || !contract.IsAssetID(id) {
			return
		}
		seen[id] = struct{}{}
	}
	for i := range doc.ImageNodes {
		node := &doc.ImageNodes[i]
		collect(node.SourceMediaRef)
		for j := range node.Outputs {
			collect(node.Outputs[j].MediaRef)
		}
	}
	for i := range doc.PromptNodes {
		node := &doc.PromptNodes[i]
		for j := range node.Inputs {
			collect(node.Inputs[j].MediaRef)
		}
	}
	for i := range doc.FusionNodes {
		node := &doc.FusionNodes[i]
		for j := range node.Inputs {
			collect(node.Inputs[j].MediaRef)
		}
		for j := range node.Outputs {
			collect(node.Outputs[j].MediaRef)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// decodeSnapshotDoc unwraps the persisted document envelope
// ({schema_version, payload}) and decodes the known node collections from
// the payload. Bare (unenveloped) snapshots decode directly for tolerance;
// anything else reports false and reconciles no references.
func decodeSnapshotDoc(snapshot []byte) (snapshotDoc, bool) {
	var doc snapshotDoc
	decoder := json.NewDecoder(strings.NewReader(string(snapshot)))
	var envelope struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := decoder.Decode(&envelope); err != nil || len(envelope.Payload) == 0 {
		if err := json.Unmarshal(snapshot, &doc); err != nil {
			return snapshotDoc{}, false
		}
		return doc, true
	}
	if err := json.Unmarshal(envelope.Payload, &doc); err != nil {
		return snapshotDoc{}, false
	}
	return doc, true
}
