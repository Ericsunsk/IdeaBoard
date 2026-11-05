package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/events"
	eventoutbox "ideaboard/api/internal/events/outbox"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tasking/settlement"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	"ideaboard/api/internal/tenancy/workspacefacts"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultTaskPriority matches the tasks table default. The workspace task
// surface carries no priority input; every insert uses the default.
const defaultTaskPriority = int32(2)

// Store implements the split workspace-task persistence seams.
type Store struct {
	db     *pgxpool.Pool
	q      *dbsqlc.Queries
	outbox *eventoutbox.TaskStatusOutboxStore
}

// Compile-time convergence with each minimal persistence seam.
var _ taskports.TaskReadStore = (*Store)(nil)
var _ taskports.TaskLifecycleStore = (*Store)(nil)
var _ taskports.TaskClaimStore = (*Store)(nil)
var _ taskports.TaskOutputStore = (*Store)(nil)
var _ taskports.TaskBillingStore = (*Store)(nil)

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// NewStore builds a postgres workspace task Store over pool.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, q: dbsqlc.New(db), outbox: eventoutbox.NewTaskStatusOutboxStore(db)}
}

func (s *Store) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

// BeginSettlementScope opens the caller-owned settlement transaction for
// the workspacetasks single-transaction boundary (task flip + Credits
// Capture/Release + River enqueue commit atomically). It satisfies
// tasks.SettlementTransactor; the Service owns commit/rollback
// via the returned Scope.
// BeginOwned (never borrows): the settlement owner borrows the guard
// carrier one layer up (settlement.BorrowOrBeginScope) and calls this only for its
// owned leg, so this leg must not join a carrier on its own.
func (s *Store) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("tasking postgres: %w", database.ErrPoolNotConfigured)
	}
	scope, err := dbutil.BeginOwned(ctx, s.db, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("tasking postgres: begin settlement scope: %w", err)
	}
	return scope, nil
}

// Compile-time convergence with the settlement tx seam.
var _ taskports.TaskSettlementStore = (*Store)(nil)

// Compile-time convergence with the durable submit idempotency claim seam.
var _ taskports.TxSubmissionClaimStore = (*Store)(nil)

// Compile-time convergence with the stable funds-identity seam.
var _ taskports.TxBillingAccountResolver = (*Store)(nil)

// workflowInputEnvelope is the tasks.workflow_input JSONB shape. Billing
// snapshots are quote-time facts (the credits ledger owns settlement
// truth); execution_token is the durable retry-fencing token; cancel
// reason rides here because tasks carries no cancel text column. The Board
// binding persists with the accepted inputs in the same task insert.
type workflowInputEnvelope struct {
	MediaAssetIDs   []string               `json:"media_asset_ids,omitempty"`
	Params          map[string]any         `json:"params,omitempty"`
	CanonicalParams string                 `json:"canonical_params,omitempty"`
	BillingSnapshot *tasks.BillingSnapshot `json:"billing_snapshot,omitempty"`
	ExecutionToken  string                 `json:"execution_token,omitempty"`
	CancelReason    string                 `json:"cancel_reason,omitempty"`
	BoardID         string                 `json:"board_id,omitempty"`
	// BoardAccessVersion snapshots the Board access_version at submit.
	BoardAccessVersion int64 `json:"board_access_version,omitempty"`
}

// outputMetadata is the task_outputs.metadata shape.
type outputMetadata struct {
	AssetID        string `json:"asset_id,omitempty"`
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
	Role           string `json:"role,omitempty"`
}

func notFound(err error) error {
	if database.IsNotFound(err) {
		return tasks.ErrNotFound
	}
	return err
}

func validTaskRef(workspaceID, taskID string) bool {
	return contract.IsValidPublicID(contract.PrefixWorkspace, strings.TrimSpace(workspaceID)) &&
		contract.IsTaskID(strings.TrimSpace(taskID))
}

func marshalEnvelope(task *tasks.WorkspaceTask) ([]byte, error) {
	envelope := workflowInputEnvelope{
		MediaAssetIDs:      append([]string(nil), task.MediaAssetIDs...),
		Params:             task.Params,
		CanonicalParams:    task.CanonicalParams,
		ExecutionToken:     task.ExecutionToken,
		CancelReason:       task.CancelReason,
		BoardID:            task.BoardID,
		BoardAccessVersion: task.BoardAccessVersion,
	}
	if task.BillingSnapshot.SKU != "" || task.BillingSnapshot.TotalCredits != 0 || task.BillingSnapshot.HoldTransactionID != "" {
		snapshot := task.BillingSnapshot
		envelope.BillingSnapshot = &snapshot
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("tasking postgres: encode workflow input: %w", err)
	}
	return raw, nil
}

func parseEnvelope(raw []byte) (workflowInputEnvelope, error) {
	var envelope workflowInputEnvelope
	if len(raw) == 0 {
		return envelope, nil
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return envelope, fmt.Errorf("tasking postgres: decode workflow input: %w", err)
	}
	return envelope, nil
}

func mapOutputRow(row dbsqlc.TaskOutput) (tasks.OutputRef, error) {
	var meta outputMetadata
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return tasks.OutputRef{}, fmt.Errorf("tasking postgres: decode output metadata: %w", err)
		}
	}
	switch strings.TrimSpace(row.OutputKind) {
	case "media_asset":
		return tasks.OutputRef{
			Kind:        tasks.OutputKindAsset,
			AssetID:     meta.AssetID,
			Role:        meta.Role,
			ResultIndex: int(row.OutputIndex),
		}, nil
	case "remote_url":
		return tasks.OutputRef{
			Kind:           tasks.OutputKindRemote,
			URL:            row.RemoteUrl.String,
			ChecksumSHA256: meta.ChecksumSHA256,
			Role:           meta.Role,
			ResultIndex:    int(row.OutputIndex),
		}, nil
	default:
		return tasks.OutputRef{}, fmt.Errorf("tasking postgres: unknown output kind %q", row.OutputKind)
	}
}

