package workflow

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCompileFeatureExecutionPlanCoversCanonicalImageFeatures(t *testing.T) {
	tests := []struct {
		feature                string
		promptPolicy           PromptPolicy
		outputCountPolicy      OutputCountPolicy
		outputResolutionPolicy OutputResolutionPolicy
		capability             RoutingCapability
		modelRequirement       RoutingModelRequirement
		operation              RuntimeOperation
		imageInputCount        int
		hasMaskInput           bool
		primitive              GenerationPrimitive
		invocation             RuntimeInvocation
		imageInputPolicy       ImageInputPolicy
		maskInputPolicy        MaskInputPolicy
		originalImagePolicy    OriginalImagePolicy
		controlContract        RuntimeControlContract
	}{
		{
			feature: FeatureSlugPromptGeneration, promptPolicy: PromptPolicyRequired,
			outputCountPolicy: OutputCountPolicySelectable, outputResolutionPolicy: OutputResolutionPolicyScalable,
			capability: RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
			operation: RuntimeOperationPromptGeneration, primitive: GenerationPrimitiveTextToImage,
			invocation: RuntimeInvocationGenerate, imageInputPolicy: ImageInputOptional,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageForwardWhenPresent,
			controlContract: RuntimeControlsNone,
		},
		{
			feature: FeatureSlugLocalInpaint, promptPolicy: PromptPolicyOptional,
			outputCountPolicy: OutputCountPolicySelectable, outputResolutionPolicy: OutputResolutionPolicyScalable,
			capability: RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
			operation: RuntimeOperationLocalInpaint, imageInputCount: 1, hasMaskInput: true, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationEdit, imageInputPolicy: ImageInputRequired,
			maskInputPolicy: MaskInputRequired, originalImagePolicy: OriginalImageForwardWhenPresent,
			controlContract: RuntimeControlsNone,
		},
		{
			feature: FeatureSlugSketchToImage, promptPolicy: PromptPolicyOptional,
			outputCountPolicy: OutputCountPolicySelectable, outputResolutionPolicy: OutputResolutionPolicyScalable,
			capability: RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
			operation: RuntimeOperationSketchToImage, imageInputCount: 1, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationEdit, imageInputPolicy: ImageInputRequired,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageForwardWhenPresent,
			controlContract: RuntimeControlsNone,
		},
		{
			feature: FeatureSlugStyleVariation, promptPolicy: PromptPolicyOptional,
			outputCountPolicy: OutputCountPolicySelectable, outputResolutionPolicy: OutputResolutionPolicyScalable,
			capability: RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
			operation: RuntimeOperationStyleVariation, imageInputCount: 1, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationEdit, imageInputPolicy: ImageInputRequired,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageExcluded,
			controlContract: RuntimeControlsVariation,
		},
		{
			feature: FeatureSlugRemoveBg, promptPolicy: PromptPolicyIgnored,
			outputCountPolicy: OutputCountPolicyFixed, outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
			capability: RoutingCapabilityBackgroundRemoval, modelRequirement: RoutingModelNotRequired,
			operation: RuntimeOperationBackgroundRemoval, imageInputCount: 1, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationTransform, imageInputPolicy: ImageInputRequired,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageIgnored,
			controlContract: RuntimeControlsNone,
		},
		{
			feature: FeatureSlugStyleFusion, promptPolicy: PromptPolicyOptional,
			outputCountPolicy: OutputCountPolicySelectable, outputResolutionPolicy: OutputResolutionPolicyScalable,
			capability: RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
			operation: RuntimeOperationStyleFusion, imageInputCount: 2, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationEdit, imageInputPolicy: MultipleImageInputsRequired,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageForwardWhenPresent,
			controlContract: RuntimeControlsFusion,
		},
		{
			feature: FeatureSlugHDUpscale, promptPolicy: PromptPolicyIgnored,
			outputCountPolicy: OutputCountPolicyFixed, outputResolutionPolicy: OutputResolutionPolicyDefaultOnly,
			capability: RoutingCapabilityImageUpscale, modelRequirement: RoutingModelNotRequired,
			operation: RuntimeOperationImageUpscale, imageInputCount: 1, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationTransform, imageInputPolicy: ImageInputRequired,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageIgnored,
			controlContract: RuntimeControlsUpscale,
		},
		{
			feature: FeatureSlugImageExtend, promptPolicy: PromptPolicyOptional,
			outputCountPolicy: OutputCountPolicySelectable, outputResolutionPolicy: OutputResolutionPolicyScalable,
			capability: RoutingCapabilityImageGeneration, modelRequirement: RoutingModelRequired,
			operation: RuntimeOperationImageExtend, imageInputCount: 1, primitive: GenerationPrimitiveImageToImage,
			invocation: RuntimeInvocationEdit, imageInputPolicy: ImageInputRequired,
			maskInputPolicy: MaskInputIgnored, originalImagePolicy: OriginalImageForwardWhenPresent,
			controlContract: RuntimeControlsExtend,
		},
	}

	canonical := CanonicalFeatureSlugs()
	if len(tests) != len(canonical) {
		t.Fatalf("feature Contract matrix has %d entries, canonical registry has %d", len(tests), len(canonical))
	}
	seen := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		t.Run(test.feature, func(t *testing.T) {
			plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{
				Feature: test.feature,
				Params: FeatureExecutionParams{
					ImageInputCount: test.imageInputCount,
					HasMaskInput:    test.hasMaskInput,
				},
			})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if plan.Feature() != test.feature {
				t.Fatalf("feature=%q, want=%q", plan.Feature(), test.feature)
			}
			if plan.PromptPolicy() != test.promptPolicy {
				t.Fatalf("prompt policy=%q, want=%q", plan.PromptPolicy(), test.promptPolicy)
			}
			if plan.OutputCountPolicy() != test.outputCountPolicy {
				t.Fatalf("output count policy=%q, want=%q", plan.OutputCountPolicy(), test.outputCountPolicy)
			}
			if plan.RequestedOutputCount().Int() != 1 {
				t.Fatalf("default output count=%d, want=1", plan.RequestedOutputCount().Int())
			}
			if plan.OutputResolutionPolicy() != test.outputResolutionPolicy {
				t.Fatalf("output resolution policy=%q, want=%q", plan.OutputResolutionPolicy(), test.outputResolutionPolicy)
			}
			wantSupportedResolutions := []string{
				OutputResolution1K,
				OutputResolution2K,
				OutputResolution4K,
				OutputResolutionDefault,
			}
			if test.outputResolutionPolicy == OutputResolutionPolicyDefaultOnly {
				wantSupportedResolutions = []string{OutputResolutionDefault}
			}
			if got := SupportedFeatureOutputResolutions(test.feature); !slices.Equal(got, wantSupportedResolutions) {
				t.Fatalf("supported resolutions=%v, want=%v", got, wantSupportedResolutions)
			}
			wantResolution := OutputResolution1K
			if test.outputResolutionPolicy == OutputResolutionPolicyDefaultOnly {
				wantResolution = OutputResolutionDefault
			}
			if plan.OutputResolution() != wantResolution {
				t.Fatalf("default output resolution=%q, want=%q", plan.OutputResolution(), wantResolution)
			}
			routing := plan.RoutingRequirement()
			if routing.Capability() != test.capability {
				t.Fatalf("routing capability=%q, want=%q", routing.Capability(), test.capability)
			}
			if routing.ModelRequirement() != test.modelRequirement {
				t.Fatalf("model requirement=%q, want=%q", routing.ModelRequirement(), test.modelRequirement)
			}
			if routing.OutputResolution() != plan.OutputResolution() {
				t.Fatalf("routing resolution=%q, plan resolution=%q", routing.OutputResolution(), plan.OutputResolution())
			}
			if plan.RuntimeOperation() != test.operation {
				t.Fatalf("runtime operation=%q, want=%q", plan.RuntimeOperation(), test.operation)
			}
			runtimeInputs := plan.RuntimeInputs()
			if runtimeInputs.ImageInputCount() != test.imageInputCount || runtimeInputs.HasMaskInput() != test.hasMaskInput {
				t.Fatalf("runtime media facts=(images=%d,mask=%v), want=(images=%d,mask=%v)", runtimeInputs.ImageInputCount(), runtimeInputs.HasMaskInput(), test.imageInputCount, test.hasMaskInput)
			}
			if runtimeInputs.GenerationPrimitive() != test.primitive || runtimeInputs.Invocation() != test.invocation {
				t.Fatalf("runtime generation=(primitive=%q,invocation=%q), want=(primitive=%q,invocation=%q)", runtimeInputs.GenerationPrimitive(), runtimeInputs.Invocation(), test.primitive, test.invocation)
			}
			if runtimeInputs.ImageInputPolicy() != test.imageInputPolicy || runtimeInputs.MaskInputPolicy() != test.maskInputPolicy {
				t.Fatalf("runtime media=(image=%q,mask=%q), want=(image=%q,mask=%q)", runtimeInputs.ImageInputPolicy(), runtimeInputs.MaskInputPolicy(), test.imageInputPolicy, test.maskInputPolicy)
			}
			if runtimeInputs.OriginalImagePolicy() != test.originalImagePolicy || runtimeInputs.ControlContract() != test.controlContract {
				t.Fatalf("runtime shaping=(original=%q,controls=%q), want=(original=%q,controls=%q)", runtimeInputs.OriginalImagePolicy(), runtimeInputs.ControlContract(), test.originalImagePolicy, test.controlContract)
			}
		})
		seen[test.feature] = struct{}{}
	}
	for _, feature := range canonical {
		if _, ok := seen[feature]; !ok {
			t.Fatalf("canonical feature %q is missing from Contract interface test", feature)
		}
	}
	if len(imageFeatureExecutionDefinitions) != len(canonical) {
		t.Fatalf("Contract registry has %d definitions, canonical registry has %d", len(imageFeatureExecutionDefinitions), len(canonical))
	}
	for feature := range imageFeatureExecutionDefinitions {
		if !slices.Contains(canonical, feature) {
			t.Fatalf("Contract definition %q is not a canonical feature", feature)
		}
	}
	if got := SupportedFeatureOutputResolutions("unknown"); got != nil {
		t.Fatalf("unknown feature resolutions=%v, want nil", got)
	}
}

