package taskqueue

import (
	"testing"

	"api/internal/platform/redis"
)

func TestAsynqPayloadEncodeDecodeRoundTrip(t *testing.T) {
	task := &redis.GlobalQueueTask{
		TaskID:     "task-1",
		UserID:     9,
		Prompt:     "draw",
		Params:     `{"ratio":"1:1"}`,
		AccountIDs: []uint64{1, 2},
		RetryCount: 1,
		CreatedAt:  12345,
	}
	data, err := EncodeAsynqPayload(task)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeAsynqPayload(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.TaskID != task.TaskID || decoded.Params != task.Params || len(decoded.AccountIDs) != 2 {
		t.Fatalf("decoded mismatch: %+v", decoded)
	}
}

func TestAsynqPayloadDecodeInvalidJSON(t *testing.T) {
	if _, err := DecodeAsynqPayload([]byte("{invalid")); err == nil {
		t.Fatal("expected decode error for invalid json")
	}
}

func TestAsynqPayloadDecodeMissingTaskID(t *testing.T) {
	if _, err := DecodeAsynqPayload([]byte(`{"user_id":1}`)); err == nil {
		t.Fatal("expected empty task_id error")
	}
}
