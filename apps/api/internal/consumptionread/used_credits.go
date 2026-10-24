package consumptionread

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Queryer executes the projection on a pool or the caller's settlement
// transaction. Using the transaction includes its own uncommitted reservations.
// pgx.Tx and *pgxpool.Pool both satisfy it structurally; there are no
// custom implementations to keep in sync.
type Queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// UsedCreditsForWorkspace reads one Workspace's pending Holds and settled
// actuals net of linked refunds. The optional bounds select the original
// reservation time in [start, end), never the settlement or refund time.
// A nonpositive Workspace ID fails closed; there is no unscoped read.
func UsedCreditsForWorkspace(ctx context.Context, q Queryer, workspaceID int64, start, end *time.Time) (int64, error) {
	if workspaceID <= 0 {
		return 0, errors.New("consumptionread: Workspace ID must be positive")
	}
	return readUsedCredits(ctx, q, workspaceUsedCreditsSQL, start, end, workspaceID)
}

// UsedCreditsForMember reads one User's usage across all Workspaces on the
// persisted Billing Account. Both IDs are required; live ownership and current
// Workspace links do not move historical consumption between accounts.
func UsedCreditsForMember(ctx context.Context, q Queryer, accountID, userID int64, start, end *time.Time) (int64, error) {
	if accountID <= 0 || userID <= 0 {
		return 0, errors.New("consumptionread: Billing Account and User IDs must be positive")
	}
	return readUsedCredits(ctx, q, memberUsedCreditsSQL, start, end, accountID, userID)
}

// UsedCreditsForMembers reads many Users' usage across all Workspaces on
// one persisted Billing Account in a SINGLE statement (GROUP BY user_id),
// replacing per-member UsedCreditsForMember round trips (quota list
// normalization, billing overview fan-out). Unknown or absent users are
// omitted from the map (callers treat missing as zero); an empty user list
// returns an empty map without touching the database.
func UsedCreditsForMembers(ctx context.Context, q Queryer, accountID int64, userIDs []int64, start, end *time.Time) (map[int64]int64, error) {
	out := make(map[int64]int64, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	deduped := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if accountID <= 0 {
		return out, errors.New("consumptionread: Billing Account ID must be positive")
	}
	if len(deduped) == 0 {
		return out, nil
	}
	if start != nil && end != nil && !start.Before(*end) {
		return out, errors.New("consumptionread: period start must precede end")
	}
	rows, err := q.Query(ctx, memberUsedCreditsBatchSQL, start, end, accountID, deduped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, used int64
		if err := rows.Scan(&userID, &used); err != nil {
			return nil, err
		}
		out[userID] = used
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func readUsedCredits(ctx context.Context, q Queryer, sql string, start, end *time.Time, ids ...any) (int64, error) {
	if start != nil && end != nil && !start.Before(*end) {
		return 0, errors.New("consumptionread: period start must precede end")
	}
	args := append([]any{start, end}, ids...)
	var total int64
	err := q.QueryRow(ctx, sql, args...).Scan(&total)
	return total, err
}

// Scope predicates are fixed here, before aggregation, so PostgreSQL can
// select their indexes. Every shape shares the same reservation projection.
var (
	workspaceUsedCreditsSQL = "SELECT COALESCE(SUM(used), 0) FROM (" + fmt.Sprintf(usedCreditsRowsSQL,
		"snap.workspace_id = $3", "op.workspace_id = $3") + ") usage"
	memberUsedCreditsSQL = "SELECT COALESCE(SUM(used), 0) FROM (" + fmt.Sprintf(usedCreditsRowsSQL,
		"snap.billing_account_id = $3 AND snap.user_id = $4", "op.billing_account_id = $3 AND op.user_id = $4") + ") usage"
	memberUsedCreditsBatchSQL = "SELECT user_id, COALESCE(SUM(used), 0) FROM (" + fmt.Sprintf(usedCreditsRowsSQL,
		"snap.billing_account_id = $3 AND snap.user_id = ANY($4::bigint[])", "op.billing_account_id = $3 AND op.user_id = ANY($4::bigint[])") + ") usage GROUP BY user_id"
)

// One statement is essential: independent Capture commits must never fall
// between separate pending and actual reads. This projection intentionally
// differs from Transactions: it includes reservations and attributes linked
// refunds to their original reservation window rather than cash-flow time.
const usedCreditsRowsSQL = `
  SELECT snap.user_id AS user_id, CASE s.state
           WHEN 'captured' THEN s.captured_credits - COALESCE(ch.refunded_amount, 0)
           ELSE snap.total_credits
         END AS used
  FROM task_billing_snapshots snap
  JOIN task_billing_settlements s ON s.task_id = snap.task_id
  LEFT JOIN credit_charges ch ON ch.capture_transaction_id = s.capture_transaction_id
    AND ch.billing_account_id = snap.billing_account_id
  WHERE (s.state = 'captured' OR (s.state = 'held' AND snap.total_credits > 0))
    AND %s
    AND ($1::timestamptz IS NULL OR snap.quoted_at >= $1)
    AND ($2::timestamptz IS NULL OR snap.quoted_at < $2)
  UNION ALL
  SELECT op.user_id AS user_id, CASE op.status
           WHEN 'succeeded' THEN op.unit_credits - COALESCE(ch.refunded_amount, 0)
           ELSE op.unit_credits
         END AS used
  FROM ai_search_operations op
  LEFT JOIN LATERAL (
    SELECT ch.refunded_amount
    FROM credit_transactions cap
    JOIN credit_charges ch ON ch.capture_transaction_id = cap.transaction_id
      AND ch.billing_account_id = op.billing_account_id
    WHERE cap.business_reference_type = 'ai_search_operation'
      AND cap.business_reference_id = op.operation_id::text
      AND cap.billing_account_id = op.billing_account_id
      AND cap.command_type = 'capture'
    ORDER BY cap.id DESC
    LIMIT 1
  ) ch ON op.status = 'succeeded'
  WHERE op.status IN ('pending', 'reconciliation_pending', 'succeeded')
    AND %s
    AND ($1::timestamptz IS NULL OR op.created_at >= $1)
    AND ($2::timestamptz IS NULL OR op.created_at < $2)
`