func TestCompileFeatureExecutionPlanAppliesSelectableOutputParams(t *testing.T) {
	count := 4
	resolution := " 2K "
	plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{
		Feature: FeatureSlugStyleFusion,
		Params: FeatureExecutionParams{
			RequestedOutputCount: &count,
			OutputResolution:     &resolution,
			ImageInputCount:      2,
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if plan.RequestedOutputCount().Int() != 4 {
		t.Fatalf("output count=%d, want=4", plan.RequestedOutputCount().Int())
	}
	if plan.OutputResolution() != OutputResolution2K {
		t.Fatalf("output resolution=%q, want=%q", plan.OutputResolution(), OutputResolution2K)
	}
	if plan.RoutingRequirement().OutputResolution() != OutputResolution2K {
		t.Fatalf("routing resolution=%q, want=%q", plan.RoutingRequirement().OutputResolution(), OutputResolution2K)
	}
}

func TestCompileFeatureExecutionPlanPromptGenerationWithImageUsesEditPrimitive(t *testing.T) {
	plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{
		Feature: FeatureSlugPromptGeneration,
		Params:  FeatureExecutionParams{ImageInputCount: 1},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	inputs := plan.RuntimeInputs()
	if inputs.GenerationPrimitive() != GenerationPrimitiveImageToImage || inputs.Invocation() != RuntimeInvocationEdit {
		t.Fatalf("runtime inputs=(primitive=%q,invocation=%q), want image-to-image edit", inputs.GenerationPrimitive(), inputs.Invocation())
	}
}

func TestCompileFeatureExecutionPlanRejectsInvalidRuntimeMediaByFeature(t *testing.T) {
	tests := []struct {
		name    string
		feature string
		params  FeatureExecutionParams
		field   string
	}{
		{name: "remove background needs image", feature: FeatureSlugRemoveBg, field: "image_input_count"},
		{name: "upscale needs image", feature: FeatureSlugHDUpscale, field: "image_input_count"},
		{name: "inpaint needs image", feature: FeatureSlugLocalInpaint, params: FeatureExecutionParams{HasMaskInput: true}, field: "image_input_count"},
		{name: "inpaint needs mask", feature: FeatureSlugLocalInpaint, params: FeatureExecutionParams{ImageInputCount: 1}, field: "mask_input"},
		{name: "sketch needs image", feature: FeatureSlugSketchToImage, field: "image_input_count"},
		{name: "variation needs image", feature: FeatureSlugStyleVariation, field: "image_input_count"},
		{name: "extend needs image", feature: FeatureSlugImageExtend, field: "image_input_count"},
		{name: "fusion needs multiple images", feature: FeatureSlugStyleFusion, params: FeatureExecutionParams{ImageInputCount: 1}, field: "image_input_count"},
		{name: "fusion caps images at four", feature: FeatureSlugStyleFusion, params: FeatureExecutionParams{ImageInputCount: 5}, field: "image_input_count"},
		{name: "non mask operation rejects mask", feature: FeatureSlugSketchToImage, params: FeatureExecutionParams{ImageInputCount: 1, HasMaskInput: true}, field: "mask_input"},
		{name: "prompt generation caps images at four", feature: FeatureSlugPromptGeneration, params: FeatureExecutionParams{ImageInputCount: 5}, field: "image_input_count"},
		{name: "general image operation caps at API maximum", feature: FeatureSlugStyleVariation, params: FeatureExecutionParams{ImageInputCount: 17}, field: "image_input_count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{Feature: test.feature, Params: test.params})
			assertFeatureContractValidation(t, err, FeatureContractInvalidRuntimeInput, test.field)
		})
	}
}

func TestCompileFeatureExecutionPlanUsesFeatureVersionedImageLimits(t *testing.T) {
	for _, test := range []struct {
		name    string
		feature string
		count   int
	}{
		{name: "prompt generation upper bound", feature: FeatureSlugPromptGeneration, count: 4},
		{name: "fusion upper bound", feature: FeatureSlugStyleFusion, count: 4},
		{name: "general operation accepts five", feature: FeatureSlugStyleVariation, count: 5},
		{name: "general operation API upper bound", feature: FeatureSlugStyleVariation, count: 16},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{
				Feature: test.feature,
				Params:  FeatureExecutionParams{ImageInputCount: test.count},
			})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if plan.RuntimeInputs().ImageInputCount() != test.count {
				t.Fatalf("image input count=%d, want=%d", plan.RuntimeInputs().ImageInputCount(), test.count)
			}
			payload, err := EncodeFeatureExecutionPlan(plan)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := DecodeFeatureExecutionPlan(payload)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded.RuntimeInputs().ImageInputCount() != test.count {
				t.Fatalf("decoded image input count=%d, want=%d", decoded.RuntimeInputs().ImageInputCount(), test.count)
			}
		})
	}
}

