package task

import (
	"context"

	tasks "ideaboard/api/internal/tasking/domain"
)

// UserQueryService is the application boundary consumed by user-facing task
// HTTP handlers. It excludes admin reads and all command operations.
type UserQueryService interface {
	GetUserTask(context.Context, string, int64) (*tasks.Task, error)
	GetUserTasks(context.Context, int64, int, int) ([]*tasks.Task, int64, error)
}

func (s *Service) GetUserTask(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	return task, nil
}

func (s *Service) GetUserTasks(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.taskRepo.GetByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	s.attachOutputsForTasks(ctx, items)
	return items, total, nil
}
