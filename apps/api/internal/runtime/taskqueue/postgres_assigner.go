package taskqueue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"

	"github.com/lib/pq"
)

type PostgresTaskAssigner struct {
	db *sql.DB
}

func NewPostgresTaskAssigner(db *sql.DB) *PostgresTaskAssigner {
	return &PostgresTaskAssigner{db: db}
}

func (a *PostgresTaskAssigner) Assign(ctx context.Context, accountIDs []int64, task *queuetypes.TaskPayload) (TaskAssignment, error) {
	if a == nil || a.db == nil {
		return TaskAssignment{}, fmt.Errorf("postgres task assigner: db not configured")
	}
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return TaskAssignment{}, fmt.Errorf("postgres task assigner: empty task")
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskAssignment{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	status, currentAccountID, currentLeaseToken, createdAt, err := getTaskStateForUpdate(ctx, tx, task.TaskID)
	if err != nil {
		if err == sql.ErrNoRows {
			if commitErr := tx.Commit(); commitErr != nil {
				return TaskAssignment{}, commitErr
			}
			committed = true
			return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
		}
		return TaskAssignment{}, err
	}
	if tasks.IsTerminalStatus(status) {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
	}
	if status == tasks.StatusRunning && currentAccountID > 0 && leaseBelongsToRiverJob(currentLeaseToken, task.RiverJobID) {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{
			TaskID:     task.TaskID,
			AccountID:  currentAccountID,
			Status:     AssignStatusAssigned,
			LeaseToken: currentLeaseToken,
		}, nil
	}
	if status == tasks.StatusRunning {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
	}

	candidates := normalizeCandidateAccountIDs(accountIDs)
	if len(candidates) == 0 {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusNoCapacity}, nil
	}

	accountID, err := selectLeastLoadedAccountTx(ctx, tx, candidates)
	if err != nil {
		if err == sql.ErrNoRows {
			if err := tx.Commit(); err != nil {
				return TaskAssignment{}, err
			}
			committed = true
			return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusNoCapacity}, nil
		}
		return TaskAssignment{}, err
	}

	leaseToken, err := newTaskLeaseToken(task.RiverJobID)
	if err != nil {
		return TaskAssignment{}, err
	}
	assignedAt := time.Now()
	updated, err := markTaskAssignedTx(ctx, tx, task.TaskID, accountID, leaseToken)
	if err != nil {
		return TaskAssignment{}, err
	}
	if updated == 0 {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
	}
	if err := patchTaskAssignmentMetricsTx(ctx, tx, task.TaskID, createdAt, assignedAt); err != nil {
		return TaskAssignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return TaskAssignment{}, err
	}
	committed = true
	return TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  accountID,
		Status:     AssignStatusAssigned,
		LeaseToken: leaseToken,
	}, nil
}

func (a *PostgresTaskAssigner) ConfirmTaskLease(ctx context.Context, assignment TaskAssignment) error {
	if assignment.AccountID == 0 || strings.TrimSpace(assignment.LeaseToken) == "" {
		return fmt.Errorf("confirm task lease: invalid assignment")
	}
	var count int64
	err := a.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tasks
WHERE task_id = $1
  AND account_id = $2
  AND lease_token = $3
  AND status = $4`,
		assignment.TaskID,
		assignment.AccountID,
		assignment.LeaseToken,
		tasks.StatusRunning,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("confirm task lease: lease not found")
	}
	return nil
}

func getTaskStateForUpdate(ctx context.Context, tx *sql.Tx, taskID string) (string, int64, string, time.Time, error) {
	var status string
	var accountID int64
	var leaseToken string
	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `
SELECT status, account_id, lease_token, created_at
FROM tasks
WHERE task_id = $1
FOR UPDATE`, taskID).Scan(&status, &accountID, &leaseToken, &createdAt)
	return status, accountID, leaseToken, createdAt, err
}

func markTaskAssignedTx(ctx context.Context, tx *sql.Tx, taskID string, accountID int64, leaseToken string) (int64, error) {
	result, err := tx.ExecContext(ctx, `
UPDATE tasks
SET status = $1,
    account_id = $2,
    lease_token = $3,
    error_msg = '',
    completed_at = NULL
WHERE task_id = $4
  AND status = $5`,
		tasks.StatusRunning,
		accountID,
		leaseToken,
		taskID,
		tasks.StatusPending,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func patchTaskAssignmentMetricsTx(ctx context.Context, tx *sql.Tx, taskID string, createdAt time.Time, assignedAt time.Time) error {
	queueWaitMS := assignedAt.Sub(createdAt).Milliseconds()
	if queueWaitMS < 0 {
		queueWaitMS = 0
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO task_execution_metrics (
  task_id, current_stage, bottleneck_source, assigned_at, queue_wait_ms
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (task_id) DO UPDATE
SET current_stage = EXCLUDED.current_stage,
    bottleneck_source = EXCLUDED.bottleneck_source,
    assigned_at = EXCLUDED.assigned_at,
    queue_wait_ms = EXCLUDED.queue_wait_ms,
    failure_stage = '',
    failure_category = ''`,
		taskID,
		tasks.ExecutionStageAssigned,
		tasks.BottleneckSourceQueue,
		assignedAt,
		queueWaitMS,
	)
	return err
}

func selectLeastLoadedAccountTx(ctx context.Context, tx *sql.Tx, accountIDs []int64) (int64, error) {
	var accountID int64
	err := tx.QueryRowContext(ctx, `
WITH locked_accounts AS (
  SELECT id, CASE WHEN max_concurrent <= 0 THEN 3 ELSE max_concurrent END AS max_concurrent
  FROM api_accounts
  WHERE status = 1 AND id = ANY($1::bigint[])
  ORDER BY id ASC
  FOR UPDATE
),
account_load AS (
  SELECT a.id,
         a.max_concurrent,
         COUNT(t.id) FILTER (WHERE t.status = $2) AS running_count
  FROM locked_accounts a
  LEFT JOIN tasks t ON t.account_id = a.id
  GROUP BY a.id, a.max_concurrent
)
SELECT id
FROM account_load
WHERE running_count < max_concurrent
ORDER BY (running_count::numeric / max_concurrent::numeric) ASC, id ASC
LIMIT 1`,
		pq.Array(accountIDs),
		tasks.StatusRunning,
	).Scan(&accountID)
	return accountID, err
}

func normalizeCandidateAccountIDs(accountIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(accountIDs))
	ids := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func normalizeMaxConcurrent(value int) int {
	if value <= 0 {
		return 3
	}
	return value
}

func newTaskLeaseToken(riverJobID int64) (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	random := hex.EncodeToString(bytes[:])
	if riverJobID <= 0 {
		return random, nil
	}
	return fmt.Sprintf("river:%d:%s", riverJobID, random), nil
}

func leaseBelongsToRiverJob(leaseToken string, riverJobID int64) bool {
	if riverJobID <= 0 {
		return false
	}
	return strings.HasPrefix(leaseToken, fmt.Sprintf("river:%d:", riverJobID))
}
