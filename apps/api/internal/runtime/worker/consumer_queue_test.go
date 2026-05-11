package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"api/internal/platform/redis"
	"api/internal/runtime/taskqueue"
)

type fakeQueue struct {
	global      []*redis.GlobalQueueTask
	wait        []*redis.GlobalQueueTask
	frontPushed []*redis.GlobalQueueTask
}

func (f *fakeQueue) Enqueue(context.Context, *redis.GlobalQueueTask) error { return nil }
func (f *fakeQueue) EnqueueFront(_ context.Context, task *redis.GlobalQueueTask) error {
	f.frontPushed = append(f.frontPushed, task)
	return nil
}
func (f *fakeQueue) Dequeue(context.Context) (*redis.GlobalQueueTask, error) {
	if len(f.global) == 0 {
		return nil, nil
	}
	task := f.global[0]
	f.global = f.global[1:]
	return task, nil
}
func (f *fakeQueue) DequeueWait(context.Context) (*redis.GlobalQueueTask, error) {
	if len(f.wait) == 0 {
		return nil, nil
	}
	task := f.wait[0]
	f.wait = f.wait[1:]
	return task, nil
}
func (f *fakeQueue) QueueLengths(context.Context) (int64, int64, error) { return 0, 0, nil }

type fakeAssigner struct {
	assignAccount uint64
	assignStatus  string
	assignErr     error
	assignCalls   int
}

func (f *fakeAssigner) Assign(_ context.Context, _ []uint64, _ *redis.GlobalQueueTask) (uint64, string, error) {
	f.assignCalls++
	return f.assignAccount, f.assignStatus, f.assignErr
}
func (f *fakeAssigner) Release(context.Context, uint64, string) error { return nil }
func (f *fakeAssigner) EnsureAccountConcurrent(context.Context, uint64, int) error {
	return nil
}
func (f *fakeAssigner) SetAccountMaxConcurrent(context.Context, uint64, int) error { return nil }
func (f *fakeAssigner) GetAccountLoadInfo(context.Context, []uint64) (map[uint64]*redis.AccountLoad, error) {
	return map[uint64]*redis.AccountLoad{}, nil
}

type fakeRuntime struct {
	refreshFn       func(task *redis.GlobalQueueTask) error
	releaseCalls    int
	failedCalls     int
	lastFailedTask  string
	lastFailedCause string
}

func (f *fakeRuntime) RefreshTaskAccounts(task *redis.GlobalQueueTask) error {
	if f.refreshFn != nil {
		return f.refreshFn(task)
	}
	return nil
}
func (f *fakeRuntime) ReleaseTask(uint64, string) error {
	f.releaseCalls++
	return nil
}
func (f *fakeRuntime) MarkTaskFailed(taskID string, reason string) error {
	f.failedCalls++
	f.lastFailedTask = taskID
	f.lastFailedCause = reason
	return nil
}

func newQueueTask(id string) *redis.GlobalQueueTask {
	return &redis.GlobalQueueTask{TaskID: id, AccountIDs: []uint64{11}}
}

func TestProcessQueueGlobalEmptySkipsAssign(t *testing.T) {
	q := &fakeQueue{}
	assigner := &fakeAssigner{}
	rt := &fakeRuntime{}
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(*redis.GlobalQueueTask, uint64) {}}
	c.processQueue()
	if assigner.assignCalls != 0 {
		t.Fatalf("assign should not be called, got %d", assigner.assignCalls)
	}
}

func TestProcessQueuePromotesWaitQueueFirst(t *testing.T) {
	q := &fakeQueue{wait: []*redis.GlobalQueueTask{newQueueTask("w1")}}
	assigner := &fakeAssigner{}
	rt := &fakeRuntime{}
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(*redis.GlobalQueueTask, uint64) {}}
	c.processQueue()
	if len(q.frontPushed) != 1 || q.frontPushed[0].TaskID != "w1" {
		t.Fatalf("wait queue task not promoted, pushed=%v", q.frontPushed)
	}
}

func TestProcessQueueNoAvailableAccountMarksFailed(t *testing.T) {
	task := newQueueTask("t-no-account")
	q := &fakeQueue{global: []*redis.GlobalQueueTask{task}}
	assigner := &fakeAssigner{}
	rt := &fakeRuntime{refreshFn: func(task *redis.GlobalQueueTask) error {
		task.AccountIDs = nil
		return nil
	}}
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(*redis.GlobalQueueTask, uint64) {}}
	c.processQueue()
	if rt.failedCalls != 1 {
		t.Fatalf("expected task failed once, got %d", rt.failedCalls)
	}
}

func TestProcessQueueAssignWaitQueueDoesNotExecute(t *testing.T) {
	task := newQueueTask("t-wait")
	q := &fakeQueue{global: []*redis.GlobalQueueTask{task}}
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusWaitQueue}
	rt := &fakeRuntime{}
	executed := 0
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(*redis.GlobalQueueTask, uint64) { executed++ }}
	c.processQueue()
	if executed != 0 {
		t.Fatalf("execute should not run for wait_queue")
	}
}

func TestProcessQueueAssignQueueFullRequeuesFront(t *testing.T) {
	task := newQueueTask("t-full")
	q := &fakeQueue{global: []*redis.GlobalQueueTask{task}}
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusQueueFull}
	rt := &fakeRuntime{}
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(*redis.GlobalQueueTask, uint64) {}}
	c.processQueue()
	if len(q.frontPushed) == 0 || q.frontPushed[len(q.frontPushed)-1].TaskID != "t-full" {
		t.Fatalf("task should be pushed to front on queue_full")
	}
}

func TestProcessQueueAssignSuccessStartsOrchestrator(t *testing.T) {
	task := newQueueTask("t-ok")
	q := &fakeQueue{global: []*redis.GlobalQueueTask{task}}
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 42}
	rt := &fakeRuntime{}
	done := make(chan bool, 1)
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(task *redis.GlobalQueueTask, accountID uint64) {
		done <- task.TaskID == "t-ok" && accountID == 42
	}}
	c.processQueue()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("unexpected execute args")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected execute to start")
	}
}

func TestProcessQueueAssignErrorMarksFailed(t *testing.T) {
	task := newQueueTask("t-err")
	q := &fakeQueue{global: []*redis.GlobalQueueTask{task}}
	assigner := &fakeAssigner{assignErr: errors.New("assign failed")}
	rt := &fakeRuntime{}
	c := &Consumer{queue: q, assigner: assigner, taskRuntime: rt, executeTask: func(*redis.GlobalQueueTask, uint64) {}}
	c.processQueue()
	if rt.failedCalls != 1 {
		t.Fatalf("expected fail mark once on assign error, got %d", rt.failedCalls)
	}
}
