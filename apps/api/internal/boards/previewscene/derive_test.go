package previewscene

import (
	"encoding/json"
	"testing"
)

// envelopeForFixture rebuilds the production envelope shape from a
// cross-language fixture snapshot: the frontend strips schemaVersion out
// of the persisted state into envelope-level schema_version, so the
// payload carries the node collections verbatim without it.
func envelopeForFixture(t *testing.T, snapshot json.RawMessage) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(snapshot, &fields); err != nil {
		t.Fatalf("decode fixture snapshot: %v", err)
	}
	versionRaw, ok := fields["schemaVersion"]
	if !ok {
		t.Fatalf("fixture snapshot has no schemaVersion")
	}
	var version int
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		t.Fatalf("decode fixture schemaVersion: %v", err)
	}
	delete(fields, "schemaVersion")
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode fixture payload: %v", err)
	}
	envelope, err := json.Marshal(map[string]json.RawMessage{
		"schema_version": versionRaw,
		"payload":        payload,
	})
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	return envelope
}

func TestDeriveCatalogMatchesFixtureExtraction(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			doc := loadFixture(t, name)
			envelope := envelopeForFixture(t, doc.Snapshot)
			version, sceneJSON, ok := DeriveCatalog(envelope)
			if !ok {
				t.Fatalf("DeriveCatalog not derivable for schema-4 fixture")
			}
			if version != ContentVersionOf(envelope) {
				t.Fatalf("content version is not the canonical hash: got %q", version)
			}
			if len(version) != 64 {
				t.Fatalf("content version is not hex sha256: %q", version)
			}
			scene, err := ParseScene(sceneJSON)
			if err != nil {
				t.Fatalf("ParseScene(derived): %v", err)
			}
			if scene.ContentVersion != version {
				t.Fatalf("derived scene carries %q, want %q", scene.ContentVersion, version)
			}
			// The derived scene must equal direct extraction over the
			// fixture snapshot: the envelope splice is byte-faithful.
			direct, err := Extract(doc.Snapshot, version)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			ApplyBudgets(direct, CatalogLimits())
			directRaw, _ := json.Marshal(direct)
			derivedCanonical, _ := json.Marshal(scene)
			if string(directRaw) != string(derivedCanonical) {
				t.Fatalf("derived scene differs from direct extraction:\n got=%s\nwant=%s", derivedCanonical, directRaw)
			}
		})
	}
}

func TestDeriveCatalogIsDeterministic(t *testing.T) {
	doc := loadFixture(t, "mixed")
	envelope := envelopeForFixture(t, doc.Snapshot)
	firstVersion, firstScene, firstOK := DeriveCatalog(envelope)
	secondVersion, secondScene, secondOK := DeriveCatalog(envelope)
	if !firstOK || !secondOK || firstVersion != secondVersion || string(firstScene) != string(secondScene) {
		t.Fatalf("DeriveCatalog is not deterministic")
	}
}

func TestDeriveCatalogNonExtractableSchemasKeepVersionWithoutScene(t *testing.T) {
	cases := map[string]string{
		"fresh empty document": `{"schema_version":1,"payload":{}}`,
		"future schema":        `{"schema_version":5,"payload":{"imageNodes":[]}}`,
		"non-object payload":   `{"schema_version":4,"payload":[]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			envelope := json.RawMessage(raw)
			version, sceneJSON, ok := DeriveCatalog(envelope)
			if ok || sceneJSON != nil {
				t.Fatalf("non-extractable input must yield no scene")
			}
			if version != ContentVersionOf(envelope) {
				t.Fatalf("non-extractable input must still carry the canonical version")
			}
		})
	}
}

func TestDeriveCatalogRejectsEmptyInput(t *testing.T) {
	version, sceneJSON, ok := DeriveCatalog(nil)
	if ok || version != "" || sceneJSON != nil {
		t.Fatalf("empty input must not derive: %q %v %v", version, sceneJSON, ok)
	}
}

func TestDeriveCatalogCanonicalVersionIgnoresWireForm(t *testing.T) {
	// The version hashes the canonical document, not wire
	// bytes. Whitespace-only and key-order-only re-serializations share
	// one version and one scene (JSONB read-backs normalize both, so a
	// byte-based rule would flap between accept time and read time).
	compact := json.RawMessage(`{"schema_version":4,"payload":{"imageNodes":[],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}}`)
	spaced := json.RawMessage("{ \"schema_version\" : 4 , \"payload\" : { \"groups\" : [ ] , \"connections\" : [ ] , \"fusionNodes\" : [ ] , \"promptNodes\" : [ ] , \"imageNodes\" : [ ] } }")
	compactVersion, compactScene, compactOK := DeriveCatalog(compact)
	spacedVersion, spacedScene, spacedOK := DeriveCatalog(spaced)
	if !compactOK || !spacedOK {
		t.Fatalf("empty schema-4 boards must derive a scene")
	}
	if compactVersion != spacedVersion {
		t.Fatalf("wire variants must share a version: %q vs %q", compactVersion, spacedVersion)
	}
	if string(compactScene) != string(spacedScene) {
		t.Fatalf("wire variants must project identical scene bytes:\n%s\n%s", compactScene, spacedScene)
	}
}

func TestDeriveCatalogVersionChangesWithContent(t *testing.T) {
	base := json.RawMessage(`{"schema_version":4,"payload":{"imageNodes":[],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}}`)
	moved := json.RawMessage(`{"schema_version":4,"payload":{"imageNodes":[{"id":1,"x":5,"y":5,"width":280,"sourceImageRatio":1,"outputImages":[]}],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}}`)
	baseVersion, _, baseOK := DeriveCatalog(base)
	movedVersion, movedScene, movedOK := DeriveCatalog(moved)
	if !baseOK || !movedOK {
		t.Fatalf("schema-4 boards must derive a scene")
	}
	if baseVersion == movedVersion {
		t.Fatalf("content change must advance the version")
	}
	if len(movedScene) == 0 {
		t.Fatalf("content change must project a scene")
	}
}
