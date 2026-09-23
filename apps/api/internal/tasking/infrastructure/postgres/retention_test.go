package postgres

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func insertRetentionTestTask(t testing.TB, ctx context.Context, repo *Repository, taskID string, userID int64, status string, terminalAt *time.Time, errorMsg, cancelReason string) {
	t.Helper()
	var completedAt, cancelledAt any
	if terminalAt != nil {
		completedAt = *terminalAt
		if status == "cancelled" || status == "cancelled_with_results" {
			cancelledAt = *terminalAt
		}
	}
	_, err := repo.db.Exec(ctx, `
INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, status, error_msg, result, cancel_reason, completed_at, cancelled_at)
VALUES ($1, $2, 'retention probe', 'retention probe', '{}', $3, $4, '', $5, $6, $7)`,
		taskID, userID, status, errorMsg, cancelReason, completedAt, cancelledAt,
	)
	if err != nil {
		t.Fatalf("insert retention task %s: %v", taskID, err)
	}
}

func TestPurgeExpiredTerminalTasks(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.OpenWithPool(t)
	repo := NewRepository(db)

	var userID int64
	if err := db.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid) VALUES ('retention-test', 'RTT001') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert retention user: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	old := now.AddDate(0, 0, -10)
	young := now.AddDate(0, 0, -1)

	insertRetentionTestTask(t, ctx, repo, "ret-old-completed", userID, "completed", &old, "", "")
	insertRetentionTestTask(t, ctx, repo, "ret-old-failed", userID, "failed", &old, "boom", "")
	insertRetentionTestTask(t, ctx, repo, "ret-old-cancelled", userID, "cancelled", &old, "", "no longer needed")
	insertRetentionTestTask(t, ctx, repo, "ret-old-cancelled-results", userID, "cancelled_with_results", &old, "", "superseded")
	insertRetentionTestTask(t, ctx, repo, "ret-young-completed", userID, "completed", &young, "", "")
	insertRetentionTestTask(t, ctx, repo, "ret-old-running", userID, "running", nil, "", "")

	for _, seed := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO task_cancellations (task_id, expires_at) VALUES ('ret-old-cancelled', $1)`, []any{old}},
		{`INSERT INTO task_execution_attempts (task_id, attempt_no, execution_token) VALUES ('ret-old-completed', 1, 'retention-token-1')`, nil},
		{`INSERT INTO task_execution_metrics (task_id) VALUES ('ret-old-completed')`, nil},
	} {
		if _, err := repo.db.Exec(ctx, seed.query, seed.args...); err != nil {
			t.Fatalf("seed retention dependent: %v", err)
		}
	}

	stats, err := repo.PurgeExpiredTerminalTasks(ctx, now.AddDate(0, 0, -7), 100)
	if err != nil {
		t.Fatalf("PurgeExpiredTerminalTasks: %v", err)
	}
	if stats.TasksDeleted != 4 {
		t.Fatalf("TasksDeleted = %d, want 4 (old terminal rows only)", stats.TasksDeleted)
	}
	if stats.CancellationsDeleted != 1 {
		t.Fatalf("CancellationsDeleted = %d, want 1", stats.CancellationsDeleted)
	}
	if stats.BackfilledDates < 1 {
		t.Fatalf("BackfilledDates = %d, want >= 1", stats.BackfilledDates)
	}

	for _, taskID := range []string{"ret-old-completed", "ret-old-failed", "ret-old-cancelled", "ret-old-cancelled-results"} {
		var remaining int
		if err := repo.db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE task_id = $1`, taskID).Scan(&remaining); err != nil {
			t.Fatalf("count remaining %s: %v", taskID, err)
		}
		if remaining != 0 {
			t.Fatalf("task %s not purged", taskID)
		}
	}
	for _, taskID := range []string{"ret-young-completed", "ret-old-running"} {
		var remaining int
		if err := repo.db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE task_id = $1`, taskID).Scan(&remaining); err != nil {
			t.Fatalf("count kept %s: %v", taskID, err)
		}
		if remaining != 1 {
			t.Fatalf("task %s should be kept", taskID)
		}
	}

	// Dependents follow tasks via ON DELETE CASCADE; cancellations are explicit.
	for _, check := range []struct {
		name  string
		query string
	}{
		{"attempts", `SELECT COUNT(*) FROM task_execution_attempts WHERE task_id = 'ret-old-completed'`},
		{"metrics", `SELECT COUNT(*) FROM task_execution_metrics WHERE task_id = 'ret-old-completed'`},
		{"cancellations", `SELECT COUNT(*) FROM task_cancellations WHERE task_id = 'ret-old-cancelled'`},
	} {
		var remaining int
		if err := repo.db.QueryRow(ctx, check.query).Scan(&remaining); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if remaining != 0 {
			t.Fatalf("%s rows not cleaned up", check.name)
		}
	}

	var backfilled int
	statDate := old.Format("2006-01-02")
	if err := repo.db.QueryRow(ctx, `SELECT COUNT(*) FROM task_daily_stat_backfills WHERE stat_date = $1::date`, statDate).Scan(&backfilled); err != nil {
		t.Fatalf("count backfill marks: %v", err)
	}
	if backfilled != 1 {
		t.Fatalf("backfill mark for %s missing", statDate)
	}
}

func TestPurgeExpiredTerminalTasksDrainsAcrossPasses(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.OpenWithPool(t)
	repo := NewRepository(db)

	var userID int64
	if err := db.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid) VALUES ('retention-multipass', 'RTM001') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert retention user: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	old := now.AddDate(0, 0, -10)
	for _, taskID := range []string{"ret-multi-a", "ret-multi-b", "ret-multi-c", "ret-multi-d", "ret-multi-e"} {
		insertRetentionTestTask(t, ctx, repo, taskID, userID, "completed", &old, "", "")
	}

	stats, err := repo.PurgeExpiredTerminalTasks(ctx, now.AddDate(0, 0, -7), 2)
	if err != nil {
		t.Fatalf("PurgeExpiredTerminalTasks: %v", err)
	}
	if stats.TasksDeleted != 5 {
		t.Fatalf("TasksDeleted = %d, want 5 drained across passes", stats.TasksDeleted)
	}
	var remaining int
	if err := repo.db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE task_id LIKE 'ret-multi-%'`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("%d multipass tasks not purged", remaining)
	}
}
