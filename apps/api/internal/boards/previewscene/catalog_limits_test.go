package previewscene

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func largeCatalogEnvelope(t *testing.T, nodes int) json.RawMessage {
	t.Helper()
	parts := make([]string, 0, nodes)
	for i := 1; i <= nodes; i++ {
		parts = append(parts, fmt.Sprintf(
			`{"id":%d,"x":%d,"y":%d,"width":280,"sourceImageUrl":"https://cdn.example.com/a-%d.jpg","sourceMediaRef":{"assetId":"asset_src_%d","derivativeTier":"preview"},"sourceMediaAssetId":%d,"sourceImageRatio":0.75,"outputImages":[{"url":"https://cdn.example.com/o-%d.jpg","ratio":0.5,"mediaRef":{"assetId":"asset_out_%d","derivativeTier":"preview"}}]}`,
			i, (i*37)%4000-2000, (i*53)%3000-1500, i, i, 100000+i, i, i))
	}
	conns := make([]string, 0, nodes-1)
	for i := 1; i < nodes; i++ {
		conns = append(conns, fmt.Sprintf(
			`{"id":%d,"fromNodeType":"image","fromNodeId":%d,"fromPort":"right","toNodeType":"image","toNodeId":%d,"toPort":"left"}`,
			i, i, i+1))
	}
	return json.RawMessage(fmt.Sprintf(`{"schema_version":4,"payload":{"imageNodes":[%s],"promptNodes":[],"fusionNodes":[],"connections":[%s],"groups":[]}}`,
		strings.Join(parts, ","), strings.Join(conns, ",")))
}

func TestDeriveCatalogEnforcesBudgets(t *testing.T) {
	envelope := largeCatalogEnvelope(t, 2000)
	version, sceneJSON, ok := DeriveCatalog(envelope)
	if !ok {
		t.Fatalf("large schema-4 board must derive a catalog scene")
	}
	if len(sceneJSON) > CatalogMaxSceneBytes {
		t.Fatalf("catalog scene = %d bytes, want <= %d", len(sceneJSON), CatalogMaxSceneBytes)
	}
	scene, err := ParseScene(sceneJSON)
	if err != nil {
		t.Fatalf("budgeted scene does not parse: %v", err)
	}
	if scene.ContentVersion != version {
		t.Fatalf("budgeted scene lost its content version")
	}
	if !scene.Truncated {
		t.Fatalf("2000-node board must report truncation")
	}
	if len(scene.Nodes) > CatalogMaxNodes {
		t.Fatalf("nodes = %d, want <= %d", len(scene.Nodes), CatalogMaxNodes)
	}
	if len(scene.Connections) > CatalogMaxConnections {
		t.Fatalf("connections = %d, want <= %d", len(scene.Connections), CatalogMaxConnections)
	}
	media := 0
	for _, n := range scene.Nodes {
		media += len(n.Media)
	}
	if scene.Omitted.MediaRefs != 4000-media {
		t.Fatalf("omitted media refs = %d, want %d", scene.Omitted.MediaRefs, 4000-media)
	}
	if media > CatalogMaxMediaRefs {
		t.Fatalf("media refs = %d, want <= %d", media, CatalogMaxMediaRefs)
	}
	limits := CatalogLimits()
	for _, check := range []struct {
		name string
		got  *int64
		want int64
	}{
		{"max_nodes", scene.Budgets.MaxNodes, *limits.MaxNodes},
		{"max_connections", scene.Budgets.MaxConnections, *limits.MaxConnections},
		{"max_media_refs", scene.Budgets.MaxMediaRefs, *limits.MaxMediaRefs},
		{"max_scene_bytes", scene.Budgets.MaxSceneBytes, *limits.MaxSceneBytes},
	} {
		if check.got == nil || *check.got != check.want {
			t.Fatalf("budgets.%s not recorded: %+v", check.name, scene.Budgets)
		}
	}
	if scene.Omitted.Nodes != 2000-len(scene.Nodes) {
		t.Fatalf("omitted nodes = %d, want %d", scene.Omitted.Nodes, 2000-len(scene.Nodes))
	}
	// Determinism: same bytes in, same bytes out.
	_, again, ok := DeriveCatalog(envelope)
	if !ok || string(again) != string(sceneJSON) {
		t.Fatalf("catalog derivation is not deterministic")
	}
}

func TestDeriveCatalogLeavesSmallBoardsUntouched(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			doc := loadFixture(t, name)
			envelope := envelopeForFixture(t, doc.Snapshot)
			_, budgeted, ok := DeriveCatalog(envelope)
			if !ok {
				t.Fatalf("fixture must derive a catalog scene")
			}
			scene, err := ParseScene(budgeted)
			if err != nil {
				t.Fatalf("ParseScene: %v", err)
			}
			if scene.Truncated {
				t.Fatalf("fixture %s must not truncate", name)
			}
			if scene.Budgets.MaxNodes == nil {
				t.Fatalf("enforced budgets must be recorded even without truncation")
			}
		})
	}
}

func TestDeriveCatalogBoundsGroupsAndMembership(t *testing.T) {
	for _, tc := range []struct {
		name    string
		groups  int
		members int
	}{
		{"many-groups", 1000, 1},
		{"large-membership", 1, 20000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := make([]snapshotGroup, tc.groups)
			for i := range groups {
				groups[i] = snapshotGroup{ID: fmt.Sprintf("group-%d", i), Width: 280, Height: 280}
				for j := 0; j < tc.members; j++ {
					groups[i].Members = append(groups[i].Members, "image-1")
				}
			}
			snapshot, err := json.Marshal(snapshotDoc{SchemaVersion: SnapshotSchemaVersion,
				ImageNodes: []snapshotImageNode{{ID: 1, Width: 280, SourceRatio: 1}}, Groups: groups})
			if err != nil {
				t.Fatal(err)
			}
			envelope := envelopeForFixture(t, snapshot)
			_, raw, ok := DeriveCatalog(envelope)
			if !ok {
				t.Fatal("group-heavy scene must derive")
			}
			if len(raw) > CatalogMaxSceneBytes {
				t.Fatalf("group-heavy scene = %d bytes, want <= %d", len(raw), CatalogMaxSceneBytes)
			}
			scene, err := ParseScene(raw)
			if err != nil {
				t.Fatal(err)
			}
			if len(scene.Nodes) != 1 || scene.Bounds == nil || scene.Bounds.Width != 280 {
				t.Fatal("group truncation must retain useful nodes and full bounds")
			}
			if scene.Omitted.Groups != tc.groups-len(scene.Groups) {
				t.Fatalf("incorrect group omissions: %+v", scene.Omitted)
			}
			keptMembers := 0
			for _, group := range scene.Groups {
				keptMembers += len(group.Members)
			}
			if scene.Omitted.GroupMembers != tc.groups*tc.members-keptMembers {
				t.Fatalf("incorrect member omissions: %+v", scene.Omitted)
			}
			_, again, _ := DeriveCatalog(envelope)
			if string(raw) != string(again) {
				t.Fatal("group trimming is not deterministic")
			}
		})
	}
}
