package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

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
	RoutingCapabilityImageGeneration   RoutingCapability = "image-generation"
	RoutingCapabilityBackgroundRemoval RoutingCapability = "background-removal"
	RoutingCapabilityImageUpscale      RoutingCapability = "image-upscale"
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
	RuntimeOperationPromptGeneration  RuntimeOperation = "prompt-generation"
	RuntimeOperationLocalInpaint      RuntimeOperation = "local-inpaint"
	RuntimeOperationSketchToImage     RuntimeOperation = "sketch-to-image"
	RuntimeOperationStyleVariation    RuntimeOperation = "style-variation"
	RuntimeOperationStyleFusion       RuntimeOperation = "style-fusion"
	RuntimeOperationImageExtend       RuntimeOperation = "image-extend"
	RuntimeOperationBackgroundRemoval RuntimeOperation = "background-removal"
	RuntimeOperationImageUpscale      RuntimeOperation = "image-upscale"
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
	OriginalImageIgnored            OriginalImagePolicy = "ignored"
	OriginalImageForwardWhenPresent OriginalImagePolicy = "forward-when-present"
	OriginalImageExcluded           OriginalImagePolicy = "excluded"
)

// RuntimeControlContract names typed Workflow controls required by the
// runtime operation. It never contains provider-specific passthrough data.
type RuntimeControlContract string

const (
	RuntimeControlsNone      RuntimeControlContract = "none"
	RuntimeControlsVariation RuntimeControlContract = "variation"
	RuntimeControlsFusion    RuntimeControlContract = "fusion"
	RuntimeControlsExtend    RuntimeControlContract = "extend"
	RuntimeControlsUpscale   RuntimeControlContract = "upscale"
)

// FeatureContractValidationCode is stable machine-readable validation output
// for callers that need to map Contract failures to their own error language.
type FeatureContractValidationCode string

const (
	FeatureContractUnknownFeature        FeatureContractValidationCode = "unknown_feature"
	FeatureContractInvalidOutputCount    FeatureContractValidationCode = "invalid_output_count"
	FeatureContractUnsupportedResolution FeatureContractValidationCode = "unsupported_output_resolution"
	FeatureContractInvalidRuntimeInput   FeatureContractValidationCode = "invalid_runtime_input"
	FeatureContractUnsupportedSchema     FeatureContractValidationCode = "unsupported_schema_version"
	FeatureContractInvalidDurablePlan    FeatureContractValidationCode = "invalid_durable_plan"
)

// FeatureContractValidationError contains no provider or persistence state.
type FeatureContractValidationError struct {
	Code    FeatureContractValidationCode
	Feature string
	Field   string
	Value   string
}

func (e *FeatureContractValidationError) Error() string {
	if e == nil {
		return "workflow feature contract validation failed"
	}
	return fmt.Sprintf("workflow feature contract validation failed: code=%s feature=%q field=%q value=%q", e.Code, e.Feature, e.Field, e.Value)
}

// FeatureExecutionParams contains only the optional user-selectable facts
// needed by the Contract. nil means the caller omitted that parameter.
type FeatureExecutionParams struct {
	RequestedOutputCount *int
	OutputResolution     *string
	ImageInputCount      int
	HasMaskInput         bool
}

type CompileFeatureExecutionInput struct {
	Feature string
	Params  FeatureExecutionParams
}

// FeatureExecutionPlan is an immutable, provider-independent execution fact.
// Its fields stay private so future callers cannot create internally
// inconsistent combinations of Feature, Routing, Output, and Runtime facts.
type FeatureExecutionPlan struct {
	feature                string
	promptPolicy           PromptPolicy
	outputCountPolicy      OutputCountPolicy
	requestedOutputCount   RequestedOutputCount
	outputResolutionPolicy OutputResolutionPolicy
	outputResolution       string
	routing                RoutingRequirement
	runtimeOperation       RuntimeOperation
	runtimeInputs          RuntimeInputSemantics
}

func (p FeatureExecutionPlan) Feature() string { return p.feature }

func (p FeatureExecutionPlan) PromptPolicy() PromptPolicy { return p.promptPolicy }

func (p FeatureExecutionPlan) OutputCountPolicy() OutputCountPolicy { return p.outputCountPolicy }

