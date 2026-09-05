package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	accountrepo "ideaboard/api/internal/accountpool/accounts/infrastructure/postgres"
	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/jackc/pgx/v5"
	riverqueue "github.com/riverqueue/river"
)

type fakeCancelStoreForRiver struct {
	cancelled map[string]bool
	cleared   map[string]bool
}

func newFakeCancelStoreForRiver() *fakeCancelStoreForRiver {
	return &fakeCancelStoreForRiver{
		cancelled: map[string]bool{},
		cleared:   map[string]bool{},
	}
}

func (f *fakeCancelStoreForRiver) MarkCancelled(_ context.Context, taskID string, _ time.Duration) error {
	f.cancelled[taskID] = true
	return nil
}

func (f *fakeCancelStoreForRiver) IsCancelled(_ context.Context, taskID string) (bool, error) {
	return f.cancelled[taskID], nil
}

func (f *fakeCancelStoreForRiver) ClearCancelled(_ context.Context, taskID string) error {
	f.cleared[taskID] = true
	delete(f.cancelled, taskID)
	return nil
}

func createRiverTestAccount(t *testing.T, db testRuntimeDB, repo *accountrepo.Repository, name string) *accounts.ApiAccount {
	t.Helper()
	secretstest.EnableEncryption(t)
	channelID := createRiverTestChannel(t, db, name)
	acc := &accounts.ApiAccount{
		Name:               name,
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "k",
		ProxyURL:           "https://example.com/" + name,
		MaxConcurrent:      3,
		Timeout:            120,
		Weight:             1,
		Status:             1,
	}
	if err := repo.Create(context.Background(), acc); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return acc
}

type testDB interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

type testRuntimeDB interface {
	testDB
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func createRiverTestChannel(t *testing.T, db testRuntimeDB, suffix string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES ('openai-compatible', 'model-api', $1, $2, 1)
RETURNING id`,
		"river-test-channel-"+suffix,
		"https://example.com/"+suffix,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create river test channel failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability) VALUES ($1, 'generation')`, id); err != nil {
		t.Fatalf("create river test channel capability failed: %v", err)
	}
	return id
}

func assertRiverTaskBillingState(t testing.TB, db *sql.DB, taskID, wantTaskStatus, wantSettlementState string, wantAvailable, wantReserved int64) {
	t.Helper()
	var taskStatus, settlementState string
	var available, reserved int64
	if err := db.QueryRowContext(t.Context(), `
SELECT tasks.status, settlements.state, available.balance, reserved.balance
FROM tasks
JOIN task_billing_settlements settlements USING (task_id)
JOIN credit_accounts available ON available.wallet_user_id = tasks.user_id AND available.account_role = 'user_available'
JOIN credit_accounts reserved ON reserved.wallet_user_id = tasks.user_id AND reserved.account_role = 'user_reserved'
WHERE tasks.task_id = $1`, taskID).Scan(&taskStatus, &settlementState, &available, &reserved); err != nil {
		t.Fatal(err)
	}
	if taskStatus != wantTaskStatus || settlementState != wantSettlementState || available != wantAvailable || reserved != wantReserved {
		t.Fatalf("task/settlement/wallet=%s/%s/%d/%d, want %s/%s/%d/%d",
			taskStatus, settlementState, available, reserved,
			wantTaskStatus, wantSettlementState, wantAvailable, wantReserved,
		)
	}
}

func stopRiverClientForSubmitToWorkerTest(t testing.TB, client *riverqueue.Client[pgx.Tx]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Stop(ctx); err != nil {
		t.Errorf("stop River client failed: %v", err)
	}
}

func waitForSubmitToRiverWorkerTaskStatus(t testing.TB, repo *taskstore.Repository, taskID string, want string, timeout time.Duration) *tasks.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *tasks.Task
	var lastErr error
	for time.Now().Before(deadline) {
		task, err := repo.GetByTaskID(t.Context(), taskID)
		if err != nil {
			lastErr = err
		} else {
			last = task
			if task.Status == want {
				return task
			}
			if tasks.IsTerminalStatus(task.Status) {
				t.Fatalf("task %q reached terminal status=%q, want %q; error_msg=%q", taskID, task.Status, want, task.ErrorMsg)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("wait for task %q status=%q timed out after %s; last error=%v", taskID, want, timeout, lastErr)
	}
	if last == nil {
		t.Fatalf("wait for task %q status=%q timed out after %s; task was never loaded", taskID, want, timeout)
		return nil
	}
	t.Fatalf("wait for task %q status=%q timed out after %s; last status=%q error_msg=%q", taskID, want, timeout, last.Status, last.ErrorMsg)
	return nil
}

func marshalWorkflowTaskParamsForTest(t testing.TB, params *tasks.WorkflowTaskParams) string {
	t.Helper()
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return string(data)
}
