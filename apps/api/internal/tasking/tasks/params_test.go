package tasks

import "testing"

func TestUnmarshalParamsPreservesUnknownKeysInExtra(t *testing.T) {
	params, err := UnmarshalParams(`{
		"schema_version": 2,
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
		"future":  `{"schema_version":3}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := UnmarshalParams(raw); err == nil {
				t.Fatal("expected unsupported schema to be rejected")
			}
		})
	}
}