func (p FeatureExecutionPlan) RequestedOutputCount() RequestedOutputCount {
	return p.requestedOutputCount
}

func (p FeatureExecutionPlan) OutputResolutionPolicy() OutputResolutionPolicy {
	return p.outputResolutionPolicy
}

func (p FeatureExecutionPlan) OutputResolution() string { return p.outputResolution }

func (p FeatureExecutionPlan) RoutingRequirement() RoutingRequirement { return p.routing }

func (p FeatureExecutionPlan) RuntimeOperation() RuntimeOperation { return p.runtimeOperation }

func (p FeatureExecutionPlan) RuntimeInputs() RuntimeInputSemantics { return p.runtimeInputs }

// RuntimeInputSemantics is immutable Workflow intent used for adapter
// selection and request shaping.
type RuntimeInputSemantics struct {
	generationPrimitive GenerationPrimitive
	invocation          RuntimeInvocation
	imageInputCount     int
	hasMaskInput        bool
	imageInputPolicy    ImageInputPolicy
	maskInputPolicy     MaskInputPolicy
	originalImagePolicy OriginalImagePolicy
	controlContract     RuntimeControlContract
}

func (s RuntimeInputSemantics) GenerationPrimitive() GenerationPrimitive {
	return s.generationPrimitive
}

func (s RuntimeInputSemantics) Invocation() RuntimeInvocation { return s.invocation }

func (s RuntimeInputSemantics) ImageInputCount() int { return s.imageInputCount }

func (s RuntimeInputSemantics) HasMaskInput() bool { return s.hasMaskInput }

func (s RuntimeInputSemantics) ImageInputPolicy() ImageInputPolicy { return s.imageInputPolicy }

func (s RuntimeInputSemantics) MaskInputPolicy() MaskInputPolicy { return s.maskInputPolicy }

func (s RuntimeInputSemantics) OriginalImagePolicy() OriginalImagePolicy {
	return s.originalImagePolicy
}

func (s RuntimeInputSemantics) ControlContract() RuntimeControlContract {
	return s.controlContract
}

// RoutingRequirement is immutable and describes eligibility only. Account,
// Model, and Endpoint selection remain owned by routing callers.
type RoutingRequirement struct {
	capability       RoutingCapability
	modelRequirement RoutingModelRequirement
	outputResolution string
}

func (r RoutingRequirement) Capability() RoutingCapability { return r.capability }

func (r RoutingRequirement) ModelRequirement() RoutingModelRequirement {
	return r.modelRequirement
}

