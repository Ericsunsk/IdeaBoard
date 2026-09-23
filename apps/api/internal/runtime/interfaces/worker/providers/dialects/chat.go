package dialects

import (
	"encoding/json"
	"fmt"
)

// ChatDialect shapes OpenAI-compatible chat-multimodal payloads. It serves
// every chat-model target regardless of provider slug (openrouter, anthropic
// and chat-keyword models all land here via Registry dispatch), because the
// gateway carries the OpenAI chat shape through for all of them.
type ChatDialect struct{}

func NewChatDialect() *ChatDialect { return &ChatDialect{} }

func (ChatDialect) Encode(req Request) ([]byte, error) {
	type imageURL struct {
		URL string `json:"url"`
	}
	type contentPart struct {
		Type     string    `json:"type"`
		Text     string    `json:"text,omitempty"`
		ImageURL *imageURL `json:"image_url,omitempty"`
	}
	content := []contentPart{{Type: "text", Text: req.Prompt}}
	for _, u := range req.Images {
		if u == "" {
			continue
		}
		content = append(content, contentPart{Type: "image_url", ImageURL: &imageURL{URL: u}})
	}
	encoded, err := json.Marshal(map[string]interface{}{
		"model": req.Model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
		"modalities": []string{"text", "image"},
	})
	if err != nil {
		return nil, fmt.Errorf("序列化 chat 请求失败: %w", err)
	}
	return encoded, nil
}

func (ChatDialect) Decode(body []byte) ([]string, error) {
	return decodeWithPaths(body)
}
