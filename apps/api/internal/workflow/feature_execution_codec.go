package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const CurrentFeatureExecutionPlanSchemaVersion = 5

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

type durableFeatureDefinition struct {
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

// durableFeatureDefinitionForCurrentSchema is an immutable schema snapshot. It must not
// consult the live compiler registry: semantic changes require a new schema.
func durableFeatureDefinitionForCurrentSchema(feature string) (durableFeatureDefinition, bool) {
	switch feature {
	case FeatureSlugGeneration:
		return durableImageGenerationDefinition(
			PromptPolicyRequired, RuntimeOperationGeneration, RuntimeInvocationGenerate,
			ImageInputOptional, 4, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsNone,
		), true
	case FeatureSlugInpaint:
		return durableImageFillDefinition(
			PromptPolicyRequired, RuntimeOperationInpaint,
			MaskInputRequired, RuntimeControlsNone,
		), true
	case FeatureSlugRender:
		return durableImageGenerationDefinition(
			PromptPolicyOptional, RuntimeOperationRender, RuntimeInvocationEdit,
			ImageInputRequired, 16, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsNone,
		), true
	case FeatureSlugVariations:
		return durableImageGenerationDefinition(
			PromptPolicyOptional, RuntimeOperationVariations, RuntimeInvocationEdit,
			ImageInputRequired, 16, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsVariation,
		), true
	case FeatureSlugFusion:
		return durableImageGenerationDefinition(
			PromptPolicyOptional, RuntimeOperationFusion, RuntimeInvocationEdit,
			MultipleImageInputsRequired, 4, MaskInputIgnored, OriginalImageExcluded, RuntimeControlsFusion,
		), true
	case FeatureSlugOutpaint:
		return durableImageFillDefinition(
			PromptPolicyOptional, RuntimeOperationOutpaint,
			MaskInputIgnored, RuntimeControlsOutpaint,
		), true
	case FeatureSlugCutout:
		return durableFeatureDefinition{
			promptPolicy: PromptPolicyIgnored, outputCountPolicy: OutputCountPolicyFixed,
			outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
			routingCapability:      RoutingCapabilityCutout, modelRequirement: RoutingModelNotRequired,
			runtimeOperation: RuntimeOperationCutout, runtimeInvocation: RuntimeInvocationTransform,
			imageInputPolicy: ImageInputRequired, maskInputPolicy: MaskInputIgnored,
			maxImageInputCount:  16,
			originalImagePolicy: OriginalImageIgnored, controlContract: RuntimeControlsNone,
		}, true
	case FeatureSlugUpscale:
		return durableFeatureDefinition{
			promptPolicy: PromptPolicyIgnored, outputCountPolicy: OutputCountPolicyFixed,
			outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
			routingCapability:      RoutingCapabilityUpscale, modelRequirement: RoutingModelNotRequired,
			runtimeOperation: RuntimeOperationUpscale, runtimeInvocation: RuntimeInvocationTransform,
			imageInputPolicy: ImageInputRequired, maskInputPolicy: MaskInputIgnored,
			maxImageInputCount:  16,
			originalImagePolicy: OriginalImageIgnored, controlContract: RuntimeControlsUpscale,
		}, true
	default:
		return durableFeatureDefinition{}, false
	}
}

func durableImageGenerationDefinition(
	promptPolicy PromptPolicy,
	operation RuntimeOperation,
	invocation RuntimeInvocation,
	imageInputPolicy ImageInputPolicy,
	maxImageInputCount int,
	maskInputPolicy MaskInputPolicy,
	originalImagePolicy OriginalImagePolicy,
	controlContract RuntimeControlContract,
) durableFeatureDefinition {
	return durableFeatureDefinition{
		promptPolicy: promptPolicy, outputCountPolicy: OutputCountPolicySelectable,
		outputResolutionPolicy: OutputResolutionPolicyScalable,
		routingCapability:      RoutingCapabilityGeneration, modelRequirement: RoutingModelRequired,
		runtimeOperation: operation, runtimeInvocation: invocation,
		imageInputPolicy: imageInputPolicy, maxImageInputCount: maxImageInputCount,
		maskInputPolicy:     maskInputPolicy,
		originalImagePolicy: originalImagePolicy, controlContract: controlContract,
	}
}

func durableImageFillDefinition(
	promptPolicy PromptPolicy,
	operation RuntimeOperation,
	maskInputPolicy MaskInputPolicy,
	controlContract RuntimeControlContract,
) durableFeatureDefinition {
	return durableFeatureDefinition{
		promptPolicy: promptPolicy, outputCountPolicy: OutputCountPolicyFixed,
		outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
		routingCapability:      RoutingCapabilityFill, modelRequirement: RoutingModelNotRequired,
		runtimeOperation: operation, runtimeInvocation: RuntimeInvocationTransform,
		imageInputPolicy: ImageInputRequired, maxImageInputCount: 1,
		maskInputPolicy:     maskInputPolicy,
		originalImagePolicy: OriginalImageExcluded, controlContract: controlContract,
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

func featureExecutionPlanFromDurable(envelope durableFeatureExecutionPlan) (FeatureExecutionPlan, error) {
	if envelope.SchemaVersion != CurrentFeatureExecutionPlanSchemaVersion {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractUnsupportedSchema, envelope.Feature, "schema_version", fmt.Sprintf("%d", envelope.SchemaVersion),
		)
	}
	definition, ok := durableFeatureDefinitionForCurrentSchema(envelope.Feature)
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

func validateDurableRuntimeV1(feature string, definition durableFeatureDefinition, runtime durableRuntimePlan) error {
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
