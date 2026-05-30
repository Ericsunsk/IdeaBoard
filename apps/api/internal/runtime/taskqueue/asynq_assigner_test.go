package taskqueue

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"api/internal/platform/redis"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestRedisAsynqAssigner(t *testing.T) (*RedisAsynqAssigner, *redis.RedisManager) {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(s.Close)

	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mgr := redis.NewRedisManager(client)
	return NewRedisAsynqAssigner(mgr), mgr
}

func TestRedisAsynqAssignerRebuildAndInspectLeases(t *testing.T) {
	assigner, mgr := newTestRedisAsynqAssigner(t)
	ctx := context.Background()

	for _, accountID := range []uint64{9101, 9102, 9103} {
		if err := mgr.EnsureAccountConcurrent(ctx, accountID, 3); err != nil {
			t.Fatalf("ensure account %d: %v", accountID, err)
		}
		if err := mgr.GetClient().HSet(ctx, fmt.Sprintf("%s:%d", redis.KeyAccountConcurrent, accountID), "count", 9).Err(); err != nil {
			t.Fatalf("seed count for %d: %v", accountID, err)
		}
	}
	if err := mgr.GetClient().HSet(ctx, redis.KeyTaskLeases, "stale-lease", 9103).Err(); err != nil {
		t.Fatalf("seed stale lease: %v", err)
	}
	if err := mgr.GetClient().SAdd(ctx, redis.KeyRunningTasks, "stale-lease").Err(); err != nil {
		t.Fatalf("seed stale running marker: %v", err)
	}

	leases := []TaskLease{
		{TaskID: "task-1", AccountID: 9101},
		{TaskID: "task-2", AccountID: 9101},
		{TaskID: "task-3", AccountID: 9102},
	}
	if err := assigner.RebuildRunningTaskLeases(ctx, leases, []uint64{9101, 9102, 9103}); err != nil {
		t.Fatalf("rebuild leases: %v", err)
	}

	gotLeases, err := assigner.ListTaskLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	sort.Slice(gotLeases, func(i, j int) bool { return gotLeases[i].TaskID < gotLeases[j].TaskID })
	if len(gotLeases) != len(leases) {
		t.Fatalf("leases len=%d want=%d", len(gotLeases), len(leases))
	}
	for i := range leases {
		if gotLeases[i] != leases[i] {
			t.Fatalf("lease[%d]=%+v want=%+v", i, gotLeases[i], leases[i])
		}
	}

	for accountID, want := range map[uint64]int{9101: 2, 9102: 1, 9103: 0} {
		got, err := mgr.GetAccountConcurrent(ctx, accountID)
		if err != nil || got != want {
			t.Fatalf("account %d count=%d want=%d err=%v", accountID, got, want, err)
		}
	}

	running, err := mgr.GetClient().SMembers(ctx, redis.KeyRunningTasks).Result()
	if err != nil {
		t.Fatalf("list running markers: %v", err)
	}
	sort.Strings(running)
	wantRunning := []string{"task-1", "task-2", "task-3"}
	if len(running) != len(wantRunning) || running[0] != wantRunning[0] || running[1] != wantRunning[1] || running[2] != wantRunning[2] {
		t.Fatalf("running=%v want=%v", running, wantRunning)
	}

	accountID, ok, err := assigner.GetTaskLease(ctx, "task-1")
	if err != nil || !ok || accountID != 9101 {
		t.Fatalf("get lease account=%d ok=%v err=%v", accountID, ok, err)
	}
	active, err := assigner.HasActiveLease(ctx, "task-1", 9101)
	if err != nil || !active {
		t.Fatalf("has active lease=%v err=%v", active, err)
	}
}

func TestRedisAsynqAssignerListTaskLeasesRejectsMalformedAccountID(t *testing.T) {
	assigner, mgr := newTestRedisAsynqAssigner(t)
	ctx := context.Background()
	if err := mgr.GetClient().HSet(ctx, redis.KeyTaskLeases, "bad-lease", "bad-owner").Err(); err != nil {
		t.Fatalf("seed malformed lease: %v", err)
	}

	if _, err := assigner.ListTaskLeases(ctx); err == nil {
		t.Fatal("expected malformed lease account id error")
	}
}
