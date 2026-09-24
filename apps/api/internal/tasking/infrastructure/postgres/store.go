package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultTaskPriority matches the tasks table default. The workspace task
// surface carries no priority input; every insert uses the default.
const defaultTaskPriority = int32(2)

// Store is the postgres workspacetasks.WorkspaceTaskStore.
type Store struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the v1-consumed seam.
var _ workspacetasks.WorkspaceTaskStore = (*Store)(nil)

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// NewStore builds a postgres workspace task Store over pool.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, q: dbsqlc.New(db)}
}

func (s *Store) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

// workflowInputEnvelope is the tasks.workflow_input JSONB shape. Billing
// snapshots are quote-time facts (the credits ledger owns settlement
// truth); execution_token is the durable retry-fencing token; cancel
// reason rides here because tasks carries no cancel text column.
type workflowInputEnvelope struct {
	MediaAssetIDs   []string                        `json:"media_asset_ids,omitempty"`
	Params          map[string]any                  `json:"params,omitempty"`
	BillingSnapshot *workspacetasks.BillingSnapshot `json:"billing_snapshot,omitempty"`
	ExecutionToken  string                          `json:"execution_token,omitempty"`
	CancelReason    string                          `json:"cancel_reason,omitempty"`
}

// outputMetadata is the task_outputs.metadata shape.
type outputMetadata struct {
	AssetID        string `json:"asset_id,omitempty"`
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
	Role           string `json:"role,omitempty"`
}

func notFound(err error) error {
	if database.IsNotFound(err) {
		return workspacetasks.ErrNotFound
	}
	return err
}

func validTaskRef(workspaceID, taskID string) bool {
	return contract.IsValidPublicID(contract.PrefixWorkspace, strings.TrimSpace(workspaceID)) &&
		contract.IsTaskID(strings.TrimSpace(taskID))
}

func marshalEnvelope(task *workspacetasks.WorkspaceTask) ([]byte, error) {
	envelope := workflowInputEnvelope{
		MediaAssetIDs:  append([]string(nil), task.MediaAssetIDs...),
		Params:         task.Params,
		ExecutionToken: task.ExecutionToken,
		CancelReason:   task.CancelReason,
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

func mapOutputRow(row dbsqlc.TaskOutput) (workspacetasks.OutputRef, error) {
	var meta outputMetadata
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return workspacetasks.OutputRef{}, fmt.Errorf("tasking postgres: decode output metadata: %w", err)
		}
	}
	switch strings.TrimSpace(row.OutputKind) {
	case "media_asset":
		return workspacetasks.OutputRef{
			Kind:        workspacetasks.OutputKindAsset,
			AssetID:     meta.AssetID,
			Role:        meta.Role,
			ResultIndex: int(row.OutputIndex),
		}, nil
	case "remote_url":
		return workspacetasks.OutputRef{
			Kind:           workspacetasks.OutputKindRemote,
			URL:            row.RemoteUrl.String,
			ChecksumSHA256: meta.ChecksumSHA256,
			Role:           meta.Role,
			ResultIndex:    int(row.OutputIndex),
		}, nil
	default:
		return workspacetasks.OutputRef{}, fmt.Errorf("tasking postgres: unknown output kind %q", row.OutputKind)
	}
}

func mapTaskRow(workspacePublicID string, row dbsqlc.Task, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	envelope, err := parseEnvelope(row.WorkflowInput)
	if err != nil {
		return nil, err
	}
	completed := dbutil.TimePtr(row.CompletedAt)
	if row.Status == workspacetasks.StatusCancelled && completed == nil {
		completed = dbutil.TimePtr(row.CancelledAt)
	}
	task := &workspacetasks.WorkspaceTask{
		ID:             row.PublicID,
		WorkspaceID:    strings.TrimSpace(workspacePublicID),
		OwnerUserID:    row.SubmitterUserID,
		WorkflowKind:   row.WorkflowType,
		MediaAssetIDs:  append([]string(nil), envelope.MediaAssetIDs...),
		Params:         envelope.Params,
		Status:         row.Status,
		RetryCount:     int(row.RetryCount),
		ExecutionToken: envelope.ExecutionToken,
		OutputRefs:     append([]workspacetasks.OutputRef(nil), outputs...),
		ErrorMsg:       row.ErrorMessage,
		CancelReason:   envelope.CancelReason,
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:      dbutil.TimeValue(row.UpdatedAt),
		CompletedAt:    completed,
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
		return 0, workspacetasks.ErrNotFound
	}
	row, err := q.GetWorkspaceByPublicID(ctx, workspacePublicID)
	if err != nil {
		return 0, notFound(err)
	}
	return row.InternalID, nil
}

