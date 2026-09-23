package providers

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

var (
	imageInputRefKeys = []string{"image_urls", "reference_image_urls"}
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
	Outpaint         *tasks.OutpaintParams
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
	if p.Canonical != nil && p.Canonical.Outpaint != nil {
		outpaint := *p.Canonical.Outpaint
		request.Outpaint = &outpaint
	}
	return request, nil
}

func validateProviderTaskContract(params *tasks.WorkflowTaskParams, plan workflow.FeatureExecutionPlan) error {
	if params == nil || params.Generation == nil {
		return fmt.Errorf("AI image task requires generation snapshot")
	}
	generation := params.Generation
	runtimeInputs := plan.RuntimeInputs()
	if err := validateGenerationSnapshot(generation, plan, runtimeInputs); err != nil {
		return err
	}
	if err := validateGenerationMedia(generation.Media, runtimeInputs); err != nil {
		return err
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

func validateGenerationSnapshot(generation *tasks.GenerationRequestSnapshot, plan workflow.FeatureExecutionPlan, runtimeInputs workflow.RuntimeInputSemantics) error {
	if generation.SchemaVersion != tasks.CurrentParamsSchemaVersion ||
		generation.Feature != plan.Feature() ||
		generation.Primitive != string(runtimeInputs.GenerationPrimitive()) ||
		generation.RequestedOutputCount != plan.RequestedOutputCount().Int() {
		return fmt.Errorf("task generation snapshot does not match execution plan")
	}
	return nil
}

func validateGenerationMedia(media []tasks.GenerationMediaIntent, runtimeInputs workflow.RuntimeInputSemantics) error {
	generationImageCount := 0
	generationHasMask := false
	for index, item := range media {
		if item.InputIndex != index || item.AssetID <= 0 || !tasks.IsGenerationMediaRole(item.Role) {
			return fmt.Errorf("task generation media is invalid")
		}
		if item.Role == tasks.MediaRoleMask {
			generationHasMask = true
			continue
		}
		generationImageCount++
	}
	if generationImageCount != runtimeInputs.ImageInputCount() || generationHasMask != runtimeInputs.HasMaskInput() {
		return fmt.Errorf("task generation media facts do not match execution plan")
	}
	return nil
}

func validateCanonicalRuntimeControls(params *tasks.WorkflowTaskParams, contract workflow.RuntimeControlContract) error {
	if params == nil {
		return fmt.Errorf("task params are required")
	}
	if err := validateCanonicalControlPresence(params, contract); err != nil {
		return err
	}

	switch contract {
	case workflow.RuntimeControlsNone:
		return nil
	case workflow.RuntimeControlsVariation:
		return validateVariationControls(params.Variation)
	case workflow.RuntimeControlsOutpaint:
		return validateOutpaintControls(params.Outpaint)
	case workflow.RuntimeControlsUpscale:
		return validateUpscaleControls(params.Upscale)
	case workflow.RuntimeControlsFusion:
		return validateFusionControls(params)
	default:
		return fmt.Errorf("unsupported runtime control contract %q", contract)
	}
}

func validateCanonicalControlPresence(params *tasks.WorkflowTaskParams, contract workflow.RuntimeControlContract) error {
	variation, outpaint, upscale, fusion := false, false, false, false
	switch contract {
	case workflow.RuntimeControlsNone:
	case workflow.RuntimeControlsVariation:
		variation = true
	case workflow.RuntimeControlsOutpaint:
		outpaint = true
	case workflow.RuntimeControlsUpscale:
		upscale = true
	case workflow.RuntimeControlsFusion:
		fusion = true
	default:
		return fmt.Errorf("unsupported runtime control contract %q", contract)
	}
	if (params.Variation != nil) != variation ||
		(params.Outpaint != nil) != outpaint ||
		(params.Upscale != nil) != upscale ||
		(params.Fusion != nil) != fusion {
		return fmt.Errorf("task canonical controls do not match execution plan: expected %s", contract)
	}
	return nil
}

func validateVariationControls(variation *tasks.VariationsParams) error {
	strength := variation.Strength
	if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 || strength > 100 || strength != math.Round(strength) {
		return fmt.Errorf("task variation controls are invalid")
	}
	return nil
}

func validateOutpaintControls(outpaint *tasks.OutpaintParams) error {
	if outpaint.TargetWidth <= 0 || outpaint.TargetHeight <= 0 || outpaint.ImageOffsetX < 0 || outpaint.ImageOffsetY < 0 {
		return fmt.Errorf("task image outpaint controls are invalid")
	}
	return nil
}

func validateUpscaleControls(upscale *tasks.UpscaleParams) error {
	scale := upscale.Scale
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < 1 || scale > 4 || math.Abs(scale*10-math.Round(scale*10)) > 1e-9 {
		return fmt.Errorf("task upscale controls are invalid")
	}
	return nil
}

func validateFusionControls(params *tasks.WorkflowTaskParams) error {
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
	imageRefs := collectOrderedProviderRefs(providerParams, imageInputRefKeys)
	maskRefs := collectOrderedProviderRefs(providerParams, []string{"mask_image_urls", "mask_image_url"})
	canvasRef := firstProviderRef(providerParams, "canvas_image_ref")
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
		Scale:            readUpscaleScale(providerParams, controlContract),
	}
}

func readUpscaleScale(params map[string]interface{}, controlContract workflow.RuntimeControlContract) float64 {
	if controlContract != workflow.RuntimeControlsUpscale {
		return 0
	}
	contract, ok := workflow.ReadUpscaleContract(params)
	if !ok {
		return 0
	}
	return contract.Scale
}

func firstProviderRef(params map[string]interface{}, key string) string {
	refs := collectOrderedProviderRefs(params, []string{key})
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
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

func parseSizeFromParams(params map[string]interface{}) (int, int) {
	ratio, _ := params["ratio"].(string)
	quality, _ := params["quality"].(string)

	var w, h int

	// 自定义尺寸优先
	if cw, ok := readPositiveSizeParam(params, "custom_width"); ok {
		if ch, okh := readPositiveSizeParam(params, "custom_height"); okh {
			w, h = cw, ch
			// 根据清晰度缩放
			switch quality {
			case "2k":
				w, h = w*2, h*2
			case "4k":
				w, h = w*4, h*4
			}
			return w, h
		}
	}

	switch ratio {
	case "3:2":
		w, h = 1536, 1024
	case "16:9":
		w, h = 1792, 1024
	case "1:1":
		w, h = 1024, 1024
	case "2:3":
		w, h = 1024, 1536
	case "3:4":
		w, h = 1024, 1365
	case "9:16":
		w, h = 1024, 1792
	default:
		w, h = 1024, 1024
	}

	// 根据清晰度缩放
	switch quality {
	case "2k":
		w, h = w*2, h*2
	case "4k":
		w, h = w*4, h*4
	}

	return w, h
}

func readPositiveSizeParam(params map[string]interface{}, key string) (int, bool) {
	if params == nil {
		return 0, false
	}
	switch value := params[key].(type) {
	case float64:
		if value > 0 {
			return int(value), true
		}
	case float32:
		if value > 0 {
			return int(value), true
		}
	case int:
		if value > 0 {
			return value, true
		}
	case int64:
		if value > 0 {
			return int(value), true
		}
	case int32:
		if value > 0 {
			return int(value), true
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(value)); err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}
