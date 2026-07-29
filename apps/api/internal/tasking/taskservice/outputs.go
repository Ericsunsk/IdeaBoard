package taskservice

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/tasking/tasks"
)

func (s *Service) attachOutputsForTasks(ctx context.Context, items []*tasks.Task) {
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

	// Short retry: a single transient DB blip should not cause all outputs
	// to be silently missing from the task list.
	const maxRetries = 2
	var outputsByTaskID map[string][]tasks.TaskOutput
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		outputsByTaskID, err = s.taskRepo.ListOutputsByTaskIDs(ctx, taskIDs)
		if err == nil {
			break
		}
		if attempt < maxRetries {
			time.Sleep(50 * time.Millisecond * time.Duration(attempt+1))
		}
	}
	if err != nil {
		slog.Warn("load task outputs failed after retries", "err", err, "retries", maxRetries)
		return
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		item.Outputs = outputsByTaskID[item.TaskID]
		if hasReclaimedAssetOutput(item.Outputs) {
			item.Result = taskResultWithoutReclaimedImageURLs(item.Result)
		}
	}
}

func hasReclaimedAssetOutput(outputs []tasks.TaskOutput) bool {
	for _, output := range outputs {
		if output.OutputKind == "asset" && output.MediaAssetID != nil && strings.TrimSpace(output.URL) == "" {
			return true
		}
	}
	return false
}

// Task results are immutable provider facts in storage. The read projection
// removes retired URLs so operators cannot be sent to an object the user chose
// to delete, while retaining the rest of the execution and billing evidence.
func taskResultWithoutReclaimedImageURLs(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result == nil {
		return raw
	}
	delete(result, "image_url")
	delete(result, "image_urls")
	result["image_availability"] = "deleted_by_user"
	normalized, err := json.Marshal(result)
	if err != nil {
		return raw
	}
	return string(normalized)
}
