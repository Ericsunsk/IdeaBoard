package redis

import (
	"context"
	"fmt"
	"sort"
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

func TestTryAssignTaskCreatesLeaseAndReleaseClearsIt(t *testing.T) {
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
	leasedAccountID, ok, err := mgr.GetTaskLease(ctx, taskID)
	if err != nil || !ok || leasedAccountID != 101 {
		t.Fatalf("lease account=%d ok=%v err=%v", leasedAccountID, ok, err)
	}
	active, err := mgr.HasActiveLease(ctx, taskID, 101)
	if err != nil || !active {
		t.Fatalf("active lease=%v err=%v", active, err)
	}

	running, err := mgr.GetClient().SMembers(ctx, KeyRunningTasks).Result()
	if err != nil || len(running) != 1 || running[0] != taskID {
		t.Fatalf("running set=%v err=%v", running, err)
	}

	if err := mgr.ReleaseTask(ctx, 101, taskID); err != nil {
		t.Fatalf("release: %v", err)
	}
	count, err = mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 0 {
		t.Fatalf("count should be 0, got %d err=%v", count, err)
	}
	running, err = mgr.GetClient().SMembers(ctx, KeyRunningTasks).Result()
	if err != nil || len(running) != 0 {
		t.Fatalf("running should be empty, got=%v err=%v", running, err)
	}
	leasedAccountID, ok, err = mgr.GetTaskLease(ctx, taskID)
	if err != nil || ok || leasedAccountID != 0 {
		t.Fatalf("lease should be cleared, account=%d ok=%v err=%v", leasedAccountID, ok, err)
	}
}

func TestReleaseTaskIsIdempotent(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 201, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "idempotent-1"
	if _, status, err := mgr.TryAssignTask(ctx, []uint64{201}, taskID); err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("assign status=%s err=%v", status, err)
	}
	if err := mgr.ReleaseTask(ctx, 201, taskID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := mgr.ReleaseTask(ctx, 201, taskID); err != nil {
		t.Fatalf("release second time: %v", err)
	}

	count, err := mgr.GetAccountConcurrent(ctx, 201)
	if err != nil || count != 0 {
		t.Fatalf("count should stay 0, got %d err=%v", count, err)
	}
}

func TestReleaseTaskWrongAccountDoesNotDecrement(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 401, 1); err != nil {
		t.Fatalf("init account 401: %v", err)
	}
	if err := mgr.EnsureAccountConcurrent(ctx, 402, 1); err != nil {
		t.Fatalf("init account 402: %v", err)
	}

	taskID := "wrong-account-1"
	if _, status, err := mgr.TryAssignTask(ctx, []uint64{401}, taskID); err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("assign status=%s err=%v", status, err)
	}
	if err := mgr.ReleaseTask(ctx, 402, taskID); err != nil {
		t.Fatalf("wrong account release: %v", err)
	}

	count401, err := mgr.GetAccountConcurrent(ctx, 401)
	if err != nil || count401 != 1 {
		t.Fatalf("account 401 count=%d err=%v", count401, err)
	}
	count402, err := mgr.GetAccountConcurrent(ctx, 402)
	if err != nil || count402 != 0 {
		t.Fatalf("account 402 count=%d err=%v", count402, err)
	}
	leasedAccountID, ok, err := mgr.GetTaskLease(ctx, taskID)
	if err != nil || !ok || leasedAccountID != 401 {
		t.Fatalf("lease account=%d ok=%v err=%v", leasedAccountID, ok, err)
	}
	running, err := mgr.GetClient().SIsMember(ctx, KeyRunningTasks, taskID).Result()
	if err != nil || !running {
		t.Fatalf("running=%v err=%v", running, err)
	}
}

func TestReleaseTaskWithoutLeaseDoesNotDecrementOrRemoveRunning(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 501, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}
	if err := mgr.GetClient().HSet(ctx, KeyAccountConcurrent+":501", "count", 1).Err(); err != nil {
		t.Fatalf("seed count: %v", err)
	}
	if err := mgr.GetClient().SAdd(ctx, KeyRunningTasks, "orphan-1").Err(); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	if err := mgr.ReleaseTask(ctx, 501, "orphan-1"); err != nil {
		t.Fatalf("release without lease: %v", err)
	}
	if err := mgr.ReleaseTask(ctx, 501, "orphan-1"); err != nil {
		t.Fatalf("release without lease second time: %v", err)
	}
	count, err := mgr.GetAccountConcurrent(ctx, 501)
	if err != nil || count != 1 {
		t.Fatalf("count should stay unchanged without lease, got %d err=%v", count, err)
	}
	running, err := mgr.GetClient().SIsMember(ctx, KeyRunningTasks, "orphan-1").Result()
	if err != nil || !running {
		t.Fatalf("running marker should stay without lease, running=%v err=%v", running, err)
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

func TestTryAssignTaskExistingLeaseDoesNotIncrement(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 701, 3); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "duplicate-lease-1"
	if _, status, err := mgr.TryAssignTask(ctx, []uint64{701}, taskID); err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("first assign status=%s err=%v", status, err)
	}
	accountID, status, err := mgr.TryAssignTask(ctx, []uint64{701}, taskID)
	if err != nil {
		t.Fatalf("second assign: %v", err)
	}
	if accountID != 0 || status != AssignTaskStatusNoCapacity {
		t.Fatalf("second assign should be no capacity, got account=%d status=%s", accountID, status)
	}
	count, err := mgr.GetAccountConcurrent(ctx, 701)
	if err != nil || count != 1 {
		t.Fatalf("count should not increment twice, got %d err=%v", count, err)
	}
}

