package tasks

import "testing"

func TestUnmarshalParamsPreservesUnknownKeysInExtra(t *testing.T) {
	params, err := UnmarshalParams(`{
		"feature": "hd-upscale",
		"hd": {"clarity_strength": 75},
		"provider_only": {"mode": "test"},
		"controls": {"sample_count": 2}
	}`)
	if err != nil {
		t.Fatalf("UnmarshalParams returned error: %v", err)
	}
	if params.Feature != "hd-upscale" {
		t.Fatalf("feature = %q, want hd-upscale", params.Feature)
	}
	if params.HD == nil || params.HD.ClarityStrength != 75 {
		t.Fatalf("hd = %#v, want clarity strength 75", params.HD)
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

func TestMarshalParamsUsesNestedCanonicalContract(t *testing.T) {
	raw, err := MarshalParams(&WorkflowTaskParams{
		Feature: "image-extend",
		Extend: &ImageExtendParams{
			TopPercent:    1,
			RightPercent:  2,
			BottomPercent: 3,
			LeftPercent:   4,
		},
	})
	if err != nil {
		t.Fatalf("MarshalParams returned error: %v", err)
	}

	params, err := UnmarshalParams(raw)
	if err != nil {
		t.Fatalf("UnmarshalParams returned error: %v", err)
	}
	if params.Extend == nil || params.Extend.RightPercent != 2 {
		t.Fatalf("extend = %#v, want nested canonical contract", params.Extend)
	}
	if _, exists := params.Extra["extend_right"]; exists {
		t.Fatalf("provider edge extend_right leaked into Extra: %#v", params.Extra)
	}
}
