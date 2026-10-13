package httpapi

// Control task routing-observability reads (#852): the workspace task
// read model exposes the requested service/resolution, the pinned
// routing snapshot + pool version, the selected vs final resolved
// model, and per-send retry-layer facts (gateway vs model_fallback vs
// river_task) with retryable failure reasons. Read-only projections:
// no task-state or billing writes happen here.
//
// PostgreSQL integration coverage: successful routing, model-fallback
// retry, final failure, and missing model data (no snapshot, no
// attempts). Skipped without API_TEST_DATABASE_DSN like the other
// postgres integration tests.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type controlTaskBillingReadError struct {
	dbsqlc.DBTX
	err error
}

func (db controlTaskBillingReadError) QueryRow(context.Context, string, ...any) pgx.Row {
	return db
}

func (db controlTaskBillingReadError) Scan(...any) error {
	return db.err
}

func TestControlTaskBillingReadErrors(t *testing.T) {
	queryErr := errors.New("billing query failed")
	for _, tt := range []struct {
		name string
		err  error
		want error
	}{
		{name: "missing", err: pgx.ErrNoRows},
		{name: "wrapped missing", err: fmt.Errorf("query: %w", pgx.ErrNoRows)},
		{name: "query failure", err: queryErr, want: queryErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			adapter := workspaceControlTaskAdapter{q: dbsqlc.New(controlTaskBillingReadError{err: tt.err})}
			snapshot, found, err := adapter.loadControlTaskBilling(context.Background(), "task_test")
			if snapshot != nil || found || !errors.Is(err, tt.want) {
				t.Fatalf("billing result = (%v, %v, %v), want (nil, false, %v)", snapshot, found, err, tt.want)
			}
		})
	}
}

func openControlTaskTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode skips postgres integration")
	}
	return testdb.Open(t)
}

func seedControlTaskWorkspace(t *testing.T, db *pgxpool.Pool) (workspacePublicID string, userID int64, workspaceInternalID, billingAccountID int64) {
	t.Helper()
	ctx := context.Background()
	workspacePublicID = "ws_obs_" + strings.ToLower(randomControlHex(t, 6))
	if err := db.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('obs-user', $1, 'user', 1) RETURNING id`,
		"O"+strings.ToUpper(randomControlHex(t, 5)),
	).Scan(&userID); err != nil {
		t.Fatalf("seed obs user failed: %v", err)
	}
	if err := db.QueryRow(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'obs-ws') RETURNING internal_id`,
		workspacePublicID, userID,
	).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("seed obs workspace failed: %v", err)
	}
	if err := db.QueryRow(ctx,
		`INSERT INTO billing_accounts (public_id, workspace_id) VALUES ($1, $2) RETURNING internal_id`,
		"ba_obs_"+strings.ToLower(randomControlHex(t, 6)), workspaceInternalID,
	).Scan(&billingAccountID); err != nil {
		t.Fatalf("seed obs billing account failed: %v", err)
	}
	return workspacePublicID, userID, workspaceInternalID, billingAccountID
}

func randomControlHex(t *testing.T, nchars int) string {
	t.Helper()
	const digits = "0123456789abcdef"
	buf := make([]byte, nchars)
	seed := time.Now().UnixNano() + int64(nchars)
	for i := range buf {
		seed = seed*6364136223846793005 + 1442695040888963407
		buf[i] = digits[byte(seed>>33)%16]
	}
	return string(buf)
}

