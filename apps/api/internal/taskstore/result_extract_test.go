package taskstore

import "testing"

func TestExtractTaskModelUsesModelFieldOnly(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "prefer model", raw: `{"model":"gpt-image-1","legacy_model":"a,b"}`, want: "gpt-image-1"},
		{name: "ignore unknown model field", raw: `{"legacy_model":"gpt-image-1"}`, want: "未识别模型"},
		{name: "empty", raw: `{}`, want: "未识别模型"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTaskModel(tt.raw)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
