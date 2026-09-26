package workflow

// PromptPolicy describes how a Workflow feature consumes Prompt facts. It
// never contains Prompt content or performs Prompt composition.
type PromptPolicy string

const (
	PromptPolicyOptional PromptPolicy = "optional"
	PromptPolicyRequired PromptPolicy = "required"
	PromptPolicyIgnored  PromptPolicy = "ignored"
)

// OutputCountPolicy describes whether callers may choose the number of stable
// result slots reserved for a Task.
type OutputCountPolicy string

const (
	OutputCountPolicySelectable OutputCountPolicy = "selectable"
	OutputCountPolicyFixed      OutputCountPolicy = "fixed"
)

// OutputResolutionPolicy describes the Workflow-level resolution choices. It
// is independent of whether a particular Provider Account is currently able
// to satisfy the resulting RoutingRequirement.
type OutputResolutionPolicy string

const (
	OutputResolutionPolicyScalable    OutputResolutionPolicy = "scalable"
	OutputResolutionPolicyDefaultOnly OutputResolutionPolicy = "default-only"
)

// RoutingCapability is the provider-independent capability that an eligible
// execution route must advertise.
type RoutingCapability string

const (
	RoutingCapabilityGeneration RoutingCapability = "generation"
	RoutingCapabilityCutout     RoutingCapability = "cutout"
	RoutingCapabilityUpscale    RoutingCapability = "upscale"
	RoutingCapabilityFill       RoutingCapability = "fill"
)

// RoutingModelRequirement states whether routing must resolve a Model. It does
// not select or read any Provider Account, Model, or Endpoint.
type RoutingModelRequirement string

const (
	RoutingModelRequired    RoutingModelRequirement = "required"
	RoutingModelNotRequired RoutingModelRequirement = "not-required"
)

// RuntimeOperation is the semantic operation that a runtime adapter must
// execute. Protocol and payload selection remain runtime implementation facts.
type RuntimeOperation string

const (
	RuntimeOperationGeneration RuntimeOperation = "generation"
	RuntimeOperationInpaint    RuntimeOperation = "inpaint"
	RuntimeOperationRender     RuntimeOperation = "render"
	RuntimeOperationVariations RuntimeOperation = "variations"
	RuntimeOperationFusion     RuntimeOperation = "fusion"
	RuntimeOperationOutpaint   RuntimeOperation = "outpaint"
	RuntimeOperationCutout     RuntimeOperation = "cutout"
	RuntimeOperationUpscale    RuntimeOperation = "upscale"
)

// RuntimeInvocation describes the provider-neutral request shape selected by
// a runtime adapter. It deliberately does not name a protocol or Endpoint.
type RuntimeInvocation string

const (
	RuntimeInvocationGenerate  RuntimeInvocation = "generate"
	RuntimeInvocationEdit      RuntimeInvocation = "edit"
	RuntimeInvocationTransform RuntimeInvocation = "transform"
)

type ImageInputPolicy string

const (
	ImageInputOptional          ImageInputPolicy = "optional"
	ImageInputRequired          ImageInputPolicy = "required"
	MultipleImageInputsRequired ImageInputPolicy = "multiple-required"
)

type MaskInputPolicy string

const (
	MaskInputIgnored  MaskInputPolicy = "ignored"
	MaskInputRequired MaskInputPolicy = "required"
)

type OriginalImagePolicy string

const (
	OriginalImageIgnored  OriginalImagePolicy = "ignored"
	OriginalImageExcluded OriginalImagePolicy = "excluded"
)

// RuntimeControlContract names typed Workflow controls required by the
// runtime operation. It never contains provider-specific passthrough data.
type RuntimeControlContract string

const (
	RuntimeControlsNone      RuntimeControlContract = "none"
	RuntimeControlsVariation RuntimeControlContract = "variation"
	RuntimeControlsFusion    RuntimeControlContract = "fusion"
	RuntimeControlsOutpaint  RuntimeControlContract = "outpaint"
	RuntimeControlsUpscale   RuntimeControlContract = "upscale"
)

type featureExecutionDefinition struct {
	promptPolicy           PromptPolicy
	outputCountPolicy      OutputCountPolicy
	outputResolutionPolicy OutputResolutionPolicy
	routingCapability      RoutingCapability
	modelRequirement       RoutingModelRequirement
	runtimeOperation       RuntimeOperation
	runtimeInvocation      RuntimeInvocation
	imageInputPolicy       ImageInputPolicy
	maxImageInputCount     int
	maskInputPolicy        MaskInputPolicy
	originalImagePolicy    OriginalImagePolicy
	controlContract        RuntimeControlContract
}

type registeredImageFeature struct {
	slug       string
	definition featureExecutionDefinition
}

