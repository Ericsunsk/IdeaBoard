package workspacetasks

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// v1 prompt input (data.prompt): contract-typed, length-bounded at
// MaxWorkspaceTaskPromptBytes (8KB, legacy parity). Empty means no prompt;
// feature policy is enforced at canonical compile time.
func TestValidateWorkspacePromptBound(t *testing.T) {
	for _, prompt := range []string{"", "   ", "a red bicycle", strings.Repeat("x", MaxWorkspaceTaskPromptBytes)} {
		if err := ValidateWorkspacePrompt(prompt); err != nil {
			t.Fatalf("prompt %q err = %v, want nil", prompt, err)
		}
	}
	over := strings.Repeat("x", MaxWorkspaceTaskPromptBytes+1)
	err := ValidateWorkspacePrompt(over)
	if !IsValidationError(err) {
		t.Fatalf("over-long prompt err = %v, want validation", err)
	}
	verr, ok := err.(*ValidationError)
	if !ok || verr.Fields["prompt"] != "too_long" {
		t.Fatalf("fields = %v, want prompt=too_long", err)
	}
	// The bound is bytes, not runes: CJK text counts 3 bytes per glyph.
	cjk := strings.Repeat("汉", MaxWorkspaceTaskPromptBytes/3+1)
	if err := ValidateWorkspacePrompt(cjk); !IsValidationError(err) {
		t.Fatalf("over-long CJK prompt err = %v, want validation", err)
	}
}

// params.mask_asset_id: the only durable mask path (workspace public
// asset_... from the upload-session flow). Data-URL/storage-URL masks and
// non-string values fail closed; params.prompt stays unknown_field so the
// discriminated union keeps rejecting it.
func TestValidateParamsMaskAssetID(t *testing.T) {
	if err := ValidateParams(map[string]any{"mask_asset_id": "asset_abc123"}); err != nil {
		t.Fatalf("mask asset err = %v, want nil", err)
	}
	for _, tc := range []struct {
		name   string
		params map[string]any
		field  string
	}{
		{"data-url mask", map[string]any{"mask_asset_id": "data:image/png;base64,AAAA"}, "params.mask_asset_id"},
		{"storage url mask", map[string]any{"mask_asset_id": "https://cdn.example.com/mask.png"}, "params.mask_asset_id"},
		{"garbage mask", map[string]any{"mask_asset_id": "not-an-id"}, "params.mask_asset_id"},
		{"numeric mask", map[string]any{"mask_asset_id": 123}, "params.mask_asset_id"},
		{"empty mask", map[string]any{"mask_asset_id": "  "}, "params.mask_asset_id"},
		{"params prompt still rejected", map[string]any{"prompt": "a sunset"}, "params.prompt"},
	} {
		err := ValidateParams(tc.params)
		if !IsValidationError(err) {
			t.Fatalf("%s err = %v, want validation", tc.name, err)
		}
		verr, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("%s err type = %T, want *ValidationError", tc.name, err)
		}
		if _, exists := verr.Fields[tc.field]; !exists {
			t.Fatalf("%s fields = %v, want %q", tc.name, verr.Fields, tc.field)
		}
	}
}

func TestMaskAssetIDFromParams(t *testing.T) {
	id, ok := MaskAssetIDFromParams(map[string]any{"mask_asset_id": "  asset_abc123  "})
	if !ok || id != "asset_abc123" {
		t.Fatalf("got (%q, %v), want (asset_abc123, true)", id, ok)
	}
	for _, params := range []map[string]any{
		nil, {}, {"mask_asset_id": 123}, {"mask_asset_id": "   "}, {"other": "x"},
	} {
		if _, ok := MaskAssetIDFromParams(params); ok {
			t.Fatalf("params %v: want absent", params)
		}
	}
}

// Submit-time mask readiness: ready masks persist; invisible, non-ready,
// and archived masks fail the submit (never a late worker failure).
func TestCreateVerifiesMaskAsset(t *testing.T) {
	ctx := context.Background()
	newMaskedService := func() (*InMemoryStore, *Service) {
		store := NewInMemoryStoreForTests()
		assets := &fakeAssets{assets: map[string]*AssetInfo{
			"ws_abc\x00asset_ready":   {ID: "asset_ready", WorkspaceID: "ws_abc", Status: "ready"},
			"ws_abc\x00asset_mask":    {ID: "asset_mask", WorkspaceID: "ws_abc", Status: "ready"},
			"ws_abc\x00asset_pending": {ID: "asset_pending", WorkspaceID: "ws_abc", Status: "pending"},
			"ws_abc\x00asset_old":     {ID: "asset_old", WorkspaceID: "ws_abc", Status: "ready", Archived: true},
		}}
		service := NewService(ServiceDeps{
			Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
			Holder: newFakeHolder(), Queue: &fakeEnqueuer{}, Assets: assets,
		})
		return store, service
	}
	_, service := newMaskedService()
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "inpaint",
		MediaAssetIDs: []string{"asset_ready"},
		Params:        map[string]any{"mask_asset_id": "asset_mask"},
	}, "mask-ok")
	if err != nil {
		t.Fatalf("ready mask create err = %v", err)
	}
	if id, ok := MaskAssetIDFromParams(created.Params); !ok || id != "asset_mask" {
		t.Fatalf("persisted mask = (%q, %v), want (asset_mask, true)", id, ok)
	}
	for _, tc := range []struct {
		name   string
		params map[string]any
		want   error
	}{
		{"missing mask", map[string]any{"mask_asset_id": "asset_missing"}, ErrMediaNotFound},
		{"pending mask", map[string]any{"mask_asset_id": "asset_pending"}, ErrMediaNotReady},
		{"archived mask", map[string]any{"mask_asset_id": "asset_old"}, ErrMediaArchived},
	} {
		_, service := newMaskedService()
		_, err := service.Create(ctx, CreateInput{
			WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "inpaint",
			MediaAssetIDs: []string{"asset_ready"}, Params: tc.params,
		}, "mask-"+tc.name)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s err = %v, want %v", tc.name, err, tc.want)
		}
	}
}
