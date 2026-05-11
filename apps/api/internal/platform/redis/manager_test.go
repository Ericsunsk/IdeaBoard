package redis

import (
	"context"
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

func TestTryAssignTaskAndReleaseTask(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 101, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "a1"
	accountID, status, err := mgr.TryAssignTask(ctx, []uint64{101}, taskID)
	if err != nil {
		t.Fatalf("try assign: %v", err)
	}
	if status != AssignTaskStatusAssigned || accountID != 101 {
		t.Fatalf("unexpected assign result: account=%d status=%s", accountID, status)
	}
	count, err := mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 1 {
		t.Fatalf("concurrent count=%d err=%v", count, err)
	}

	running, err := mgr.GetClient().SMembers(ctx, KeyRunningTasks).Result()
	if err != nil || len(running) != 1 || running[0] != taskID {
		t.Fatalf("running set=%v err=%v", running, err)
	}

	if err := mgr.ReleaseTask(ctx, 101, taskID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := mgr.ReleaseTask(ctx, 101, taskID); err != nil {
		t.Fatalf("release second time: %v", err)
	}
	count, err = mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 0 {
		t.Fatalf("count should clamp to 0, got %d err=%v", count, err)
	}
	running, err = mgr.GetClient().SMembers(ctx, KeyRunningTasks).Result()
	if err != nil || len(running) != 0 {
		t.Fatalf("running should be empty, got=%v err=%v", running, err)
	}
}

func TestTryAssignTaskNoCapacity(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 301, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}
	_ = mgr.GetClient().HSet(ctx, KeyAccountConcurrent+":301", "count", 1).Err()

	accountID, status, err := mgr.TryAssignTask(ctx, []uint64{301}, "cap-1")
	if err != nil {
		t.Fatalf("try assign: %v", err)
	}
	if accountID != 0 || status != AssignTaskStatusNoCapacity {
		t.Fatalf("expected no capacity, got account=%d status=%s", accountID, status)
	}
}

func TestTryAssignTaskNoAccountsError(t *testing.T) {
	mgr := newTestRedisManager(t)
	_, _, err := mgr.TryAssignTask(context.Background(), nil, "none")
	if err == nil {
		t.Fatal("expected error when no account ids")
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
