package workspacetasks

import "testing"

func TestBuildTaskCompletedOutputsKeepsMeasuredMetadata(t *testing.T) {
	outputs := BuildTaskCompletedOutputs("ws_test", []OutputRef{
		{AssetID: "asset_unknown", Role: "result"},
		{AssetID: "asset_measured", Role: "result", Width: 768, Height: 512, MIMEType: "image/webp"},
	})
	if len(outputs) != 2 {
		t.Fatalf("outputs = %+v", outputs)
	}
	if got := outputs[0]; got.Width != 0 || got.Height != 0 || got.MIMEType != "" {
		t.Fatalf("unknown metadata was fabricated: %+v", got)
	}
	if got := outputs[1]; got.Width != 768 || got.Height != 512 || got.MIMEType != "image/webp" {
		t.Fatalf("measured metadata changed: %+v", got)
	}
	for _, output := range outputs {
		if !output.PreviewReady || output.URL == "" {
			t.Fatalf("ready asset projection = %+v", output)
		}
	}
}
