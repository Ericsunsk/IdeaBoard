package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestRedisManager(t *testing.T) *RedisManager {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(s.Close)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisManager(client)
}

func testTask(id string) *GlobalQueueTask {
	return &GlobalQueueTask{TaskID: id, UserID: 1, Prompt: "p", Params: "{}", AccountIDs: []uint64{1}, CreatedAt: time.Now().Unix()}
}

func TestGlobalQueueFIFOAndFrontPriority(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()

	if err := mgr.PushGlobalQueue(ctx, testTask("t1")); err != nil {
		t.Fatalf("push t1: %v", err)
	}
	if err := mgr.PushGlobalQueue(ctx, testTask("t2")); err != nil {
		t.Fatalf("push t2: %v", err)
	}
	if err := mgr.PushGlobalQueueFront(ctx, testTask("front")); err != nil {
		t.Fatalf("push front: %v", err)
	}

	first, _ := mgr.PopGlobalQueue(ctx)
	second, _ := mgr.PopGlobalQueue(ctx)
	third, _ := mgr.PopGlobalQueue(ctx)
	if first == nil || first.TaskID != "front" || second == nil || second.TaskID != "t1" || third == nil || third.TaskID != "t2" {
		t.Fatalf("unexpected dequeue order: first=%v second=%v third=%v", first, second, third)
	}
}

func TestPopGlobalQueueEmptyReturnsNil(t *testing.T) {
	mgr := newTestRedisManager(t)
	task, err := mgr.PopGlobalQueue(context.Background())
	if err != nil {
		t.Fatalf("pop empty: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task, got %+v", task)
	}
}

func TestAssignTaskAndReleaseTask(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 101, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	task := testTask("a1")
	task.AccountIDs = []uint64{101}
	accountID, status, err := mgr.AssignTask(ctx, task.AccountIDs, task)
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if status != "assigned" || accountID != 101 {
		t.Fatalf("unexpected assign result: account=%d status=%s", accountID, status)
	}
	count, err := mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 1 {
		t.Fatalf("concurrent count=%d err=%v", count, err)
	}

	running, err := mgr.GetAllRunningTasks(ctx)
	if err != nil || len(running) != 1 || running[0] != task.TaskID {
		t.Fatalf("running set=%v err=%v", running, err)
	}

	if err := mgr.ReleaseTask(ctx, 101, task.TaskID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := mgr.ReleaseTask(ctx, 101, task.TaskID); err != nil {
		t.Fatalf("release second time: %v", err)
	}
	count, err = mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 0 {
		t.Fatalf("count should clamp to 0, got %d err=%v", count, err)
	}
	running, err = mgr.GetAllRunningTasks(ctx)
	if err != nil || len(running) != 0 {
		t.Fatalf("running should be empty, got=%v err=%v", running, err)
	}
}

func TestAssignTaskWaitQueueAndQueueFull(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 201, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}
	_ = mgr.GetClient().HSet(ctx, KeyAccountConcurrent+":201", "count", 1).Err()

	accountID, status, err := mgr.AssignTask(ctx, []uint64{201}, testTask("w1"))
	if err != nil {
		t.Fatalf("assign wait_queue: %v", err)
	}
	if accountID != 0 || status != "wait_queue" {
		t.Fatalf("expected wait_queue, got account=%d status=%s", accountID, status)
	}
	waitLen, _ := mgr.GetWaitQueueLength(ctx)
	if waitLen != 1 {
		t.Fatalf("wait len=%d want 1", waitLen)
	}

	for i := int64(1); i < MaxWaitQueueLen; i++ {
		payload, _ := json.Marshal(testTask("fill"))
		_ = mgr.GetClient().RPush(ctx, KeyWaitQueue, string(payload)).Err()
	}
	_, status, err = mgr.AssignTask(ctx, []uint64{201}, testTask("w2"))
	if err != nil {
		t.Fatalf("assign queue_full: %v", err)
	}
	if status != "queue_full" {
		t.Fatalf("expected queue_full, got %s", status)
	}
}

func TestCancelMarkCheckClear(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	taskID := "cancel-1"

	if err := mgr.MarkTaskCancelled(ctx, taskID, 5*time.Minute); err != nil {
		t.Fatalf("mark cancelled: %v", err)
	}
	cancelled, err := mgr.IsTaskCancelled(ctx, taskID)
	if err != nil || !cancelled {
		t.Fatalf("is cancelled=%v err=%v", cancelled, err)
	}
	if err := mgr.ClearTaskCancelled(ctx, taskID); err != nil {
		t.Fatalf("clear cancelled: %v", err)
	}
	cancelled, err = mgr.IsTaskCancelled(ctx, taskID)
	if err != nil || cancelled {
		t.Fatalf("is cancelled after clear=%v err=%v", cancelled, err)
	}
}

func TestAssignTaskNoAccountsError(t *testing.T) {
	mgr := newTestRedisManager(t)
	_, _, err := mgr.AssignTask(context.Background(), nil, testTask("none"))
	if err == nil {
		t.Fatal("expected error when no account ids")
	}
}
