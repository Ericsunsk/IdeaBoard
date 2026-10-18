package boards

// Board document -> media reference reconciliation (#936, in-memory twin).
//
// Pins that ReplaceDocument reconciles the accepted reference set: newly
// referenced asset ids appear through the narrow port, removed ids stop,
// and non-identities never become references.

import (
	"context"
	"encoding/json"
	"testing"
)

func refsTestEnvelope(t *testing.T, assetIDs ...string) json.RawMessage {
	t.Helper()
	type mediaRef struct {
		AssetID string `json:"assetId"`
	}
	type output struct {
		URL      string    `json:"url"`
		MediaRef *mediaRef `json:"mediaRef,omitempty"`
	}
	type imageNode struct {
		ID             float64   `json:"id"`
		X              float64   `json:"x"`
		Y              float64   `json:"y"`
		Width          float64   `json:"width"`
		SourceImageURL string    `json:"sourceImageUrl"`
		SourceMediaRef *mediaRef `json:"sourceMediaRef,omitempty"`
		Outputs        []output  `json:"outputImages"`
	}
	nodes := make([]imageNode, 0, len(assetIDs))
	for i, id := range assetIDs {
		ref := &mediaRef{AssetID: id}
		nodes = append(nodes, imageNode{
			ID: float64(i + 1), Width: 280,
			SourceImageURL: "https://example.com/y.png",
			SourceMediaRef: ref,
			Outputs:        []output{{URL: "https://example.com/o.png", MediaRef: ref}},
		})
	}
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": 4,
		"imageNodes":    nodes,
		"promptNodes":   []any{},
		"fusionNodes":   []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(map[string]any{"schema_version": 1, "payload": json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func exerciseBoardMediaRefs(t *testing.T, ctx context.Context, store WorkspaceBoardStore, workspaceID string, owner int64) {
	t.Helper()
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBoard(ctx, workspaceID, owner, "refs", boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	list := func(asset string) []string {
		t.Helper()
		rows, err := store.ListBoardsReferencingAsset(ctx, workspaceID, asset)
		if err != nil {
			t.Fatalf("list refs: %v", err)
		}
		return rows
	}
	const assetA = "asset_aaaaaaaaaaaaaaaa"
	const assetB = "asset_bbbbbbbbbbbbbbbb"
	if got := list(assetA); len(got) != 0 {
		t.Fatalf("fresh board refs = %v, want none", got)
	}
	if _, err := store.ReplaceDocument(ctx, workspaceID, boardID, owner, 1, 1, refsTestEnvelope(t, assetA, assetB)); err != nil {
		t.Fatalf("replace with refs: %v", err)
	}
	if got := list(assetA); len(got) != 1 || got[0] != boardID {
		t.Fatalf("refs for A = %v, want [%s]", got, boardID)
	}
	if got := list(assetB); len(got) != 1 || got[0] != boardID {
		t.Fatalf("refs for B = %v, want [%s]", got, boardID)
	}
	// Removal reconciles in the accepting transaction: B drops out, A stays.
	if _, err := store.ReplaceDocument(ctx, workspaceID, boardID, owner, 2, 1, refsTestEnvelope(t, assetA)); err != nil {
		t.Fatalf("replace without B: %v", err)
	}
	if got := list(assetA); len(got) != 1 {
		t.Fatalf("refs for A after removal = %v, want one", got)
	}
	if got := list(assetB); len(got) != 0 {
		t.Fatalf("refs for B after removal = %v, want none", got)
	}
	// Non-identities never become references.
	if _, err := store.ReplaceDocument(ctx, workspaceID, boardID, owner, 3, 1, refsTestEnvelope(t, "nope", "")); err != nil {
		t.Fatalf("replace with non-identities: %v", err)
	}
	if got := list("nope"); len(got) != 0 {
		t.Fatalf("refs for malformed = %v, want none", got)
	}
	if got := list(assetA); len(got) != 0 {
		t.Fatalf("refs for A after clearing = %v, want none", got)
	}
}

func TestBoardMediaRefsInMemory(t *testing.T) {
	ctx := context.Background()
	exerciseBoardMediaRefs(t, ctx, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_refs", 1)
}
