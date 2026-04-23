package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

var workflowFeatureSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	FeatureSlugSketchToDesign  = "sketch-to-design"
	FeatureSlugStyleInnovate   = "style-innovate"
	FeatureSlugPartialEdit     = "partial-edit"
	FeatureSlugBgSwap          = "bg-swap"
	FeatureSlugFaceSwap        = "face-swap"
	FeatureSlugStyleFusion     = "style-fusion"
	FeatureSlugSeriesColor     = "series-color"
	FeatureSlugDirectedRecolor = "directed-recolor"
	FeatureSlugFabricOnBody    = "fabric-on-body"
	FeatureSlugMultiAngle      = "multi-angle"
	FeatureSlugVirtual3D       = "virtual-3d"
	FeatureSlugPatternGen      = "pattern-gen"
	FeatureSlugHDUpscale       = "hd-upscale"
	FeatureSlugCanvasExtend    = "canvas-extend"
)

var supportedTaskFeatureSlugs = map[string]struct{}{
	FeatureSlugSketchToDesign:  {},
	FeatureSlugStyleInnovate:   {},
	FeatureSlugPartialEdit:     {},
	FeatureSlugBgSwap:          {},
	FeatureSlugFaceSwap:        {},
	FeatureSlugStyleFusion:     {},
	FeatureSlugSeriesColor:     {},
	FeatureSlugDirectedRecolor: {},
	FeatureSlugFabricOnBody:    {},
	FeatureSlugMultiAngle:      {},
	FeatureSlugVirtual3D:       {},
	FeatureSlugPatternGen:      {},
	FeatureSlugHDUpscale:       {},
	FeatureSlugCanvasExtend:    {},
}

var workflowParamAliases = map[string][]string{
	"feature": {
		"feature",
		"feature_slug",
	},
	"use_mask": {
		"use_mask",
		"mask_enabled",
		"is_masked",
		"masked",
		"local_enhance",
		"is_local_enhance",
		"partial_enhance",
		"is_partial_enhance",
	},
	"mask_image_url": {
		"mask_image_url",
		"mask_url",
		"mask_image",
	},
	"mask_image_urls": {
		"mask_image_urls",
		"mask_urls",
	},
	"upscale_mode": {
		"upscale_mode",
		"mode",
	},
}

func normalizeWorkflowFeatureSlug(raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if slug == "" {
		return ""
	}
	return strings.ReplaceAll(slug, "_", "-")
}

func ParseWorkflowFeatureSlug(raw string) (string, bool) {
	slug := normalizeWorkflowFeatureSlug(raw)
	if slug == "" || !workflowFeatureSlugRe.MatchString(slug) {
		return "", false
	}
	if _, ok := supportedTaskFeatureSlugs[slug]; !ok {
		return "", false
	}
	return slug, true
}

func NormalizeWorkflowFeatureSlugOrFallback(raw, fallback string) string {
	if slug, ok := ParseWorkflowFeatureSlug(raw); ok {
		return slug
	}
	return fallback
}

func ValidateWorkflowFeatureSlug(raw string) (string, error) {
	slug, ok := ParseWorkflowFeatureSlug(raw)
	if !ok {
		return "", fmt.Errorf("feature %q 不支持", raw)
	}
	return slug, nil
}

func ReadWorkflowParam(params map[string]interface{}, canonicalKey string) (interface{}, bool) {
	if params == nil {
		return nil, false
	}
	keys := workflowParamAliases[canonicalKey]
	if len(keys) == 0 {
		v, ok := params[canonicalKey]
		return v, ok
	}
	for _, key := range keys {
		if v, ok := params[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func ReadWorkflowStringParam(params map[string]interface{}, canonicalKey string) (string, bool) {
	v, ok := ReadWorkflowParam(params, canonicalKey)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func ResolveWorkflowFeatureSlug(params map[string]interface{}) (string, error) {
	rawFeature, ok := ReadWorkflowStringParam(params, "feature")
	if !ok {
		return "", nil
	}
	slug, err := ValidateWorkflowFeatureSlug(rawFeature)
	if err != nil {
		return "", err
	}
	return slug, nil
}