// resolveBillingAccount returns the workspace billing account (1:1),
// provisioning it on demand. Task inserts must never fail on a missing
// billing account; full billing lifecycle stays in #804.
func resolveBillingAccount(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64) (int64, error) {
	workspaceID := pgtype.Int8{Int64: workspaceInternalID, Valid: true}
	row, err := q.GetBillingAccountByWorkspaceID(ctx, workspaceID)
	if err == nil {
		return row.InternalID, nil
	}
	if !database.IsNotFound(err) {
		return 0, err
	}
	publicID, mintErr := contract.MintPublicID("ba_", 12)
	if mintErr != nil {
		return 0, mintErr
	}
	created, createErr := q.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID:       publicID,
		WorkspaceID:    pgtype.Int8{Int64: workspaceInternalID, Valid: true},
		OrganizationID: pgtype.Int8{},
	})
	if createErr != nil {
		if database.IsDuplicateConstraintError(createErr) {
			existing, getErr := q.GetBillingAccountByWorkspaceID(ctx, workspaceID)
			if getErr != nil {
				return 0, notFound(getErr)
			}
			return existing.InternalID, nil
		}
		return 0, createErr
	}
	return created.InternalID, nil
}

// loadOutputs reads ordered output refs for one task row.
func loadOutputs(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, taskPublicID string) ([]workspacetasks.OutputRef, error) {
	rows, err := q.ListWorkspaceTaskOutputs(ctx, dbsqlc.ListWorkspaceTaskOutputsParams{
		PublicID:          strings.TrimSpace(taskPublicID),
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if err != nil {
		return nil, err
	}
	outputs := make([]workspacetasks.OutputRef, 0, len(rows))
	for _, row := range rows {
		ref, err := mapOutputRow(row)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, ref)
	}
	return outputs, nil
}

func (s *Store) mapRowWithOutputs(ctx context.Context, workspacePublicID string, row dbsqlc.Task) (*workspacetasks.WorkspaceTask, error) {
	outputs, err := loadOutputs(ctx, s.dbWithContext(ctx).q, workspacePublicID, row.PublicID)
	if err != nil {
		return nil, err
	}
	return mapTaskRow(workspacePublicID, row, outputs)
}

// CreateTask inserts a pending task. The input carries Service-validated
// facts; the repository re-validates the discriminated union (canonical
// workflow kind, asset shapes, params) so direct callers fail closed too.
func (s *Store) CreateTask(ctx context.Context, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	if task == nil {
		return nil, fmt.Errorf("tasking postgres: task is nil: %w", workspacetasks.ErrValidation)
	}
	taskID := strings.TrimSpace(task.ID)
	workspaceID := strings.TrimSpace(task.WorkspaceID)
	if !contract.IsTaskID(taskID) || !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return nil, fmt.Errorf("tasking postgres: task identity required: %w", workspacetasks.ErrValidation)
	}
	canonical, err := workspacetasks.ValidateCreate(task.WorkflowKind, task.MediaAssetIDs, task.Params)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(task.ExecutionToken)
	if token == "" {
		token, err = workspacetasks.NewExecutionToken()
		if err != nil {
			return nil, err
		}
	}
	stored := *task
	stored.ID = taskID
	stored.WorkspaceID = workspaceID
	stored.WorkflowKind = canonical
	stored.ExecutionToken = token
	envelope, err := marshalEnvelope(&stored)
	if err != nil {
		return nil, err
	}
	dbCtx := s.dbWithContext(ctx)
	workspaceInternalID, err := resolveWorkspace(ctx, dbCtx.q, workspaceID)
	if err != nil {
		return nil, err
	}
	billingAccountID, err := resolveBillingAccount(ctx, dbCtx.q, workspaceInternalID)
	if err != nil {
		return nil, err
	}
	row, err := dbCtx.q.CreateWorkspaceTask(ctx, dbsqlc.CreateWorkspaceTaskParams{
		PublicID:         taskID,
		WorkspaceID:      workspaceInternalID,
		SubmitterUserID:  stored.OwnerUserID,
		BillingAccountID: billingAccountID,
		WorkflowType:     canonical,
		WorkflowInput:    envelope,
		Priority:         defaultTaskPriority,
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			return nil, fmt.Errorf("tasking postgres: duplicate task: %w", workspacetasks.ErrStateConflict)
		}
		return nil, err
	}
	return mapTaskRow(workspaceID, row, nil)
}

