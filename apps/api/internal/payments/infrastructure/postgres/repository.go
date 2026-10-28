package postgres

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const userCheckoutLifecycleLockSeed int64 = 412456873631731953

type Repository struct {
	db *pgxpool.Pool
}

type pgxTransactionStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type ContextDB struct {
	ctx                context.Context
	executor           dbsqlc.DBTX
	transactionStarter pgxTransactionStarter
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Available reports whether the repository was constructed with a database
// pool. Aggregate subpackages (adminreads, refunds, disputes) embed
// *Repository and use this check so a missing pool surfaces as an error
// instead of a nil-pool panic.
func (r *Repository) Available() bool {
	return r != nil && r.db != nil
}

func (r *Repository) DBWithContext(ctx context.Context) ContextDB {
	if connection, ok := dbutil.ConnectionFromContext(ctx); ok {
		return ContextDB{ctx: ctx, executor: connection, transactionStarter: connection}
	}
	return ContextDB{ctx: ctx, executor: r.db, transactionStarter: r.db}
}

func (r *Repository) WithUserCheckoutLock(ctx context.Context, userID int64, run func(context.Context) error) (resultErr error) {
	if r == nil || r.db == nil || userID <= 0 || run == nil {
		return fmt.Errorf("payments repository: invalid user Checkout lock")
	}
	lockKey := fmt.Sprintf("payment-user-checkout:%d", userID)
	return WithAdvisoryLock(ctx, r.db, lockKey, userCheckoutLifecycleLockSeed, run)
}

// accountCheckoutLifecycleLockSeed separates the payer-account Checkout
// critical section from the retained per-operator dispute lock (#919).
const accountCheckoutLifecycleLockSeed int64 = 412456873631731954

// WithBillingAccountCheckoutLock serializes Checkout create, continue, and
// abandon per payer Billing Account (#919). Like the user lock, it never
// holds a database transaction across Stripe network calls: callers converge
// provider state inside the lock with short transactions only.
func (r *Repository) WithBillingAccountCheckoutLock(ctx context.Context, billingAccountID int64, run func(context.Context) error) (resultErr error) {
	if r == nil || r.db == nil || billingAccountID <= 0 || run == nil {
		return fmt.Errorf("payments repository: invalid Billing Account Checkout lock")
	}
	lockKey := fmt.Sprintf("payment-account-checkout:%d", billingAccountID)
	return WithAdvisoryLock(ctx, r.db, lockKey, accountCheckoutLifecycleLockSeed, run)
}

func (db ContextDB) Queries() *dbsqlc.Queries {
	return dbsqlc.New(db.executor)
}

// Executor exposes the context-bound query executor (pool, advisory-lock
// connection, or ambient guard transaction) for aggregate subpackages.
func (db ContextDB) Executor() dbsqlc.DBTX {
	return db.executor
}

// BeginTx opens the write transaction for one repository unit, or borrows
// the ambient guard transaction staged in the context (control audit
// atomicity, step 3). The bool reports ownership: true means the caller
// opened the transaction and must commit or roll it back; false means
// the transaction is BORROWED from the guard, which alone commits or
// rolls it back, so the caller must do neither. The borrow-or-begin
// decision and the borrowed handle live in the deep Tx module
// (dbutil.BeginOn): the returned borrowed handle refuses Commit and
// Rollback with dbutil.ErrBorrowedTxLifecycle, so the "never touch a
// borrowed lifecycle" rule is enforced by the module instead of by this
// comment. With no carrier the starter path below runs unchanged
// (including the advisory-lock session starter when one is staged).
func (db ContextDB) BeginTx() (pgx.Tx, bool, error) {
	if _, ok := dbutil.AmbientTxFromContext(db.ctx); !ok && db.transactionStarter == nil {
		return nil, false, fmt.Errorf("payments repository: database unavailable")
	}
	scope, err := dbutil.BeginOn(db.ctx, db.transactionStarter, pgx.TxOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("begin payments transaction: %w", err)
	}
	return scope.Tx(), scope.Owned(), nil
}

// BeginDirectTx opens a write transaction for refund/dispute persistence, or
// reuses the ambient guard transaction staged in the context (control audit
// atomicity, step 3). The bool reports ownership: true means the caller
// opened the transaction and must commit or roll it back; false means the
// transaction is BORROWED from the guard, which alone commits or rolls it
// back, so the caller must do neither. These paths never run under an
// advisory lock, so the fallback matches the previous r.db.BeginTx exactly.
// The admin refund reservation and post-provider settlement use
// BeginCommittedRefundTx instead because Stripe I/O sits between their
// transaction boundaries.
func (r *Repository) BeginDirectTx(ctx context.Context) (pgx.Tx, bool, error) {
	// Borrow-or-begin lives in the deep Tx module (dbutil.BeginOn); the
	// branch shape and every error below stay exactly as before,
	// including the nil-receiver borrow (ambient first, r untouched).
	if _, ok := dbutil.AmbientTxFromContext(ctx); ok {
		scope, err := dbutil.BeginOn(ctx, nil, pgx.TxOptions{})
		if err != nil {
			return nil, false, err
		}
		return scope.Tx(), false, nil
	}
	if r == nil || r.db == nil {
		return nil, false, fmt.Errorf("payments repository: database unavailable")
	}
	scope, err := dbutil.BeginOn(ctx, r.db, pgx.TxOptions{})
	if err != nil {
		return nil, false, err
	}
	return scope.Tx(), true, nil
}

// BeginCommittedRefundTx starts a transaction whose commit is part of the
// refund protocol itself. The administrator refund path performs Stripe I/O
// immediately after reserving Ideas, so the reservation (and its phase audit)
// must be durable before the provider call. It deliberately does not borrow
// the Control guard carrier: the guard transaction remains open until the
// HTTP response is audited, which is too late to establish the provider
// boundary and would let a later audit failure roll back a cash-side effect.
func (r *Repository) BeginCommittedRefundTx(ctx context.Context) (pgx.Tx, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, fmt.Errorf("payments repository: database unavailable")
	}
	// BeginOwned (never borrows): the no-borrow rule above is now enforced
	// by the module, not by avoiding the carrier check.
	scope, err := dbutil.BeginOwned(ctx, r.db, pgx.TxOptions{})
	if err != nil {
		return nil, false, err
	}
	return scope.Tx(), true, nil
}

