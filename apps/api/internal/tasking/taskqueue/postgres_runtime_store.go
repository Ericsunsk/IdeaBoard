package taskqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/lib/pq"
)

type PostgresTaskRuntimeStore struct {
	db *sql.DB
}

func NewPostgresTaskRuntimeStore(db *sql.DB) *PostgresTaskRuntimeStore {
	return &PostgresTaskRuntimeStore{db: db}
}

func (s *PostgresTaskRuntimeStore) GetAccountLoadInfo(ctx context.Context, accountIDs []int64) (map[int64]*tasks.AccountLoad, error) {
	loads := make(map[int64]*tasks.AccountLoad, len(accountIDs))
	if s == nil || s.db == nil {
		return loads, nil
	}
	ids := normalizeCandidateAccountIDs(accountIDs)
	if len(ids) == 0 {
		return loads, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,
		       a.max_concurrent,
		       COUNT(ta.id) AS running_count
		FROM api_accounts a
		LEFT JOIN task_attempts ta ON ta.account_id = a.id
		  AND ta.status = $2
		WHERE a.deleted_at IS NULL
		  AND a.id = ANY($1::bigint[])
	GROUP BY a.id, a.max_concurrent`,
		pq.Array(ids),
		tasks.TaskAttemptStatusRunning,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID int64
		var maxConcurrent int
		var running int
		if err := rows.Scan(&accountID, &maxConcurrent, &running); err != nil {
			return nil, err
		}
		loads[accountID] = &tasks.AccountLoad{
			CurrentConcurrent: running,
			MaxConcurrent:     accountlimits.NormalizeMaxConcurrent(maxConcurrent),
		}
	}
	return loads, rows.Err()
}

func (s *PostgresTaskRuntimeStore) GetAccountConcurrent(ctx context.Context, accountID int64) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("postgres task runtime store: db not configured")
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
	SELECT COUNT(*)
	FROM tasks
	WHERE account_id = $1 AND status = $2`, accountID, tasks.StatusRunning).Scan(&count)
	return count, err
}

func (s *PostgresTaskRuntimeStore) GetAccountMaxConcurrent(ctx context.Context, accountID int64) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("postgres task runtime store: db not configured")
	}
	var maxConcurrent int
	err := s.db.QueryRowContext(ctx, `
	SELECT max_concurrent
	FROM api_accounts
	WHERE id = $1
	  AND deleted_at IS NULL`, accountID).Scan(&maxConcurrent)
	if err != nil {
		return 0, err
	}
	return accountlimits.NormalizeMaxConcurrent(maxConcurrent), nil
}

func (s *PostgresTaskRuntimeStore) MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres task runtime store: db not configured")
	}
	// Best-effort housekeeping; expired rows are already filtered by expires_at > now().
	if err := s.pruneExpiredTaskCancellations(ctx); err != nil {
		slog.Warn("prune expired task cancellations failed", "err", err)
	}
	expiresAt := time.Now().Add(ttl)
	if ttl <= 0 {
		expiresAt = time.Now().Add(24 * time.Hour)
	}
	_, err := s.db.ExecContext(ctx, `
	INSERT INTO task_cancellations (task_id, expires_at)
	VALUES ($1, $2)
	ON CONFLICT (task_id)
	DO UPDATE SET expires_at = EXCLUDED.expires_at`,
		taskID,
		expiresAt,
	)
	return err
}

func (s *PostgresTaskRuntimeStore) IsCancelled(ctx context.Context, taskID string) (bool, error) {
	if strings.TrimSpace(taskID) == "" {
		return false, nil
	}
	if s == nil || s.db == nil {
		return false, fmt.Errorf("postgres task runtime store: db not configured")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
	SELECT EXISTS (
		SELECT 1
		FROM task_cancellations
		WHERE task_id = $1 AND expires_at > now()
	)`, taskID).Scan(&exists)
	return exists, err
}

func (s *PostgresTaskRuntimeStore) ClearCancelled(ctx context.Context, taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres task runtime store: db not configured")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM task_cancellations WHERE task_id = $1`, taskID)
	return err
}

func (s *PostgresTaskRuntimeStore) pruneExpiredTaskCancellations(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM task_cancellations WHERE expires_at <= now()`)
	return err
}