func (r RoutingRequirement) OutputResolution() string { return r.outputResolution }

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
	{slug: FeatureSlugPromptGeneration, definition: imageGenerationDefinition(
		PromptPolicyRequired, RuntimeOperationPromptGeneration, RuntimeInvocationGenerate,
		ImageInputOptional, 4, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsNone,
	)},
	{slug: FeatureSlugLocalInpaint, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationLocalInpaint, RuntimeInvocationEdit,
		ImageInputRequired, 16, MaskInputRequired, OriginalImageForwardWhenPresent, RuntimeControlsNone,
	)},
	{slug: FeatureSlugSketchToImage, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationSketchToImage, RuntimeInvocationEdit,
		ImageInputRequired, 16, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsNone,
	)},
	{slug: FeatureSlugStyleVariation, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationStyleVariation, RuntimeInvocationEdit,
		ImageInputRequired, 16, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsVariation,
	)},
	{slug: FeatureSlugRemoveBg, definition: featureExecutionDefinition{
		promptPolicy:           PromptPolicyIgnored,
		outputCountPolicy:      OutputCountPolicyFixed,
		outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
		routingCapability:      RoutingCapabilityBackgroundRemoval,
		modelRequirement:       RoutingModelNotRequired,
		runtimeOperation:       RuntimeOperationBackgroundRemoval,
		runtimeInvocation:      RuntimeInvocationTransform,
		imageInputPolicy:       ImageInputRequired,
		maxImageInputCount:     16,
		maskInputPolicy:        MaskInputIgnored,
		originalImagePolicy:    OriginalImageIgnored,
		controlContract:        RuntimeControlsNone,
	}},
	{slug: FeatureSlugStyleFusion, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationStyleFusion, RuntimeInvocationEdit,
		MultipleImageInputsRequired, 4, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsFusion,
	)},
	{slug: FeatureSlugHDUpscale, definition: featureExecutionDefinition{
		promptPolicy:           PromptPolicyIgnored,
		outputCountPolicy:      OutputCountPolicyFixed,
		outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
		routingCapability:      RoutingCapabilityImageUpscale,
		modelRequirement:       RoutingModelNotRequired,
		runtimeOperation:       RuntimeOperationImageUpscale,
		runtimeInvocation:      RuntimeInvocationTransform,
		imageInputPolicy:       ImageInputRequired,
		maxImageInputCount:     16,
		maskInputPolicy:        MaskInputIgnored,
		originalImagePolicy:    OriginalImageIgnored,
		controlContract:        RuntimeControlsUpscale,
	}},
	{slug: FeatureSlugImageExtend, definition: imageGenerationDefinition(
		PromptPolicyOptional, RuntimeOperationImageExtend, RuntimeInvocationEdit,
		ImageInputRequired, 16, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsExtend,
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
		routingCapability:      RoutingCapabilityImageGeneration,
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

// CompileFeatureExecutionPlan resolves all stable execution facts for one
// canonical AI image Workflow without I/O or runtime resource selection.
func CompileFeatureExecutionPlan(input CompileFeatureExecutionInput) (FeatureExecutionPlan, error) {
	feature, ok := ParseAIImageFeatureSlug(input.Feature)
	if !ok {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractUnknownFeature, strings.TrimSpace(input.Feature), "feature", input.Feature,
		)
	}
	definition, ok := imageFeatureExecutionDefinitions[feature]
	if !ok {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractUnknownFeature, feature, "feature", input.Feature,
		)
	}

	outputCount, err := compileRequestedOutputCount(feature, definition.outputCountPolicy, input.Params.RequestedOutputCount)
	if err != nil {
		return FeatureExecutionPlan{}, err
	}
	outputResolution, err := compileOutputResolution(feature, definition.outputResolutionPolicy, input.Params.OutputResolution)
	if err != nil {
		return FeatureExecutionPlan{}, err
	}
	if violation := validateRuntimeMediaPolicy(
		runtimeMediaPolicy{
			imageInputPolicy:   definition.imageInputPolicy,
			maxImageInputCount: definition.maxImageInputCount,
			maskInputPolicy:    definition.maskInputPolicy,
		},
		runtimeMediaFacts{
			imageInputCount: input.Params.ImageInputCount,
			hasMaskInput:    input.Params.HasMaskInput,
		},
	); violation != nil {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidRuntimeInput, feature, violation.field, violation.value,
		)
	}
	generationPrimitive := GenerationPrimitiveTextToImage
	if input.Params.ImageInputCount > 0 {
		generationPrimitive = GenerationPrimitiveImageToImage
	}
	runtimeInvocation := definition.runtimeInvocation
	if runtimeInvocation == RuntimeInvocationGenerate && generationPrimitive == GenerationPrimitiveImageToImage {
		runtimeInvocation = RuntimeInvocationEdit
	}

	return FeatureExecutionPlan{
		feature:                feature,
		promptPolicy:           definition.promptPolicy,
		outputCountPolicy:      definition.outputCountPolicy,
		requestedOutputCount:   outputCount,
		outputResolutionPolicy: definition.outputResolutionPolicy,
		outputResolution:       outputResolution,
		routing: RoutingRequirement{
			capability:       definition.routingCapability,
			modelRequirement: definition.modelRequirement,
			outputResolution: outputResolution,
		},
		runtimeOperation: definition.runtimeOperation,
		runtimeInputs: RuntimeInputSemantics{
			generationPrimitive: generationPrimitive,
			invocation:          runtimeInvocation,
			imageInputCount:     input.Params.ImageInputCount,
			hasMaskInput:        input.Params.HasMaskInput,
			imageInputPolicy:    definition.imageInputPolicy,
			maskInputPolicy:     definition.maskInputPolicy,
			originalImagePolicy: definition.originalImagePolicy,
			controlContract:     definition.controlContract,
		},
	}, nil
}