// commitIfOwned commits an owned transaction and is a no-op for a
// borrowed guard transaction (the guard commits it with the audit row).
func commitIfOwned(ctx context.Context, tx pgx.Tx, owned bool) error {
	if !owned {
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit payments transaction: %w", err)
	}
	return nil
}

// CommitDirectIfOwned commits an owned direct transaction and is a
// no-op for a borrowed guard transaction (the guard commits it with the
// audit row).
func CommitDirectIfOwned(ctx context.Context, tx pgx.Tx, owned bool) error {
	if !owned {
		return nil
	}
	return tx.Commit(ctx)
}

// RollbackDirectIfOwned rolls back an owned direct transaction and is a
// no-op for a borrowed guard transaction (the guard owns its fate).
func RollbackDirectIfOwned(ctx context.Context, tx pgx.Tx, owned bool) {
	if !owned {
		return
	}
	_ = tx.Rollback(ctx)
}

func (db ContextDB) withTx(run func(*dbsqlc.Queries, pgx.Tx) error) (resultErr error) {
	tx, owned, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer func() {
		// Only an owned transaction is cleaned up here: a borrowed
		// guard transaction stays open for the guard, which rolls it
		// back on error and commits it with the audit row on success.
		if !owned {
			return
		}
		if rollbackErr := tx.Rollback(db.ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback payments transaction: %w", rollbackErr))
		}
	}()
	if err := run(dbsqlc.New(tx), tx); err != nil {
		return err
	}
	return commitIfOwned(db.ctx, tx, owned)
}

// WithReadSnapshot runs read-only admin queries inside a repeatable-read
// snapshot. It lives on the root kernel (not in adminreads) because it only
// needs the transaction starter, and both the root adapter and the adminreads
// subpackage share it.
func (db ContextDB) WithReadSnapshot(run func(*dbsqlc.Queries) error) (resultErr error) {
	if db.transactionStarter == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	tx, err := db.transactionStarter.BeginTx(db.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin payments read snapshot: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(db.ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback payments read snapshot: %w", rollbackErr))
		}
	}()
	if err := run(dbsqlc.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(db.ctx); err != nil {
		return fmt.Errorf("commit payments read snapshot: %w", err)
	}
	return nil
}

// ListBillingAccountManagerIDs resolves the server-internal SSE fanout
// audience (#919): the current financial managers of one payer account.
// No auth gate here; the lifecycle fans post-commit hints only to these
// users, so exited members and non-managers never observe the settlement.
func (r *Repository) ListBillingAccountManagerIDs(ctx context.Context, billingAccountID int64) ([]int64, error) {
	if r == nil || r.db == nil || billingAccountID <= 0 {
		return nil, fmt.Errorf("payments repository: invalid Billing Account audience query")
	}
	return r.DBWithContext(ctx).Queries().ListBillingAccountManagerIDs(ctx, billingAccountID)
}

func (r *Repository) ConvergePaidPaymentOrder(ctx context.Context, facts paymentsdomain.PaidCheckoutFacts, creditedAt time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("payments repository: database unavailable")
	}
	order, _, err := r.RecordPaidPaymentOrder(ctx, facts)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, err
	}
	// Replay + already-credited short-circuit lives inside
	// creditLockedPaidOrderInTx; the two phases stay separate tx so a
	// credit failure leaves paid_pending_credit for replay/reconcile.
	return r.CreditPaidPaymentOrder(ctx, order.ID, creditedAt)
}

