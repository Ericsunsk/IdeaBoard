package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	creditports "ideaboard/api/internal/credits/ports"
	taskbilling "ideaboard/api/internal/tasking/ports/billing"
	"ideaboard/api/internal/workflow"
)

// settleTerminal applies the durable Task Billing state transition inside the
// caller-owned SQL transaction. Keeping this recipe in the PostgreSQL adapter
// preserves the atomic lock and commit boundary with Task terminal facts.
func settleTerminal(ctx context.Context, tx *sql.Tx, taskID string) error {
	if tx == nil {
		return fmt.Errorf("task billing transaction unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	facts, complete, err := loadSettlementFacts(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if !complete {
		return nil
	}
	decision, err := decideSettlement(facts)
	if err != nil {
		return err
	}
	if decision.noop {
		return nil
	}
	transactions, err := applySettlementCommands(ctx, tx, taskID, facts, decision)
	if err != nil {
		return err
	}
	return persistSettlement(ctx, tx, taskID, decision, transactions)
}

type settlementFacts struct {
	userID             int64
	unitCredits        int64
	quantity           int64
	totalCredits       int64
	holdTransactionID  sql.NullString
	currentState       taskbilling.SettlementState
	settledOutputCount int64
	actualOutputCount  int64
}

func loadSettlementFacts(ctx context.Context, tx *sql.Tx, taskID string) (settlementFacts, bool, error) {
	var facts settlementFacts
	err := tx.QueryRowContext(ctx, `
SELECT snapshots.user_id, snapshots.unit_credits, snapshots.quantity, snapshots.total_credits,
       snapshots.hold_transaction_id, settlements.state, settlements.actual_output_count
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements USING (task_id)
WHERE snapshots.task_id = $1
	FOR UPDATE OF settlements`, taskID).Scan(&facts.userID, &facts.unitCredits, &facts.quantity, &facts.totalCredits, &facts.holdTransactionID, &facts.currentState, &facts.settledOutputCount)
	if errors.Is(err, sql.ErrNoRows) {
		var snapshotExists bool
		if existsErr := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM task_billing_snapshots WHERE task_id = $1)`, taskID).Scan(&snapshotExists); existsErr != nil {
			return settlementFacts{}, false, fmt.Errorf("check task billing snapshot: %w", existsErr)
		}
		var feature string
		taskErr := tx.QueryRowContext(ctx, `SELECT feature_slug FROM tasks WHERE task_id = $1`, taskID).Scan(&feature)
		if taskErr != nil && !errors.Is(taskErr, sql.ErrNoRows) {
			return settlementFacts{}, false, fmt.Errorf("check task billing requirement: %w", taskErr)
		}
		if snapshotExists || workflow.IsAIImageFeature(feature) {
			return settlementFacts{}, false, fmt.Errorf("%w: task %s has incomplete billing facts", taskbilling.ErrIntegrity, taskID)
		}
		return settlementFacts{}, false, nil
	}
	if err != nil {
		return settlementFacts{}, false, fmt.Errorf("load task billing settlement: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM task_outputs WHERE task_id = $1`, taskID).Scan(&facts.actualOutputCount); err != nil {
		return settlementFacts{}, false, fmt.Errorf("count persisted task outputs: %w", err)
	}
	return facts, true, nil
}

type settlementDecision struct {
	target            taskbilling.SettlementState
	captureAmount     int64
	releaseAmount     int64
	actualOutputCount int64
	noop              bool
}

