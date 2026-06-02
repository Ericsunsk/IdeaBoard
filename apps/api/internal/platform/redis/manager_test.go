package redis

import (
	"context"
	"encoding/json"
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

func TestTaskLeaseAssigningConfirmRunningRelease(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 101, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "a1"
	accountID, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{101}, taskID)
	if err != nil {
		t.Fatalf("try assign: %v", err)
	}
	if status != AssignTaskStatusAssigned || accountID != 101 || leaseToken == "" {
		t.Fatalf("unexpected assign result: account=%d token=%q status=%s", accountID, leaseToken, status)
	}
	count, err := mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 1 {
		t.Fatalf("concurrent count=%d err=%v", count, err)
	}
	assertSingleLease(t, mgr, ctx, TaskLease{
		TaskID:     taskID,
		AccountID:  101,
		LeaseToken: leaseToken,
		State:      TaskLeaseStateAssigning,
	})
	active, err := mgr.HasActiveLease(ctx, taskID, 101)
	if err != nil || !active {
		t.Fatalf("active lease=%v err=%v", active, err)
	}
	assertSetMembers(t, mgr, ctx, KeyRunningTasks, []string{taskID})

	if err := mgr.ConfirmTaskRunning(ctx, 101, taskID, leaseToken); err != nil {
		t.Fatalf("confirm running: %v", err)
	}
	lease := assertSingleLease(t, mgr, ctx, TaskLease{
		TaskID:     taskID,
		AccountID:  101,
		LeaseToken: leaseToken,
		State:      TaskLeaseStateRunning,
	})
	if lease.RunningAtUnixMilli == 0 || lease.UpdatedAtUnixMilli == 0 {
		t.Fatalf("running lease should include timestamps: %+v", lease)
	}

	if err := mgr.ReleaseTask(ctx, 101, taskID, leaseToken); err != nil {
		t.Fatalf("release: %v", err)
	}
	count, err = mgr.GetAccountConcurrent(ctx, 101)
	if err != nil || count != 0 {
		t.Fatalf("count should be 0, got %d err=%v", count, err)
	}
	assertSetMembers(t, mgr, ctx, KeyRunningTasks, nil)
	leasedAccountID, ok, err := mgr.GetTaskLease(ctx, taskID)
	if err != nil || ok || leasedAccountID != 0 {
		t.Fatalf("lease should be cleared, account=%d ok=%v err=%v", leasedAccountID, ok, err)
	}
}

func TestReleaseTaskTokenMismatchDoesNotReleaseOrDecrement(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 201, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "token-mismatch-1"
	accountID, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{201}, taskID)
	if err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("assign account=%d token=%q status=%s err=%v", accountID, leaseToken, status, err)
	}
	if err := mgr.ReleaseTask(ctx, 201, taskID, "wrong-token"); err == nil {
		t.Fatal("expected token mismatch release to fail")
	}

	count, err := mgr.GetAccountConcurrent(ctx, 201)
	if err != nil || count != 1 {
		t.Fatalf("count should stay 1, got %d err=%v", count, err)
	}
	assertSingleLease(t, mgr, ctx, TaskLease{
		TaskID:     taskID,
		AccountID:  201,
		LeaseToken: leaseToken,
		State:      TaskLeaseStateAssigning,
	})
	assertSetMembers(t, mgr, ctx, KeyRunningTasks, []string{taskID})

	if err := mgr.ReleaseTask(ctx, 201, taskID, leaseToken); err != nil {
		t.Fatalf("release with correct token: %v", err)
	}
	count, err = mgr.GetAccountConcurrent(ctx, 201)
	if err != nil || count != 0 {
		t.Fatalf("count should be 0 after correct release, got %d err=%v", count, err)
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
	_, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{401}, taskID)
	if err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("assign status=%s err=%v", status, err)
	}
	if err := mgr.ReleaseTask(ctx, 402, taskID, leaseToken); err == nil {
		t.Fatal("expected wrong account release to fail")
	}

	count401, err := mgr.GetAccountConcurrent(ctx, 401)
	if err != nil || count401 != 1 {
		t.Fatalf("account 401 count=%d err=%v", count401, err)
	}
	count402, err := mgr.GetAccountConcurrent(ctx, 402)
	if err != nil || count402 != 0 {
		t.Fatalf("account 402 count=%d err=%v", count402, err)
	}
	assertSingleLease(t, mgr, ctx, TaskLease{
		TaskID:     taskID,
		AccountID:  401,
		LeaseToken: leaseToken,
		State:      TaskLeaseStateAssigning,
	})
	assertSetMembers(t, mgr, ctx, KeyRunningTasks, []string{taskID})
}

