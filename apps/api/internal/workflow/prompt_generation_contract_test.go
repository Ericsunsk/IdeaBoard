package workflow

import (
	"strings"
	"testing"
)

func TestValidatePromptGenerationSubmitContract(t *testing.T) {
	for _, params := range []map[string]interface{}{
		nil,
		{"image_urls": []string{}},
		{"image_urls": []string{"one", "two", "three", "four"}},
	} {
		if err := ValidatePromptGenerationSubmitContract(params); err != nil {
			t.Fatalf("valid params %#v: %v", params, err)
		}
	}

	for _, params := range []map[string]interface{}{
		{"mode": "text-to-image"},
		{"primitive": "image-to-image"},
		{"reference_image_urls": []string{"one"}},
		{"image_urls": []string{"one", "two", "three", "four", "five"}},
	} {
		if err := ValidatePromptGenerationSubmitContract(params); err == nil || !strings.Contains(err.Error(), "prompt-generation") {
			t.Fatalf("invalid params %#v error=%v", params, err)
		}
	}
}
