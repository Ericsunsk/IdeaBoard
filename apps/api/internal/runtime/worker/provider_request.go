package worker

import (
	"fmt"

	"api/internal/platform/mediarefs"
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

func newProviderRequest(prompt string, width, height int, params map[string]interface{}, resolvedModel string) providerRequest {
	return providerRequest{
		Prompt:    prompt,
		Model:     resolvedModel,
		Size:      providerImageSize(width, height),
		Params:    params,
		MediaRefs: collectProviderMediaRefs(params),
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

func imageURLObjectRefs(refs []string) []map[string]string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, map[string]string{"image_url": ref})
	}
	return out
}