func mapTaskRow(workspacePublicID string, row dbsqlc.Task, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	envelope, err := parseEnvelope(row.WorkflowInput)
	if err != nil {
		return nil, err
	}
	completed := dbutil.TimePtr(row.CompletedAt)
	if row.Status == string(tasks.StatusCancelled) && completed == nil {
		completed = dbutil.TimePtr(row.CancelledAt)
	}
	task := &tasks.WorkspaceTask{
		ID:               row.PublicID,
		WorkspaceID:      strings.TrimSpace(workspacePublicID),
		OwnerUserID:      row.SubmitterUserID,
		BillingAccountID: row.BillingAccountID,
		Service:          row.Service,
		MediaAssetIDs:    append([]string(nil), envelope.MediaAssetIDs...),
		Params:           envelope.Params,
		CanonicalParams:  envelope.CanonicalParams,
		// The pinned submit-time snapshot rides the row untouched: the
		// worker parses and Verify()s it, every other reader ignores it.
		RoutingSnapshot:    append([]byte(nil), row.RoutingSnapshot...),
		BoardID:            strings.TrimSpace(envelope.BoardID),
		BoardAccessVersion: envelope.BoardAccessVersion,
		Status:             tasks.Status(row.Status),
		RetryCount:         int(row.RetryCount),
		ExecutionToken:     envelope.ExecutionToken,
		OutputRefs:         append([]tasks.OutputRef(nil), outputs...),
		ErrorMsg:           row.ErrorMessage,
		CancelReason:       envelope.CancelReason,
		CreatedAt:          dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:          dbutil.TimeValue(row.UpdatedAt),
		CompletedAt:        completed,
	}
	if envelope.BillingSnapshot != nil {
		task.BillingSnapshot = *envelope.BillingSnapshot
	}
	return task, nil
}

// resolveWorkspace maps a ws_ public id to its internal id. Unknown or
// malformed ids read as ErrNotFound (invisible, never 403).
func resolveWorkspace(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspacePublicID) {
		return 0, tasks.ErrNotFound
	}
	internalID, err := workspacefacts.ResolveInternalID(ctx, q, workspacePublicID)
	if err != nil {
		return 0, notFound(err)
	}
	return internalID, nil
}

// resolveBillingAccount returns the workspace billing account (1:1),
// provisioning it on demand through the funds kernel. Task
// inserts must never fail on a missing billing account; full billing
// lifecycle stays in #804.
func resolveBillingAccount(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64) (int64, error) {
	row, err := funds.ResolveAccountRowByInternalIDWithQueries(ctx, q, workspaceInternalID)
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

// ResolveBillingAccountInTx resolves the settlement payer inside the
// caller's settlement transaction (#915 resolution, #916 funds identity,
// payer = actor per #973/#974). It satisfies
// tasks.TxBillingAccountResolver so submit-time Holds spend the actor's
// account, never the workspace owner's. The fixed payer order lives in
// tenancy/funds (actor-aware order implementation); this stays a thin
// adapter that applies the closed-organization gate and maps funds errors
// onto the tasking not-found sentinel.
func (s *Store) ResolveBillingAccountInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string, actorUserID int64) (int64, string, error) {
	if tx == nil {
		return 0, "", fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	q := s.q.WithTx(tx)
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspacePublicID) {
		return 0, "", tasks.ErrNotFound
	}
	workspace, err := q.GetWorkspaceByPublicID(ctx, workspacePublicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", tasks.ErrNotFound
		}
		return 0, "", err
	}
	if workspace.OrganizationID.Valid {
		closed, err := workspacefacts.IsOrganizationClosed(ctx, q, workspacePublicID)
		if err != nil {
			return 0, "", err
		}
		if closed {
			return 0, "", tasks.ErrNotFound
		}
	}
	identity, err := funds.ResolveByPublicIDForActorWithQueries(ctx, dbsqlc.New(tx), workspacePublicID, actorUserID)
	if err != nil {
		if errors.Is(err, tenancy.ErrBillingWorkspaceInvalid) || errors.Is(err, tenancy.ErrBillingNotFound) {
			return 0, "", tasks.ErrNotFound
		}
		return 0, "", err
	}
	if identity.BillingAccountID <= 0 {
		return 0, "", tasks.ErrNotFound
	}
	return identity.BillingAccountID, identity.OwnerType, nil
}

// loadOutputs reads ordered output refs for one task row.
func loadOutputs(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, taskPublicID string) ([]tasks.OutputRef, error) {
	rows, err := q.ListWorkspaceTaskOutputs(ctx, dbsqlc.ListWorkspaceTaskOutputsParams{
		PublicID:          strings.TrimSpace(taskPublicID),
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if err != nil {
		return nil, err
	}
	outputs := make([]tasks.OutputRef, 0, len(rows))
	for _, row := range rows {
		ref, err := mapOutputRow(row)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, ref)
	}
	return outputs, nil
}

func (s *Store) mapRowWithOutputsWithQueries(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string, row dbsqlc.Task) (*tasks.WorkspaceTask, error) {
	outputs, err := loadOutputs(ctx, q, workspacePublicID, row.PublicID)
	if err != nil {
		return nil, err
	}
	return mapTaskRow(workspacePublicID, row, outputs)
}

func (s *Store) getTaskWithQueries(ctx context.Context, q *dbsqlc.Queries, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	return getTaskWithQueries(ctx, q, workspaceID, taskID)
}

func getTaskWithQueries(ctx context.Context, q *dbsqlc.Queries, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	row, err := q.GetWorkspaceTaskByPublicID(ctx, dbsqlc.GetWorkspaceTaskByPublicIDParams{
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		return nil, notFound(err)
	}
	outputs, err := loadOutputs(ctx, q, workspaceID, row.PublicID)
	if err != nil {
		return nil, err
	}
	return mapTaskRow(workspaceID, row, outputs)
}

// CreateTask inserts a pending task. The input carries Service-validated
// facts; the repository re-validates the discriminated union (canonical
// workflow kind, asset shapes, params) so direct callers fail closed too.
func (s *Store) CreateTask(ctx context.Context, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error) {
	var created *tasks.WorkspaceTask
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		var err error
		created, err = createTaskWithQueries(ctx, q, task)
		if err != nil {
			return err
		}
		return s.insertTaskOutboxTx(ctx, tx, created, tasks.StatusPending, events.EventV1TaskCreated,
			events.V1TaskCreatedData{TaskID: created.ID, Service: created.Service})
	})
	return created, err
}

