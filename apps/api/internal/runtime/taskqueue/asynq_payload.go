package taskqueue

import (
	"encoding/json"
	"fmt"
	"strings"

	"api/internal/runtime/queuetypes"
)

const (
	AsynqTaskTypeDrawingGenerate = "drawing:generate"
	AsynqQueueDefault            = "drawing:default"
)

func EncodeAsynqPayload(task *queuetypes.TaskPayload) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("encode asynq payload: task is nil")
	}
	payload := *task
	payload.TaskID = strings.TrimSpace(payload.TaskID)
	if payload.TaskID == "" {
		return nil, fmt.Errorf("encode asynq payload: empty task_id")
	}
	return json.Marshal(payload)
}

func DecodeAsynqPayload(data []byte) (*queuetypes.TaskPayload, error) {
	var payload queuetypes.TaskPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode asynq payload: %w", err)
	}
	payload.TaskID = strings.TrimSpace(payload.TaskID)
	if payload.TaskID == "" {
		return nil, fmt.Errorf("decode asynq payload: empty task_id")
	}
	return &payload, nil
}
