package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

var workflowFeatureSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var supportedTaskFeatureSlugs = map[string]struct{}{
	"sketch-to-design": {},
	"style-innovate":   {},
	"partial-edit":     {},
	"bg-swap":          {},
	"face-swap":        {},
	"style-fusion":     {},
	"series-color":     {},
	"directed-recolor": {},
	"fabric-on-body":   {},
	"multi-angle":      {},
	"virtual-3d":       {},
	"pattern-gen":      {},
	"hd-upscale":       {},
	"canvas-extend":    {},
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
