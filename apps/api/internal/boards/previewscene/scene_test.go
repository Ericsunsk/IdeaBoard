package previewscene

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var fixtureNames = []string{
	"empty",
	"image-input-output",
	"fusion",
	"fusion-input-mediaref",
	"groups-connections",
	"mixed",
	"theme-switch",
}

type fixtureDoc struct {
	Name           string          `json:"name"`
	ContentVersion string          `json:"content_version"`
	Snapshot       json.RawMessage `json:"snapshot"`
	Scene          json.RawMessage `json:"scene"`
}

func loadFixture(t *testing.T, name string) fixtureDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var doc fixtureDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return doc
}

func TestExtractMatchesFixtureScene(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			doc := loadFixture(t, name)
			scene, err := Extract(doc.Snapshot, doc.ContentVersion)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			want, err := ParseScene(doc.Scene)
			if err != nil {
				t.Fatalf("ParseScene(fixture scene): %v", err)
			}
			if !reflect.DeepEqual(scene, want) {
				gotRaw, _ := json.MarshalIndent(scene, "", "  ")
				t.Fatalf("scene mismatch:\n got=%s", gotRaw)
			}
		})
	}
}

func TestThemeSwitchEqualsMixedScene(t *testing.T) {
	mixed := loadFixture(t, "mixed")
	switched := loadFixture(t, "theme-switch")
	var mixedSnap, switchedSnap any
	if err := json.Unmarshal(mixed.Snapshot, &mixedSnap); err != nil {
		t.Fatalf("decode mixed snapshot: %v", err)
	}
	if err := json.Unmarshal(switched.Snapshot, &switchedSnap); err != nil {
		t.Fatalf("decode theme-switch snapshot: %v", err)
	}
	if !reflect.DeepEqual(mixedSnap, switchedSnap) {
		t.Fatal("theme-switch snapshot diverged from mixed snapshot")
	}
	mixedScene, err := ParseScene(mixed.Scene)
	if err != nil {
		t.Fatalf("parse mixed scene: %v", err)
	}
	switchedScene, err := ParseScene(switched.Scene)
	if err != nil {
		t.Fatalf("parse theme-switch scene: %v", err)
	}
	if !reflect.DeepEqual(mixedScene, switchedScene) {
		t.Fatal("theme-switch scene differs from mixed scene: theme must not alter the projection")
	}
}

func TestSceneCarriesNoForbiddenContent(t *testing.T) {
	forbidden := []string{"http://", "https://", "data:", "blob:", "base64", "<svg", "<div", "secret"}
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			doc := loadFixture(t, name)
			scene, err := Extract(doc.Snapshot, doc.ContentVersion)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			raw, err := json.Marshal(scene)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			lowered := strings.ToLower(string(raw))
			for _, marker := range forbidden {
				if strings.Contains(lowered, marker) {
					t.Fatalf("scene contains forbidden marker %q", marker)
				}
			}
		})
	}
}

func TestExtractToleratesMediaRefOnlyMedia(t *testing.T) {
	snapshot := json.RawMessage(`{"schemaVersion":4,
		"imageNodes":[{"id":1,"type":"image-node","x":0,"y":0,"width":280,
			"sourceImageUrl":"","sourceMediaRef":{"assetId":"asset_tol","derivativeTier":"preview"},
			"sourceImageRatio":1,"outputStatus":"ready",
			"outputImages":[{"url":"","ratio":2,"mediaRef":{"assetId":"asset_tol_out","derivativeTier":"thumb"}}]}],
		"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}`)
	scene, err := Extract(snapshot, "cv-tolerance")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(scene.Nodes) != 1 {
		t.Fatalf("nodes=%d", len(scene.Nodes))
	}
	node := scene.Nodes[0]
	if node.Height != 1 {
		t.Fatalf("single frame must fill the long side: height=%v", node.Height)
	}
	if scene.Bounds == nil || scene.Bounds.Height != 560 {
		t.Fatalf("output ratio not applied: bounds=%+v", scene.Bounds)
	}
	if len(node.Media) != 2 {
		t.Fatalf("media=%+v", node.Media)
	}
}

func TestExtractRejects(t *testing.T) {
	snapshot := json.RawMessage(`{"schemaVersion":4,"imageNodes":[],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}`)

	if _, err := Extract(snapshot, ""); err != ErrEmptyContentVersion {
		t.Fatalf("empty content version: got %v", err)
	}
	if _, err := Extract([]byte(`{"schemaVersion":3}`), "cv"); err == nil {
		t.Fatal("schema version 3 accepted")
	} else if !strings.Contains(err.Error(), ErrUnsupportedSnapshot.Error()) {
		t.Fatalf("wrong error: %v", err)
	}
	if _, err := Extract([]byte(`{"schemaVersion":4`), "cv"); err == nil {
		t.Fatal("truncated snapshot accepted")
	}
}

func TestParseSceneRejects(t *testing.T) {
	doc := loadFixture(t, "mixed")

	var mutated map[string]any
	if err := json.Unmarshal(doc.Scene, &mutated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	mutated["version"] = 2
	raw, _ := json.Marshal(mutated)
	if _, err := ParseScene(raw); err == nil {
		t.Fatal("scene version 2 accepted")
	} else if !strings.Contains(err.Error(), ErrUnsupportedSceneVersion.Error()) {
		t.Fatalf("wrong error: %v", err)
	}

	mutated["version"] = 1
	mutated["compat_alias"] = true
	raw, _ = json.Marshal(mutated)
	if _, err := ParseScene(raw); err == nil {
		t.Fatal("unknown scene field accepted")
	}

	empty := `{"version":1,"content_version":"","bounds":null,"extent":{"width":0,"height":0},"nodes":[],"connections":[],"groups":[],"budgets":{"max_nodes":null,"max_connections":null,"max_media_refs":null,"max_scene_bytes":null},"omitted":{"nodes":0,"connections":0,"media_refs":0},"truncated":false}`
	if _, err := ParseScene([]byte(empty)); err == nil {
		t.Fatal("empty content version accepted by ParseScene")
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestApplyBudgets(t *testing.T) {
	doc := loadFixture(t, "mixed")
	scene, err := Extract(doc.Snapshot, doc.ContentVersion)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	ApplyBudgets(scene, Limits{MaxNodes: int64Ptr(2)})
	if !scene.Truncated {
		t.Fatal("expected truncated flag")
	}
	if len(scene.Nodes) != 2 || scene.Omitted.Nodes != 1 {
		t.Fatalf("nodes=%d omitted=%+v", len(scene.Nodes), scene.Omitted)
	}
	for _, c := range scene.Connections {
		if c.From == "fusion-4" || c.To == "fusion-4" {
			t.Fatalf("dangling connection kept: %+v", c)
		}
	}

	scene, err = Extract(doc.Snapshot, doc.ContentVersion)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	ApplyBudgets(scene, Limits{MaxMediaRefs: int64Ptr(3)})
	total := 0
	for _, n := range scene.Nodes {
		total += len(n.Media)
	}
	if total != 3 || scene.Omitted.MediaRefs != 2 {
		t.Fatalf("media total=%d omitted=%+v", total, scene.Omitted)
	}

	scene, err = Extract(doc.Snapshot, doc.ContentVersion)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	before, _ := json.Marshal(scene)
	ApplyBudgets(scene, Limits{})
	after, _ := json.Marshal(scene)
	if string(before) != string(after) {
		t.Fatal("nil limits must not mutate the scene")
	}
}