func decideSettlement(facts settlementFacts) (settlementDecision, error) {
	if facts.actualOutputCount < 0 || facts.actualOutputCount > facts.quantity {
		return settlementDecision{}, fmt.Errorf("actual output count %d exceeds requested quantity %d", facts.actualOutputCount, facts.quantity)
	}
	target := taskbilling.SettlementStateReleased
	if facts.actualOutputCount > 0 {
		target = taskbilling.SettlementStateCaptured
	}
	if facts.currentState == taskbilling.SettlementStateFree {
		return settlementDecision{noop: true}, nil
	}
	if facts.currentState == target {
		if facts.settledOutputCount == facts.actualOutputCount {
			return settlementDecision{noop: true}, nil
		}
		return settlementDecision{}, taskbilling.ErrSettlementConflict
	}
	if facts.currentState != taskbilling.SettlementStateHeld || facts.totalCredits <= 0 || !facts.holdTransactionID.Valid || strings.TrimSpace(facts.holdTransactionID.String) == "" {
		return settlementDecision{}, taskbilling.ErrSettlementConflict
	}
	return settlementDecision{
		target: target, captureAmount: facts.actualOutputCount * facts.unitCredits,
		releaseAmount:     facts.totalCredits - facts.actualOutputCount*facts.unitCredits,
		actualOutputCount: facts.actualOutputCount,
	}, nil
}

type settlementTransactions struct {
	capture   any
	release   any
	settledAt time.Time
}

func applySettlementCommands(ctx context.Context, tx *sql.Tx, taskID string, facts settlementFacts, decision settlementDecision) (settlementTransactions, error) {
	commands, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return settlementTransactions{}, err
	}
	var transactions settlementTransactions
	if decision.releaseAmount > 0 {
		amount, amountErr := domain.NewCreditAmount(decision.releaseAmount)
		if amountErr != nil {
			return settlementTransactions{}, amountErr
		}
		result, releaseErr := commands.Release(ctx, creditports.ReleaseCommand{
			UserID: facts.userID, HoldTransactionID: facts.holdTransactionID.String, Amount: amount,
			Metadata: settlementCommandMetadata("task-release:"+taskID, domain.ActorService, "task-runtime", "task_terminal", "AI 生图任务未产出部分释放", taskID),
		})
		if releaseErr != nil {
			return settlementTransactions{}, releaseErr
		}
		transactions.release = result.TransactionID
		transactions.settledAt = result.CreatedAt
	}
	if decision.captureAmount > 0 {
		amount, amountErr := domain.NewCreditAmount(decision.captureAmount)
		if amountErr != nil {
			return settlementTransactions{}, amountErr
		}
		result, captureErr := commands.Capture(ctx, creditports.CaptureCommand{
			UserID: facts.userID, HoldTransactionID: facts.holdTransactionID.String, Amount: amount,
			Metadata: settlementCommandMetadata("task-capture:"+taskID, domain.ActorService, "task-runtime", "task_terminal", "AI 生图任务实际产出结算", taskID),
		})
		if captureErr != nil {
			return settlementTransactions{}, captureErr
		}
		transactions.capture = result.TransactionID
		transactions.settledAt = result.CreatedAt
	}
	return transactions, nil
}

func persistSettlement(ctx context.Context, tx *sql.Tx, taskID string, decision settlementDecision, transactions settlementTransactions) error {
	resultUpdate, err := tx.ExecContext(ctx, `
UPDATE task_billing_settlements
SET state = $2, actual_output_count = $3, captured_credits = $4, released_credits = $5,
    capture_transaction_id = $6, release_transaction_id = $7, settled_at = $8, updated_at = clock_timestamp()
WHERE task_id = $1 AND state = 'held'`, taskID, decision.target, decision.actualOutputCount, decision.captureAmount, decision.releaseAmount,
		transactions.capture, transactions.release, transactions.settledAt)
	if err != nil {
		return fmt.Errorf("persist task billing settlement: %w", err)
	}
	rows, err := resultUpdate.RowsAffected()
	if err != nil {
		return fmt.Errorf("read task billing settlement affected rows: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("persist task billing settlement affected %d rows", rows)
	}
	return nil
}

func settlementCommandMetadata(key string, actorType domain.ActorType, actorID, source, reason, taskID string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             domain.Actor{Type: actorType, ID: actorID},
		Source:            source,
		Reason:            reason,
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}