func TestCompileFeatureExecutionPlanDedicatedTransformsAreFixedAndModeless(t *testing.T) {
	count := 1
	resolution := " DEFAULT "
	for _, test := range []struct {
		feature    string
		capability RoutingCapability
		operation  RuntimeOperation
	}{
		{feature: FeatureSlugRemoveBg, capability: RoutingCapabilityBackgroundRemoval, operation: RuntimeOperationBackgroundRemoval},
		{feature: FeatureSlugHDUpscale, capability: RoutingCapabilityImageUpscale, operation: RuntimeOperationImageUpscale},
	} {
		t.Run(test.feature, func(t *testing.T) {
			plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{
				Feature: test.feature,
				Params: FeatureExecutionParams{
					RequestedOutputCount: &count,
					OutputResolution:     &resolution,
					ImageInputCount:      1,
				},
			})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if plan.PromptPolicy() != PromptPolicyIgnored || plan.RequestedOutputCount().Int() != 1 || plan.OutputResolution() != OutputResolutionDefault {
				t.Fatalf("dedicated plan=(prompt=%q,count=%d,resolution=%q)", plan.PromptPolicy(), plan.RequestedOutputCount().Int(), plan.OutputResolution())
			}
			routing := plan.RoutingRequirement()
			if routing.Capability() != test.capability || routing.ModelRequirement() != RoutingModelNotRequired {
				t.Fatalf("routing=(capability=%q,model=%q)", routing.Capability(), routing.ModelRequirement())
			}
			if plan.RuntimeOperation() != test.operation {
				t.Fatalf("operation=%q, want=%q", plan.RuntimeOperation(), test.operation)
			}
		})
	}
}