// CompileFeatureReadinessPlan resolves routing and Billing facts without
// requiring media that only exists when the user executes the operation.
func CompileFeatureReadinessPlan(input CompileFeatureExecutionInput) (FeatureExecutionPlan, error) {
	feature, ok := ParseAIImageFeatureSlug(input.Feature)
	if !ok {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractUnknownFeature, strings.TrimSpace(input.Feature), "feature", input.Feature,
		)
	}
	definition, ok := imageFeatureExecutionDefinitions[feature]
	if !ok {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractUnknownFeature, feature, "feature", input.Feature,
		)
	}

	params := input.Params
	switch definition.imageInputPolicy {
	case ImageInputOptional:
		params.ImageInputCount = 0
	case ImageInputRequired:
		params.ImageInputCount = 1
	case MultipleImageInputsRequired:
		params.ImageInputCount = 2
	}
	params.HasMaskInput = definition.maskInputPolicy == MaskInputRequired
	return CompileFeatureExecutionPlan(CompileFeatureExecutionInput{Feature: feature, Params: params})
}

const CurrentFeatureExecutionPlanSchemaVersion = 1

type durableFeatureExecutionPlan struct {
	SchemaVersion int                       `json:"schema_version"`
	Feature       string                    `json:"feature"`
	PromptPolicy  PromptPolicy              `json:"prompt_policy"`
	Output        durableOutputPlan         `json:"output"`
	Routing       durableRoutingRequirement `json:"routing"`
	Runtime       durableRuntimePlan        `json:"runtime"`
}

type durableOutputPlan struct {
	CountPolicy      OutputCountPolicy      `json:"count_policy"`
	RequestedCount   int                    `json:"requested_count"`
	ResolutionPolicy OutputResolutionPolicy `json:"resolution_policy"`
	Resolution       string                 `json:"resolution"`
}

type durableRoutingRequirement struct {
	Capability       RoutingCapability       `json:"capability"`
	ModelRequirement RoutingModelRequirement `json:"model_requirement"`
	OutputResolution string                  `json:"output_resolution"`
}

type durableRuntimePlan struct {
	Operation           RuntimeOperation       `json:"operation"`
	GenerationPrimitive GenerationPrimitive    `json:"generation_primitive"`
	Invocation          RuntimeInvocation      `json:"invocation"`
	ImageInputCount     int                    `json:"image_input_count"`
	HasMaskInput        bool                   `json:"has_mask_input"`
	ImageInputPolicy    ImageInputPolicy       `json:"image_input_policy"`
	MaskInputPolicy     MaskInputPolicy        `json:"mask_input_policy"`
	OriginalImagePolicy OriginalImagePolicy    `json:"original_image_policy"`
	ControlContract     RuntimeControlContract `json:"control_contract"`
}