func TestReleaseTaskIsIdempotentAfterLeaseRemoved(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 501, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "idempotent-1"
	_, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{501}, taskID)
	if err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("assign status=%s err=%v", status, err)
	}
	if err := mgr.ReleaseTask(ctx, 501, taskID, leaseToken); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := mgr.ReleaseTask(ctx, 501, taskID, leaseToken); err != nil {
		t.Fatalf("release second time: %v", err)
	}

	count, err := mgr.GetAccountConcurrent(ctx, 501)
	if err != nil || count != 0 {
		t.Fatalf("count should stay 0, got %d err=%v", count, err)
	}
}

func TestTaskLeaseReconcileLockBlocksAssignment(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mgr.EnsureAccountConcurrent(ctx, 151, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}

	locked := make(chan struct{})
	release := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.WithTaskLeaseReconcileLock(ctx, func(lockCtx context.Context) error {
			close(locked)
			select {
			case <-release:
				return nil
			case <-lockCtx.Done():
				return lockCtx.Err()
			}
		})
	}()

	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for reconcile lock: %v", ctx.Err())
	}

	accountID, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{151}, "blocked-by-reconcile")
	if err != nil {
		t.Fatalf("try assign during reconcile lock: %v", err)
	}
	if accountID != 0 || leaseToken != "" || status != AssignTaskStatusReconcileLocked {
		t.Fatalf("assign during lock account=%d token=%q status=%s", accountID, leaseToken, status)
	}

	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("reconcile lock returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for reconcile lock release: %v", ctx.Err())
	}

	accountID, leaseToken, status, err = mgr.TryAssignTask(ctx, []int64{151}, "assigned-after-reconcile")
	if err != nil {
		t.Fatalf("try assign after reconcile lock: %v", err)
	}
	if accountID != 151 || leaseToken == "" || status != AssignTaskStatusAssigned {
		t.Fatalf("assign after lock account=%d token=%q status=%s", accountID, leaseToken, status)
	}
}

func TestTryAssignTaskNoCapacity(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 301, 1); err != nil {
		t.Fatalf("init account: %v", err)
	}
	_ = mgr.GetClient().HSet(ctx, KeyAccountConcurrent+":301", "count", 1).Err()

	accountID, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{301}, "cap-1")
	if err != nil {
		t.Fatalf("try assign: %v", err)
	}
	if accountID != 0 || leaseToken != "" || status != AssignTaskStatusNoCapacity {
		t.Fatalf("expected no capacity, got account=%d token=%q status=%s", accountID, leaseToken, status)
	}
}

func TestTryAssignTaskExistingLeaseDoesNotIncrement(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	if err := mgr.EnsureAccountConcurrent(ctx, 701, 3); err != nil {
		t.Fatalf("init account: %v", err)
	}

	taskID := "duplicate-lease-1"
	if _, _, status, err := mgr.TryAssignTask(ctx, []int64{701}, taskID); err != nil || status != AssignTaskStatusAssigned {
		t.Fatalf("first assign status=%s err=%v", status, err)
	}
	accountID, leaseToken, status, err := mgr.TryAssignTask(ctx, []int64{701}, taskID)
	if err != nil {
		t.Fatalf("second assign: %v", err)
	}
	if accountID != 0 || leaseToken != "" || status != AssignTaskStatusActiveLease {
		t.Fatalf("second assign should be active lease, got account=%d token=%q status=%s", accountID, leaseToken, status)
	}
	count, err := mgr.GetAccountConcurrent(ctx, 701)
	if err != nil || count != 1 {
		t.Fatalf("count should not increment twice, got %d err=%v", count, err)
	}
}

func TestTryAssignTaskNoAccountsError(t *testing.T) {
	mgr := newTestRedisManager(t)
	_, _, _, err := mgr.TryAssignTask(context.Background(), nil, "none")
	if err == nil {
		t.Fatal("expected error when no account ids")
	}
}

