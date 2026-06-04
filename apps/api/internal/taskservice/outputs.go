package taskservice

import (
	"context"
	"log/slog"
	"strings"

	"ideaboard/api/internal/tasks"
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

	outputsByTaskID, err := s.taskRepo.ListOutputsByTaskIDs(ctx, taskIDs)
	if err != nil {
		slog.Warn("load task outputs failed", "err", err)
		return
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		item.Outputs = outputsByTaskID[item.TaskID]
	}
}
