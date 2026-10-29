package workflow

import (
	"fmt"
	"slices"
	"strings"
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

// ReadOption tunes execution-plan compilation without forking entry
// points. The zero value reads live caller media facts; ReadReady
// synthesizes contract-valid media facts from policy for
// readiness/quote paths where user media does not exist yet.
type ReadOption int

const (
	// ReadLive compiles against the caller's media facts (submit path).
	ReadLive ReadOption = iota
	// ReadReady synthesizes media facts from policy (readiness path).
	ReadReady
)

// CompileFeatureExecutionPlan resolves all stable execution facts for one
// canonical AI image Workflow without I/O or runtime resource selection.
// Output count and resolution are determined here exactly once
// (compileRequestedOutputCount/compileOutputResolution); downstream
// stamping (canonical_intake.finalizeSubmitTypedParams) only propagates
// these compiled facts into the typed map so persisted Controls cannot
// disagree with the plan.
func CompileFeatureExecutionPlan(input CompileFeatureExecutionInput, opts ...ReadOption) (FeatureExecutionPlan, error) {
	ready := slices.Contains(opts, ReadReady)
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
	if ready {
		// Readiness synthesis (moved from CompileFeatureReadinessPlan):
		// resolve routing and Billing facts without requiring media
		// that only exists when the user executes the operation.
		switch definition.imageInputPolicy {
		case ImageInputOptional:
			params.ImageInputCount = 0
		case ImageInputRequired:
			params.ImageInputCount = 1
		case MultipleImageInputsRequired:
			params.ImageInputCount = 2
		}
		params.HasMaskInput = definition.maskInputPolicy == MaskInputRequired
	}

	outputCount, err := compileRequestedOutputCount(feature, definition.outputCountPolicy, params.RequestedOutputCount)
	if err != nil {
		return FeatureExecutionPlan{}, err
	}
	outputResolution, err := compileOutputResolution(feature, definition.outputResolutionPolicy, params.OutputResolution)
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
			imageInputCount: params.ImageInputCount,
			hasMaskInput:    params.HasMaskInput,
		},
	); violation != nil {
		return FeatureExecutionPlan{}, newFeatureContractValidationError(
			FeatureContractInvalidRuntimeInput, feature, violation.field, violation.value,
		)
	}
	generationPrimitive := GenerationPrimitiveTextToImage
	if params.ImageInputCount > 0 {
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
			imageInputCount:     params.ImageInputCount,
			hasMaskInput:        params.HasMaskInput,
			imageInputPolicy:    definition.imageInputPolicy,
			maskInputPolicy:     definition.maskInputPolicy,
			originalImagePolicy: definition.originalImagePolicy,
			controlContract:     definition.controlContract,
		},
	}, nil
}

// CompileFeatureReadinessPlan resolves routing and Billing facts without
// requiring media that only exists when the user executes the operation.
// Thin alias over CompileFeatureExecutionPlan with the ReadReady option;
// the single compiler owns output_count/resolution determination.
func CompileFeatureReadinessPlan(input CompileFeatureExecutionInput) (FeatureExecutionPlan, error) {
	return CompileFeatureExecutionPlan(input, ReadReady)
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
	// An omitted (nil) or blank request carries no explicit resolution:
	// normalize it once to the Service policy's canonical value (1k for
	// both fixed and scalable policies, ADR-0012 rule 3). An explicit
	// non-blank value — including "default" — must be one of 1k/2k/4k,
	// and a fixed-policy Service additionally accepts only 1k.
	if strings.TrimSpace(value) == "" {
		return OutputResolution1K, nil
	}
	resolution, ok := NormalizeOutputResolution(value)
	if !ok || policy == OutputResolutionPolicyFixed && resolution != OutputResolution1K {
		return "", newFeatureContractValidationError(
			FeatureContractUnsupportedResolution, feature, "output_resolution", value,
		)
	}
	return resolution, nil
}

func newFeatureContractValidationError(code FeatureContractValidationCode, feature, field, value string) error {
	return &FeatureContractValidationError{Code: code, Feature: feature, Field: field, Value: value}
}
