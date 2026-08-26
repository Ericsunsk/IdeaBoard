package worker

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

var (
	openAIImageInputRefKeys = []string{"image_urls", "reference_image_urls"}
)

type providerRequest struct {
	Primitive        workflow.GenerationPrimitive
	Prompt           string
	SystemPrompt     string
	UserPrompt       string
	Seed             uint64
	Model            string
	Size             string
	OutputResolution string
	OutputCount      int
	ImageRefs        []string
	MaskRefs         []string
	CanvasRef        string
	Scale            float64
	OriginalWidth    int
	OriginalHeight   int
	Extend           *tasks.OutpaintParams
	OpenAI           openAIProviderControls
	Google           googleProviderControls
}

type openAIProviderControls struct {
	UseEditEndpoint      bool
	DropOriginalImageRef bool
	VariationStrength    *float64
}

type googleProviderControls struct {
	AspectRatio string
}

type providerTaskParams struct {
	Canonical     *tasks.WorkflowTaskParams
	ExecutionPlan *workflow.FeatureExecutionPlan
	Provider      map[string]interface{}
}

func newProviderTaskParams(params *tasks.WorkflowTaskParams, runtimeMedia *tasks.RuntimeMediaInputs) providerTaskParams {
	return providerTaskParams{
		Canonical: params,
		Provider:  workflow.BuildProviderTaskParams(params, runtimeMedia),
	}
}

func (p providerTaskParams) imageSize() (int, int) {
	return parseSizeFromParams(p.Provider)
}

func (p providerTaskParams) providerRequest(prompt string, resolvedModel string) (providerRequest, error) {
	if p.ExecutionPlan == nil {
		return providerRequest{}, fmt.Errorf("provider request requires a durable execution plan")
	}
	plan := *p.ExecutionPlan
	runtimeInputs := plan.RuntimeInputs()
	width, height := p.imageSize()
	outputResolution := plan.OutputResolution()
	if outputResolution == workflow.OutputResolutionDefault {
		outputResolution = workflow.OutputResolution1K
	}
	request := newProviderRequest(
		prompt, width, height, p.Provider, resolvedModel,
		resolvedProviderRequestFacts{
			primitive:        runtimeInputs.GenerationPrimitive(),
			outputCount:      plan.RequestedOutputCount().Int(),
			outputResolution: outputResolution,
		},
		plan.RuntimeOperation(), runtimeInputs.Invocation(), runtimeInputs.ControlContract(),
	)
	if p.Canonical != nil && p.Canonical.Generation != nil {
		request.SystemPrompt = p.Canonical.Generation.SystemPrompt
		request.UserPrompt = p.Canonical.Generation.UserPrompt
		request.Seed = p.Canonical.Generation.Seed
	}
	if p.Canonical != nil && p.Canonical.Controls != nil {
		request.OriginalWidth = int(math.Round(p.Canonical.Controls.OriginalWidth))
		request.OriginalHeight = int(math.Round(p.Canonical.Controls.OriginalHeight))
	}
	if p.Canonical != nil && p.Canonical.Extend != nil {
		extend := *p.Canonical.Extend
		request.Extend = &extend
	}
	return request, nil
}

func validateProviderTaskContract(params *tasks.WorkflowTaskParams, plan workflow.FeatureExecutionPlan) error {
	if params == nil || params.Generation == nil {
		return fmt.Errorf("AI image task requires generation snapshot")
	}
	generation := params.Generation
	runtimeInputs := plan.RuntimeInputs()
	if generation.SchemaVersion != tasks.CurrentParamsSchemaVersion ||
		generation.Feature != plan.Feature() ||
		generation.Primitive != string(runtimeInputs.GenerationPrimitive()) ||
		generation.RequestedOutputCount != plan.RequestedOutputCount().Int() {
		return fmt.Errorf("task generation snapshot does not match execution plan")
	}

	generationImageCount := 0
	generationHasMask := false
	for index, media := range generation.Media {
		if media.InputIndex != index || media.AssetID <= 0 || !tasks.IsGenerationMediaRole(media.Role) {
			return fmt.Errorf("task generation media is invalid")
		}
		if media.Role == tasks.MediaRoleMask {
			generationHasMask = true
			continue
		}
		generationImageCount++
	}
	if generationImageCount != runtimeInputs.ImageInputCount() || generationHasMask != runtimeInputs.HasMaskInput() {
		return fmt.Errorf("task generation media facts do not match execution plan")
	}

	if params.Controls == nil || params.Controls.OutputCount != plan.RequestedOutputCount().Int() {
		return fmt.Errorf("task output controls do not match execution plan")
	}
	mediaImageCount, mediaHasMask := providerMediaFacts(params.Media)
	if mediaImageCount != runtimeInputs.ImageInputCount() || mediaHasMask != runtimeInputs.HasMaskInput() {
		return fmt.Errorf("task provider media facts do not match execution plan")
	}
	if err := validateProviderMediaMatchesGeneration(params.Media, generation.Media); err != nil {
		return err
	}
	return validateCanonicalRuntimeControls(params, runtimeInputs.ControlContract())
}

