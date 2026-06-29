package taskservice

import (
	"context"
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
	}
}