// CreateTaskInTx inserts a pending task inside the caller's settlement
// transaction (single-transaction Quote -> Hold -> Insert -> Enqueue).
func (s *Store) CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error) {
	if tx == nil {
		return nil, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	created, err := createTaskWithQueries(ctx, s.q.WithTx(tx), task)
	if err != nil {
		return nil, err
	}
	if err := s.insertTaskOutboxTx(ctx, tx, created, tasks.StatusPending, events.EventV1TaskCreated,
		events.V1TaskCreatedData{TaskID: created.ID, Service: created.Service}); err != nil {
		return nil, err
	}
	return created, nil
}

// CreateBillingFactsInTx records the immutable quote and initial settlement
// state alongside the task insert transaction.
func (s *Store) CreateBillingFactsInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) error {
	if tx == nil || task == nil {
		return fmt.Errorf("tasking postgres: billing facts require transaction and task")
	}
	snapshot := task.BillingSnapshot
	snapshotAccountID := snapshot.BillingAccountID
	if snapshotAccountID <= 0 {
		snapshotAccountID = task.BillingAccountID
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, billing_account_id, workspace_id, service, sku, display_name, price_version_id, price_version,
  output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id
) VALUES ($1,$2,$3,(SELECT internal_id FROM workspaces WHERE public_id = $15),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		task.ID, task.OwnerUserID, snapshotAccountID, task.Service, snapshot.SKU, snapshot.DisplayName,
		snapshot.PriceVersionID, snapshot.PriceVersion, snapshot.OutputResolution,
		snapshot.UnitCredits, snapshot.Quantity, snapshot.TotalCredits, snapshot.QuotedAt,
		nilIfEmpty(snapshot.HoldTransactionID), task.WorkspaceID); err != nil {
		return fmt.Errorf("insert task billing snapshot: %w", err)
	}
	if !settlement.IsInitialState(snapshot.SettlementState) {
		return fmt.Errorf("insert task billing settlement: invalid initial state %q", snapshot.SettlementState)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO task_billing_settlements (task_id, state, settled_at) VALUES ($1,$2,$3)`, task.ID, snapshot.SettlementState, settledAtForInitialState(snapshot.SettlementState)); err != nil {
		return fmt.Errorf("insert task billing settlement: %w", err)
	}
	return nil
}

// settledAtForInitialState resolves the settled_at value client-side. The
// state is already known in Go, so no SQL CASE over the state parameter is
// needed — and such a CASE would pin $2 to text (unknown literals default
// to text in operator resolution), conflicting with the varchar(16)
// column (SQLSTATE 42P08).
func settledAtForInitialState(state string) any {
	if state == "free" {
		return time.Now().UTC()
	}
	return nil
}

func nilIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func taskOutboxRecord(task *tasks.WorkspaceTask, status tasks.Status, eventType string, data any) (eventoutbox.TaskStatusOutboxRecord, error) {
	if task == nil {
		return eventoutbox.TaskStatusOutboxRecord{}, fmt.Errorf("tasking postgres: outbox task is nil")
	}
	raw := json.RawMessage(`{}`)
	if data != nil {
		encoded, err := json.Marshal(data)
		if err != nil {
			return eventoutbox.TaskStatusOutboxRecord{}, fmt.Errorf("tasking postgres: encode outbox event: %w", err)
		}
		raw = encoded
	}
	return eventoutbox.TaskStatusOutboxRecord{
		EventID:      events.StableEventID(task.ID, string(status), task.ExecutionToken),
		WorkspaceID:  task.WorkspaceID,
		TaskID:       task.ID,
		Status:       events.TaskStatus(status),
		EventType:    eventType,
		EventVersion: 1,
		ActorType:    events.V1ActorService,
		ActorID:      "tasking",
		ResourceType: events.V1ResourceTask,
		ResourceID:   task.ID,
		EventData:    raw,
		OccurredAt:   task.UpdatedAt.UTC(),
	}, nil
}

func (s *Store) insertTaskOutboxTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask, status tasks.Status, eventType string, data any) error {
	if s == nil || s.outbox == nil {
		return fmt.Errorf("tasking postgres: task outbox is not configured (fail-closed: settlement must not commit without its delivery fact): %w", tasks.ErrSettlementUnavailable)
	}
	record, err := taskOutboxRecord(task, status, eventType, data)
	if err != nil {
		return err
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now().UTC()
	}
	return s.outbox.InsertTx(ctx, tx, record)
}

// SetBillingSettlementInTx updates only the mutable settlement read model.
// The snapshot itself remains immutable by database trigger contract.
//
// The update joins the caller's terminal settlement transaction: a failed
// read-model write rolls the state flip and the ledger Capture/Release
// back with it, so funds are never lost to a half-committed terminal.
//
// Missing settlement rows fail closed: submit writes snapshot and settlement atomically.
func (s *Store) SetBillingSettlementInTx(ctx context.Context, tx pgx.Tx, taskID, state string, outputCount, captured, released int64, captureTransactionID, releaseTransactionID string) error {
	if tx == nil {
		return fmt.Errorf("tasking postgres: billing settlement transaction is nil")
	}
	if !settlement.IsTerminalState(state) {
		return fmt.Errorf("update task billing settlement: invalid terminal state %q", state)
	}
	var settledAt *time.Time
	now := time.Now().UTC()
	settledAt = &now
	tag, err := tx.Exec(ctx, `UPDATE task_billing_settlements
SET state=$2, actual_output_count=$3, captured_credits=$4, released_credits=$5,
    capture_transaction_id=$6, release_transaction_id=$7, settled_at=$8
WHERE task_id=$1`, taskID, state, outputCount, captured, released,
		nilIfEmpty(captureTransactionID), nilIfEmpty(releaseTransactionID), settledAt)
	if err != nil {
		return fmt.Errorf("update task billing settlement: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("update task billing settlement: task %q not found", taskID)
	}
	return nil
}

func createTaskWithQueries(ctx context.Context, q *dbsqlc.Queries, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error) {
	if task == nil {
		return nil, fmt.Errorf("tasking postgres: task is nil: %w", tasks.ErrValidation)
	}
	taskID := strings.TrimSpace(task.ID)
	workspaceID := strings.TrimSpace(task.WorkspaceID)
	if !contract.IsTaskID(taskID) || !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return nil, fmt.Errorf("tasking postgres: task identity required: %w", tasks.ErrValidation)
	}
	canonical, err := tasks.ValidateCreate(task.Service, task.MediaAssetIDs, task.Params)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(task.ExecutionToken)
	if token == "" {
		token, err = tasks.NewExecutionToken()
		if err != nil {
			return nil, err
		}
	}
	stored := *task
	stored.ID = taskID
	stored.WorkspaceID = workspaceID
	stored.Service = canonical
	stored.ExecutionToken = token
	envelope, err := marshalEnvelope(&stored)
	if err != nil {
		return nil, err
	}
	workspaceInternalID, err := resolveWorkspace(ctx, q, workspaceID)
	if err != nil {
		return nil, err
	}
	// The submit path resolves the stable account before the Hold and pins
	// it on the task; re-resolution here would risk spending a different
	// account than the Hold. Fall back to resolution only for callers that
	// did not settle through createInTx.
	billingAccountID := stored.BillingAccountID
	if billingAccountID <= 0 {
		billingAccountID, err = resolveBillingAccount(ctx, q, workspaceInternalID)
		if err != nil {
			return nil, err
		}
	}
	row, err := q.CreateWorkspaceTask(ctx, dbsqlc.CreateWorkspaceTaskParams{
		PublicID:         taskID,
		WorkspaceID:      workspaceInternalID,
		SubmitterUserID:  stored.OwnerUserID,
		BillingAccountID: billingAccountID,
		Service:          canonical,
		WorkflowInput:    envelope,
		Priority:         defaultTaskPriority,
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			return nil, fmt.Errorf("tasking postgres: duplicate task: %w", tasks.ErrStateConflict)
		}
		return nil, err
	}
	return mapTaskRow(workspaceID, row, nil)
}

// GetTask resolves a workspace-scoped task with its outputs.
func (s *Store) GetTask(ctx context.Context, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	return s.getTaskWithQueries(ctx, s.dbWithContext(ctx).q, workspaceID, taskID)
}

// GetTaskInTx resolves a workspace-scoped task inside the caller's
// transaction. The idempotent replay path uses it so the task a durable claim
// names is read from the same snapshot that observed the claim.
func (s *Store) GetTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	if tx == nil {
		return nil, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	return s.getTaskWithQueries(ctx, s.q.WithTx(tx), workspaceID, taskID)
}

// ClaimSubmissionInTx records the durable submit idempotency claim inside the
// caller's settlement transaction, or reports the claim that already holds
// the key.
//
// Concurrency: the INSERT ... ON CONFLICT DO NOTHING serializes on the
// primary key (workspace_id, operation, idempotency_key). A concurrent submit
// of the same key blocks until the first transaction commits or rolls back,
// then reads the winner. Exactly one submit settles.
//
// The workspace is resolved by public id inside the same statement, so a
// caller that cannot see the workspace inserts nothing and reads no claim:
// it fails closed instead of settling outside its workspace.
func (s *Store) ClaimSubmissionInTx(ctx context.Context, tx pgx.Tx, claim tasks.SubmissionClaim) (*tasks.SubmissionClaim, bool, error) {
	if tx == nil {
		return nil, false, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	workspaceID := strings.TrimSpace(claim.WorkspaceID)
	operation := strings.TrimSpace(claim.Operation)
	key := strings.TrimSpace(claim.IdempotencyKey)
	taskPublicID := strings.TrimSpace(claim.TaskPublicID)
	requestHash := strings.TrimSpace(claim.RequestHash)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return nil, false, fmt.Errorf("tasking postgres: claim workspace invalid: %w", tasks.ErrValidation)
	}
	if operation == "" || key == "" || taskPublicID == "" || requestHash == "" {
		return nil, false, fmt.Errorf("tasking postgres: claim fields required: %w", tasks.ErrValidation)
	}
	if !contract.IsTaskID(taskPublicID) {
		return nil, false, fmt.Errorf("tasking postgres: claim task identity invalid: %w", tasks.ErrValidation)
	}
	// ON CONFLICT DO NOTHING: pgx.ErrNoRows means the key is already claimed.
	// The RETURNING clause only fires for the row this transaction inserted.
	if _, err := s.q.WithTx(tx).ClaimTaskSubmissionIdempotency(ctx, dbsqlc.ClaimTaskSubmissionIdempotencyParams{
		WorkspacePublicID: workspaceID, Operation: operation,
		IdempotencyKey: key, RequestHash: requestHash, TaskPublicID: taskPublicID,
	}); err == nil {
		return nil, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("tasking postgres: claim submission: %w", err)
	}
	// Either the key is already claimed, or the workspace does not exist.
	// Distinguish them: a missing workspace must fail closed, never look like
	// a replay of a claim nobody can see.
	existing, readErr := s.q.WithTx(tx).GetTaskSubmissionIdempotency(ctx, dbsqlc.GetTaskSubmissionIdempotencyParams{
		WorkspacePublicID: workspaceID, Operation: operation, IdempotencyKey: key,
	})
	if readErr != nil {
		if errors.Is(readErr, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf("tasking postgres: claim workspace not resolvable: %w", tasks.ErrNotFound)
		}
		return nil, false, fmt.Errorf("tasking postgres: read submission claim: %w", readErr)
	}
	return &tasks.SubmissionClaim{
		WorkspaceID: workspaceID, Operation: operation, IdempotencyKey: key,
		RequestHash: existing.RequestHash, TaskPublicID: existing.TaskPublicID,
	}, false, nil
}

// ListTasks pages workspace tasks newest-first with limit/offset (the v1
// layer converts cursor to offset). Outputs batch in one query.
func (s *Store) ListTasks(ctx context.Context, workspaceID string, limit, offset int) ([]*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return nil, tasks.ErrNotFound
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	dbCtx := s.dbWithContext(ctx)
	rows, err := dbCtx.q.ListWorkspaceTasks(ctx, dbsqlc.ListWorkspaceTasksParams{
		WorkspacePublicID: workspaceID,
		RowLimit:          int32(limit),
		RowOffset:         int32(offset),
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.InternalID)
	}
	byTask := make(map[int64][]tasks.OutputRef, len(rows))
	if len(ids) > 0 {
		outputRows, err := dbCtx.q.ListWorkspaceTaskOutputsByTaskIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, outputRow := range outputRows {
			ref, err := mapOutputRow(outputRow)
			if err != nil {
				return nil, err
			}
			byTask[outputRow.TaskID] = append(byTask[outputRow.TaskID], ref)
		}
	}
	out := make([]*tasks.WorkspaceTask, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapTaskRow(workspaceID, row, byTask[row.InternalID])
		if err != nil {
			return nil, err
		}
		out = append(out, mapped)
	}
	return out, nil
}

// CountTasks counts workspace tasks.
func (s *Store) CountTasks(ctx context.Context, workspaceID string) (int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return 0, tasks.ErrNotFound
	}
	count, err := s.dbWithContext(ctx).q.CountWorkspaceTasks(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// Compile-time convergence with the final-timeout sweep discovery seam.
var _ taskports.StalledSettlementScanner = (*Store)(nil)

// ListStalledHeldSettlements names workspace-scoped tasks whose Credit Hold
// was abandoned: the task is still live, still owes a hold, its settlement has
// not reached captured/released, and no worker has touched its row since
// stalledBefore (see the query comment for why tasks.updated_at is the
// liveness clock).
//
// It is READ-ONLY discovery: convergence happens through the workspace-scoped
// Fail seam, never here. That is what keeps the sweep's cross-workspace scan
// safe — the scan may see every workspace, but the write it triggers is fenced
// by workspace public id exactly like every other terminal settlement.
//
// Oldest-first with a caller-supplied bound; a non-positive limit fails closed
// rather than scanning the whole table.
func (s *Store) ListStalledHeldSettlements(ctx context.Context, stalledBefore time.Time, limit int) ([]tasks.StalledSettlement, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("tasking postgres: %w", database.ErrPoolNotConfigured)
	}
	if stalledBefore.IsZero() {
		return nil, fmt.Errorf("tasking postgres: stalled settlement cutoff required: %w", tasks.ErrValidation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("tasking postgres: stalled settlement limit must be positive: %w", tasks.ErrValidation)
	}
	rows, err := s.dbWithContext(ctx).q.ListStalledHeldWorkspaceTasks(ctx, dbsqlc.ListStalledHeldWorkspaceTasksParams{
		StalledBefore: pgtype.Timestamptz{Time: stalledBefore.UTC(), Valid: true},
		RowLimit:      int32(limit),
	})
	if err != nil {
		return nil, err
	}
	stalled := make([]tasks.StalledSettlement, 0, len(rows))
	for _, row := range rows {
		workspaceID := strings.TrimSpace(row.WorkspacePublicID)
		taskID := strings.TrimSpace(row.PublicID)
		// A row the store cannot address is not a candidate: dropping it here
		// keeps a malformed pair from ever reaching the settle transaction.
		if !validTaskRef(workspaceID, taskID) {
			continue
		}
		stalled = append(stalled, tasks.StalledSettlement{WorkspaceID: workspaceID, TaskID: taskID})
	}
	return stalled, nil
}

// DeleteTask removes a task row (settlement compensation rollback).
func (s *Store) DeleteTask(ctx context.Context, workspaceID, taskID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return tasks.ErrNotFound
	}
	return dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		rows, err := q.DeleteWorkspaceTask(ctx, dbsqlc.DeleteWorkspaceTaskParams{PublicID: taskID, WorkspacePublicID: workspaceID})
		if err != nil {
			return err
		}
		if rows == 0 {
			return tasks.ErrNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM task_status_event_outbox WHERE task_id=$1`, taskID)
		return err
	})
}

