package accounts

import (
	"log"
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
