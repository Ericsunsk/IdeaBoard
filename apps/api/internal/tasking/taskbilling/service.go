package taskbilling

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
)

const imageWorkflowQuantity int64 = 1

var (
	ErrUnsupportedProduct  = errors.New("task product is not billable")
	ErrPricingUnavailable  = errors.New("task pricing is unavailable")
	ErrInsufficientCredits = errors.New("insufficient credits for task")
	ErrWalletRestricted    = errors.New("task wallet is restricted")
	ErrSettlementConflict  = errors.New("task billing settlement conflict")
	ErrIntegrity           = errors.New("task billing integrity error")
	ErrIdempotencyConflict = errors.New("task billing idempotency conflict")
)

type Quoter interface {
	Quote(context.Context, string, int64) (billing.Quote, error)
	QuoteInTx(context.Context, pgx.Tx, string, int64) (billing.Quote, error)
}

type CreditService interface {
	credits.WalletReader
	InTx(pgx.Tx) credits.Commands
}

type Service struct {
	quoter  Quoter
	credits CreditService
}

// SettlementState is the durable Task Billing state machine persisted beside
// each immutable Quote snapshot.
type SettlementState string

const (
	SettlementStateFree     SettlementState = "free"
	SettlementStateHeld     SettlementState = "held"
	SettlementStateCaptured SettlementState = "captured"
	SettlementStateReleased SettlementState = "released"
)

// TerminalOutcome is the only caller choice exposed by terminal Task Billing.
// It deliberately excludes non-terminal settlement states.
type TerminalOutcome string

const (
	TerminalOutcomeSucceeded    TerminalOutcome = "succeeded"
	TerminalOutcomeUnsuccessful TerminalOutcome = "unsuccessful"
)

type Request struct {
	Feature  string
	Quantity int64
}

type Readiness struct {
	billing.Quote
	AvailableCredits int64 `json:"available_credits"`
	ReservedCredits  int64 `json:"reserved_credits"`
	Sufficient       bool  `json:"sufficient"`
}

type Snapshot struct {
	TaskID            string
	UserID            int64
	Feature           string
	SKU               billing.SKU
	DisplayName       string
	PriceVersionID    int64
	PriceVersion      int64
	UnitCredits       int64
	Quantity          int64
	TotalCredits      int64
	QuotedAt          time.Time
	HoldTransactionID string
}

func NewService(quoter Quoter, creditService CreditService) *Service {
	return &Service{quoter: quoter, credits: creditService}
}

func AIImageRequest(feature string) (Request, error) {
	canonicalFeature, ok := workflow.ParseAIImageFeatureSlug(feature)
	if !ok {
		return Request{}, ErrUnsupportedProduct
	}
	return Request{Feature: canonicalFeature, Quantity: imageWorkflowQuantity}, nil
}

// RequiresBilling identifies task products whose terminal transition must
// never commit without a complete billing snapshot and settlement fact.
func RequiresBilling(feature string) bool {
	return workflow.IsAIImageFeature(feature)
}

func ResolveSKU(request Request) (billing.SKU, error) {
	feature, ok := workflow.ParseAIImageFeatureSlug(request.Feature)
	if !ok {
		return "", ErrUnsupportedProduct
	}
	return billing.SKU("ai-image:" + feature), nil
}

func RequestFingerprint(request Request) (string, error) {
	sku, err := validateRequest(request)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(string(sku) + "\x00" + strconv.FormatInt(request.Quantity, 10)))
	return hex.EncodeToString(digest[:]), nil
}

func validateRequest(request Request) (billing.SKU, error) {
	sku, err := ResolveSKU(request)
	if err != nil {
		return "", err
	}
	if request.Quantity <= 0 {
		return "", billing.ErrInvalidQuantity
	}
	return sku, nil
}

