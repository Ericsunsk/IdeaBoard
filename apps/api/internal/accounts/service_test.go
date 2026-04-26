package accounts

import (
	"strings"
	"testing"

	"api/internal/prompts"
	"api/internal/workflow"
)

func TestTemplateValueDescriptions(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "viewpoint uses display label",
			got:  prompts.DescribeViewpointTemplateValue("top_45"),
			want: "45° 侧视",
		},
		{
			name: "lighting uses readable label",
			got:  prompts.DescribeViewpointLightingTemplateValue("hard_light"),
			want: "质感硬光",
		},
		{
			name: "wear tile mode uses readable label",
			got:  prompts.DescribeWearTileModeTemplateValue("seamless"),
			want: "平铺重复",
		},
		{
			name: "unknown values are preserved after trim",
			got:  prompts.DescribeWearTileModeTemplateValue(" custom-mode "),
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

func TestParseWorkflowFeatureSlug(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "canonical slug is valid", raw: "style-fusion", want: "style-fusion", ok: true},
		{name: "remove-bg slug is valid", raw: "remove-bg", want: "remove-bg", ok: true},
		{name: "underscore slug is invalid", raw: "style_fusion", want: "", ok: false},
		{name: "unsupported slug is invalid", raw: "line-draft", want: "", ok: false},
		{name: "invalid chars are rejected", raw: "style fusion", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := workflow.ParseWorkflowFeatureSlug(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("raw=%q got=(%q,%v), want=(%q,%v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestApplyFeatureSystemPromptUsesCanonicalKeys(t *testing.T) {
	params := map[string]interface{}{
		"mask_image_url": "https://mask.example/a.png",
		"use_mask":       true,
		"upscale_mode":   "enhancement",
	}

	got := prompts.ApplyFeatureSystemPrompt(
		"base prompt",
		"user prompt",
		"hd-upscale",
		params,
		func(slug string) (string, error) {
			return "SYS={{user_prompt}}|{{mask_image_url}}|{{use_mask}}|{{upscale_mode}}", nil
		},
	)

	want := "SYS=user prompt|https://mask.example/a.png|true|enhancement"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMaskedRequestDetectionUsesCanonicalAliases(t *testing.T) {
	hdUpscaleParams := map[string]interface{}{
		"mask_image_url": "https://mask.example/1.png",
	}
	if !prompts.IsMaskedHDUpscaleRequest(hdUpscaleParams) {
		t.Fatalf("expected hd-upscale masked request to be detected by canonical mask_image_url")
	}

	fabricParams := map[string]interface{}{
		"mask_image_urls": []interface{}{"https://mask.example/2.png"},
	}
	if !prompts.IsMaskedFabricOnBodyRequest(fabricParams) {
		t.Fatalf("expected fabric-on-body masked request to be detected by canonical mask_image_urls")
	}
}

func TestFeatureSystemPromptDoesNotContainHiddenConstraintSnippet(t *testing.T) {
	got := prompts.ApplyFeatureSystemPrompt(
		"user prompt",
		"user prompt",
		"canvas-extend",
		map[string]interface{}{
			"extend_top": 30,
		},
		func(slug string) (string, error) {
			return "SYS=canvas extend only", nil
		},
	)

	if got != "SYS=canvas extend only user prompt" {
		t.Fatalf("got %q, want only configured system prompt + base prompt", got)
	}
	if strings.Contains(got, "【执行语义】") {
		t.Fatalf("expected no hidden constraint text, got %q", got)
	}
}

func TestApplyTaskSystemPromptUsesAdminConfiguredPromptOnly(t *testing.T) {
	tests := []struct {
		name       string
		feature    string
		basePrompt string
		userPrompt string
		params     map[string]interface{}
		template   string
		want       string
	}{
		{
			name:       "style fusion",
			feature:    workflow.FeatureSlugStyleFusion,
			basePrompt: "base fusion prompt",
			userPrompt: "融合但不要太夸张",
			params: map[string]interface{}{
				"blend_ratio":    55,
				"image_a_weight": 60,
				"image_b_weight": 40,
			},
			template: "SYS {{user_prompt}} ratio={{blend_ratio}} a={{image_a_weight}} b={{image_b_weight}}",
			want:     "SYS 融合但不要太夸张 ratio=55 a=60 b=40",
		},
		{
			name:       "multi angle",
			feature:    workflow.FeatureSlugMultiAngle,
			basePrompt: "user prompt",
			userPrompt: "输出右前45度",
			params: map[string]interface{}{
				"model_url":      "https://example.com/model.png",
				"pose_url":       "https://example.com/pose.png",
				"model_strength": 0.8,
				"pose_strength":  0.6,
				"angle_spec":     "front-right-45",
			},
			template: "SYS {{user_prompt}} model={{model_url}} pose={{pose_url}} ms={{model_strength}} ps={{pose_strength}} angle={{angle_spec}}",
			want:     `SYS 输出右前45度 model=https://example.com/model.png pose=https://example.com/pose.png ms=0.8 ps=0.6 angle="front-right-45"`,
		},
		{
			name:       "canvas extend",
			feature:    workflow.FeatureSlugCanvasExtend,
			basePrompt: "user prompt",
			userPrompt: "右侧延展为工作室背景",
			params: map[string]interface{}{
				"extend_top":    0,
				"extend_right":  22,
				"extend_bottom": 0,
				"extend_left":   0,
			},
			template: "SYS {{user_prompt}} top={{extend_top}} right={{extend_right}} bottom={{extend_bottom}} left={{extend_left}}",
			want:     "SYS 右侧延展为工作室背景 top=0 right=22 bottom=0 left=0",
		},
		{
			name:       "masked hd-upscale",
			feature:    workflow.FeatureSlugHDUpscale,
			basePrompt: "user prompt",
			userPrompt: "",
			params: map[string]interface{}{
				"original_image_url": "https://example.com/original.png",
				"mask_image_url":     "https://example.com/mask.png",
				"use_mask":           true,
				"upscale_mode":       "enhancement",
			},
			template: "SYS original={{original_image_url}} mask={{mask_image_url}} use_mask={{use_mask}} mode={{upscale_mode}}",
			want:     "SYS original=https://example.com/original.png mask=https://example.com/mask.png use_mask=true mode=enhancement",
		},
		{
			name:       "masked fabric-on-body",
			feature:    workflow.FeatureSlugFabricOnBody,
			basePrompt: "user prompt",
			userPrompt: "把面料贴到主体上",
			params: map[string]interface{}{
				"original_image_url": "https://example.com/body.png",
				"mask_image_url":     "https://example.com/wear-mask.png",
				"use_mask":           true,
				"tile_mode":          "seamless",
			},
			template: "SYS {{user_prompt}} original={{original_image_url}} mask={{mask_image_url}} use_mask={{use_mask}} tile={{tile_mode}}",
			want:     "SYS 把面料贴到主体上 original=https://example.com/body.png mask=https://example.com/wear-mask.png use_mask=true tile=平铺重复",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyTaskSystemPrompt(tt.basePrompt, tt.userPrompt, tt.feature, tt.params, func(string) (string, error) {
				return tt.template, nil
			})

			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "【执行语义】") {
				t.Fatalf("got unexpected hidden constraint in prompt: %q", got)
			}
		})
	}
}

func TestCanvasExtendEmptyUserPromptCanStillBuildExecutablePrompt(t *testing.T) {
	fullPrompt := applyTaskSystemPrompt(
		"",
		"",
		workflow.FeatureSlugCanvasExtend,
		map[string]interface{}{
			"extend_right": 30,
		},
		func(string) (string, error) {
			return "SYS 扩图 right={{extend_right}}", nil
		},
	)

	if strings.TrimSpace(fullPrompt) == "" {
		t.Fatalf("expected non-empty full prompt")
	}
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		t.Fatalf("expected executable prompt, got err: %v", err)
	}
}

func TestRemoveBgWithoutConfiguredSystemPromptIsRejected(t *testing.T) {
	fullPrompt := applyTaskSystemPrompt(
		"",
		"",
		workflow.FeatureSlugRemoveBg,
		map[string]interface{}{
			"feature": workflow.FeatureSlugRemoveBg,
		},
		func(string) (string, error) {
			return "", nil
		},
	)

	if err := validateExecutablePrompt(fullPrompt); err == nil {
		t.Fatalf("expected validation error when remove-bg has no executable prompt")
	}
}

func TestValidateExecutablePromptReturnsExplicitErrorWhenFinalPromptEmpty(t *testing.T) {
	err := validateExecutablePrompt("   ")
	if err == nil {
		t.Fatalf("expected validation error for empty prompt")
	}
	want := "缺少提示词，请填写 Prompt 或配置后台 system prompt"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}
