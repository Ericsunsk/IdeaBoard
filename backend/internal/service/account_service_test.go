package service

import "testing"

func TestTemplateValueDescriptions(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "viewpoint uses display label",
			got:  describeViewpointTemplateValue("top_45"),
			want: "45° 侧视",
		},
		{
			name: "lighting uses readable label",
			got:  describeViewpointLightingTemplateValue("hard_light"),
			want: "质感硬光",
		},
		{
			name: "wear tile mode uses readable label",
			got:  describeWearTileModeTemplateValue("seamless"),
			want: "平铺重复",
		},
		{
			name: "unknown values are preserved after trim",
			got:  describeWearTileModeTemplateValue(" custom-mode "),
			want: "custom-mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