type durableFeatureDefinitionV1 struct {
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

// durableFeatureDefinitionForV1 is an immutable schema snapshot. It must not
// consult the live compiler registry: semantic changes require a new schema.
func durableFeatureDefinitionForV1(feature string) (durableFeatureDefinitionV1, bool) {
	switch feature {
	case FeatureSlugPromptGeneration:
		return durableImageGenerationDefinitionV1(
			PromptPolicyRequired, RuntimeOperationPromptGeneration, RuntimeInvocationGenerate,
			ImageInputOptional, 4, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsNone,
		), true
	case FeatureSlugLocalInpaint:
		return durableImageGenerationDefinitionV1(
			PromptPolicyOptional, RuntimeOperationLocalInpaint, RuntimeInvocationEdit,
			ImageInputRequired, 16, MaskInputRequired, OriginalImageForwardWhenPresent, RuntimeControlsNone,
		), true
	case FeatureSlugSketchToImage:
		return durableImageGenerationDefinitionV1(
			PromptPolicyOptional, RuntimeOperationSketchToImage, RuntimeInvocationEdit,
			ImageInputRequired, 16, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsNone,
		), true
	case FeatureSlugStyleVariation:
		return durableImageGenerationDefinitionV1(
			PromptPolicyOptional, RuntimeOperationStyleVariation, RuntimeInvocationEdit,
			ImageInputRequired, 16, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsVariation,
		), true
	case FeatureSlugStyleFusion:
		return durableImageGenerationDefinitionV1(
			PromptPolicyOptional, RuntimeOperationStyleFusion, RuntimeInvocationEdit,
			MultipleImageInputsRequired, 4, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsFusion,
		), true
	case FeatureSlugImageExtend:
		return durableImageGenerationDefinitionV1(
			PromptPolicyOptional, RuntimeOperationImageExtend, RuntimeInvocationEdit,
			ImageInputRequired, 16, MaskInputIgnored, OriginalImageForwardWhenPresent, RuntimeControlsExtend,
		), true
	case FeatureSlugRemoveBg:
		return durableFeatureDefinitionV1{
			promptPolicy: PromptPolicyIgnored, outputCountPolicy: OutputCountPolicyFixed,
			outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
			routingCapability:      RoutingCapabilityBackgroundRemoval, modelRequirement: RoutingModelNotRequired,
			runtimeOperation: RuntimeOperationBackgroundRemoval, runtimeInvocation: RuntimeInvocationTransform,
			imageInputPolicy: ImageInputRequired, maskInputPolicy: MaskInputIgnored,
			maxImageInputCount:  16,
			originalImagePolicy: OriginalImageIgnored, controlContract: RuntimeControlsNone,
		}, true
	case FeatureSlugHDUpscale:
		return durableFeatureDefinitionV1{
			promptPolicy: PromptPolicyIgnored, outputCountPolicy: OutputCountPolicyFixed,
			outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
			routingCapability:      RoutingCapabilityImageUpscale, modelRequirement: RoutingModelNotRequired,
			runtimeOperation: RuntimeOperationImageUpscale, runtimeInvocation: RuntimeInvocationTransform,
			imageInputPolicy: ImageInputRequired, maskInputPolicy: MaskInputIgnored,
			maxImageInputCount:  16,
			originalImagePolicy: OriginalImageIgnored, controlContract: RuntimeControlsUpscale,
		}, true
	default:
		return durableFeatureDefinitionV1{}, false
	}
}

func durableImageGenerationDefinitionV1(
	promptPolicy PromptPolicy,
	operation RuntimeOperation,
	invocation RuntimeInvocation,
	imageInputPolicy ImageInputPolicy,
	maxImageInputCount int,
	maskInputPolicy MaskInputPolicy,
	originalImagePolicy OriginalImagePolicy,
	controlContract RuntimeControlContract,
) durableFeatureDefinitionV1 {
	return durableFeatureDefinitionV1{
		promptPolicy: promptPolicy, outputCountPolicy: OutputCountPolicySelectable,
		outputResolutionPolicy: OutputResolutionPolicyScalable,
		routingCapability:      RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
		runtimeOperation: operation, runtimeInvocation: invocation,
		imageInputPolicy: imageInputPolicy, maxImageInputCount: maxImageInputCount,
		maskInputPolicy:     maskInputPolicy,
		originalImagePolicy: originalImagePolicy, controlContract: controlContract,
	}
}

// EncodeFeatureExecutionPlan is the only durable encoding seam for a Plan.
// The explicit envelope prevents private implementation fields from silently
// encoding as an empty JSON object.
func EncodeFeatureExecutionPlan(plan FeatureExecutionPlan) ([]byte, error) {
	envelope := durableFeatureExecutionPlanFromPlan(plan)
	if _, err := featureExecutionPlanFromDurable(envelope); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, plan.Feature(), "payload", err.Error(),
		)
	}
	return payload, nil
}

func (p FeatureExecutionPlan) MarshalJSON() ([]byte, error) {
	return EncodeFeatureExecutionPlan(p)
}

func (p *FeatureExecutionPlan) UnmarshalJSON(payload []byte) error {
	if p == nil {
		return newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, "", "plan", "nil decode target",
		)
	}
	plan, err := DecodeFeatureExecutionPlan(payload)
	if err != nil {
		return err
	}
	*p = plan
	return nil
}

