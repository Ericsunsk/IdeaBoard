package taskqueue

import (
	"encoding/json"
	"fmt"
	"strings"

	"api/internal/platform/redis"
)

const (
	AsynqTaskTypeDrawingGenerate = "drawing:generate"
	AsynqQueueDefault            = "drawing:default"
)

type AsynqTaskPayload struct {
	TaskID     string   `json:"task_id"`
	UserID     uint64   `json:"user_id"`
	Prompt     string   `json:"prompt"`
	Params     string   `json:"params"`
	AccountIDs []uint64 `json:"account_ids"`
	RetryCount int      `json:"retry_count"`
	CreatedAt  int64    `json:"created_at"`
}

func EncodeAsynqPayload(task *redis.GlobalQueueTask) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("encode asynq payload: task is nil")
	}
	payload := AsynqTaskPayload{
		TaskID:     strings.TrimSpace(task.TaskID),
		UserID:     task.UserID,
		Prompt:     task.Prompt,
		Params:     task.Params,
		AccountIDs: task.AccountIDs,
		RetryCount: task.RetryCount,
		CreatedAt:  task.CreatedAt,
	}
	if payload.TaskID == "" {
		return nil, fmt.Errorf("encode asynq payload: empty task_id")
	}
	return json.Marshal(payload)
}

func DecodeAsynqPayload(data []byte) (*redis.GlobalQueueTask, error) {
	var payload AsynqTaskPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode asynq payload: %w", err)
	}
	if strings.TrimSpace(payload.TaskID) == "" {
		return nil, fmt.Errorf("decode asynq payload: empty task_id")
	}
	return &redis.GlobalQueueTask{
		TaskID:     payload.TaskID,
		UserID:     payload.UserID,
		Prompt:     payload.Prompt,
		Params:     payload.Params,
		AccountIDs: payload.AccountIDs,
		RetryCount: payload.RetryCount,
		CreatedAt:  payload.CreatedAt,
	}, nil
}
