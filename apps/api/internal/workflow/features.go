package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

var workflowFeatureSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	FeatureSlugSketchToDesign = "sketch-to-design"
	FeatureSlugStyleInnovate  = "style-innovate"
	FeatureSlugPartialEdit    = "partial-edit"
	FeatureSlugBgSwap         = "bg-swap"
	FeatureSlugRemoveBg       = "remove-bg"
	FeatureSlugFaceSwap       = "face-swap"
	FeatureSlugStyleFusion    = "style-fusion"
	FeatureSlugFabricOnBody   = "fabric-on-body"
	FeatureSlugMultiAngle     = "multi-angle"
	FeatureSlugVirtual3D      = "virtual-3d"
	FeatureSlugPatternGen     = "pattern-gen"
	FeatureSlugHDUpscale      = "hd-upscale"
	FeatureSlugCanvasExtend   = "canvas-extend"
)

var supportedTaskFeatureSlugs = map[string]struct{}{
	FeatureSlugSketchToDesign: {},
	FeatureSlugStyleInnovate:  {},
	FeatureSlugPartialEdit:    {},
	FeatureSlugBgSwap:         {},
	FeatureSlugRemoveBg:       {},
	FeatureSlugFaceSwap:       {},
	FeatureSlugStyleFusion:    {},
	FeatureSlugFabricOnBody:   {},
	FeatureSlugMultiAngle:     {},
	FeatureSlugVirtual3D:      {},
	FeatureSlugPatternGen:     {},
	FeatureSlugHDUpscale:      {},
	FeatureSlugCanvasExtend:   {},
}

func normalizeWorkflowFeatureSlug(raw string) string {
	return strings.TrimSpace(raw)
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
	v, ok := params[canonicalKey]
	return v, ok
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
