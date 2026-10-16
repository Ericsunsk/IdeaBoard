package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLibrarySourceFixturesContract pins the unified Library Source wire
// rules (issues #930/#931) against the canonical fixtures: external rows
// carry provider/source identity plus real URLs and a complete positive
// dimension pair; media rows carry a media_asset_id plus a paired
// (both-null-or-both-positive) dimension and MUST NOT contain
// provider/source_id/source_url/preview_url/original_url keys. Task
// lineage is provenance only; pagination keeps next_cursor/has_more.
func TestLibrarySourceFixturesContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "contracts", "library", "library-source.fixtures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	decode := func(t *testing.T, key string) map[string]any {
		t.Helper()
		raw, ok := fixtures[key]
		if !ok {
			t.Fatalf("fixture %q missing", key)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %q: %v", key, err)
		}
		return doc
	}

	external := decode(t, "external")
	assertExternalShape(t, external)
	for _, key := range []string{"media_known", "media_unknown", "task_lineage"} {
		assertMediaShape(t, decode(t, key))
	}

	mixed := decode(t, "mixed_list")
	items, ok := mixed["data"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("mixed_list has %d items, want 3", len(items))
	}
	seen := map[string]bool{}
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatal("mixed_list item is not an object")
		}
		source, _ := row["source"].(string)
		seen[source] = true
		switch source {
		case "external":
			assertExternalShape(t, row)
		case "media":
			assertMediaShape(t, row)
		default:
			t.Fatalf("mixed_list source = %q, want external|media", source)
		}
		if row["display"] == nil {
			t.Fatalf("mixed_list %v lacks inline display", row["id"])
		}
	}
	if !seen["external"] || !seen["media"] {
		t.Fatalf("mixed_list sources = %v, want both", seen)
	}
	meta, ok := mixed["meta"].(map[string]any)
	if !ok {
		t.Fatal("mixed_list lacks meta")
	}
	if _, ok := meta["next_cursor"]; !ok {
		t.Fatal("mixed_list meta lacks next_cursor")
	}
	if _, ok := meta["has_more"]; !ok {
		t.Fatal("mixed_list meta lacks has_more")
	}

	lineage := decode(t, "task_lineage")
	if lineage["task_id"] == nil || lineage["task_result_index"] == nil {
		t.Fatal("task_lineage lacks provenance pair")
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url", "saved_by_user_id"} {
		if _, present := lineage[key]; present {
			t.Fatalf("task_lineage carries %q", key)
		}
	}
}

func assertExternalShape(t *testing.T, row map[string]any) {
	t.Helper()
	if row["source"] != "external" {
		t.Fatalf("source = %v, want external", row["source"])
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url"} {
		value, _ := row[key].(string)
		if value == "" {
			t.Fatalf("external %q is empty", key)
		}
	}
	if _, present := row["media_asset_id"]; present {
		t.Fatal("external row carries media_asset_id")
	}
	width, wok := row["width"].(float64)
	height, hok := row["height"].(float64)
	if !wok || !hok || width <= 0 || height <= 0 {
		t.Fatalf("external dims = %v x %v, want complete positive pair", row["width"], row["height"])
	}
}

func assertMediaShape(t *testing.T, row map[string]any) {
	t.Helper()
	if row["source"] != "media" {
		t.Fatalf("source = %v, want media", row["source"])
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url", "saved_by_user_id"} {
		if _, present := row[key]; present {
			t.Fatalf("media row carries %q", key)
		}
	}
	link, _ := row["media_asset_id"].(string)
	if link == "" {
		t.Fatal("media row lacks media_asset_id")
	}
	width, hasWidth := row["width"]
	height, hasHeight := row["height"]
	if !hasWidth || !hasHeight {
		t.Fatal("media row lacks width/height keys (null = unknown)")
	}
	widthNum, wok := width.(float64)
	heightNum, hok := height.(float64)
	switch {
	case width == nil && height == nil:
	case wok && hok && widthNum > 0 && heightNum > 0:
	default:
		t.Fatalf("media dims = %v x %v, want paired null-or-positive", width, height)
	}
}
