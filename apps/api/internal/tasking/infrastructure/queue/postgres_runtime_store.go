package queue

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresTaskRuntimeStore struct {
	db *pgxpool.Pool
}

func NewPostgresTaskRuntimeStore(db *pgxpool.Pool) *PostgresTaskRuntimeStore {
	return &PostgresTaskRuntimeStore{db: db}
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
	_, err := s.db.Exec(ctx, `
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
	err := s.db.QueryRow(ctx, `
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
	_, err := s.db.Exec(ctx, `DELETE FROM task_cancellations WHERE task_id = $1`, taskID)
	return err
}

func (s *PostgresTaskRuntimeStore) pruneExpiredTaskCancellations(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM task_cancellations WHERE expires_at <= now()`)
	return err
}
