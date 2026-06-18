package worker

import (
	"fmt"
	"strings"

	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"
)

var (
	openAIImageInputRefKeys = []string{"image_urls", "reference_image_urls"}
	googleImageInputRefKeys = []string{"image_urls", "reference_image_urls", "mask_image_urls", "original_image_url", "mask_image_url"}
)

type providerRequest struct {
	Prompt    string
	Model     string
	Size      string
	Params    map[string]interface{}
	MediaRefs providerMediaRefs
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

func (p providerTaskParams) requestedModel() string {
	if p.Canonical != nil {
		if model := strings.TrimSpace(p.Canonical.Model); model != "" {
			return model
		}
	}
	if model, ok := workflow.ReadWorkflowStringParam(p.Provider, "model"); ok {
		return strings.TrimSpace(model)
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

func newProviderTaskParamsFromProviderMap(params map[string]interface{}) providerTaskParams {
	return providerTaskParams{Provider: params}
}

func (p providerTaskParams) providerRequestWithSize(prompt string, width, height int, resolvedModel string) providerRequest {
	return newProviderRequest(prompt, width, height, p.Provider, resolvedModel)
}

func newProviderRequest(prompt string, width, height int, params map[string]interface{}, resolvedModel string) providerRequest {
	providerParams := workflow.BuildProviderPromptParams(params)
	return providerRequest{
		Prompt:    prompt,
		Model:     resolvedModel,
		Size:      providerImageSize(width, height),
		Params:    providerParams,
		MediaRefs: collectProviderMediaRefs(providerParams),
	}
}

func providerImageSize(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func collectProviderMediaRefs(params map[string]interface{}) providerMediaRefs {
	return providerMediaRefs{
		OpenAIImages: mediarefs.CollectUniqueParamRefs(params, openAIImageInputRefKeys),
		GoogleImages: mediarefs.CollectUniqueParamRefs(params, googleImageInputRefKeys),
	}
}