func TestFeatureExecutionPlanDurableRoundTripPreservesCanonicalFacts(t *testing.T) {
	count := 3
	resolution := OutputResolution2K
	for _, feature := range CanonicalFeatureSlugs() {
		t.Run(feature, func(t *testing.T) {
			params := FeatureExecutionParams{ImageInputCount: 1}
			if feature == FeatureSlugLocalInpaint {
				params.HasMaskInput = true
			}
			if feature == FeatureSlugStyleFusion {
				params.ImageInputCount = 2
			}
			if feature != FeatureSlugRemoveBg && feature != FeatureSlugHDUpscale {
				params.RequestedOutputCount = &count
				params.OutputResolution = &resolution
			}
			plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{Feature: feature, Params: params})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			payload, err := EncodeFeatureExecutionPlan(plan)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := DecodeFeatureExecutionPlan(payload)
			if err != nil {
				t.Fatalf("decode: %v\npayload=%s", err, payload)
			}
			reencoded, err := EncodeFeatureExecutionPlan(decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if string(reencoded) != string(payload) {
				t.Fatalf("round-trip changed durable facts\nbefore=%s\nafter=%s", payload, reencoded)
			}
		})
	}
}

func TestFeatureExecutionPlanJSONUsesVersionedDurableSeam(t *testing.T) {
	plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{Feature: FeatureSlugPromptGeneration})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(payload) == `{}` || !strings.Contains(string(payload), `"schema_version":1`) {
		t.Fatalf("plan did not use versioned durable encoding: %s", payload)
	}
	var decoded FeatureExecutionPlan
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Feature() != plan.Feature() || decoded.RuntimeOperation() != plan.RuntimeOperation() {
		t.Fatalf("decoded plan=(feature=%q,operation=%q)", decoded.Feature(), decoded.RuntimeOperation())
	}
}

