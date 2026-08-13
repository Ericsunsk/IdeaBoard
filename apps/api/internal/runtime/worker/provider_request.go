package worker

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

var (
	openAIImageInputRefKeys = []string{"image_urls", "reference_image_urls"}
	googleImageInputRefKeys = []string{"image_urls", "reference_image_urls", "mask_image_urls", "original_image_url", "mask_image_url"}
)

type providerRequest struct {
	Prompt      string
	Model       string
	Size        string
	OutputCount int
	Params      map[string]interface{}
	MediaRefs   providerMediaRefs
}

type providerMediaRefs struct {
	OpenAIImages []string
	GoogleImages []string
}

type providerTaskParams struct {
	Canonical *tasks.WorkflowTaskParams
	Provider  map[string]interface{}
}

func newProviderTaskParams(params *tasks.WorkflowTaskParams) providerTaskParams {
	return providerTaskParams{
		Canonical: params,
		Provider:  workflow.BuildProviderTaskParams(params),
	}
}

func (p providerTaskParams) featureSlug() string {
	if p.Canonical != nil {
		if feature := strings.TrimSpace(p.Canonical.Feature); feature != "" {
			return feature
		}
	}
	if slug, err := workflow.ResolveWorkflowFeatureSlug(p.Provider); err == nil {
		return slug
	}
	return ""
}

func (p providerTaskParams) imageSize() (int, int) {
	return parseSizeFromParams(p.Provider)
}

func (p providerTaskParams) providerRequest(prompt string, resolvedModel string) providerRequest {
	width, height := p.imageSize()
	return newProviderRequest(prompt, width, height, p.Provider, resolvedModel)
}

func newProviderRequest(prompt string, width, height int, params map[string]interface{}, resolvedModel string) providerRequest {
	providerParams := workflow.BuildProviderPromptParams(params)
	return providerRequest{
		Prompt:      prompt,
		Model:       resolvedModel,
		Size:        providerImageSize(width, height),
		OutputCount: readProviderOutputCount(providerParams),
		Params:      providerParams,
		MediaRefs:   collectProviderMediaRefs(providerParams),
	}
}

func readProviderOutputCount(params map[string]interface{}) int {
	if params == nil {
		return 1
	}
	raw, ok := params["output_count"]
	if !ok {
		return 1
	}
	var count int
	switch value := raw.(type) {
	case int:
		count = value
	case int64:
		count = int(value)
	case float64:
		count = int(value)
	case json.Number:
		count, _ = strconv.Atoi(string(value))
	case string:
		count, _ = strconv.Atoi(strings.TrimSpace(value))
	}
	if count < 1 || count > 4 {
		return 0
	}
	return count
}

func (r providerRequest) withOutputCount(count int) providerRequest {
	clone := r
	clone.OutputCount = count
	clone.Params = make(map[string]interface{}, len(r.Params)+1)
	for key, value := range r.Params {
		clone.Params[key] = value
	}
	clone.Params["output_count"] = count
	return clone
}

func providerImageSize(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func collectProviderMediaRefs(params map[string]interface{}) providerMediaRefs {
	return providerMediaRefs{
		OpenAIImages: collectOrderedProviderRefs(params, openAIImageInputRefKeys),
		GoogleImages: collectOrderedProviderRefs(params, googleImageInputRefKeys),
	}
}

func collectOrderedProviderRefs(params map[string]interface{}, keys []string) []string {
	refs := make([]string, 0, len(keys))
	seenFromEarlierKeys := make(map[string]struct{})
	for _, key := range keys {
		var values []string
		switch raw := params[key].(type) {
		case string:
			values = []string{raw}
		default:
			values = mediarefs.CollectStringList(raw)
		}
		currentKeyRefs := make([]string, 0, len(values))
		for _, raw := range values {
			if ref := strings.TrimSpace(raw); ref != "" {
				if _, duplicateAlias := seenFromEarlierKeys[ref]; !duplicateAlias {
					refs = append(refs, ref)
					currentKeyRefs = append(currentKeyRefs, ref)
				}
			}
		}
		for _, ref := range currentKeyRefs {
			seenFromEarlierKeys[ref] = struct{}{}
		}
	}
	return refs
}
