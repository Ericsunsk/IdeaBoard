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
	Model   string `json:"model,omitempty"`

	// Runtime media refs. Provider adapters flatten this object at the edge.
	Media *MediaInputs `json:"media,omitempty"`

	// Generation controls shared by multiple providers.
	Controls *GenerationControls `json:"controls,omitempty"`

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
	ClarityStrength float64 `json:"clarity_strength"`
}

type StyleFusionParams struct {
	Mode         string    `json:"mode,omitempty"`
	InputWeights []float64 `json:"input_weights,omitempty"`
}

// MarshalParams serialises params to the JSON string stored in task.Params.
func MarshalParams(p *WorkflowTaskParams) (string, error) {
	if p == nil {
		return "{}", nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
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