func (s *Service) Readiness(ctx context.Context, userID int64, request Request) (Readiness, error) {
	if s == nil || s.quoter == nil || s.credits == nil || userID <= 0 {
		return Readiness{}, fmt.Errorf("task billing service unavailable")
	}
	sku, err := validateRequest(request)
	if err != nil {
		return Readiness{}, err
	}
	quote, err := s.quoter.Quote(ctx, string(sku), request.Quantity)
	if err != nil {
		return Readiness{}, fmt.Errorf("%w: %w", ErrPricingUnavailable, err)
	}
	wallet, err := s.credits.GetWallet(ctx, userID)
	if err != nil {
		return Readiness{}, err
	}
	if len(wallet.ActiveRestrictions) > 0 {
		return Readiness{}, ErrWalletRestricted
	}
	return Readiness{
		Quote: quote, AvailableCredits: wallet.Available, ReservedCredits: wallet.Reserved,
		Sufficient: wallet.Available >= quote.TotalCredits,
	}, nil
}

func (s *Service) HoldAndPersist(ctx context.Context, tx pgx.Tx, taskID string, userID int64, request Request) (Snapshot, error) {
	if s == nil || s.quoter == nil || s.credits == nil || tx == nil || strings.TrimSpace(taskID) == "" || userID <= 0 {
		return Snapshot{}, fmt.Errorf("task billing service unavailable")
	}
	sku, err := validateRequest(request)
	if err != nil {
		return Snapshot{}, err
	}
	quote, err := s.quoter.QuoteInTx(ctx, tx, string(sku), request.Quantity)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %w", ErrPricingUnavailable, err)
	}

	snapshot := Snapshot{
		TaskID: taskID, UserID: userID, Feature: request.Feature,
		SKU: quote.SKU, DisplayName: quote.DisplayName, PriceVersionID: quote.PriceVersionID, PriceVersion: quote.PriceVersion,
		UnitCredits: quote.UnitCredits, Quantity: quote.Quantity, TotalCredits: quote.TotalCredits, QuotedAt: quote.QuotedAt,
	}
	state := SettlementStateFree
	settledAt := any(quote.QuotedAt)
	var holdTransactionID any
	if quote.TotalCredits > 0 {
		amount, amountErr := credits.NewCreditAmount(quote.TotalCredits)
		if amountErr != nil {
			return Snapshot{}, amountErr
		}
		hold, holdErr := s.credits.InTx(tx).Hold(ctx, credits.HoldCommand{
			UserID:   userID,
			Amount:   amount,
			Metadata: taskCommandMetadata("task-hold:"+taskID, credits.ActorUser, fmt.Sprint(userID), "task_submission", "AI 生图任务 Idea 预授权", taskID),
		})
		if errors.Is(holdErr, credits.ErrInsufficientCredits) {
			return Snapshot{}, ErrInsufficientCredits
		}
		if errors.Is(holdErr, credits.ErrWalletRestricted) {
			return Snapshot{}, ErrWalletRestricted
		}
		if holdErr != nil {
			return Snapshot{}, holdErr
		}
		snapshot.HoldTransactionID = hold.TransactionID
		holdTransactionID = hold.TransactionID
		state = SettlementStateHeld
		settledAt = nil
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, sku, display_name, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at, hold_transaction_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		snapshot.TaskID, snapshot.UserID, snapshot.Feature, snapshot.SKU,
		snapshot.DisplayName, snapshot.PriceVersionID, snapshot.PriceVersion, snapshot.UnitCredits, snapshot.Quantity,
		snapshot.TotalCredits, snapshot.QuotedAt, holdTransactionID,
	); err != nil {
		return Snapshot{}, fmt.Errorf("persist task billing snapshot: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO task_billing_settlements (task_id, state, settled_at)
VALUES ($1, $2, $3)`, snapshot.TaskID, state, settledAt); err != nil {
		return Snapshot{}, fmt.Errorf("persist task billing settlement: %w", err)
	}
	return snapshot, nil
}

func (s *Service) PersistSubmissionClaim(ctx context.Context, tx pgx.Tx, userID int64, clientActionID, taskID string, request Request) error {
	clientActionID = strings.TrimSpace(clientActionID)
	taskID = strings.TrimSpace(taskID)
	if tx == nil || userID <= 0 || clientActionID == "" || taskID == "" {
		return fmt.Errorf("persist task submission claim: invalid input")
	}
	requestHash, err := RequestFingerprint(request)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO task_submission_claims (user_id, client_action_id, task_id, request_hash)
VALUES ($1, $2, $3, $4)`, userID, clientActionID, taskID, requestHash); err != nil {
		return fmt.Errorf("persist task submission claim: %w", err)
	}
	return nil
}