// convergeFailedUpdate distinguishes a lost race (unknown id or
// cross-workspace id -> ErrNotFound) from a terminal-state guard
// (-> ErrStateConflict) after a transition UPDATE matched zero rows.
func (s *Store) convergeFailedUpdate(ctx context.Context, workspaceID, taskID string) error {
	return convergeFailedUpdateWithQueries(ctx, s.dbWithContext(ctx).q, workspaceID, taskID)
}

func convergeFailedUpdateWithQueries(ctx context.Context, q *dbsqlc.Queries, workspaceID, taskID string) error {
	current, err := getTaskWithQueries(ctx, q, workspaceID, taskID)
	if err != nil {
		return err
	}
	if current.IsTerminal() {
		return tasks.ErrStateConflict
	}
	return tasks.ErrStateConflict
}

// ClaimTask fences one pending task for the worker: pending -> running
// with the lease token. Callers pass the task execution token; the lease
// mirrors it while running and clears on every terminal/retry flip.
func (s *Store) ClaimTask(ctx context.Context, workspaceID, taskID, leaseToken string) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	leaseToken = strings.TrimSpace(leaseToken)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	if leaseToken == "" {
		return nil, fmt.Errorf("tasking postgres: lease token required: %w", tasks.ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	row, err := s.q.WithTx(tx).ClaimWorkspaceTask(ctx, dbsqlc.ClaimWorkspaceTaskParams{
		LeaseToken:        leaseToken,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, s.convergeFailedUpdate(ctx, workspaceID, taskID)
		}
		return nil, err
	}
	claimed, err := s.mapRowWithOutputsWithQueries(ctx, s.q.WithTx(tx), workspaceID, row)
	if err != nil {
		return nil, err
	}
	if err := s.insertTaskOutboxTx(ctx, tx, claimed, tasks.StatusRunning, events.EventV1TaskRunning,
		events.V1TaskRunningData{TaskID: claimed.ID}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	return claimed, nil
}

// claimRowToTask narrows the merged claim row back to the task projection the
// shared row mappers consume. The merged statement appends attempt_no and
// attempt_started to the task columns, so the projection has to be rebuilt
// field by field; TestClaimRowToTaskPreservesEveryTaskColumn fails the build
// when tasks grows a column this narrowing drops.
func claimRowToTask(row dbsqlc.ClaimWorkspaceTaskWithAttemptRow) dbsqlc.Task {
	return dbsqlc.Task{
		InternalID:        row.InternalID,
		PublicID:          row.PublicID,
		WorkspaceID:       row.WorkspaceID,
		SubmitterUserID:   row.SubmitterUserID,
		BillingAccountID:  row.BillingAccountID,
		Service:           row.Service,
		WorkflowInput:     row.WorkflowInput,
		Status:            row.Status,
		Priority:          row.Priority,
		RetryCount:        row.RetryCount,
		LeaseToken:        row.LeaseToken,
		ErrorMessage:      row.ErrorMessage,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		StartedAt:         row.StartedAt,
		CompletedAt:       row.CompletedAt,
		CancelledAt:       row.CancelledAt,
		CancelledByUserID: row.CancelledByUserID,
		RoutingSnapshot:   row.RoutingSnapshot,
	}
}

// ClaimTaskWithAttempt fences one execution and opens its durable attempt
// fact in ONE statement (ClaimWorkspaceTaskWithAttempt), satisfying the
// DESIGN.md contract 4 invariant: the lease flip and the attempt row commit
// atomically, so no running lease can exist without the attempt fact that
// explains it.
//
// The attempt row's execution_token IS leaseToken. Passing a second token
// through start would recreate the two-token split the single statement
// removes, so the field is deliberately ignored: this method is the only
// writer of that token.
//
// started is false when the lease was re-acquired by a River redelivery that
// already owns an attempt row: the same-token branch of the lease fence lets
// the earlier attempt resume, and ON CONFLICT DO NOTHING keeps exactly one
// attempt row (and one attempt_no) per lease token. Callers treat a resume as
// the same send continuing, never as a second attempt fact.
//
// ErrStateConflict on a fenced-out claim keeps ClaimTask's convergence
// contract: terminal/cancelled rows read back through the same reload path.
func (s *Store) ClaimTaskWithAttempt(ctx context.Context, workspaceID, taskID, leaseToken string, start tasks.AttemptStart) (*tasks.WorkspaceTask, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	leaseToken = strings.TrimSpace(leaseToken)
	if !validTaskRef(workspaceID, taskID) {
		return nil, false, tasks.ErrNotFound
	}
	if leaseToken == "" {
		return nil, false, fmt.Errorf("tasking postgres: lease token required: %w", tasks.ErrValidation)
	}
	layer := strings.TrimSpace(start.RetryLayer)
	if layer == "" {
		return nil, false, fmt.Errorf("tasking postgres: attempt retry layer required")
	}
	if start.GatewayRetryCount < 0 {
		return nil, false, fmt.Errorf("tasking postgres: attempt gateway retry count is negative")
	}
	if start.RiverJobID < 0 {
		return nil, false, fmt.Errorf("tasking postgres: attempt river job id is negative")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	row, err := s.q.WithTx(tx).ClaimWorkspaceTaskWithAttempt(ctx, dbsqlc.ClaimWorkspaceTaskWithAttemptParams{
		LeaseToken:        leaseToken,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
		RiverJobID:        start.RiverJobID,
		Service:           strings.TrimSpace(start.Service),
		RequestedModel:    strings.TrimSpace(start.RequestedModel),
		ResolvedModel:     strings.TrimSpace(start.ResolvedModel),
		Provider:          strings.TrimSpace(start.Provider),
		RetryLayer:        layer,
		GatewayRetryCount: int32(start.GatewayRetryCount),
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, false, s.convergeFailedUpdate(ctx, workspaceID, taskID)
		}
		return nil, false, err
	}
	claimed, err := s.mapRowWithOutputsWithQueries(ctx, s.q.WithTx(tx), workspaceID, claimRowToTask(row))
	if err != nil {
		return nil, false, err
	}
	if row.AttemptStarted {
		if err := s.insertTaskOutboxTx(ctx, tx, claimed, tasks.StatusRunning, events.EventV1TaskRunning,
			events.V1TaskRunningData{TaskID: claimed.ID}); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	committed = true
	return claimed, row.AttemptStarted, nil
}

// CancelTask flips pending/running to cancelled, records actor + reason,
// and releases the lease. Terminal tasks replay idempotently
// (already=true).
func (s *Store) CancelTask(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	var cancelled *tasks.WorkspaceTask
	var already bool
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		var err error
		cancelled, already, err = cancelTaskWithQueries(ctx, s, q, workspaceID, taskID, actorUserID, reason)
		if err != nil || already {
			return err
		}
		return s.insertTaskOutboxTx(ctx, tx, cancelled, tasks.StatusCancelled, events.EventV1TaskCancelled,
			events.V1TaskCancelledData{TaskID: cancelled.ID, CancelReason: cancelled.CancelReason})
	})
	return cancelled, already, err
}

// CancelTaskInTx flips to cancelled inside the caller's settlement
// transaction (state flip + Credits Release commit atomically).
func (s *Store) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	if tx == nil {
		return nil, false, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	cancelled, already, err := cancelTaskWithQueries(ctx, s, s.q.WithTx(tx), workspaceID, taskID, actorUserID, reason)
	if err != nil || already {
		return cancelled, already, err
	}
	if err := s.insertTaskOutboxTx(ctx, tx, cancelled, tasks.StatusCancelled, events.EventV1TaskCancelled,
		events.V1TaskCancelledData{TaskID: cancelled.ID, CancelReason: cancelled.CancelReason}); err != nil {
		return nil, false, err
	}
	return cancelled, false, nil
}

func cancelTaskWithQueries(ctx context.Context, s *Store, q *dbsqlc.Queries, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, false, tasks.ErrNotFound
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "cancelled"
	}
	var cancelledBy pgtype.Int8
	if actorUserID > 0 {
		cancelledBy = pgtype.Int8{Int64: actorUserID, Valid: true}
	}
	row, err := q.CancelWorkspaceTask(ctx, dbsqlc.CancelWorkspaceTaskParams{
		CancelledByUserID: cancelledBy,
		CancelReason:      reason,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if !database.IsNotFound(err) {
			return nil, false, err
		}
		current, getErr := getTaskWithQueries(ctx, q, workspaceID, taskID)
		if getErr != nil {
			return nil, false, getErr
		}
		if current.IsTerminal() {
			return current, true, nil
		}
		return nil, false, tasks.ErrStateConflict
	}
	mapped, err := s.mapRowWithOutputsWithQueries(ctx, q, workspaceID, row)
	if err != nil {
		return nil, false, err
	}
	return mapped, false, nil
}

// insertOutputRefs validates and appends output refs with dense
// output_index values starting at baseIndex. Asset refs resolve to
// workspace-scoped media assets; the asset public id rides in metadata
// for read-back (task_outputs carries only the internal FK).
func insertOutputRefs(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, taskPublicID string, baseIndex int, outputs []tasks.OutputRef) error {
	for i, ref := range outputs {
		if err := tasks.ValidateOutputRef(ref); err != nil {
			return err
		}
		var kind string
		var mediaAssetID pgtype.Int8
		var remoteURL pgtype.Text
		meta := outputMetadata{Role: ref.Role, ChecksumSHA256: ref.ChecksumSHA256}
		switch ref.Kind {
		case tasks.OutputKindAsset:
			kind = "media_asset"
			internalID, err := q.GetWorkspaceMediaAssetID(ctx, dbsqlc.GetWorkspaceMediaAssetIDParams{
				PublicID:          strings.TrimSpace(ref.AssetID),
				WorkspacePublicID: strings.TrimSpace(workspacePublicID),
			})
			if err != nil {
				return fmt.Errorf("tasking postgres: output asset: %w", tasks.ErrMediaNotFound)
			}
			mediaAssetID = pgtype.Int8{Int64: internalID, Valid: true}
			meta.AssetID = strings.TrimSpace(ref.AssetID)
			meta.ChecksumSHA256 = ""
		case tasks.OutputKindRemote:
			kind = "remote_url"
			remoteURL = pgtype.Text{String: strings.TrimSpace(ref.URL), Valid: true}
		default:
			return fmt.Errorf("tasking postgres: output kind %q: %w", ref.Kind, tasks.ErrValidation)
		}
		metaRaw, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("tasking postgres: encode output metadata: %w", err)
		}
		if _, err := q.InsertWorkspaceTaskOutput(ctx, dbsqlc.InsertWorkspaceTaskOutputParams{
			OutputIndex:       int32(baseIndex + i),
			OutputKind:        kind,
			MediaAssetID:      mediaAssetID,
			RemoteUrl:         remoteURL,
			Metadata:          metaRaw,
			PublicID:          strings.TrimSpace(taskPublicID),
			WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		}); err != nil {
			return err
		}
	}
	return nil
}

// AddOutputs appends output refs to a task (bounded by MaxOutputsPerTask).
func (s *Store) AddOutputs(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	var refreshed *tasks.WorkspaceTask
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		current, err := q.GetWorkspaceTaskByPublicID(ctx, dbsqlc.GetWorkspaceTaskByPublicIDParams{
			PublicID:          taskID,
			WorkspacePublicID: workspaceID,
		})
		if err != nil {
			return notFound(err)
		}
		existing, err := q.ListWorkspaceTaskOutputs(ctx, dbsqlc.ListWorkspaceTaskOutputsParams{
			PublicID:          taskID,
			WorkspacePublicID: workspaceID,
		})
		if err != nil {
			return err
		}
		if len(existing)+len(outputs) > tasks.MaxOutputsPerTask {
			return fmt.Errorf("tasking postgres: too many outputs: %w", tasks.ErrValidation)
		}
		if err := insertOutputRefs(ctx, q, workspaceID, taskID, len(existing), outputs); err != nil {
			return err
		}
		loaded, err := loadOutputs(ctx, q, workspaceID, taskID)
		if err != nil {
			return err
		}
		refreshed, err = mapTaskRow(workspaceID, current, loaded)
		return err
	})
	if err != nil {
		return nil, err
	}
	return refreshed, nil
}

