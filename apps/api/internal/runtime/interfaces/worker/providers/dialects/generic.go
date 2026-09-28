package dialects

import (
	"encoding/json"
	"fmt"

	"ideaboard/api/internal/workflow"
)

// GenericDialect shapes OpenAI-compatible generation/edit payloads and
// decodes the long tail of vendor responses. It serves every non-chat,
// non-Fal target (openai image models, vertex, cloudflare, custom) until
// vendor evidence justifies a dedicated file.
type GenericDialect struct{}

func NewGenericDialect() *GenericDialect { return &GenericDialect{} }

func (GenericDialect) Encode(req Request) ([]byte, error) {
	var body map[string]interface{}
	if len(req.Images) > 0 && (req.Operation == workflow.RuntimeOperationInpaint || req.Operation == workflow.RuntimeOperationOutpaint) {
		body = map[string]interface{}{
			"model":     req.Model,
			"prompt":    req.Prompt,
			"image":     req.Images[0],
			"image_url": req.Images[0],
		}
		if len(req.Images) > 1 {
			body["mask"] = req.Images[1]
			body["mask_url"] = req.Images[1]
		}
	} else {
		body = map[string]interface{}{
			"model":  req.Model,
			"prompt": req.Prompt,
			"n":      1,
		}
		if len(req.Images) > 0 {
			body["image_url"] = req.Images[0]
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf(msgEncode3, err)
	}
	return encoded, nil
}

func (GenericDialect) Decode(body []byte) ([]string, error) {
	return decodeWithPaths(body)
}