func TestRebuildRunningTaskLeasesRebuildsRunningAndPreservesRecentAssigning(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()
	for _, accountID := range []int64{601, 602, 603} {
		if err := mgr.EnsureAccountConcurrent(ctx, accountID, 3); err != nil {
			t.Fatalf("init account %d: %v", accountID, err)
		}
		if err := mgr.GetClient().HSet(ctx, fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID), "count", 9).Err(); err != nil {
			t.Fatalf("seed account %d count: %v", accountID, err)
		}
	}

	now := time.Now()
	seedLease(t, mgr, ctx, TaskLease{
		TaskID:              "stale-assigning",
		AccountID:           603,
		LeaseToken:          "stale-token",
		State:               TaskLeaseStateAssigning,
		AssignedAtUnixMilli: now.Add(-taskAssigningLeaseRebuildGrace - time.Second).UnixMilli(),
		UpdatedAtUnixMilli:  now.Add(-taskAssigningLeaseRebuildGrace - time.Second).UnixMilli(),
	})
	seedLease(t, mgr, ctx, TaskLease{
		TaskID:              "recent-assigning",
		AccountID:           603,
		LeaseToken:          "recent-token",
		State:               TaskLeaseStateAssigning,
		AssignedAtUnixMilli: now.UnixMilli(),
		UpdatedAtUnixMilli:  now.UnixMilli(),
	})

	runningLeases := []TaskLease{
		{TaskID: "task-a", AccountID: 601, LeaseToken: "lease-token-a"},
		{TaskID: "task-b", AccountID: 601, LeaseToken: "lease-token-b"},
		{TaskID: "task-c", AccountID: 602, LeaseToken: "known-token"},
	}
	if err := mgr.RebuildRunningTaskLeases(ctx, runningLeases, []int64{601, 602, 603}); err != nil {
		t.Fatalf("rebuild leases: %v", err)
	}

	gotLeases, err := mgr.ListTaskLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	sort.Slice(gotLeases, func(i, j int) bool {
		return gotLeases[i].TaskID < gotLeases[j].TaskID
	})
	gotByTask := make(map[string]TaskLease, len(gotLeases))
	for _, lease := range gotLeases {
		gotByTask[lease.TaskID] = lease
	}
	for _, taskID := range []string{"recent-assigning", "task-a", "task-b", "task-c"} {
		if _, ok := gotByTask[taskID]; !ok {
			t.Fatalf("missing rebuilt lease %s in %+v", taskID, gotLeases)
		}
	}
	if _, ok := gotByTask["stale-assigning"]; ok {
		t.Fatalf("stale assigning lease should be dropped: %+v", gotLeases)
	}
	if gotByTask["recent-assigning"].State != TaskLeaseStateAssigning {
		t.Fatalf("recent assigning state=%s", gotByTask["recent-assigning"].State)
	}
	for _, taskID := range []string{"task-a", "task-b", "task-c"} {
		lease := gotByTask[taskID]
		if lease.State != TaskLeaseStateRunning || lease.LeaseToken == "" || lease.RunningAtUnixMilli == 0 {
			t.Fatalf("running lease %s not normalized: %+v", taskID, lease)
		}
	}
	for accountID, want := range map[int64]int{601: 2, 602: 1, 603: 1} {
		count, err := mgr.GetAccountConcurrent(ctx, accountID)
		if err != nil || count != want {
			t.Fatalf("account %d count=%d want=%d err=%v", accountID, count, want, err)
		}
	}
	assertSetMembers(t, mgr, ctx, KeyRunningTasks, []string{"recent-assigning", "task-a", "task-b", "task-c"})
}

func TestListTaskLeasesRejectsMalformedJSON(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()

	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, "bad-task", "123abc").Err(); err != nil {
		t.Fatalf("seed malformed lease: %v", err)
	}

	if _, err := mgr.ListTaskLeases(ctx); err == nil {
		t.Fatal("expected malformed lease JSON to fail")
	}
}

func TestGetTaskLeaseRejectsMalformedJSON(t *testing.T) {
	mgr := newTestRedisManager(t)
	ctx := context.Background()

	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, "bad-task", "x-uid").Err(); err != nil {
		t.Fatalf("seed malformed lease: %v", err)
	}

	if _, _, err := mgr.GetTaskLease(ctx, "bad-task"); err == nil {
		t.Fatal("expected malformed lease JSON to fail")
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

	if err := mgr.ReleaseTask(ctx, 801, "malformed-lease-1", "token"); err == nil {
		t.Fatal("expected malformed release to fail")
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

func assertSingleLease(t *testing.T, mgr *RedisManager, ctx context.Context, want TaskLease) TaskLease {
	t.Helper()
	leases, err := mgr.ListTaskLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("leases len=%d want=1 leases=%+v", len(leases), leases)
	}
	got := leases[0]
	if got.TaskID != want.TaskID || got.AccountID != want.AccountID || got.LeaseToken != want.LeaseToken || got.State != want.State {
		t.Fatalf("lease=%+v want task=%s account=%d token=%q state=%s", got, want.TaskID, want.AccountID, want.LeaseToken, want.State)
	}
	return got
}

func assertSetMembers(t *testing.T, mgr *RedisManager, ctx context.Context, key string, want []string) {
	t.Helper()
	got, err := mgr.GetClient().SMembers(ctx, key).Result()
	if err != nil {
		t.Fatalf("set members %s: %v", key, err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("%s members=%v want=%v", key, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s members=%v want=%v", key, got, want)
		}
	}
}

func seedLease(t *testing.T, mgr *RedisManager, ctx context.Context, lease TaskLease) {
	t.Helper()
	raw, err := jsonMarshalTaskLease(lease)
	if err != nil {
		t.Fatalf("marshal seed lease: %v", err)
	}
	if err := mgr.GetClient().HSet(ctx, KeyTaskLeases, lease.TaskID, raw).Err(); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	if err := mgr.GetClient().SAdd(ctx, KeyRunningTasks, lease.TaskID).Err(); err != nil {
		t.Fatalf("seed active marker: %v", err)
	}
}

func jsonMarshalTaskLease(lease TaskLease) ([]byte, error) {
	type leaseJSON TaskLease
	return json.Marshal(leaseJSON(lease))
}
