package tasksubmission

import (
	"crypto/sha256"
	"encoding/binary"
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
	Plan                workflow.FeatureExecutionPlan
	UserPrompt          string
	SystemPrompt        string
	CompiledPrompt      string
	TemplateVersion     string
	TemplateFingerprint string
	SeedMaterial        string
	Params              *tasks.WorkflowTaskParams
}

// CompileGenerationSnapshot resolves execution semantics without selecting a
// provider, account, model, or endpoint. The returned value is safe to retain
// as a Task fact.
func CompileGenerationSnapshot(input GenerationCompilerInput) (tasks.GenerationRequestSnapshot, error) {
	if _, err := workflow.EncodeFeatureExecutionPlan(input.Plan); err != nil {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler execution plan: %w", err)
	}
	feature := input.Plan.Feature()
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
		Primitive:            string(input.Plan.RuntimeInputs().GenerationPrimitive()),
		UserPrompt:           input.UserPrompt,
		SystemPrompt:         input.SystemPrompt,
		CompiledPrompt:       input.CompiledPrompt,
		TemplateVersion:      strings.TrimSpace(input.TemplateVersion),
		TemplateFingerprint:  input.TemplateFingerprint,
		Seed:                 generationSeed(input.SeedMaterial, input.TemplateFingerprint, input.CompiledPrompt),
		RequestedOutputCount: input.Plan.RequestedOutputCount().Int(),
	}
	if input.Params == nil {
		if input.Plan.RuntimeInputs().ImageInputCount() != 0 || input.Plan.RuntimeInputs().HasMaskInput() {
			return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler media facts do not match execution plan")
		}
		return snapshot, nil
	}
	if input.Params.Controls != nil {
		controls := *input.Params.Controls
		if controls.PromptUpsampling != nil {
			value := *controls.PromptUpsampling
			controls.PromptUpsampling = &value
		}
		snapshot.Controls = &controls
		if controls.OutputCount > 0 && controls.OutputCount != snapshot.RequestedOutputCount {
			return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler output count does not match execution plan")
		}
	}
	appendMedia := func(ref tasks.MediaRef) {
		if strings.TrimSpace(ref.Source) == "" {
			return
		}
		snapshot.Media = append(snapshot.Media, tasks.GenerationMediaIntent{
			InputIndex: len(snapshot.Media), Role: ref.Role, AssetID: ref.AssetID, Weight: ref.Weight, Source: strings.TrimSpace(ref.Source),
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
	imageInputCount := 0
	hasMaskInput := false
	for _, media := range snapshot.Media {
		if !tasks.IsGenerationMediaRole(media.Role) {
			return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler media role is invalid")
		}
		if media.Role == tasks.MediaRoleMask {
			hasMaskInput = true
			continue
		}
		imageInputCount++
	}
	runtimeInputs := input.Plan.RuntimeInputs()
	if imageInputCount != runtimeInputs.ImageInputCount() || hasMaskInput != runtimeInputs.HasMaskInput() {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation compiler media facts do not match execution plan")
	}
	return snapshot, nil
}

func generationSeed(parts ...string) uint64 {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return binary.BigEndian.Uint64(sum[:8])
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