// DecodeFeatureExecutionPlan accepts only the current complete schema. Unknown
// fields, enum values, trailing JSON, and inconsistent facts fail closed.
func DecodeFeatureExecutionPlan(payload []byte) (FeatureExecutionPlan, error) {
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, "", "payload", err.Error(),
		)
	}
	if err := validateDurableRequiredFields(payload); err != nil {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, "", "payload", err.Error(),
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope durableFeatureExecutionPlan
	if err := decoder.Decode(&envelope); err != nil {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, "", "payload", err.Error(),
		)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, envelope.Feature, "payload", err.Error(),
		)
	}
	return featureExecutionPlanFromDurable(envelope)
}

func validateDurableRequiredFields(payload []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(payload, &top); err != nil {
		return err
	}
	if err := requireJSONKeys(top, "plan", []string{
		"schema_version", "feature", "prompt_policy", "output", "routing", "runtime",
	}); err != nil {
		return err
	}
	if err := requireNestedJSONKeys(top, "output", []string{
		"count_policy", "requested_count", "resolution_policy", "resolution",
	}); err != nil {
		return err
	}
	if err := requireNestedJSONKeys(top, "routing", []string{
		"capability", "model_requirement", "output_resolution",
	}); err != nil {
		return err
	}
	return requireNestedJSONKeys(top, "runtime", []string{
		"operation", "generation_primitive", "invocation",
		"image_input_count", "has_mask_input", "image_input_policy", "mask_input_policy",
		"original_image_policy", "control_contract",
	})
}

func requireNestedJSONKeys(parent map[string]json.RawMessage, key string, required []string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(parent[key], &object); err != nil {
		return fmt.Errorf("%s must be an object: %w", key, err)
	}
	return requireJSONKeys(object, key, required)
}

func requireJSONKeys(object map[string]json.RawMessage, path string, required []string) error {
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("%s is missing required field %q", path, key)
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := consumeUniqueJSONValue(decoder); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value is not allowed")
		}
		return err
	}
	return nil
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, duplicate := keys[key]; duplicate {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("invalid JSON object closing token")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("invalid JSON array closing token")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value is not allowed")
		}
		return err
	}
	return nil
}

func featureExecutionPlanFromDurable(envelope durableFeatureExecutionPlan) (FeatureExecutionPlan, error) {
	if envelope.SchemaVersion != CurrentFeatureExecutionPlanSchemaVersion {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractUnsupportedSchema, envelope.Feature, "schema_version", fmt.Sprintf("%d", envelope.SchemaVersion),
		)
	}
	definition, ok := durableFeatureDefinitionForV1(envelope.Feature)
	if !ok {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, envelope.Feature, "feature", envelope.Feature,
		)
	}
	if envelope.PromptPolicy != definition.promptPolicy ||
		envelope.Output.CountPolicy != definition.outputCountPolicy ||
		envelope.Output.ResolutionPolicy != definition.outputResolutionPolicy ||
		envelope.Routing.Capability != definition.routingCapability ||
		envelope.Routing.ModelRequirement != definition.modelRequirement ||
		envelope.Runtime.Operation != definition.runtimeOperation ||
		envelope.Runtime.ImageInputPolicy != definition.imageInputPolicy ||
		envelope.Runtime.MaskInputPolicy != definition.maskInputPolicy ||
		envelope.Runtime.OriginalImagePolicy != definition.originalImagePolicy ||
		envelope.Runtime.ControlContract != definition.controlContract {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, envelope.Feature, "facts", "inconsistent or unknown execution facts",
		)
	}
	if err := validateDurableOutputV1(envelope.Feature, envelope.Output, envelope.Routing); err != nil {
		return FeatureExecutionPlan{}, err
	}
	if err := validateDurableRuntimeV1(envelope.Feature, definition, envelope.Runtime); err != nil {
		return FeatureExecutionPlan{}, err
	}
	return FeatureExecutionPlan{
		feature: envelope.Feature, promptPolicy: envelope.PromptPolicy,
		outputCountPolicy:      envelope.Output.CountPolicy,
		requestedOutputCount:   RequestedOutputCount(envelope.Output.RequestedCount),
		outputResolutionPolicy: envelope.Output.ResolutionPolicy,
		outputResolution:       envelope.Output.Resolution,
		routing: RoutingRequirement{
			capability: envelope.Routing.Capability, modelRequirement: envelope.Routing.ModelRequirement,
			outputResolution: envelope.Routing.OutputResolution,
		},
		runtimeOperation: envelope.Runtime.Operation,
		runtimeInputs: RuntimeInputSemantics{
			generationPrimitive: envelope.Runtime.GenerationPrimitive,
			invocation:          envelope.Runtime.Invocation,
			imageInputCount:     envelope.Runtime.ImageInputCount,
			hasMaskInput:        envelope.Runtime.HasMaskInput,
			imageInputPolicy:    envelope.Runtime.ImageInputPolicy,
			maskInputPolicy:     envelope.Runtime.MaskInputPolicy,
			originalImagePolicy: envelope.Runtime.OriginalImagePolicy,
			controlContract:     envelope.Runtime.ControlContract,
		},
	}, nil
}