func (r *Repository) RecordPaidPaymentOrder(ctx context.Context, facts paymentsdomain.PaidCheckoutFacts) (paymentsdomain.PaymentOrder, bool, error) {
	id, err := uuid.Parse(facts.PaymentOrderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	var result paymentsdomain.PaymentOrder
	var replayed bool
	err = r.DBWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ pgx.Tx) error {
		row, err := q.GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
		if err != nil {
			return fmt.Errorf("lock payment order to record paid fact: %w", err)
		}
		recorded, isReplay, err := recordPaidOrderInTx(ctx, q, row, facts)
		if err != nil {
			return err
		}
		result, replayed = recorded, isReplay
		return nil
	})
	return result, replayed, err
}

func recordPaidOrderInTx(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) (paymentsdomain.PaymentOrder, bool, error) {
	order := PaymentOrder(row)
	if err := validatePaidCheckoutFacts(order, facts); err != nil {
		return paymentsdomain.PaymentOrder{}, false, err
	}
	if order.Status == paymentsdomain.PaymentOrderPaidPendingCredit || order.Status == paymentsdomain.PaymentOrderCredited {
		if err := validateRecordedPaidCheckoutFacts(order, facts); err != nil {
			return paymentsdomain.PaymentOrder{}, false, err
		}
		return order, true, nil
	}
	updated, err := q.RecordPaymentOrderPaid(ctx, dbsqlc.RecordPaymentOrderPaidParams{
		ID: row.ID, StripeCustomerID: facts.StripeCustomerID,
		StripeCheckoutSessionID: pgtype.Text{String: facts.StripeSessionID, Valid: true},
		ExpiresAt:               dbutil.TimestamptzFromTime(facts.ExpiresAt),
		StripePaymentIntentID:   facts.StripePaymentIntentID, StripeChargeID: facts.StripeChargeID,
		PaidSubtotalMinor: pgtype.Int8{Int64: facts.PaidSubtotalMinor, Valid: true},
		PaidTaxMinor:      pgtype.Int8{Int64: facts.PaidTaxMinor, Valid: true},
		PaidTotalMinor:    pgtype.Int8{Int64: facts.PaidTotalMinor, Valid: true},
		PaidCurrency:      strings.ToLower(strings.TrimSpace(facts.PaidCurrency)), ReceiptUrl: facts.ReceiptURL,
		PaidAt: pgtype.Timestamptz{Time: facts.PaidAt, Valid: true},
	})
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("record paid payment order: %w", err)
	}
	return PaymentOrder(updated), false, nil
}

func (r *Repository) CreditPaidPaymentOrder(ctx context.Context, orderID string, creditedAt time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	var result paymentsdomain.PaymentConvergenceResult
	err = r.DBWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		row, err := q.GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
		if err != nil {
			return fmt.Errorf("lock paid payment order for credit: %w", err)
		}
		credited, err := creditLockedPaidOrderInTx(ctx, q, tx, PaymentOrder(row), creditedAt)
		if err != nil {
			return err
		}
		result = credited
		return nil
	})
	return result, err
}