func seedControlTask(t *testing.T, db *pgxpool.Pool, workspaceInternalID, userID, billingAccountID int64, taskID, status, snapshot string) {
	t.Helper()
	var snapshotArg any
	if snapshot == "" {
		snapshotArg = nil
	} else {
		snapshotArg = snapshot
	}
	// chk_tasks_terminal_consistency: terminal rows carry their
	// terminal timestamp; live rows carry none.
	var completedAt, cancelledAt any
	switch status {
	case "completed", "failed":
		completedAt = time.Now()
	case "cancelled":
		cancelledAt = time.Now()
	}
	if _, err := db.Exec(context.Background(), `
INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status, priority, retry_count, lease_token, error_message, routing_snapshot, completed_at, cancelled_at)
VALUES ($1,$2,$3,$4,'render','{"params":{}}',$5,2,0,'','', $6, $7, $8)`,
		taskID, workspaceInternalID, userID, billingAccountID, status, snapshotArg, completedAt, cancelledAt); err != nil {
		t.Fatalf("seed control task %s failed: %v", taskID, err)
	}
}

func seedControlAttempt(t *testing.T, db *pgxpool.Pool, taskID string, attemptNo int, layer, status, requestedModel, resolvedModel, failureStage, failureCategory string, gatewayRetryCount int, completed bool) {
	t.Helper()
	var completedAt any
	if completed {
		completedAt = time.Now()
	}
	token := "tok_" + taskID + "_" + strings.Repeat("0", attemptNo) + randomControlHex(t, 4)
	if _, err := db.Exec(context.Background(), `
INSERT INTO task_execution_attempts (task_id, river_job_id, attempt_no, execution_token, service, requested_model, resolved_model, provider, gateway_step, retry_layer, gateway_retry_count, status, failure_stage, failure_category, error_msg, completed_at)
VALUES ($1, 0, $2, $3, 'render', $4, $5, 'openrouter', '', $6, $7, $8, $9, $10, '', $11)`,
		taskID, attemptNo, token, requestedModel, resolvedModel, layer, gatewayRetryCount, status, failureStage, failureCategory, completedAt); err != nil {
		t.Fatalf("seed control attempt %s#%d failed: %v", taskID, attemptNo, err)
	}
}

func obsSnapshot(selected, retry string) string {
	candidate := ""
	if retry != "" {
		candidate = `, "retry_candidate": "` + retry + `"`
	}
	return `{"service":"render","requested_resolution":"2k",` +
		`"eligible_models":["openrouter/model-a","openrouter/model-b"],` +
		`"selected_model":"` + selected + `"` + candidate + `,` +
		`"selection_seed":"00000000000000000000000000000000","pool_version":3}`
}

func seedControlObservabilityFixture(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, userID, workspaceInternalID, billingAccountID := seedControlTaskWorkspace(t, db)
	seed := func(taskID, status, snapshot string) {
		seedControlTask(t, db, workspaceInternalID, userID, billingAccountID, taskID, status, snapshot)
	}
	// Successful routing: single gateway send, no fallback spent.
	seed("task_obs_ok", "completed", obsSnapshot("openrouter/model-a", "openrouter/model-b"))
	seedControlAttempt(t, db, "task_obs_ok", 1, "gateway", "completed", "openrouter/model-a", "openrouter/model-a", "", "", 0, true)
	// Model-fallback retry: retryable first failure, bounded fallback completes.
	seed("task_obs_retry", "completed", obsSnapshot("openrouter/model-a", "openrouter/model-b"))
	seedControlAttempt(t, db, "task_obs_retry", 1, "gateway", "failed", "openrouter/model-a", "openrouter/model-a", "provider", "provider_error", 2, true)
	seedControlAttempt(t, db, "task_obs_retry", 2, "model_fallback", "completed", "openrouter/model-b", "openrouter/model-b", "", "", 0, true)
	// Final failure: fallback budget absent (single candidate), river
	// redelivery also fails deterministically-by-policy; reasons stay
	// per-send and layers stay distinct.
	seed("task_obs_fail", "failed", obsSnapshot("openrouter/model-a", ""))
	seedControlAttempt(t, db, "task_obs_fail", 1, "gateway", "failed", "openrouter/model-a", "openrouter/model-a", "provider", "timeout", 1, true)
	seedControlAttempt(t, db, "task_obs_fail", 2, "river_task", "failed", "openrouter/model-a", "openrouter/model-a", "provider", "provider_error", 0, true)
	// Missing model data: pre-pool row, never pinned, never executed.
	seed("task_obs_bare", "pending", "")
}

