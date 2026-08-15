package tasksubmission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

// GenerationCompilerInput is the provider-independent intent required to
// freeze a generation request before Task submission.
type GenerationCompilerInput struct {
	Feature             string
	UserPrompt          string
	CompiledPrompt      string
	TemplateVersion     string
	TemplateFingerprint string
	Params              *tasks.WorkflowTaskParams
}

// CompileGenerationSnapshot resolves execution semantics without selecting a
// provider, account, model, or endpoint. The returned value is safe to retain
// as a Task fact.
func CompileGenerationSnapshot(input GenerationCompilerInput) (tasks.GenerationRequestSnapshot, error) {
	feature := strings.TrimSpace(input.Feature)
	if feature == "" {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler feature is required")
	}
	if strings.TrimSpace(input.TemplateFingerprint) == "" {
		input.TemplateFingerprint = TemplateFingerprint(input.TemplateVersion, input.CompiledPrompt)
	}
	if input.TemplateFingerprint == "" {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler template fingerprint is required")
	}
	if strings.TrimSpace(input.UserPrompt) != input.UserPrompt || strings.TrimSpace(input.CompiledPrompt) != input.CompiledPrompt {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler prompts must be normalized")
	}

	snapshot := tasks.GenerationRequestSnapshot{
		SchemaVersion:        tasks.CurrentParamsSchemaVersion,
		Feature:              feature,
		Primitive:            string(workflow.GenerationPrimitiveTextToImage),
		UserPrompt:           input.UserPrompt,
		CompiledPrompt:       input.CompiledPrompt,
		TemplateVersion:      strings.TrimSpace(input.TemplateVersion),
		TemplateFingerprint:  input.TemplateFingerprint,
		RequestedOutputCount: 1,
	}
	if input.Params == nil {
		return snapshot, nil
	}
	if input.Params.Controls != nil {
		controls := *input.Params.Controls
		if controls.PromptUpsampling != nil {
			value := *controls.PromptUpsampling
			controls.PromptUpsampling = &value
		}
		snapshot.Controls = &controls
		if controls.OutputCount > 0 {
			count, err := workflow.ParseRequestedOutputCount(controls.OutputCount)
			if err != nil {
				return tasks.GenerationRequestSnapshot{}, err
			}
			snapshot.RequestedOutputCount = count.Int()
		}
	}
	if feature == workflow.FeatureSlugRemoveBg || feature == workflow.FeatureSlugHDUpscale {
		snapshot.RequestedOutputCount = 1
		if snapshot.Controls != nil {
			snapshot.Controls.OutputCount = 1
		}
	}
	appendMedia := func(ref tasks.MediaRef) {
		if strings.TrimSpace(ref.URL) == "" {
			return
		}
		snapshot.Media = append(snapshot.Media, tasks.GenerationMediaIntent{
			InputIndex: len(snapshot.Media), Role: ref.Role, Ref: strings.TrimSpace(ref.URL), Weight: ref.Weight,
		})
	}
	if media := input.Params.Media; media != nil {
		for _, ref := range media.Inputs {
			appendMedia(ref)
		}
		for _, ref := range media.References {
			appendMedia(ref)
		}
		if media.Original != nil {
			appendMedia(*media.Original)
		}
		if media.Mask != nil {
			appendMedia(*media.Mask)
		}
	}
	if len(snapshot.Media) > 0 {
		snapshot.Primitive = string(workflow.GenerationPrimitiveImageToImage)
	}
	return snapshot, nil
}

// TemplateFingerprint is a stable content-addressed version for the current
// prompt template. The version is intentionally independent of providers.
func TemplateFingerprint(version, template string) string {
	payload, _ := json.Marshal(struct {
		Version string `json:"version"`
		Prompt  string `json:"prompt"`
	}{Version: strings.TrimSpace(version), Prompt: template})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