func validateDurableOutputV1(feature string, output durableOutputPlan, routing durableRoutingRequirement) error {
	if output.RequestedCount < MinRequestedOutputCount || output.RequestedCount > MaxRequestedOutputCount {
		return newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, feature, "output.requested_count", fmt.Sprintf("%d", output.RequestedCount),
		)
	}
	if output.CountPolicy == OutputCountPolicyFixed && output.RequestedCount != 1 {
		return newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, feature, "output.requested_count", fmt.Sprintf("%d", output.RequestedCount),
		)
	}
	validResolution := false
	switch output.ResolutionPolicy {
	case OutputResolutionPolicyDefaultOnly:
		validResolution = output.Resolution == OutputResolutionDefault
	case OutputResolutionPolicyScalable:
		switch output.Resolution {
		case OutputResolutionDefault, OutputResolution1K, OutputResolution2K, OutputResolution4K:
			validResolution = true
		}
	}
	if !validResolution || routing.OutputResolution != output.Resolution {
		return newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, feature, "output.resolution", output.Resolution,
		)
	}
	return nil
}

func validateDurableRuntimeV1(feature string, definition durableFeatureDefinitionV1, runtime durableRuntimePlan) error {
	if violation := validateRuntimeMediaPolicy(
		runtimeMediaPolicy{
			imageInputPolicy:   definition.imageInputPolicy,
			maxImageInputCount: definition.maxImageInputCount,
			maskInputPolicy:    definition.maskInputPolicy,
		},
		runtimeMediaFacts{
			imageInputCount: runtime.ImageInputCount,
			hasMaskInput:    runtime.HasMaskInput,
		},
	); violation != nil {
		return newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, feature, "runtime."+violation.field, violation.value,
		)
	}
	expectedPrimitive := GenerationPrimitiveTextToImage
	if runtime.ImageInputCount > 0 {
		expectedPrimitive = GenerationPrimitiveImageToImage
	}
	expectedInvocation := definition.runtimeInvocation
	if expectedInvocation == RuntimeInvocationGenerate && expectedPrimitive == GenerationPrimitiveImageToImage {
		expectedInvocation = RuntimeInvocationEdit
	}
	if runtime.GenerationPrimitive != expectedPrimitive || runtime.Invocation != expectedInvocation {
		return newFeatureContractValidationError(
			FeatureContractInvalidDurablePlan, feature, "runtime.execution", "primitive or invocation is inconsistent with media facts",
		)
	}
	return nil
}

func durableFeatureExecutionPlanFromPlan(plan FeatureExecutionPlan) durableFeatureExecutionPlan {
	routing := plan.RoutingRequirement()
	runtimeInputs := plan.RuntimeInputs()
	return durableFeatureExecutionPlan{
		SchemaVersion: CurrentFeatureExecutionPlanSchemaVersion,
		Feature:       plan.Feature(),
		PromptPolicy:  plan.PromptPolicy(),
		Output: durableOutputPlan{
			CountPolicy:      plan.OutputCountPolicy(),
			RequestedCount:   plan.RequestedOutputCount().Int(),
			ResolutionPolicy: plan.OutputResolutionPolicy(),
			Resolution:       plan.OutputResolution(),
		},
		Routing: durableRoutingRequirement{
			Capability:       routing.Capability(),
			ModelRequirement: routing.ModelRequirement(),
			OutputResolution: routing.OutputResolution(),
		},
		Runtime: durableRuntimePlan{
			Operation:           plan.RuntimeOperation(),
			GenerationPrimitive: runtimeInputs.GenerationPrimitive(),
			Invocation:          runtimeInputs.Invocation(),
			ImageInputCount:     runtimeInputs.ImageInputCount(),
			HasMaskInput:        runtimeInputs.HasMaskInput(),
			ImageInputPolicy:    runtimeInputs.ImageInputPolicy(),
			MaskInputPolicy:     runtimeInputs.MaskInputPolicy(),
			OriginalImagePolicy: runtimeInputs.OriginalImagePolicy(),
			ControlContract:     runtimeInputs.ControlContract(),
		},
	}
}