func TestDecodeFeatureExecutionPlanFailsClosed(t *testing.T) {
	plan, err := CompileFeatureExecutionPlan(CompileFeatureExecutionInput{Feature: FeatureSlugStyleVariation, Params: FeatureExecutionParams{ImageInputCount: 1}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	payload, err := EncodeFeatureExecutionPlan(plan)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	tests := []struct {
		name    string
		payload string
		code    FeatureContractValidationCode
	}{
		{
			name:    "unknown schema",
			payload: strings.Replace(string(payload), `"schema_version":1`, `"schema_version":99`, 1),
			code:    FeatureContractUnsupportedSchema,
		},
		{
			name:    "unknown runtime operation",
			payload: strings.Replace(string(payload), `"operation":"style-variation"`, `"operation":"unknown-operation"`, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "inconsistent routing capability",
			payload: strings.Replace(string(payload), `"capability":"image-generation"`, `"capability":"image-upscale"`, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "unknown field",
			payload: strings.Replace(string(payload), `"feature":"style-variation"`, `"feature":"style-variation","legacy":true`, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "missing top-level field",
			payload: strings.Replace(string(payload), `"prompt_policy":"optional",`, ``, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "missing output field",
			payload: strings.Replace(string(payload), `"requested_count":1,`, ``, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "missing routing field",
			payload: strings.Replace(string(payload), `,"output_resolution":"1k"`, ``, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "missing runtime image count",
			payload: strings.Replace(string(payload), `"image_input_count":1,`, ``, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "missing runtime mask presence",
			payload: strings.Replace(string(payload), `"has_mask_input":false,`, ``, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "duplicate top-level key",
			payload: strings.Replace(string(payload), `"feature":"style-variation"`, `"feature":"style-variation","feature":"style-variation"`, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "duplicate nested key",
			payload: strings.Replace(string(payload), `"operation":"style-variation"`, `"operation":"style-variation","operation":"style-variation"`, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "runtime media violates frozen policy",
			payload: strings.Replace(string(payload), `"image_input_count":1`, `"image_input_count":0`, 1),
			code:    FeatureContractInvalidDurablePlan,
		},
		{
			name:    "trailing JSON",
			payload: string(payload) + `{}`,
			code:    FeatureContractInvalidDurablePlan,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.payload == string(payload) {
				t.Fatal("test mutation did not change payload")
			}
			_, err := DecodeFeatureExecutionPlan([]byte(test.payload))
			assertFeatureContractValidationCode(t, err, test.code)
		})
	}
}

func TestEncodeFeatureExecutionPlanRejectsZeroPlan(t *testing.T) {
	_, err := EncodeFeatureExecutionPlan(FeatureExecutionPlan{})
	assertFeatureContractValidationCode(t, err, FeatureContractInvalidDurablePlan)
}

func TestCompileFeatureExecutionPlanReturnsRecognizableValidationErrors(t *testing.T) {
	zero := 0
	five := 5
	two := 2
	resolution4K := OutputResolution4K
	resolution8K := "8k"
	tests := []struct {
		name  string
		input CompileFeatureExecutionInput
		code  FeatureContractValidationCode
		field string
	}{
		{
			name: "unknown feature", input: CompileFeatureExecutionInput{Feature: "unknown-feature"},
			code: FeatureContractUnknownFeature, field: "feature",
		},
		{
			name:  "invalid output count lower bound",
			input: CompileFeatureExecutionInput{Feature: FeatureSlugStyleFusion, Params: FeatureExecutionParams{RequestedOutputCount: &zero}},
			code:  FeatureContractInvalidOutputCount, field: "output_count",
		},
		{
			name:  "invalid output count upper bound",
			input: CompileFeatureExecutionInput{Feature: FeatureSlugStyleFusion, Params: FeatureExecutionParams{RequestedOutputCount: &five}},
			code:  FeatureContractInvalidOutputCount, field: "output_count",
		},
		{
			name:  "fixed output count rejects selectable value",
			input: CompileFeatureExecutionInput{Feature: FeatureSlugRemoveBg, Params: FeatureExecutionParams{RequestedOutputCount: &two}},
			code:  FeatureContractInvalidOutputCount, field: "output_count",
		},
		{
			name:  "unknown scalable resolution",
			input: CompileFeatureExecutionInput{Feature: FeatureSlugLocalInpaint, Params: FeatureExecutionParams{OutputResolution: &resolution8K}},
			code:  FeatureContractUnsupportedResolution, field: "output_resolution",
		},
		{
			name:  "dedicated transform rejects scaled resolution",
			input: CompileFeatureExecutionInput{Feature: FeatureSlugHDUpscale, Params: FeatureExecutionParams{OutputResolution: &resolution4K}},
			code:  FeatureContractUnsupportedResolution, field: "output_resolution",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompileFeatureExecutionPlan(test.input)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *FeatureContractValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type=%T, want *FeatureContractValidationError", err)
			}
			if validationErr.Code != test.code || validationErr.Field != test.field {
				t.Fatalf("validation=(code=%q,field=%q), want=(code=%q,field=%q)", validationErr.Code, validationErr.Field, test.code, test.field)
			}
		})
	}
}

func assertFeatureContractValidationCode(t *testing.T, err error, want FeatureContractValidationCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation code %q", want)
	}
	var validationErr *FeatureContractValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type=%T, want *FeatureContractValidationError", err)
	}
	if validationErr.Code != want {
		t.Fatalf("validation code=%q, want=%q (error=%v)", validationErr.Code, want, err)
	}
}

func assertFeatureContractValidation(
	t *testing.T,
	err error,
	wantCode FeatureContractValidationCode,
	wantField string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation code %q", wantCode)
	}
	var validationErr *FeatureContractValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type=%T, want *FeatureContractValidationError", err)
	}
	if validationErr.Code != wantCode || validationErr.Field != wantField {
		t.Fatalf("validation=(code=%q,field=%q), want=(code=%q,field=%q)", validationErr.Code, validationErr.Field, wantCode, wantField)
	}
}
