package tasks

import (
	"encoding/json"
	"fmt"

	"ideaboard/api/internal/workflow/paramkeys"
)

// SchemaVersion tracks the WorkflowTaskParams schema carried by task records.
const CurrentParamsSchemaVersion = 1

// WorkflowTaskParams is the typed contract for task parameters flowing
// from frontend submit through API, River, and into the worker/provider layer.
type WorkflowTaskParams struct {
	SchemaVersion int `json:"schema_version,omitempty"`

	// Feature routing.
	Feature string `json:"feature,omitempty"`

	// Runtime media refs. Provider adapters flatten this object at the edge.
	Media *MediaInputs `json:"media,omitempty"`

	// Generation controls shared by multiple providers.
	Controls *GenerationControls `json:"controls,omitempty"`

	// Generation is the immutable, provider-independent request snapshot
	// captured before Task submission. Provider adapters must treat it as a
	// fact snapshot and derive their own request at the runtime boundary.
	Generation *GenerationRequestSnapshot `json:"generation,omitempty"`

	// Feature-specific canonical prompt contracts. Keep these in sync with
	// workflow/prompt_contract.go and SYSTEM_PROMPT.md.
	Variation *StyleVariationParams `json:"variation,omitempty"`
	Extend    *ImageExtendParams    `json:"extend,omitempty"`
	HD        *HDUpscaleParams      `json:"hd,omitempty"`
	Fusion    *StyleFusionParams    `json:"fusion,omitempty"`

	// Provider hints (passthrough, not validated).
	// Extra carries provider-specific parameters that don't have a
	// first-class typed field. Consumers MUST NOT rely on Extra keys
	// for security or correctness — only for provider API shaping.
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// GenerationRequestSnapshot records the user intent and resolved workflow
// inputs used to create a Task. It deliberately contains no provider model,
// account, or endpoint selection.
type GenerationRequestSnapshot struct {
	SchemaVersion        int                     `json:"schema_version"`
	Feature              string                  `json:"feature"`
	Primitive            string                  `json:"primitive"`
	UserPrompt           string                  `json:"user_prompt"`
	CompiledPrompt       string                  `json:"compiled_prompt"`
	TemplateVersion      string                  `json:"template_version,omitempty"`
	TemplateFingerprint  string                  `json:"template_fingerprint"`
	Controls             *GenerationControls     `json:"controls,omitempty"`
	Media                []GenerationMediaIntent `json:"media,omitempty"`
	RequestedOutputCount int                     `json:"requested_output_count"`
}

// GenerationMediaIntent is an ordered, role-aware media input intent. The
// Ref is resolved to a controlled media reference before Task persistence.
type GenerationMediaIntent struct {
	InputIndex int     `json:"input_index"`
	Role       string  `json:"role"`
	Ref        string  `json:"ref"`
	AssetID    int64   `json:"asset_id,omitempty"`
	Weight     float64 `json:"weight,omitempty"`
}

type MediaInputs struct {
	Inputs     []MediaRef `json:"inputs,omitempty"`
	References []MediaRef `json:"references,omitempty"`
	Mask       *MediaRef  `json:"mask,omitempty"`
	Original   *MediaRef  `json:"original,omitempty"`
	UseMask    *bool      `json:"use_mask,omitempty"`
}

type MediaRef struct {
	URL    string  `json:"url"`
	Role   string  `json:"role,omitempty"`
	Weight float64 `json:"weight,omitempty"`
}

type GenerationControls struct {
	CustomWidth      float64 `json:"custom_width,omitempty"`
	CustomHeight     float64 `json:"custom_height,omitempty"`
	OriginalWidth    float64 `json:"original_image_width,omitempty"`
	OriginalHeight   float64 `json:"original_image_height,omitempty"`
	Ratio            string  `json:"ratio,omitempty"`
	Quality          string  `json:"quality,omitempty"`
	SampleCount      int     `json:"sample_count,omitempty"`
	OutputCount      int     `json:"output_count,omitempty"`
	ImageCount       int     `json:"image_count,omitempty"`
	PromptUpsampling *bool   `json:"prompt_upsampling,omitempty"`
}

type StyleVariationParams struct {
	Strength float64 `json:"strength"`
}

type ImageExtendParams struct {
	TopPercent    float64 `json:"top_percent"`
	RightPercent  float64 `json:"right_percent"`
	BottomPercent float64 `json:"bottom_percent"`
	LeftPercent   float64 `json:"left_percent"`
}

type HDUpscaleParams struct {
	Scale float64 `json:"scale"`
}

type StyleFusionParams struct {
	Mode         string    `json:"mode,omitempty"`
	InputWeights []float64 `json:"input_weights,omitempty"`
}

// UnmarshalParams deserialises the JSON string from task.Params.
// Unknown provider-specific keys are preserved in Extra.
func UnmarshalParams(raw string) (*WorkflowTaskParams, error) {
	if raw == "" {
		return &WorkflowTaskParams{SchemaVersion: CurrentParamsSchemaVersion}, nil
	}
	p := &WorkflowTaskParams{}
	if err := p.UnmarshalJSON([]byte(raw)); err != nil {
		return nil, err
	}
	if p.SchemaVersion == 0 {
		p.SchemaVersion = CurrentParamsSchemaVersion
	}
	return p, nil
}

func (p *WorkflowTaskParams) UnmarshalJSON(data []byte) error {
	type workflowTaskParams WorkflowTaskParams
	var alias workflowTaskParams
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	extra := map[string]interface{}{}
	for key, value := range raw {
		if paramkeys.IsCanonicalTaskParamKey(key) {
			continue
		}
		var decoded interface{}
		if err := json.Unmarshal(value, &decoded); err != nil {
			return fmt.Errorf("decode extra task param %q: %w", key, err)
		}
		extra[key] = decoded
	}
	*p = WorkflowTaskParams(alias)
	if len(extra) > 0 {
		if p.Extra == nil {
			p.Extra = map[string]interface{}{}
		}
		for key, value := range extra {
			p.Extra[key] = value
		}
	}
	if p.SchemaVersion == 0 {
		p.SchemaVersion = CurrentParamsSchemaVersion
	}
	return nil
}