// TransitionForRetry consumes one retry budget entry: the task returns to
// pending with retry_count incremented, the lease released, and the fresh
// execution token recorded in the envelope.
func (s *Store) TransitionForRetry(ctx context.Context, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error) {
	var retried *tasks.WorkspaceTask
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		var err error
		retried, err = transitionForRetryWithQueries(ctx, s, q, workspaceID, taskID, newToken)
		if err != nil {
			return err
		}
		return s.insertTaskOutboxTx(ctx, tx, retried, tasks.StatusPending, "", nil)
	})
	return retried, err
}

// TransitionForRetryInTx consumes one retry budget entry inside the
// caller's settlement transaction (retry flip + River requeue atomic).
func (s *Store) TransitionForRetryInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error) {
	if tx == nil {
		return nil, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	retried, err := transitionForRetryWithQueries(ctx, s, s.q.WithTx(tx), workspaceID, taskID, newToken)
	if err != nil {
		return nil, err
	}
	if err := s.insertTaskOutboxTx(ctx, tx, retried, tasks.StatusPending, "", nil); err != nil {
		return nil, err
	}
	return retried, nil
}

func transitionForRetryWithQueries(ctx context.Context, s *Store, q *dbsqlc.Queries, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	newToken = strings.TrimSpace(newToken)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	if newToken == "" {
		return nil, fmt.Errorf("tasking postgres: execution token required: %w", tasks.ErrValidation)
	}
	row, err := q.RetryWorkspaceTask(ctx, dbsqlc.RetryWorkspaceTaskParams{
		ExecutionToken:    newToken,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, convergeFailedUpdateWithQueries(ctx, q, workspaceID, taskID)
		}
		return nil, err
	}
	return s.mapRowWithOutputsWithQueries(ctx, q, workspaceID, row)
}

// CompleteTask flips a live task to completed and persists outputs
// atomically in one transaction.
func (s *Store) CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	for _, ref := range outputs {
		if err := tasks.ValidateOutputRef(ref); err != nil {
			return nil, err
		}
	}
	var completed *tasks.WorkspaceTask
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		done, err := completeTaskWithQueries(ctx, q, workspaceID, taskID, outputs)
		if err != nil {
			return err
		}
		completed = done
		outputsData := tasks.BuildTaskCompletedOutputs(completed.WorkspaceID, outputs)
		return s.insertTaskOutboxTx(ctx, tx, completed, tasks.StatusCompleted, events.EventV1TaskCompleted,
			events.V1TaskCompletedData{
				TaskID:     completed.ID,
				OutputsURI: "/api/v1/workspaces/" + completed.WorkspaceID + "/tasks/" + completed.ID + "/outputs",
				Outputs:    outputsData,
			})
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, s.convergeFailedUpdate(ctx, workspaceID, taskID)
		}
		return nil, err
	}
	return completed, nil
}

