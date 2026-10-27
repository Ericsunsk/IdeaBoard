package workspacetasks

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Transaction capabilities are compile-time requirements. Missing dependencies
// must still fail closed before the first state or ledger write.
func TestTerminalSeamsMissingIsRetryableUnavailable(t *testing.T) {
	store := newTxTestStore()
	svc := NewService(ServiceDeps{TaskRead: store, TaskLifecycle: store})
	ctx := context.Background()
	if _, err := store.CreateTask(ctx, &WorkspaceTask{
		ID: "task_1", WorkspaceID: "ws_1", OwnerUserID: 7,
		Service: "render", Status: StatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"Complete": func() error { _, err := svc.Complete(ctx, "ws_1", "task_1", nil); return err },
		"Fail":     func() error { _, err := svc.Fail(ctx, "ws_1", "task_1", "boom"); return err },
		"Retry":    func() error { _, err := svc.Retry(ctx, "ws_1", "task_1", true); return err },
		"Cancel":   func() error { _, _, err := svc.Cancel(ctx, "ws_1", "task_1", 7, "stop"); return err },
	} {
		err := call()
		if !errors.Is(err, ErrSettlementUnavailable) || !strings.Contains(err.Error(), "caller must retry") {
			t.Fatalf("%s err = %v, want explicit retryable unavailable", name, err)
		}
	}
	current, err := store.GetTask(ctx, "ws_1", "task_1")
	if err != nil || current.Status != StatusRunning {
		t.Fatalf("missing dependencies changed task: %+v, err=%v", current, err)
	}
}

func TestCreateSeamsMissingIsRetryableUnavailable(t *testing.T) {
	for _, missing := range []string{"claims", "accounts"} {
		t.Run(missing, func(t *testing.T) {
			store := newTxTestStore()
			deps := ServiceDeps{
				TaskRead: store, TaskLifecycle: store,
				Quoter: &fakeQuoter{}, Holder: newFakeHolder(), Queue: &txTestEnqueuer{fakeEnqueuer: &fakeEnqueuer{}},
				SubmissionClaims: store, BillingAccounts: store,
			}
			if missing == "claims" {
				deps.SubmissionClaims = nil
			} else {
				deps.BillingAccounts = nil
			}
			svc := NewService(deps)
			_, err := svc.Create(context.Background(), CreateInput{
				WorkspaceID: "ws_1", OwnerUserID: 7, Service: "render",
			}, "seam-matrix-create-1")
			if !errors.Is(err, ErrSettlementUnavailable) || !strings.Contains(err.Error(), "caller must retry") {
				t.Fatalf("Create err = %v, want retryable unavailable", err)
			}
			if len(store.claims) != 0 {
				t.Fatal("missing dependency must not claim a submission")
			}
		})
	}
}
