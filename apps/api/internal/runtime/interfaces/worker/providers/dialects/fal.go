package dialects

import (
	"encoding/json"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
)

// FalDialect shapes Fal.ai media-route payloads (generation, inpaint,
// outpaint share one prompt + image_urls shape on this provider).
type FalDialect struct{}

func NewFalDialect() *FalDialect { return &FalDialect{} }

func (FalDialect) Encode(req Request) ([]byte, error) {
	body := map[string]interface{}{
		"prompt": req.Prompt,
	}
	if len(req.Images) > 0 {
		body["image_urls"] = req.Images
		body["image_url"] = req.Images[0]
		if len(req.Images) > 1 {
			body["mask_url"] = req.Images[1]
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgEncode2, err)
	}
	return encoded, nil
}
