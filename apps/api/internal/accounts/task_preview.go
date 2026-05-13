package accounts

import (
	"encoding/json"
	"strings"

	"api/internal/tasks"
)

func (s *AccountService) applyPreviewForTasks(items []*tasks.Task) {
	if len(items) == 0 || s.previewResolver == nil {
		return
	}
	transform := s.previewResolver.PreviewURLTransformer()
	if transform == nil {
		return
	}
	for _, item := range items {
		s.applyPreviewForTask(item, transform)
	}
}

func (s *AccountService) applyPreviewForTask(task *tasks.Task, transform func(string) string) {
	if task == nil || strings.TrimSpace(task.Result) == "" {
		return
	}
	if !strings.Contains(task.Result, "image_url") && !strings.Contains(task.Result, "image_urls") {
		return
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(task.Result), &resultMap); err != nil {
		return
	}

	changed := false
	if raw, ok := resultMap["image_url"].(string); ok {
		next := transform(strings.TrimSpace(raw))
		if next != raw {
			resultMap["image_url"] = next
			changed = true
		}
	}

	if rawList, ok := resultMap["image_urls"].([]interface{}); ok {
		for i, item := range rawList {
			raw, ok := item.(string)
			if !ok {
				continue
			}
			next := transform(strings.TrimSpace(raw))
			if next != raw {
				rawList[i] = next
				changed = true
			}
		}
		resultMap["image_urls"] = rawList
	}

	if !changed {
		return
	}
	if encoded, err := json.Marshal(resultMap); err == nil {
		task.Result = string(encoded)
	}
}
