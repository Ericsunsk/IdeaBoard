package postgres

import (
	"context"
	"strings"
	"testing"
)

// Temporary R6 reproduction: any insert into task_submission_claims fires
// guard_task_submission_claim(), whose body references dropped tasks(task_id).
func TestTempR6GuardClaimRepro(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(ctx, `INSERT INTO task_submission_claims (user_id, client_action_id, task_id, request_hash)
		VALUES (1, 'act-1', 'task_deadbeef', $1)`, strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("expected guard error, insert succeeded")
	}
	t.Logf("R6 REPRODUCED: insert failed with: %v", err)
}