// CompleteTaskInTx flips to completed (persisting outputs) inside the
// caller's settlement transaction (state flip + outputs + Credits
// Capture commit atomically).
func (s *Store) CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	for _, ref := range outputs {
		if err := tasks.ValidateOutputRef(ref); err != nil {
			return nil, err
		}
	}
	if tx == nil {
		return nil, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	q := s.q.WithTx(tx)
	completed, err := completeTaskWithQueries(ctx, q, workspaceID, taskID, outputs)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, convergeFailedUpdateWithQueries(ctx, q, workspaceID, taskID)
		}
		return nil, err
	}
	outputsData := tasks.BuildTaskCompletedOutputs(completed.WorkspaceID, outputs)
	if err := s.insertTaskOutboxTx(ctx, tx, completed, tasks.StatusCompleted, events.EventV1TaskCompleted,
		events.V1TaskCompletedData{
			TaskID:     completed.ID,
			OutputsURI: "/api/v1/workspaces/" + completed.WorkspaceID + "/tasks/" + completed.ID + "/outputs",
			Outputs:    outputsData,
		}); err != nil {
		return nil, err
	}
	return completed, nil
}

func completeTaskWithQueries(ctx context.Context, q *dbsqlc.Queries, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	row, err := q.CompleteWorkspaceTask(ctx, dbsqlc.CompleteWorkspaceTaskParams{
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	if err := insertOutputRefs(ctx, q, workspaceID, taskID, 0, outputs); err != nil {
		return nil, err
	}
	loaded, err := loadOutputs(ctx, q, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	return mapTaskRow(workspaceID, row, loaded)
}

// FailTask flips a live task to failed with the terminal error and
// releases the lease.
func (s *Store) FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	var failed *tasks.WorkspaceTask
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		var err error
		failed, err = failTaskWithQueries(ctx, s, q, workspaceID, taskID, errorMsg)
		if err != nil {
			return err
		}
		return s.insertTaskOutboxTx(ctx, tx, failed, tasks.StatusFailed, events.EventV1TaskFailed,
			events.V1TaskFailedData{TaskID: failed.ID, ErrorCode: "task_failed"})
	})
	return failed, err
}

// FailTaskInTx flips to failed inside the caller's settlement transaction
// (state flip + Credits Release commit atomically).
func (s *Store) FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	if tx == nil {
		return nil, fmt.Errorf("tasking postgres: settlement transaction is nil")
	}
	failed, err := failTaskWithQueries(ctx, s, s.q.WithTx(tx), workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	if err := s.insertTaskOutboxTx(ctx, tx, failed, tasks.StatusFailed, events.EventV1TaskFailed,
		events.V1TaskFailedData{TaskID: failed.ID, ErrorCode: "task_failed"}); err != nil {
		return nil, err
	}
	return failed, nil
}

func failTaskWithQueries(ctx context.Context, s *Store, q *dbsqlc.Queries, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, tasks.ErrNotFound
	}
	errorMsg = strings.TrimSpace(errorMsg)
	if errorMsg == "" {
		errorMsg = "task failed"
	}
	row, err := q.FailWorkspaceTask(ctx, dbsqlc.FailWorkspaceTaskParams{
		ErrorMessage:      errorMsg,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, convergeFailedUpdateWithQueries(ctx, q, workspaceID, taskID)
		}
		return nil, err
	}
	return s.mapRowWithOutputsWithQueries(ctx, q, workspaceID, row)
}
