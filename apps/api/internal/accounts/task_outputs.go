package accounts

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"api/internal/tasks"
)

func (s *AccountService) attachOutputsForTasks(items []*tasks.Task) {
	if len(items) == 0 || s == nil || s.taskRepo == nil {
		return
	}

	taskIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.TaskID) == "" {
			continue
		}
		taskIDs = append(taskIDs, item.TaskID)
	}
	if len(taskIDs) == 0 {
		return
	}

	outputsByTaskID, err := s.taskRepo.ListOutputsByTaskIDs(taskIDs)
	if err != nil {
		log.Printf("[AccountService] load task outputs failed: %v", err)
		return
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		item.Outputs = outputsByTaskID[item.TaskID]
	}
}

func taskOutputsFromResult(taskID string, rawResult string) ([]tasks.TaskOutput, error) {
	rawResult = strings.TrimSpace(rawResult)
	if rawResult == "" {
		return nil, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(rawResult), &payload); err != nil {
		return nil, nil
	}

	urls := resultImageURLs(payload)
	outputs := make([]tasks.TaskOutput, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, rawURL := range urls {
		imageURL := strings.TrimSpace(rawURL)
		if imageURL == "" {
			continue
		}
		if _, ok := seen[imageURL]; ok {
			continue
		}
		seen[imageURL] = struct{}{}
		if isInlineImageDataURL(imageURL) {
			return nil, fmt.Errorf("任务结果包含未持久化的内联图片")
		}
		outputs = append(outputs, tasks.TaskOutput{
			TaskID:     taskID,
			Role:       "image",
			URL:        imageURL,
			StorageKey: outputStorageKey(imageURL),
			SortIndex:  len(outputs),
		})
	}
	return outputs, nil
}

func resultImageURLs(payload map[string]interface{}) []string {
	urls := make([]string, 0, 4)
	if raw, ok := payload["image_urls"]; ok {
		switch values := raw.(type) {
		case []interface{}:
			for _, value := range values {
				if imageURL, ok := value.(string); ok {
					urls = append(urls, imageURL)
				}
			}
		case []string:
			urls = append(urls, values...)
		}
	}
	if raw, ok := payload["image_url"].(string); ok {
		urls = append(urls, raw)
	}
	return urls
}

func isInlineImageDataURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:image/")
}

func outputStorageKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "/uploads/") {
		return strings.TrimPrefix(trimmed, "/")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return ""
	}
	path := strings.TrimSpace(parsed.Path)
	index := strings.Index(path, "/uploads/")
	if index < 0 {
		return ""
	}
	return strings.TrimPrefix(path[index:], "/")
}
