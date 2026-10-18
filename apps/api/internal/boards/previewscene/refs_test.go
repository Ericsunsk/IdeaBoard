package previewscene

// Reference extraction pins (#936): envelope unwrap, bare tolerance,
// contract-valid ids only, dedupe, stable order.

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestExtractMediaAssetIDsEnvelope(t *testing.T) {
	envelope := `{"schema_version":1,"payload":{"schemaVersion":4,` +
		`"imageNodes":[{"id":1,"x":0,"y":0,"width":280,` +
		`"sourceImageUrl":"https://x/y.png",` +
		`"sourceMediaRef":{"assetId":"asset_aaaaaaaaaaaaaaaa"},` +
		`"outputImages":[{"url":"https://x/o.png","mediaRef":{"assetId":"asset_bbbbbbbbbbbbbbbb"}}]}],` +
		`"promptNodes":[{"id":2,"x":0,"y":0,"width":280,"aspectRatio":"auto",` +
		`"inputImages":[{"url":"https://x/i.png","mediaRef":{"assetId":"asset_aaaaaaaaaaaaaaaa"}}]}],` +
		`"fusionNodes":[]}}`
	got := ExtractMediaAssetIDs([]byte(envelope))
	if len(got) != 2 || got[0] != "asset_aaaaaaaaaaaaaaaa" || got[1] != "asset_bbbbbbbbbbbbbbbb" {
		t.Fatalf("extracted = %v, want deduped sorted pair", got)
	}
}

func TestExtractMediaAssetIDsRejectsNonIdentities(t *testing.T) {
	payload := `{"schemaVersion":4,` +
		`"imageNodes":[{"id":1,"x":0,"y":0,"width":280,` +
		`"sourceImageUrl":"https://x/asset_cccccccccccccccc.png",` +
		`"sourceMediaRef":{"assetId":"not-an-asset"},` +
		`"outputImages":[{"url":"https://x/o.png","mediaRef":{"assetId":"  "}}]}],` +
		`"promptNodes":[{"id":2,"x":0,"y":0,"width":280,"aspectRatio":"auto",` +
		`"inputImages":[{"url":"https://x/i.png","mediaAssetId":301}]}],` +
		`"fusionNodes":[]}`
	if got := ExtractMediaAssetIDs([]byte(payload)); len(got) != 0 {
		t.Fatalf("extracted = %v, want no references (URLs/text/numerics grant nothing)", got)
	}
	if got := ExtractMediaAssetIDs([]byte(`{"schema_version":1}`)); len(got) != 0 {
		t.Fatalf("envelope without payload = %v, want none", got)
	}
	if got := ExtractMediaAssetIDs([]byte(`not json`)); len(got) != 0 {
		t.Fatalf("invalid snapshot = %v, want none", got)
	}
	if got := ExtractMediaAssetIDs(nil); len(got) != 0 {
		t.Fatalf("empty snapshot = %v, want none", got)
	}
}

func TestExtractMediaAssetIDsKeepsEntireDocument(t *testing.T) {
	const count = 600
	nodes := make([]map[string]any, count)
	for i := range nodes {
		nodes[i] = map[string]any{"sourceMediaRef": map[string]any{"assetId": fmt.Sprintf("asset_%024d", i)}}
	}
	snapshot, err := json.Marshal(map[string]any{"imageNodes": nodes})
	if err != nil {
		t.Fatal(err)
	}
	ids := ExtractMediaAssetIDs(snapshot)
	if len(ids) != count {
		t.Fatalf("references = %d, want %d", len(ids), count)
	}
	for i, id := range ids {
		if want := fmt.Sprintf("asset_%024d", i); id != want {
			t.Fatalf("reference %d = %q, want %q", i, id, want)
		}
	}
}
