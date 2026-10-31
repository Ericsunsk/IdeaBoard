package workflow

import (
	"slices"
	"testing"
)

// TestLiveRegistryMatchesDurableSnapshotForCurrentSchema cross-checks the live
// compiler registry against the immutable durable snapshot for the current
// schema version. Any semantic drift must force a schema version bump instead
// of silently changing durable facts.
func TestLiveRegistryMatchesDurableSnapshotForCurrentSchema(t *testing.T) {
	liveSlugs := registeredImageFeatureSlugs()
	if !slices.Equal(liveSlugs, AIImageFeatureSlugs()) {
		t.Fatalf("live Workflow registry %v differs from canonical feature slugs %v", liveSlugs, AIImageFeatureSlugs())
	}
	if len(liveSlugs) != len(imageFeatureExecutionDefinitions) {
		t.Fatalf("live registry has %d slugs but %d definitions", len(liveSlugs), len(imageFeatureExecutionDefinitions))
	}
	for _, slug := range liveSlugs {
		t.Run(slug, func(t *testing.T) {
			live, ok := imageFeatureExecutionDefinitions[slug]
			if !ok {
				t.Fatalf("live registry slug %q has no definition", slug)
			}
			frozen, ok := durableFeatureDefinitionForCurrentSchema(slug)
			if !ok {
				t.Fatalf("durable snapshot for schema %d has no definition for live feature %q; bump the schema or remove the feature", CurrentFeatureExecutionPlanSchemaVersion, slug)
			}
			if live.promptPolicy != frozen.promptPolicy {
				t.Errorf("promptPolicy live=%q frozen=%q", live.promptPolicy, frozen.promptPolicy)
			}
			if live.outputCountPolicy != frozen.outputCountPolicy {
				t.Errorf("outputCountPolicy live=%q frozen=%q", live.outputCountPolicy, frozen.outputCountPolicy)
			}
			if live.outputResolutionPolicy != frozen.outputResolutionPolicy {
				t.Errorf("outputResolutionPolicy live=%q frozen=%q", live.outputResolutionPolicy, frozen.outputResolutionPolicy)
			}
			if live.routingCapability != frozen.routingCapability {
				t.Errorf("routingCapability live=%q frozen=%q", live.routingCapability, frozen.routingCapability)
			}
			if live.modelRequirement != frozen.modelRequirement {
				t.Errorf("modelRequirement live=%q frozen=%q", live.modelRequirement, frozen.modelRequirement)
			}
			if live.runtimeOperation != frozen.runtimeOperation {
				t.Errorf("runtimeOperation live=%q frozen=%q", live.runtimeOperation, frozen.runtimeOperation)
			}
			if live.runtimeInvocation != frozen.runtimeInvocation {
				t.Errorf("runtimeInvocation live=%q frozen=%q", live.runtimeInvocation, frozen.runtimeInvocation)
			}
			if live.imageInputPolicy != frozen.imageInputPolicy {
				t.Errorf("imageInputPolicy live=%q frozen=%q", live.imageInputPolicy, frozen.imageInputPolicy)
			}
			if live.maxImageInputCount != frozen.maxImageInputCount {
				t.Errorf("maxImageInputCount live=%d frozen=%d", live.maxImageInputCount, frozen.maxImageInputCount)
			}
			if live.maskInputPolicy != frozen.maskInputPolicy {
				t.Errorf("maskInputPolicy live=%q frozen=%q", live.maskInputPolicy, frozen.maskInputPolicy)
			}
			if live.originalImagePolicy != frozen.originalImagePolicy {
				t.Errorf("originalImagePolicy live=%q frozen=%q", live.originalImagePolicy, frozen.originalImagePolicy)
			}
			if live.controlContract != frozen.controlContract {
				t.Errorf("controlContract live=%q frozen=%q", live.controlContract, frozen.controlContract)
			}
		})
	}
	for _, slug := range AIImageFeatureSlugs() {
		if !slices.Contains(liveSlugs, slug) {
			t.Fatalf("canonical slug %q missing from live registry", slug)
		}
		if _, ok := durableFeatureDefinitionForCurrentSchema(slug); !ok {
			t.Fatalf("canonical slug %q missing from durable snapshot for schema %d", slug, CurrentFeatureExecutionPlanSchemaVersion)
		}
	}
}