var registeredImageFeatures = []registeredImageFeature{
	{slug: FeatureSlugGeneration, definition: imageGenerationDefinition(
		PromptPolicyRequired, RuntimeOperationGeneration, RuntimeInvocationGenerate,
		ImageInputOptional, 4, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsNone,
	)},
	{slug: FeatureSlugInpaint, definition: imageFillDefinition(
		PromptPolicyRequired, RuntimeOperationInpaint,
		ImageInputRequired, MaskInputRequired, OriginalImageExcluded, RuntimeControlsNone,
	)},
	{slug: FeatureSlugRender, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationRender, RuntimeInvocationEdit,
		ImageInputRequired, 16, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsNone,
	)},
	{slug: FeatureSlugVariations, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationVariations, RuntimeInvocationEdit,
		ImageInputRequired, 16, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsVariation,
	)},
	{slug: FeatureSlugCutout, definition: featureExecutionDefinition{
		promptPolicy:           PromptPolicyIgnored,
		outputCountPolicy:      OutputCountPolicyFixed,
		outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
		routingCapability:      RoutingCapabilityCutout,
		modelRequirement:       RoutingModelNotRequired,
		runtimeOperation:       RuntimeOperationCutout,
		runtimeInvocation:      RuntimeInvocationTransform,
		imageInputPolicy:       ImageInputRequired,
		maxImageInputCount:     16,
		maskInputPolicy:        MaskInputIgnored,
		originalImagePolicy:    OriginalImageIgnored,
		controlContract:        RuntimeControlsNone,
	}},
	{slug: FeatureSlugFusion, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationFusion, RuntimeInvocationEdit,
		MultipleImageInputsRequired, 4, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsFusion,
	)},
	{slug: FeatureSlugUpscale, definition: featureExecutionDefinition{
		promptPolicy:           PromptPolicyIgnored,
		outputCountPolicy:      OutputCountPolicyFixed,
		outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
		routingCapability:      RoutingCapabilityUpscale,
		modelRequirement:       RoutingModelNotRequired,
		runtimeOperation:       RuntimeOperationUpscale,
		runtimeInvocation:      RuntimeInvocationTransform,
		imageInputPolicy:       ImageInputRequired,
		maxImageInputCount:     16,
		maskInputPolicy:        MaskInputIgnored,
		originalImagePolicy:    OriginalImageIgnored,
		controlContract:        RuntimeControlsUpscale,
	}},
	{slug: FeatureSlugOutpaint, definition: imageFillDefinition(
		PromptPolicyOptional, RuntimeOperationOutpaint,
		ImageInputRequired, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsOutpaint,
	)},
}

var imageFeatureExecutionDefinitions = func() map[string]featureExecutionDefinition {
	definitions := make(map[string]featureExecutionDefinition, len(registeredImageFeatures))
	for _, feature := range registeredImageFeatures {
		definitions[feature.slug] = feature.definition
	}
	return definitions
}()

func registeredImageFeatureSlugs() []string {
	slugs := make([]string, 0, len(registeredImageFeatures))
	for _, feature := range registeredImageFeatures {
		slugs = append(slugs, feature.slug)
	}
	return slugs
}

func imageGenerationDefinition(
	promptPolicy PromptPolicy,
	operation RuntimeOperation,
	invocation RuntimeInvocation,
	imageInputPolicy ImageInputPolicy,
	maxImageInputCount int,
	maskInputPolicy MaskInputPolicy,
	originalImagePolicy OriginalImagePolicy,
	controlContract RuntimeControlContract,
) featureExecutionDefinition {
	return featureExecutionDefinition{
		promptPolicy:           promptPolicy,
		outputCountPolicy:      OutputCountPolicySelectable,
		outputResolutionPolicy: OutputResolutionPolicyScalable,
		routingCapability:      RoutingCapabilityGeneration,
		modelRequirement:       RoutingModelRequired,
		runtimeOperation:       operation,
		runtimeInvocation:      invocation,
		imageInputPolicy:       imageInputPolicy,
		maxImageInputCount:     maxImageInputCount,
		maskInputPolicy:        maskInputPolicy,
		originalImagePolicy:    originalImagePolicy,
		controlContract:        controlContract,
	}
}

func imageFillDefinition(
	promptPolicy PromptPolicy,
	operation RuntimeOperation,
	imageInputPolicy ImageInputPolicy,
	maskInputPolicy MaskInputPolicy,
	originalImagePolicy OriginalImagePolicy,
	controlContract RuntimeControlContract,
) featureExecutionDefinition {
	return featureExecutionDefinition{
		promptPolicy:           promptPolicy,
		outputCountPolicy:      OutputCountPolicyFixed,
		outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
		routingCapability:      RoutingCapabilityFill,
		modelRequirement:       RoutingModelNotRequired,
		runtimeOperation:       operation,
		runtimeInvocation:      RuntimeInvocationTransform,
		imageInputPolicy:       imageInputPolicy,
		maxImageInputCount:     1,
		maskInputPolicy:        maskInputPolicy,
		originalImagePolicy:    originalImagePolicy,
		controlContract:        controlContract,
	}
}

// SupportedFeatureOutputResolutions exposes the Workflow-owned output policy
// without requiring Billing or UI adapters to duplicate feature decisions.
func SupportedFeatureOutputResolutions(feature string) []string {
	canonical, ok := ParseAIImageFeatureSlug(feature)
	if !ok {
		return nil
	}
	definition, ok := imageFeatureExecutionDefinitions[canonical]
	if !ok {
		return nil
	}
	switch definition.outputResolutionPolicy {
	case OutputResolutionPolicyDefaultOnly:
		return []string{OutputResolutionDefault}
	case OutputResolutionPolicyScalable:
		return []string{
			OutputResolution1K,
			OutputResolution2K,
			OutputResolution4K,
			OutputResolutionDefault,
		}
	default:
		return nil
	}
}
