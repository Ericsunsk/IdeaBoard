package taskstore

import "testing"

func TestExtractTaskModelPrefersModelAndAvoidsMixedModelSet(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "prefer model", raw: `{"model":"gpt-image-1","model_set":"a,b"}`, want: "gpt-image-1"},
		{name: "single model_set", raw: `{"model_set":"gpt-image-1"}`, want: "gpt-image-1"},
		{name: "mixed model_set", raw: `{"model_set":"a,b"}`, want: "未识别模型"},
		{name: "empty", raw: `{}`, want: "未识别模型"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTaskModel(tt.raw)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
