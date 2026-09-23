package querytests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"
)

func TestCreateAndGetTaskByID(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	taskID := "task-create-get-1"

	createdID, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     taskID,
		UserID:     1,
		UserPrompt: "test prompt",
		Status:     "pending",
		Params:     `{}`,
		CreatedAt:  dbutil.TimestamptzFromTime(time.Now()),
		UpdatedAt:  dbutil.TimestamptzFromTime(time.Now()),
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for index, params := range []string{
		"{broken", "", `{"generation":{"requested_output_count":"not-a-number"}}`,
		`{"generation":{"requested_output_count":"999999999999999999999999999"}}`,
	} {
		_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
			TaskID:     fmt.Sprintf("task-count-invalid-%d", index),
			UserID:     1,
			UserPrompt: "count invalid",
			Status:     "pending",
			Params:     params,
			CreatedAt:  dbutil.TimestamptzFromTime(time.Now()),
			UpdatedAt:  dbutil.TimestamptzFromTime(time.Now()),
		})
		if err != nil {
			t.Fatalf("CreateTask(invalid params): %v", err)
		}
	}
	if createdID <= 0 {
		t.Fatalf("CreateTask returned id=%d", createdID)
	}

	taskRow, err := q.GetTaskByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskByTaskID: %v", err)
	}
	task := taskRow.Task
	if task.TaskID != taskID || task.UserID != 1 || task.Status != "pending" {
		t.Fatalf("unexpected: id=%q user=%d status=%q", task.TaskID, task.UserID, task.Status)
	}
}

func TestGetTaskByTaskIDAndUserIDFiltersUser(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     "task-user-filter-1",
		UserID:     3,
		UserPrompt: "user 3 task",
		Status:     "pending",
		Params:     `{}`,
		CreatedAt:  dbutil.TimestamptzFromTime(time.Now()),
		UpdatedAt:  dbutil.TimestamptzFromTime(time.Now()),
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, err = q.GetTaskByTaskIDAndUserID(context.Background(), dbsqlc.GetTaskByTaskIDAndUserIDParams{
		TaskID: "task-user-filter-1",
		UserID: 1, // wrong user
	})
	if err == nil {
		t.Fatal("expected error when fetching task with wrong user ID")
	}
}

func TestCountTasksByStatus(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     "task-count-1",
		UserID:     1,
		UserPrompt: "count me",
		Status:     "pending",
		Params:     `{}`,
		CreatedAt:  dbutil.TimestamptzFromTime(time.Now()),
		UpdatedAt:  dbutil.TimestamptzFromTime(time.Now()),
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count, err := q.CountTasksByStatus(context.Background(), "pending")
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountTasksByStatus(pending)=%d, want 1", count)
	}
}

func TestCountAllTaskCalls(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     "task-count-all-1",
		UserID:     1,
		UserPrompt: "count all",
		Status:     "pending",
		Params:     `{"generation":{"requested_output_count":2}}`,
		CreatedAt:  dbutil.TimestamptzFromTime(time.Now()),
		UpdatedAt:  dbutil.TimestamptzFromTime(time.Now()),
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count, err := q.CountAllTaskCalls(context.Background())
	if err != nil {
		t.Fatalf("CountAllTaskCalls: %v", err)
	}
	if count != 6 {
		t.Fatalf("CountAllTaskCalls=%d, want 6", count)
	}
}