func TestControlTaskDetailSuccessfulRouting(t *testing.T) {
	db := openControlTaskTestDB(t)
	seedControlObservabilityFixture(t, db)
	adapter := NewWorkspaceControlTasks(db, nil, nil, nil, nil)

	detail, err := adapter.GetTask(context.Background(), "task_obs_ok")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if detail.Service != "render" {
		t.Fatalf("service = %q, want render", detail.Service)
	}
	if detail.Routing == nil {
		t.Fatal("routing must project the pinned snapshot")
	}
	routing := detail.Routing
	if routing.RequestedResolution != "2k" || routing.PoolVersion != 3 {
		t.Fatalf("resolution/pool_version = %q/%d, want 2k/3", routing.RequestedResolution, routing.PoolVersion)
	}
	if routing.SelectedModel != "openrouter/model-a" || routing.RetryCandidate != "openrouter/model-b" {
		t.Fatalf("selected/retry = %q/%q, want model-a/model-b", routing.SelectedModel, routing.RetryCandidate)
	}
	if len(routing.EligibleModels) != 2 {
		t.Fatalf("eligible = %+v, want 2 models", routing.EligibleModels)
	}
	if detail.FinalResolvedModel != "openrouter/model-a" {
		t.Fatalf("final = %q, want openrouter/model-a", detail.FinalResolvedModel)
	}
	if detail.LatestExecution == nil || detail.LatestExecution.ResolvedModel != "openrouter/model-a" {
		t.Fatalf("latest execution = %+v, want model-a", detail.LatestExecution)
	}
	if len(detail.ExecutionAttempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(detail.ExecutionAttempts))
	}
	first := detail.ExecutionAttempts[0]
	if first.RetryLayer != "gateway" || first.GatewayRetryCount != 0 {
		t.Fatalf("layer/gateway_retries = %q/%d, want gateway/0", first.RetryLayer, first.GatewayRetryCount)
	}
	if first.RequestedModel != "openrouter/model-a" || first.Service != "render" {
		t.Fatalf("requested/service = %q/%q, want model-a/render", first.RequestedModel, first.Service)
	}
}

func TestControlTaskDetailModelFallbackRetry(t *testing.T) {
	db := openControlTaskTestDB(t)
	seedControlObservabilityFixture(t, db)
	adapter := NewWorkspaceControlTasks(db, nil, nil, nil, nil)

	detail, err := adapter.GetTask(context.Background(), "task_obs_retry")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	// Selected (first send) vs final (fallback send) stay distinguishable.
	if detail.Routing == nil || detail.Routing.SelectedModel != "openrouter/model-a" {
		t.Fatalf("selected = %+v, want model-a", detail.Routing)
	}
	if detail.FinalResolvedModel != "openrouter/model-b" {
		t.Fatalf("final = %q, want fallback openrouter/model-b", detail.FinalResolvedModel)
	}
	if len(detail.ExecutionAttempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(detail.ExecutionAttempts))
	}
	first, second := detail.ExecutionAttempts[0], detail.ExecutionAttempts[1]
	if first.RetryLayer != "gateway" || second.RetryLayer != "model_fallback" {
		t.Fatalf("layers = %q/%q, want gateway/model_fallback", first.RetryLayer, second.RetryLayer)
	}
	if first.FailureCategory != "provider_error" || first.GatewayRetryCount != 2 {
		t.Fatalf("first failure/retries = %q/%d, want provider_error/2", first.FailureCategory, first.GatewayRetryCount)
	}
	if second.ResolvedModel != "openrouter/model-b" || second.Status != "completed" {
		t.Fatalf("fallback attempt = %+v, want completed model-b", second)
	}
}