type runtimeMediaPolicy struct {
	imageInputPolicy   ImageInputPolicy
	maxImageInputCount int
	maskInputPolicy    MaskInputPolicy
}

type runtimeMediaFacts struct {
	imageInputCount int
	hasMaskInput    bool
}

type runtimeMediaViolation struct {
	field string
	value string
}

// validateRuntimeMediaPolicy is shared by live compilation and versioned
// durable validation. It contains no registry or error-transport knowledge.
func validateRuntimeMediaPolicy(policy runtimeMediaPolicy, facts runtimeMediaFacts) *runtimeMediaViolation {
	minImages := 0
	switch policy.imageInputPolicy {
	case ImageInputOptional:
	case ImageInputRequired:
		minImages = 1
	case MultipleImageInputsRequired:
		minImages = 2
	default:
		return &runtimeMediaViolation{field: "image_input_policy", value: string(policy.imageInputPolicy)}
	}
	if policy.maxImageInputCount < minImages {
		return &runtimeMediaViolation{field: "max_image_input_count", value: fmt.Sprintf("%d", policy.maxImageInputCount)}
	}
	if facts.imageInputCount < minImages || facts.imageInputCount > policy.maxImageInputCount {
		return &runtimeMediaViolation{field: "image_input_count", value: fmt.Sprintf("%d", facts.imageInputCount)}
	}
	switch policy.maskInputPolicy {
	case MaskInputRequired:
		if !facts.hasMaskInput {
			return &runtimeMediaViolation{field: "mask_input", value: "required"}
		}
	case MaskInputIgnored:
		if facts.hasMaskInput {
			return &runtimeMediaViolation{field: "mask_input", value: "not-supported"}
		}
	default:
		return &runtimeMediaViolation{field: "mask_input_policy", value: string(policy.maskInputPolicy)}
	}
	return nil
}

func compileRequestedOutputCount(feature string, policy OutputCountPolicy, raw *int) (RequestedOutputCount, error) {
	value := MinRequestedOutputCount
	if raw != nil {
		value = *raw
	}
	if policy == OutputCountPolicyFixed && value != MinRequestedOutputCount {
		return 0, newFeatureContractValidationError(
			FeatureContractInvalidOutputCount, feature, "output_count", fmt.Sprintf("%d", value),
		)
	}
	count, err := ParseRequestedOutputCount(value)
	if err != nil {
		return 0, newFeatureContractValidationError(
			FeatureContractInvalidOutputCount, feature, "output_count", fmt.Sprintf("%d", value),
		)
	}
	return count, nil
}

func compileOutputResolution(feature string, policy OutputResolutionPolicy, raw *string) (string, error) {
	value := ""
	if raw != nil {
		value = *raw
	}
	resolution, ok := NormalizeOutputResolution(value)
	if policy == OutputResolutionPolicyDefaultOnly && strings.TrimSpace(value) == "" {
		resolution, ok = OutputResolutionDefault, true
	}
	if !ok || policy == OutputResolutionPolicyDefaultOnly && resolution != OutputResolutionDefault {
		return "", newFeatureContractValidationError(
			FeatureContractUnsupportedResolution, feature, "output_resolution", value,
		)
	}
	return resolution, nil
}

func newFeatureContractValidationError(code FeatureContractValidationCode, feature, field, value string) error {
	return &FeatureContractValidationError{Code: code, Feature: feature, Field: field, Value: value}
}