// GetTask resolves a workspace-scoped task with its outputs.
func (s *Store) GetTask(ctx context.Context, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, workspacetasks.ErrNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetWorkspaceTaskByPublicID(ctx, dbsqlc.GetWorkspaceTaskByPublicIDParams{
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		return nil, notFound(err)
	}
	return s.mapRowWithOutputs(ctx, workspaceID, row)
}

// ListTasks pages workspace tasks newest-first with limit/offset (the v1
// layer converts cursor to offset). Outputs batch in one query.
func (s *Store) ListTasks(ctx context.Context, workspaceID string, limit, offset int) ([]*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return nil, workspacetasks.ErrNotFound
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
	byTask := make(map[int64][]workspacetasks.OutputRef, len(rows))
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
	out := make([]*workspacetasks.WorkspaceTask, 0, len(rows))
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
		return 0, workspacetasks.ErrNotFound
	}
	count, err := s.dbWithContext(ctx).q.CountWorkspaceTasks(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// DeleteTask removes a task row (settlement compensation rollback).
func (s *Store) DeleteTask(ctx context.Context, workspaceID, taskID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return workspacetasks.ErrNotFound
	}
	rows, err := s.dbWithContext(ctx).q.DeleteWorkspaceTask(ctx, dbsqlc.DeleteWorkspaceTaskParams{
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return workspacetasks.ErrNotFound
	}
	return nil
}

// convergeFailedUpdate distinguishes a lost race (unknown id or
// cross-workspace id -> ErrNotFound) from a terminal-state guard
// (-> ErrStateConflict) after a transition UPDATE matched zero rows.
func (s *Store) convergeFailedUpdate(ctx context.Context, workspaceID, taskID string) error {
	current, err := s.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		return err
	}
	if current.IsTerminal() {
		return workspacetasks.ErrStateConflict
	}
	return workspacetasks.ErrStateConflict
}

// ClaimTask fences one pending task for the worker: pending -> running
// with the lease token. Callers pass the task execution token; the lease
// mirrors it while running and clears on every terminal/retry flip.
func (s *Store) ClaimTask(ctx context.Context, workspaceID, taskID, leaseToken string) (*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	leaseToken = strings.TrimSpace(leaseToken)
	if !validTaskRef(workspaceID, taskID) {
		return nil, workspacetasks.ErrNotFound
	}
	if leaseToken == "" {
		return nil, fmt.Errorf("tasking postgres: lease token required: %w", workspacetasks.ErrValidation)
	}
	row, err := s.dbWithContext(ctx).q.ClaimWorkspaceTask(ctx, dbsqlc.ClaimWorkspaceTaskParams{
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
	return s.mapRowWithOutputs(ctx, workspaceID, row)
}

// CancelTask flips pending/running to cancelled, records actor + reason,
// and releases the lease. Terminal tasks replay idempotently
// (already=true).
func (s *Store) CancelTask(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*workspacetasks.WorkspaceTask, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, false, workspacetasks.ErrNotFound
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "cancelled"
	}
	var cancelledBy pgtype.Int8
	if actorUserID > 0 {
		cancelledBy = pgtype.Int8{Int64: actorUserID, Valid: true}
	}
	row, err := s.dbWithContext(ctx).q.CancelWorkspaceTask(ctx, dbsqlc.CancelWorkspaceTaskParams{
		CancelledByUserID: cancelledBy,
		CancelReason:      reason,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if !database.IsNotFound(err) {
			return nil, false, err
		}
		current, getErr := s.GetTask(ctx, workspaceID, taskID)
		if getErr != nil {
			return nil, false, getErr
		}
		if current.IsTerminal() {
			return current, true, nil
		}
		return nil, false, workspacetasks.ErrStateConflict
	}
	mapped, err := s.mapRowWithOutputs(ctx, workspaceID, row)
	if err != nil {
		return nil, false, err
	}
	return mapped, false, nil
}

// insertOutputRefs validates and appends output refs with dense
// output_index values starting at baseIndex. Asset refs resolve to
// workspace-scoped media assets; the asset public id rides in metadata
// for read-back (task_outputs carries only the internal FK).
func insertOutputRefs(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, taskPublicID string, baseIndex int, outputs []workspacetasks.OutputRef) error {
	for i, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return err
		}
		var kind string
		var mediaAssetID pgtype.Int8
		var remoteURL pgtype.Text
		meta := outputMetadata{Role: ref.Role, ChecksumSHA256: ref.ChecksumSHA256}
		switch ref.Kind {
		case workspacetasks.OutputKindAsset:
			kind = "media_asset"
			internalID, err := q.GetWorkspaceMediaAssetID(ctx, dbsqlc.GetWorkspaceMediaAssetIDParams{
				PublicID:          strings.TrimSpace(ref.AssetID),
				WorkspacePublicID: strings.TrimSpace(workspacePublicID),
			})
			if err != nil {
				return fmt.Errorf("tasking postgres: output asset: %w", workspacetasks.ErrMediaNotFound)
			}
			mediaAssetID = pgtype.Int8{Int64: internalID, Valid: true}
			meta.AssetID = strings.TrimSpace(ref.AssetID)
			meta.ChecksumSHA256 = ""
		case workspacetasks.OutputKindRemote:
			kind = "remote_url"
			remoteURL = pgtype.Text{String: strings.TrimSpace(ref.URL), Valid: true}
		default:
			return fmt.Errorf("tasking postgres: output kind %q: %w", ref.Kind, workspacetasks.ErrValidation)
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
func (s *Store) AddOutputs(ctx context.Context, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, workspacetasks.ErrNotFound
	}
	var refreshed *workspacetasks.WorkspaceTask
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
		if len(existing)+len(outputs) > workspacetasks.MaxOutputsPerTask {
			return fmt.Errorf("tasking postgres: too many outputs: %w", workspacetasks.ErrValidation)
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
func (s *Store) TransitionForRetry(ctx context.Context, workspaceID, taskID, newToken string) (*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	newToken = strings.TrimSpace(newToken)
	if !validTaskRef(workspaceID, taskID) {
		return nil, workspacetasks.ErrNotFound
	}
	if newToken == "" {
		return nil, fmt.Errorf("tasking postgres: execution token required: %w", workspacetasks.ErrValidation)
	}
	row, err := s.dbWithContext(ctx).q.RetryWorkspaceTask(ctx, dbsqlc.RetryWorkspaceTaskParams{
		ExecutionToken:    newToken,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, s.convergeFailedUpdate(ctx, workspaceID, taskID)
		}
		return nil, err
	}
	return s.mapRowWithOutputs(ctx, workspaceID, row)
}

// CompleteTask flips a live task to completed and persists outputs
// atomically in one transaction.
func (s *Store) CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, workspacetasks.ErrNotFound
	}
	for _, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, err
		}
	}
	var completed *workspacetasks.WorkspaceTask
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		row, err := q.CompleteWorkspaceTask(ctx, dbsqlc.CompleteWorkspaceTaskParams{
			PublicID:          taskID,
			WorkspacePublicID: workspaceID,
		})
		if err != nil {
			return err
		}
		if err := insertOutputRefs(ctx, q, workspaceID, taskID, 0, outputs); err != nil {
			return err
		}
		loaded, err := loadOutputs(ctx, q, workspaceID, taskID)
		if err != nil {
			return err
		}
		completed, err = mapTaskRow(workspaceID, row, loaded)
		return err
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, s.convergeFailedUpdate(ctx, workspaceID, taskID)
		}
		return nil, err
	}
	return completed, nil
}

// FailTask flips a live task to failed with the terminal error and
// releases the lease.
func (s *Store) FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	taskID = strings.TrimSpace(taskID)
	if !validTaskRef(workspaceID, taskID) {
		return nil, workspacetasks.ErrNotFound
	}
	errorMsg = strings.TrimSpace(errorMsg)
	if errorMsg == "" {
		errorMsg = "task failed"
	}
	row, err := s.dbWithContext(ctx).q.FailWorkspaceTask(ctx, dbsqlc.FailWorkspaceTaskParams{
		ErrorMessage:      errorMsg,
		PublicID:          taskID,
		WorkspacePublicID: workspaceID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, s.convergeFailedUpdate(ctx, workspaceID, taskID)
		}
		return nil, err
	}
	return s.mapRowWithOutputs(ctx, workspaceID, row)
}