func validateCanonicalRuntimeControls(params *tasks.WorkflowTaskParams, contract workflow.RuntimeControlContract) error {
	if params == nil {
		return fmt.Errorf("task params are required")
	}
	unexpected := func(expected string) error {
		return fmt.Errorf("task canonical controls do not match execution plan: expected %s", expected)
	}
	rejectOtherControls := func(variation, extend, hd, fusion bool) error {
		if (params.Variation != nil) != variation ||
			(params.Extend != nil) != extend ||
			(params.HD != nil) != hd ||
			(params.Fusion != nil) != fusion {
			return unexpected(string(contract))
		}
		return nil
	}

	switch contract {
	case workflow.RuntimeControlsNone:
		return rejectOtherControls(false, false, false, false)
	case workflow.RuntimeControlsVariation:
		if err := rejectOtherControls(true, false, false, false); err != nil {
			return err
		}
		strength := params.Variation.Strength
		if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 || strength > 100 || strength != math.Round(strength) {
			return fmt.Errorf("task variation controls are invalid")
		}
		return nil
	case workflow.RuntimeControlsExtend:
		if err := rejectOtherControls(false, true, false, false); err != nil {
			return err
		}
		extend := params.Extend
		if extend.TargetWidth <= 0 || extend.TargetHeight <= 0 || extend.ImageOffsetX < 0 || extend.ImageOffsetY < 0 {
			return fmt.Errorf("task image extend controls are invalid")
		}
		return nil
	case workflow.RuntimeControlsUpscale:
		if err := rejectOtherControls(false, false, true, false); err != nil {
			return err
		}
		scale := params.HD.Scale
		if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < 1 || scale > 4 || math.Abs(scale*10-math.Round(scale*10)) > 1e-9 {
			return fmt.Errorf("task upscale controls are invalid")
		}
		return nil
	case workflow.RuntimeControlsFusion:
		if err := rejectOtherControls(false, false, false, true); err != nil {
			return err
		}
		if strings.TrimSpace(params.Fusion.Mode) != "weighted_reference" || len(params.Fusion.InputWeights) == 0 {
			return fmt.Errorf("task fusion controls are invalid")
		}
		if params.Media == nil || len(params.Fusion.InputWeights) != len(params.Media.Inputs) {
			return fmt.Errorf("task fusion controls do not match input media")
		}
		for _, weight := range params.Fusion.InputWeights {
			if math.IsNaN(weight) || math.IsInf(weight, 0) {
				return fmt.Errorf("task fusion controls are invalid")
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported runtime control contract %q", contract)
	}
}

func providerMediaFacts(media *tasks.MediaInputs) (int, bool) {
	if media == nil {
		return 0, false
	}
	count := 0
	for _, ref := range media.Inputs {
		if ref.AssetID > 0 {
			count++
		}
	}
	for _, ref := range media.References {
		if ref.AssetID > 0 {
			count++
		}
	}
	if media.Original != nil && media.Original.AssetID > 0 {
		count++
	}
	hasMask := media.Mask != nil && media.Mask.AssetID > 0
	return count, hasMask
}

func validateProviderMediaMatchesGeneration(media *tasks.MediaInputs, generation []tasks.GenerationMediaIntent) error {
	refs := make([]tasks.MediaRef, 0, len(generation))
	if media != nil {
		refs = append(refs, media.Inputs...)
		refs = append(refs, media.References...)
		if media.Original != nil {
			refs = append(refs, *media.Original)
		}
		if media.Mask != nil {
			refs = append(refs, *media.Mask)
		}
	}
	if len(refs) != len(generation) {
		return fmt.Errorf("task provider media facts do not match generation snapshot")
	}
	for index, ref := range refs {
		intent := generation[index]
		if ref.AssetID <= 0 || ref.AssetID != intent.AssetID || ref.Role != intent.Role {
			return fmt.Errorf("task provider media fact %d does not match generation snapshot", index)
		}
	}
	return nil
}

type resolvedProviderRequestFacts struct {
	primitive        workflow.GenerationPrimitive
	outputCount      int
	outputResolution string
}

func newProviderRequest(
	prompt string,
	width, height int,
	params map[string]interface{},
	resolvedModel string,
	facts resolvedProviderRequestFacts,
	operation workflow.RuntimeOperation,
	invocation workflow.RuntimeInvocation,
	controlContract workflow.RuntimeControlContract,
) providerRequest {
	providerParams := params
	imageRefs := collectOrderedProviderRefs(providerParams, openAIImageInputRefKeys)
	maskRefs := collectOrderedProviderRefs(providerParams, []string{"mask_image_urls", "mask_image_url"})
	canvasRef := firstProviderRef(providerParams, "original_image_url")
	controls := compileProviderControls(providerParams, operation, invocation, controlContract)
	return providerRequest{
		Primitive:        facts.primitive,
		Prompt:           prompt,
		Model:            resolvedModel,
		Size:             providerImageSize(width, height),
		OutputResolution: facts.outputResolution,
		OutputCount:      facts.outputCount,
		ImageRefs:        imageRefs,
		MaskRefs:         maskRefs,
		CanvasRef:        canvasRef,
		Scale:            readHDUpscaleScale(providerParams, controlContract),
		OpenAI:           controls.OpenAI,
		Google:           controls.Google,
	}
}

func readHDUpscaleScale(params map[string]interface{}, controlContract workflow.RuntimeControlContract) float64 {
	if controlContract != workflow.RuntimeControlsUpscale {
		return 0
	}
	contract, ok := workflow.ReadHDUpscaleContract(params)
	if !ok {
		return 0
	}
	return contract.Scale
}

type compiledProviderControls struct {
	OpenAI openAIProviderControls
	Google googleProviderControls
}

func compileProviderControls(params map[string]interface{}, operation workflow.RuntimeOperation, invocation workflow.RuntimeInvocation, controlContract workflow.RuntimeControlContract) compiledProviderControls {
	controls := compiledProviderControls{Google: googleProviderControls{AspectRatio: parseGoogleAspectRatioValue(params)}}
	controls.OpenAI.UseEditEndpoint = invocation == workflow.RuntimeInvocationEdit
	controls.OpenAI.DropOriginalImageRef = operation == workflow.RuntimeOperationVariations
	if params == nil {
		return controls
	}
	switch controlContract {
	case workflow.RuntimeControlsVariation:
		if raw, ok := readProviderNumber(params, "variation_strength"); ok {
			controls.OpenAI.VariationStrength = float64Ptr(raw)
		}
	}
	return controls
}

func float64Ptr(value float64) *float64 { return &value }

func readProviderNumber(params map[string]interface{}, key string) (float64, bool) {
	raw, ok := workflow.ReadWorkflowParam(params, key)
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case float32:
		return float64(value), !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}

func firstProviderRef(params map[string]interface{}, key string) string {
	refs := collectOrderedProviderRefs(params, []string{key})
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

func parseGoogleAspectRatioValue(params map[string]interface{}) string {
	raw, ok := workflow.ReadWorkflowStringParam(params, "ratio")
	if !ok {
		return ""
	}
	ratio := strings.TrimSpace(raw)
	switch ratio {
	case "1:1", "16:9", "9:16", "3:4", "4:3":
		return ratio
	default:
		return ""
	}
}

func (r providerRequest) withOutputCount(count int) providerRequest {
	clone := r
	clone.OutputCount = count
	return clone
}

func providerImageSize(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func collectOrderedProviderRefs(params map[string]interface{}, keys []string) []string {
	refs := make([]string, 0, len(keys))
	seenFromEarlierKeys := make(map[string]struct{})
	for _, key := range keys {
		var values []string
		switch raw := params[key].(type) {
		case string:
			values = []string{raw}
		default:
			values = mediarefs.CollectStringList(raw)
		}
		currentKeyRefs := make([]string, 0, len(values))
		for _, raw := range values {
			if ref := strings.TrimSpace(raw); ref != "" {
				if _, duplicateAlias := seenFromEarlierKeys[ref]; !duplicateAlias {
					refs = append(refs, ref)
					currentKeyRefs = append(currentKeyRefs, ref)
				}
			}
		}
		for _, ref := range currentKeyRefs {
			seenFromEarlierKeys[ref] = struct{}{}
		}
	}
	return refs
}