func TestControlTaskDetailFinalFailure(t *testing.T) {
	db := openControlTaskTestDB(t)
	seedControlObservabilityFixture(t, db)
	adapter := NewWorkspaceControlTasks(db, nil, nil, nil, nil)

	detail, err := adapter.GetTask(context.Background(), "task_obs_fail")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if detail.FinalResolvedModel != "openrouter/model-a" {
		t.Fatalf("final = %q, want openrouter/model-a", detail.FinalResolvedModel)
	}
	if len(detail.ExecutionAttempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(detail.ExecutionAttempts))
	}
	first, second := detail.ExecutionAttempts[0], detail.ExecutionAttempts[1]
	// Retryable reasons are retained per send, separate from the
	// river_task layer fact: timeout on the gateway send, provider
	// error on the redelivery.
	if first.RetryLayer != "gateway" || first.FailureCategory != "timeout" {
		t.Fatalf("first = %q/%q, want gateway/timeout", first.RetryLayer, first.FailureCategory)
	}
	if second.RetryLayer != "river_task" || second.FailureCategory != "provider_error" {
		t.Fatalf("second = %q/%q, want river_task/provider_error", second.RetryLayer, second.FailureCategory)
	}
}

func TestControlTaskDetailMissingModelData(t *testing.T) {
	db := openControlTaskTestDB(t)
	seedControlObservabilityFixture(t, db)
	adapter := NewWorkspaceControlTasks(db, nil, nil, nil, nil)

	detail, err := adapter.GetTask(context.Background(), "task_obs_bare")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if detail.Routing != nil {
		t.Fatalf("routing = %+v, want nil for unpinned task", detail.Routing)
	}
	if detail.FinalResolvedModel != "" {
		t.Fatalf("final = %q, want empty", detail.FinalResolvedModel)
	}
	if detail.ExecutionAttempts == nil || len(detail.ExecutionAttempts) != 0 {
		t.Fatalf("attempts = %+v, want empty non-nil", detail.ExecutionAttempts)
	}
	if detail.LatestExecution != nil {
		t.Fatalf("latest execution = %+v, want nil", detail.LatestExecution)
	}
}

func TestControlTaskListExposesRoutingAndFinalModel(t *testing.T) {
	db := openControlTaskTestDB(t)
	seedControlObservabilityFixture(t, db)
	adapter := NewWorkspaceControlTasks(db, nil, nil, nil, nil)

	items, counts, err := adapter.ListTasks(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if counts.Total != 4 {
		t.Fatalf("total = %d, want 4", counts.Total)
	}
	byID := map[string]int{}
	for i, item := range items {
		byID[item.TaskID] = i
	}
	ok := items[byID["task_obs_ok"]]
	if ok.RequestedResolution != "2k" || ok.PoolVersion != 3 || ok.SelectedModel != "openrouter/model-a" {
		t.Fatalf("ok routing = %+v, want 2k/3/model-a", ok)
	}
	if ok.FinalResolvedModel != "openrouter/model-a" {
		t.Fatalf("ok final = %q, want model-a", ok.FinalResolvedModel)
	}
	retry := items[byID["task_obs_retry"]]
	if retry.FinalResolvedModel != "openrouter/model-b" || retry.SelectedModel != "openrouter/model-a" {
		t.Fatalf("retry selected/final = %q/%q, want model-a/model-b", retry.SelectedModel, retry.FinalResolvedModel)
	}
	fail := items[byID["task_obs_fail"]]
	if fail.FinalResolvedModel != "openrouter/model-a" || fail.RequestedResolution != "2k" {
		t.Fatalf("fail final/resolution = %q/%q, want model-a/2k", fail.FinalResolvedModel, fail.RequestedResolution)
	}
	bare := items[byID["task_obs_bare"]]
	if bare.RequestedResolution != "" || bare.PoolVersion != 0 || bare.FinalResolvedModel != "" {
		t.Fatalf("bare routing = %+v, want empty", bare)
	}
}
