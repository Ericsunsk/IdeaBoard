// Package features owns the dependency-free Workflow feature slug contract.
// Tasking domain and Workflow execution share it without depending on each other.
package features

import "strings"

const (
	FeatureSlugGeneration = "generation"
	FeatureSlugInpaint    = "inpaint"
	FeatureSlugRender     = "render"
	FeatureSlugVariations = "variations"
	FeatureSlugCutout     = "cutout"
	FeatureSlugFusion     = "fusion"
	FeatureSlugUpscale    = "upscale"
	FeatureSlugOutpaint   = "outpaint"
	ReservedVisionSlug    = "vision"
)

var imageFeatureSlugs = [...]string{
	FeatureSlugGeneration,
	FeatureSlugInpaint,
	FeatureSlugRender,
	FeatureSlugVariations,
	FeatureSlugCutout,
	FeatureSlugFusion,
	FeatureSlugUpscale,
	FeatureSlugOutpaint,
}

// AIImageFeatureSlugs returns a copy of the canonical AI image workflow registry.
func AIImageFeatureSlugs() []string {
	return append([]string(nil), imageFeatureSlugs[:]...)
}

// ParseAIImageFeatureSlug accepts only registered AI image workflows.
func ParseAIImageFeatureSlug(raw string) (string, bool) {
	slug := strings.TrimSpace(raw)
	for _, candidate := range imageFeatureSlugs {
		if slug == candidate {
			return slug, true
		}
	}
	return "", false
}