func TestTryAssignTaskNoAccountsError(t *testing.T) {
	mgr := newTestRedisManager(t)
	_, _, err := mgr.TryAssignTask(context.Background(), nil, "none")
	if err == nil {
		t.Fatal("expected error when no account ids")
	}
}

func TestRebuildRunningTaskLeases(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	for _, accountID := range []uint64{601, 602, 603} {
		if err := mgr.EnsureAccountConcurrent(ctx, accountID, 3); err != nil {
			t.Fatalf("init account %d: %v", accountID, err)
		}
		if err := mgr.GetClient().HSet(ctx, fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID), "count", 9).Err(); err != nil {
			t.Fatalf("seed account %d count: %v", accountID, err)
		}
	}
	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, "stale", 603).Err(); err != nil {
		t.Fatalf("seed stale lease: %v", err)
	}
	if err := mgr.GetClient().SAdd(ctx, KeyRunningTasks, "stale").Err(); err != nil {
		t.Fatalf("seed stale running: %v", err)
	}

	leases := []TaskLease{
		{TaskID: "task-a", AccountID: 601},
		{TaskID: "task-b", AccountID: 601},
		{TaskID: "task-c", AccountID: 602},
	}
	if err := mgr.RebuildRunningTaskLeases(ctx, leases, []uint64{601, 602, 603}); err != nil {
		t.Fatalf("rebuild leases: %v", err)
	}

	gotLeases, err := mgr.ListTaskLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	sort.Slice(gotLeases, func(i, j int) bool {
		return gotLeases[i].TaskID < gotLeases[j].TaskID
	})
	if len(gotLeases) != len(leases) {
		t.Fatalf("leases len=%d want=%d leases=%v", len(gotLeases), len(leases), gotLeases)
	}
	for i, want := range leases {
		if gotLeases[i] != want {
			t.Fatalf("lease[%d]=%+v want=%+v", i, gotLeases[i], want)
		}
	}
	for accountID, want := range map[uint64]int{601: 2, 602: 1, 603: 0} {
		count, err := mgr.GetAccountConcurrent(ctx, accountID)
		if err != nil || count != want {
			t.Fatalf("account %d count=%d want=%d err=%v", accountID, count, want, err)
		}
	}
	running, err := mgr.GetClient().SMembers(ctx, KeyRunningTasks).Result()
	if err != nil {
		t.Fatalf("running members: %v", err)
	}
	sort.Strings(running)
	if got, want := running, []string{"task-a", "task-b", "task-c"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("running=%v want=%v", got, want)
	}
}

func TestListTaskLeasesRejectsMalformedAccountID(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()

	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, "bad-task", "123abc").Err(); err != nil {
		t.Fatalf("seed malformed lease: %v", err)
	}

	if _, err := mgr.ListTaskLeases(ctx); err == nil {
		t.Fatal("expected malformed lease account id to fail")
	}
}

func TestGetTaskLeaseRejectsMalformedAccountID(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()

	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, "bad-task", "x-uid").Err(); err != nil {
		t.Fatalf("seed malformed lease: %v", err)
	}

	if _, _, err := mgr.GetTaskLease(ctx, "bad-task"); err == nil {
		t.Fatal("expected malformed lease account id to fail")
	}
}

func TestReleaseTaskMalformedLeaseDoesNotDecrement(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 801, 2); err != nil {
		t.Fatalf("init account: %v", err)
	}
	if err := mgr.GetClient().HSet(ctx, fmt.Sprintf("%s:%d", KeyAccountConcurrent, 801), "count", 1).Err(); err != nil {
		t.Fatalf("seed count: %v", err)
	}
	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, "malformed-lease-1", "invalid-owner").Err(); err != nil {
		t.Fatalf("seed malformed lease: %v", err)
	}
	if err := mgr.GetClient().SAdd(ctx, KeyRunningTasks, "malformed-lease-1").Err(); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	if err := mgr.ReleaseTask(ctx, 801, "malformed-lease-1"); err != nil {
		t.Fatalf("release with malformed lease: %v", err)
	}

	count, err := mgr.GetAccountConcurrent(ctx, 801)
	if err != nil || count != 1 {
		t.Fatalf("count should not change on malformed lease, got %d err=%v", count, err)
	}
	running, err := mgr.GetClient().SIsMember(ctx, KeyRunningTasks, "malformed-lease-1").Result()
	if err != nil || !running {
		t.Fatalf("running marker should be kept, running=%v err=%v", running, err)
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