// SettleTerminal composes the Task Billing state machine and Credits command
// into the caller-owned Task terminal transaction.
func SettleTerminal(ctx context.Context, tx *sql.Tx, taskID string, outcome TerminalOutcome) error {
	if tx == nil {
		return fmt.Errorf("task billing transaction unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	target, err := terminalSettlementState(outcome)
	if err != nil {
		return err
	}
	var userID int64
	var totalCredits int64
	var holdTransactionID sql.NullString
	var currentState SettlementState
	err = tx.QueryRowContext(ctx, `
SELECT snapshots.user_id, snapshots.total_credits, snapshots.hold_transaction_id, settlements.state
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements USING (task_id)
WHERE snapshots.task_id = $1
FOR UPDATE OF settlements`, taskID).Scan(&userID, &totalCredits, &holdTransactionID, &currentState)
	if errors.Is(err, sql.ErrNoRows) {
		var snapshotExists bool
		if existsErr := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM task_billing_snapshots WHERE task_id = $1)`, taskID).Scan(&snapshotExists); existsErr != nil {
			return fmt.Errorf("check task billing snapshot: %w", existsErr)
		}
		var feature string
		taskErr := tx.QueryRowContext(ctx, `SELECT feature_slug FROM tasks WHERE task_id = $1`, taskID).Scan(&feature)
		if taskErr != nil && !errors.Is(taskErr, sql.ErrNoRows) {
			return fmt.Errorf("check task billing requirement: %w", taskErr)
		}
		if snapshotExists || RequiresBilling(feature) {
			return fmt.Errorf("%w: task %s has incomplete billing facts", ErrIntegrity, taskID)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load task billing settlement: %w", err)
	}
	if currentState == SettlementStateFree || currentState == target {
		return nil
	}
	if currentState != SettlementStateHeld || totalCredits <= 0 || !holdTransactionID.Valid || strings.TrimSpace(holdTransactionID.String) == "" {
		return ErrSettlementConflict
	}

	commands, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return err
	}
	metadata := taskCommandMetadata("task-"+string(target)+":"+taskID, credits.ActorService, "task-runtime", "task_terminal", "AI 生图任务终态结算", taskID)
	var result credits.CommandResult
	switch target {
	case SettlementStateCaptured:
		result, err = commands.Capture(ctx, credits.CaptureCommand{UserID: userID, HoldTransactionID: holdTransactionID.String, Metadata: metadata})
	case SettlementStateReleased:
		result, err = commands.Release(ctx, credits.ReleaseCommand{UserID: userID, HoldTransactionID: holdTransactionID.String, Metadata: metadata})
	default:
		return fmt.Errorf("invalid task billing settlement target %q", target)
	}
	if err != nil {
		return err
	}
	resultUpdate, err := tx.ExecContext(ctx, `
UPDATE task_billing_settlements
SET state = $2, settlement_transaction_id = $3, settled_at = $4, updated_at = clock_timestamp()
WHERE task_id = $1 AND state = 'held'`, taskID, target, result.TransactionID, result.CreatedAt)
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

func terminalSettlementState(outcome TerminalOutcome) (SettlementState, error) {
	switch outcome {
	case TerminalOutcomeSucceeded:
		return SettlementStateCaptured, nil
	case TerminalOutcomeUnsuccessful:
		return SettlementStateReleased, nil
	default:
		return "", fmt.Errorf("invalid task billing terminal outcome %q", outcome)
	}
}

func taskCommandMetadata(key string, actorType credits.ActorType, actorID, source, reason, taskID string) credits.CommandMetadata {
	return credits.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             credits.Actor{Type: actorType, ID: actorID},
		Source:            source,
		Reason:            reason,
		BusinessReference: credits.BusinessReference{Type: "task", ID: taskID},
	}
}