func creditLockedPaidOrderInTx(ctx context.Context, q *dbsqlc.Queries, tx pgx.Tx, order paymentsdomain.PaymentOrder, creditedAt time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	if order.Status == paymentsdomain.PaymentOrderCredited {
		var available int64
		if err := tx.QueryRow(ctx, `SELECT available_balance_after FROM credit_transactions WHERE transaction_id = $1`, order.CreditTransactionID).Scan(&available); err != nil {
			return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("read replayed purchase balance: %w", err)
		}
		return paymentsdomain.PaymentConvergenceResult{Order: order, AvailableIdeas: available, Replayed: true}, nil
	}
	if order.Status != paymentsdomain.PaymentOrderPaidPendingCredit {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("payment order is not ready for credit from status %q", order.Status)
	}
	amount, err := domain.NewCreditAmount(order.IdeaQuantity)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("build purchase credit amount: %w", err)
	}
	// Settlement converges on the order's persisted payer account (#919):
	// workspace switches, member exits, and management changes never move
	// the obligation. There is no per-operator fallback; a missing payer
	// fails closed for replay/reconciliation instead of crediting elsewhere.
	if order.BillingAccountID <= 0 {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("credit paid payment order without payer Billing Account")
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, err
	}
	creditResult, err := commands.Purchase(ctx, ports.PurchaseCommand{
		BillingAccountID: order.BillingAccountID, Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "payment-purchase:" + order.ID,
			Actor:          domain.Actor{Type: domain.ActorService, ID: "payments"},
			Source:         "stripe_purchase", Reason: "Stripe payment credited",
			BusinessReference: domain.BusinessReference{Type: "payment_order", ID: order.ID},
		},
	})
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("credit paid payment order: %w", err)
	}
	id, err := uuid.Parse(order.ID)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	updated, err := q.MarkPaymentOrderCredited(ctx, dbsqlc.MarkPaymentOrderCreditedParams{
		ID: pgUUIDToPgType(id), CreditTransactionID: pgtype.Text{String: creditResult.TransactionID, Valid: true},
		CreditedAt: pgtype.Timestamptz{Time: creditedAt, Valid: true},
	})
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("mark payment order credited: %w", err)
	}
	return paymentsdomain.PaymentConvergenceResult{
		Order: PaymentOrder(updated), AvailableIdeas: creditResult.Wallet.Available, Replayed: creditResult.Replayed,
	}, nil
}

func validatePaidCheckoutFacts(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) error {
	if paidCheckoutIdentityConflict(order, facts) {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	if paidCheckoutBindingConflict(order, facts) {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	if invalidPaidCheckoutPayload(facts) {
		return fmt.Errorf("invalid paid Checkout facts")
	}
	return nil
}

func paidCheckoutIdentityConflict(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) bool {
	if facts.StripeAccountID != order.StripeAccountID {
		return true
	}
	if facts.PublicOrderNumber != order.PublicOrderNumber {
		return true
	}
	if facts.PaymentOrderID != order.ID {
		return true
	}
	if facts.RequestHash != order.RequestHash {
		return true
	}
	return facts.IdeaQuantity != order.IdeaQuantity
}

func paidCheckoutBindingConflict(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) bool {
	if order.StripeCheckoutSessionID != "" && facts.StripeSessionID != order.StripeCheckoutSessionID {
		return true
	}
	if order.StripeCustomerID != "" && facts.StripeCustomerID != order.StripeCustomerID {
		return true
	}
	return order.StripeCheckoutSessionID != "" && !facts.ExpiresAt.Equal(order.ExpiresAt)
}

func invalidPaidCheckoutPayload(facts paymentsdomain.PaidCheckoutFacts) bool {
	currency := strings.ToLower(strings.TrimSpace(facts.PaidCurrency))
	if strings.TrimSpace(facts.StripePaymentIntentID) == "" {
		return true
	}
	if facts.PaidSubtotalMinor < 0 || facts.PaidTaxMinor < 0 {
		return true
	}
	if facts.PaidTotalMinor <= 0 || facts.PaidSubtotalMinor+facts.PaidTaxMinor != facts.PaidTotalMinor {
		return true
	}
	if len(currency) != 3 || facts.PaidAt.IsZero() || facts.ExpiresAt.IsZero() {
		return true
	}
	return strings.TrimSpace(facts.StripeCustomerID) == "" || strings.TrimSpace(facts.StripeSessionID) == ""
}

func validateRecordedPaidCheckoutFacts(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) error {
	if order.StripePaymentIntentID != facts.StripePaymentIntentID || order.StripeChargeID != facts.StripeChargeID ||
		order.PaidSubtotalMinor == nil || *order.PaidSubtotalMinor != facts.PaidSubtotalMinor ||
		order.PaidTaxMinor == nil || *order.PaidTaxMinor != facts.PaidTaxMinor ||
		order.PaidTotalMinor == nil || *order.PaidTotalMinor != facts.PaidTotalMinor ||
		order.PaidCurrency != strings.ToLower(strings.TrimSpace(facts.PaidCurrency)) || order.ReceiptURL != facts.ReceiptURL ||
		!order.ExpiresAt.Equal(facts.ExpiresAt) {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	return nil
}
