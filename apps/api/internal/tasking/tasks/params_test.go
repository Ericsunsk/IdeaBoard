package tasks

import "testing"

func TestUnmarshalParamsPreservesUnknownKeysInExtra(t *testing.T) {
	params, err := UnmarshalParams(`{
		"schema_version": 3,
		"feature": "hd-upscale",
		"hd": {"scale": 2.0},
		"provider_only": {"mode": "test"},
		"controls": {"sample_count": 2}
	}`)
	if err != nil {
		t.Fatalf("UnmarshalParams returned error: %v", err)
	}
	if params.Feature != "hd-upscale" {
		t.Fatalf("feature = %q, want hd-upscale", params.Feature)
	}
	if params.HD == nil || params.HD.Scale != 2 {
		t.Fatalf("hd = %#v, want scale 2", params.HD)
	}
	if params.Extra == nil {
		t.Fatal("Extra is nil, want unknown keys preserved")
	}
	if _, ok := params.Extra["provider_only"].(map[string]interface{}); !ok {
		t.Fatalf("provider_only extra = %#v, want object", params.Extra["provider_only"])
	}
	if params.Controls == nil || params.Controls.SampleCount != 2 {
		t.Fatalf("controls = %#v, want sample_count 2", params.Controls)
	}
}

func TestUnmarshalParamsRejectsMissingAndUnsupportedSchemaVersions(t *testing.T) {
	tests := map[string]string{
		"empty":   "",
		"missing": `{"feature":"prompt-generation"}`,
		"zero":    `{"schema_version":0}`,
		"legacy":  `{"schema_version":1}`,
		"future":  `{"schema_version":4}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := UnmarshalParams(raw); err == nil {
				t.Fatal("expected unsupported schema to be rejected")
			}
		})
	}
}

func TestUnmarshalParamsRejectsLegacyPersistedMediaURLs(t *testing.T) {
	tests := []string{
		`{"schema_version":3,"image_urls":["https://legacy.example/input.png"]}`,
		`{"schema_version":3,"media":{"inputs":[{"asset_id":1,"url":"https://legacy.example/input.png"}]}}`,
		`{"schema_version":3,"generation":{"media":[{"input_index":0,"role":"input","asset_id":1,"ref":"https://legacy.example/input.png"}]}}`,
		`{"schema_version":3,"extra":{"original_image_url":"https://legacy.example/input.png"}}`,
	}
	for _, raw := range tests {
		if _, err := UnmarshalParams(raw); err == nil {
			t.Fatalf("expected legacy media payload to be rejected: %s", raw)
		}
	}
}

func TestApplyGenerationMediaFactsBindsAssetIDsByStableRoleOrder(t *testing.T) {
	media := &MediaInputs{
		Inputs:     []MediaRef{{Source: "input-old", Role: MediaRoleInput}},
		References: []MediaRef{{Source: "reference-old", Role: MediaRoleReference, Weight: 0.4}},
		Original:   &MediaRef{Source: "original-old", Role: MediaRoleOriginal},
		Mask:       &MediaRef{Source: "mask-old", Role: MediaRoleMask},
	}
	resolved, err := ApplyGenerationMediaFacts(media, []GenerationMediaIntent{
		{InputIndex: 0, Role: MediaRoleInput, AssetID: 11},
		{InputIndex: 1, Role: MediaRoleReference, AssetID: 12},
		{InputIndex: 2, Role: MediaRoleOriginal, AssetID: 13},
		{InputIndex: 3, Role: MediaRoleMask, AssetID: 14},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Inputs[0].AssetID != 11 || resolved.References[0].AssetID != 12 || resolved.Original.AssetID != 13 || resolved.Mask.AssetID != 14 {
		t.Fatalf("resolved media facts = %#v", resolved)
	}
	if resolved.Inputs[0].Source != "" || resolved.References[0].Source != "" || resolved.Original.Source != "" || resolved.Mask.Source != "" {
		t.Fatalf("resolved durable media retained source URLs = %#v", resolved)
	}
	if media.Inputs[0].AssetID != 0 || media.Inputs[0].Source != "input-old" {
		t.Fatalf("source media was mutated: %#v", media)
	}
}

func TestApplyGenerationMediaFactsRejectsRoleMismatch(t *testing.T) {
	_, err := ApplyGenerationMediaFacts(
		&MediaInputs{Inputs: []MediaRef{{Source: "input", Role: MediaRoleInput}}},
		[]GenerationMediaIntent{{InputIndex: 0, Role: MediaRoleMask, AssetID: 1}},
	)
	if err == nil {
		t.Fatal("expected role mismatch to fail")
	}
}
