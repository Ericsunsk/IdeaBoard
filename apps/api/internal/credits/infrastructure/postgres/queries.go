package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/consumptionread"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/pagination"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetAvailableBalances(
	ctx context.Context,
	pool *pgxpool.Pool,
	billingAccountIDs []int64,
) (map[int64]int64, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits service: %w", err)
	}
	if len(billingAccountIDs) == 0 {
		return map[int64]int64{}, nil
	}
	for _, billingAccountID := range billingAccountIDs {
		if billingAccountID <= 0 {
			return nil, domain.ErrInvalidCommand
		}
	}

	rows, err := pool.Query(ctx, `
SELECT requested.billing_account_id,
        wallets.billing_account_id IS NOT NULL AS wallet_exists,
        available.id IS NOT NULL AS available_exists,
        reserved.id IS NOT NULL AS reserved_exists,
        COALESCE(available.balance, 0)
FROM unnest($1::bigint[]) AS requested(billing_account_id)
LEFT JOIN credit_wallets wallets ON wallets.billing_account_id = requested.billing_account_id
LEFT JOIN credit_accounts available
  ON available.billing_account_id = requested.billing_account_id
 AND available.asset_code = 'credits'
 AND available.account_role = 'user_available'
LEFT JOIN credit_accounts reserved
  ON reserved.billing_account_id = requested.billing_account_id
 AND reserved.asset_code = 'credits'
 AND reserved.account_role = 'user_reserved'
ORDER BY requested.billing_account_id`, billingAccountIDs)
	if err != nil {
		return nil, fmt.Errorf("credits service: read available balances: %w", err)
	}
	defer rows.Close()

	balances := make(map[int64]int64, len(billingAccountIDs))
	for rows.Next() {
		var (
			billingAccountID int64
			walletExists     bool
			availableExists  bool
			reservedExists   bool
			available        int64
		)
		if err := rows.Scan(
			&billingAccountID,
			&walletExists,
			&availableExists,
			&reservedExists,
			&available,
		); err != nil {
			return nil, fmt.Errorf("credits service: scan available balance: %w", err)
		}
		if !walletExists || !availableExists || !reservedExists {
			return nil, fmt.Errorf(
				"%w for user %d (wallet=%t available=%t reserved=%t)",
				domain.ErrWalletBalanceIntegrity,
				billingAccountID,
				walletExists,
				availableExists,
				reservedExists,
			)
		}
		balances[billingAccountID] = available
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("credits service: iterate available balances: %w", err)
	}
	return balances, nil
}

func GetWallet(ctx context.Context, pool *pgxpool.Pool, billingAccountID int64) (domain.WalletDetails, error) {
	if err := database.RequirePool(pool); err != nil {
		return domain.WalletDetails{}, fmt.Errorf("credits service: %w", err)
	}
	if connection, ok := dbutil.ConnectionFromContext(ctx); ok {
		return getWallet(ctx, connection, billingAccountID)
	}
	return getWallet(ctx, pool, billingAccountID)
}

// GetWalletInTx reads the same Wallet projection as GetWallet but on the
// caller's transaction instead of the pool. Borrowed participants (notably
// identity provisioning replays) use it so a wallet read never acquires a
// second pool connection while the caller-owned transaction is held.
func GetWalletInTx(ctx context.Context, tx pgx.Tx, billingAccountID int64) (domain.WalletDetails, error) {
	if tx == nil {
		return domain.WalletDetails{}, fmt.Errorf("credits service: database transaction not configured")
	}
	return getWallet(ctx, tx, billingAccountID)
}

type walletQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getWallet(ctx context.Context, q walletQuerier, billingAccountID int64) (domain.WalletDetails, error) {
	if billingAccountID <= 0 {
		return domain.WalletDetails{}, domain.ErrInvalidCommand
	}
	wallet := domain.WalletDetails{
		WalletBalance:      domain.WalletBalance{BillingAccountID: billingAccountID},
		ActiveRestrictions: make([]domain.WalletRestriction, 0),
	}
	var activeRestrictionsJSON []byte
	err := q.QueryRow(ctx, `
	WITH wallet_balance AS (
	  SELECT wallets.billing_account_id,
	         wallets.status,
	         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available') AS available_credits,
	         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved') AS reserved_credits
	  FROM credit_wallets wallets
	  JOIN credit_accounts accounts ON accounts.billing_account_id = wallets.billing_account_id
	  WHERE wallets.billing_account_id = $1
	  GROUP BY wallets.billing_account_id, wallets.status
	  HAVING COUNT(*) FILTER (WHERE accounts.account_role IN ('user_available', 'user_reserved')) = 2
	)
	SELECT wallet_balance.status,
	       wallet_balance.available_credits,
	       wallet_balance.reserved_credits,
	       COALESCE((
	         SELECT jsonb_agg(
	           jsonb_build_object(
	             'restriction_id', restrictions.restriction_id,
	             'billing_account_id', restrictions.billing_account_id,
	             'type', restrictions.restriction_type,
	             'source', restrictions.source,
	             'related_object', jsonb_build_object(
	               'type', restrictions.related_object_type,
	               'id', restrictions.related_object_id
	             ),
	             'reason', restrictions.reason,
	             'created_by', jsonb_build_object(
	               'type', restrictions.created_by_actor_type,
	               'id', restrictions.created_by_actor_id
	             ),
	             'created_at', restrictions.created_at
	           ) ORDER BY restrictions.created_at, restrictions.restriction_id
	         )
	         FROM credit_wallet_restrictions restrictions
	         WHERE restrictions.billing_account_id = wallet_balance.billing_account_id
	           AND restrictions.released_at IS NULL
	       ), '[]'::jsonb)
	FROM wallet_balance`, billingAccountID).Scan(
		&wallet.Status,
		&wallet.Available,
		&wallet.Reserved,
		&activeRestrictionsJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WalletDetails{}, domain.ErrWalletNotFound
	}
	if err != nil {
		return domain.WalletDetails{}, fmt.Errorf("credits service: read wallet: %w", err)
	}
	if err := json.Unmarshal(activeRestrictionsJSON, &wallet.ActiveRestrictions); err != nil {
		return domain.WalletDetails{}, fmt.Errorf("credits service: decode active Wallet restrictions: %w", err)
	}
	for index := range wallet.ActiveRestrictions {
		wallet.ActiveRestrictions[index].BillingAccountID = wallet.BillingAccountID
	}
	return wallet, nil
}

func ListLedger(ctx context.Context, pool *pgxpool.Pool, input domain.ListLedgerInput) (domain.LedgerPage, error) {
	if err := database.RequirePool(pool); err != nil {
		return domain.LedgerPage{}, fmt.Errorf("credits service: %w", err)
	}
	if input.BillingAccountID <= 0 {
		return domain.LedgerPage{}, domain.ErrInvalidCommand
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := pool.Query(ctx, `
SELECT id,
       transaction_id,
       command_type,
       command_amount,
       actor_type,
       actor_id,
       source,
       reason,
       business_reference_type,
       business_reference_id,
       COALESCE(related_transaction_id, ''),
       available_balance_after,
       reserved_balance_after,
       created_at
FROM credit_transactions
WHERE billing_account_id = $1
  AND ($2::BIGINT <= 0 OR id < $2)
ORDER BY id DESC
LIMIT $3`, input.BillingAccountID, input.BeforeSequence, limit+1)
	if err != nil {
		return domain.LedgerPage{}, fmt.Errorf("credits service: list ledger: %w", err)
	}
	defer rows.Close()
	transactions := make([]domain.LedgerTransaction, 0, limit+1)
	for rows.Next() {
		var transaction domain.LedgerTransaction
		if err := rows.Scan(
			&transaction.SequenceID,
			&transaction.TransactionID,
			&transaction.CommandType,
			&transaction.Amount,
			&transaction.Actor.Type,
			&transaction.Actor.ID,
			&transaction.Source,
			&transaction.Reason,
			&transaction.BusinessReference.Type,
			&transaction.BusinessReference.ID,
			&transaction.RelatedTransactionID,
			&transaction.AvailableBalanceAfter,
			&transaction.ReservedBalanceAfter,
			&transaction.CreatedAt,
		); err != nil {
			return domain.LedgerPage{}, fmt.Errorf("credits service: scan ledger transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return domain.LedgerPage{}, fmt.Errorf("credits service: read ledger transactions: %w", err)
	}
	page := domain.LedgerPage{}
	transactions, page.NextBeforeSequence = pagination.TruncateBeforeID(transactions, limit, func(tx domain.LedgerTransaction) int64 { return tx.SequenceID })
	if err := loadPostings(ctx, pool, transactions); err != nil {
		return domain.LedgerPage{}, err
	}
	if err := loadReferences(ctx, pool, transactions); err != nil {
		return domain.LedgerPage{}, err
	}
	page.Transactions = transactions
	return page, nil
}

func loadReferences(ctx context.Context, pool *pgxpool.Pool, transactions []domain.LedgerTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	transactionIDs, byID := collectReferenceTransactionIDs(transactions)
	holdsByTransactionID, err := loadHoldReferences(ctx, pool, transactionIDs, byID)
	if err != nil {
		return err
	}
	chargesByTransactionID, err := loadChargeReferences(ctx, pool, transactionIDs, byID)
	if err != nil {
		return err
	}
	attachReferenceFallbacks(transactions, holdsByTransactionID, chargesByTransactionID)
	return nil
}

func collectReferenceTransactionIDs(transactions []domain.LedgerTransaction) ([]string, map[string]*domain.LedgerTransaction) {
	transactionIDs := make([]string, 0, len(transactions)*2)
	seenTransactionIDs := make(map[string]struct{}, len(transactions)*2)
	byID := make(map[string]*domain.LedgerTransaction, len(transactions))
	for index := range transactions {
		transactionIDs = append(transactionIDs, transactions[index].TransactionID)
		seenTransactionIDs[transactions[index].TransactionID] = struct{}{}
		if relatedID := transactions[index].RelatedTransactionID; relatedID != "" {
			if _, seen := seenTransactionIDs[relatedID]; !seen {
				transactionIDs = append(transactionIDs, relatedID)
				seenTransactionIDs[relatedID] = struct{}{}
			}
		}
		byID[transactions[index].TransactionID] = &transactions[index]
	}
	return transactionIDs, byID
}

func loadHoldReferences(
	ctx context.Context,
	pool *pgxpool.Pool,
	transactionIDs []string,
	byID map[string]*domain.LedgerTransaction,
) (map[string]*domain.HoldReference, error) {
	holds, err := pool.Query(ctx, `
	SELECT hold_transaction_id, amount, captured_amount, released_amount, status,
	       COALESCE(capture_transaction_id, ''), COALESCE(release_transaction_id, '')
	FROM credit_holds
	WHERE hold_transaction_id = ANY($1::VARCHAR[])
	   OR capture_transaction_id = ANY($1::VARCHAR[])
	   OR release_transaction_id = ANY($1::VARCHAR[])`, transactionIDs)
	if err != nil {
		return nil, fmt.Errorf("credits service: list hold references: %w", err)
	}
	defer holds.Close()
	holdsByTransactionID := make(map[string]*domain.HoldReference, len(byID))
	for holds.Next() {
		var holdTransactionID, captureTransactionID, releaseTransactionID string
		reference := domain.HoldReference{}
		if err := holds.Scan(
			&holdTransactionID,
			&reference.Amount,
			&reference.CapturedAmount,
			&reference.ReleasedAmount,
			&reference.Status,
			&captureTransactionID,
			&releaseTransactionID,
		); err != nil {
			return nil, fmt.Errorf("credits service: scan hold reference: %w", err)
		}
		reference.CaptureTransactionID = captureTransactionID
		reference.ReleaseTransactionID = releaseTransactionID
		holdsByTransactionID[holdTransactionID] = &reference
		holdsByTransactionID[captureTransactionID] = &reference
		holdsByTransactionID[releaseTransactionID] = &reference
		if transaction := byID[holdTransactionID]; transaction != nil {
			transaction.Hold = &reference
		}
		if transaction := byID[captureTransactionID]; transaction != nil {
			transaction.Hold = &reference
		}
		if transaction := byID[releaseTransactionID]; transaction != nil {
			transaction.Hold = &reference
		}
	}
	if err := holds.Err(); err != nil {
		return nil, fmt.Errorf("credits service: read hold references: %w", err)
	}
	return holdsByTransactionID, nil
}

func loadChargeReferences(
	ctx context.Context,
	pool *pgxpool.Pool,
	transactionIDs []string,
	byID map[string]*domain.LedgerTransaction,
) (map[string]*domain.ChargeReference, error) {
	charges, err := pool.Query(ctx, `
SELECT charge.capture_transaction_id, charge.amount, charge.refunded_amount, COALESCE(refund.transaction_id, '')
FROM credit_charges charge
LEFT JOIN credit_transactions refund
  ON refund.related_transaction_id = charge.capture_transaction_id AND refund.command_type = 'refund'
WHERE charge.capture_transaction_id = ANY($1::VARCHAR[])
   OR charge.capture_transaction_id IN (
     SELECT related_transaction_id
     FROM credit_transactions
     WHERE transaction_id = ANY($1::VARCHAR[])
       AND related_transaction_id IS NOT NULL
   )`, transactionIDs)
	if err != nil {
		return nil, fmt.Errorf("credits service: list charge references: %w", err)
	}
	defer charges.Close()
	chargesByTransactionID := make(map[string]*domain.ChargeReference, len(byID))
	for charges.Next() {
		var captureTransactionID, refundTransactionID string
		reference := domain.ChargeReference{}
		if err := charges.Scan(&captureTransactionID, &reference.Amount, &reference.RefundedAmount, &refundTransactionID); err != nil {
			return nil, fmt.Errorf("credits service: scan charge reference: %w", err)
		}
		reference.RemainingRefundable = reference.Amount - reference.RefundedAmount
		chargesByTransactionID[captureTransactionID] = &reference
		chargesByTransactionID[refundTransactionID] = &reference
		if transaction := byID[captureTransactionID]; transaction != nil {
			transaction.Charge = &reference
		}
	}
	if err := charges.Err(); err != nil {
		return nil, fmt.Errorf("credits service: read charge references: %w", err)
	}
	return chargesByTransactionID, nil
}

// A capture/refund can refer to a transaction outside this page. Query keys
// include related IDs and this loop attaches those facts without N+1 reads.
func attachReferenceFallbacks(
	transactions []domain.LedgerTransaction,
	holdsByTransactionID map[string]*domain.HoldReference,
	chargesByTransactionID map[string]*domain.ChargeReference,
) {
	for index := range transactions {
		transaction := &transactions[index]
		if transaction.Hold == nil {
			transaction.Hold = holdsByTransactionID[transaction.TransactionID]
		}
		if transaction.Charge == nil {
			transaction.Charge = chargesByTransactionID[transaction.TransactionID]
		}
		if transaction.RelatedTransactionID != "" {
			if transaction.Hold == nil {
				transaction.Hold = holdsByTransactionID[transaction.RelatedTransactionID]
			}
			if transaction.Charge == nil {
				transaction.Charge = chargesByTransactionID[transaction.RelatedTransactionID]
			}
		}
	}
}

func loadPostings(ctx context.Context, pool *pgxpool.Pool, transactions []domain.LedgerTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	transactionIDs := make([]string, len(transactions))
	byID := make(map[string]*domain.LedgerTransaction, len(transactions))
	for index := range transactions {
		transactionIDs[index] = transactions[index].TransactionID
		byID[transactions[index].TransactionID] = &transactions[index]
	}
	rows, err := pool.Query(ctx, `
SELECT posting.transaction_id, account.account_role, posting.amount
FROM credit_postings posting
JOIN credit_accounts account ON account.id = posting.account_id
WHERE posting.transaction_id = ANY($1::VARCHAR[])
ORDER BY posting.transaction_id, posting.id`, transactionIDs)
	if err != nil {
		return fmt.Errorf("credits service: list postings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var transactionID string
		var posting domain.Posting
		if err := rows.Scan(&transactionID, &posting.AccountRole, &posting.Amount); err != nil {
			return fmt.Errorf("credits service: scan posting: %w", err)
		}
		transaction := byID[transactionID]
		if transaction == nil {
			return fmt.Errorf("credits service: posting references unknown transaction %s", transactionID)
		}
		transaction.Postings = append(transaction.Postings, posting)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("credits service: read postings: %w", err)
	}
	return nil
}

func GetCaptureUsage(
	ctx context.Context,
	pool *pgxpool.Pool,
	billingAccountID int64,
	filter domain.UsageFilter,
	location *time.Location,
) (totalCredits int64, peakDailyCredits int64, firstCaptureDay time.Time, err error) {
	if err := database.RequirePool(pool); err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: %w", err)
	}
	if billingAccountID <= 0 || !filter.Valid() {
		return 0, 0, time.Time{}, domain.ErrInvalidCommand
	}
	if location == nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: usage location is required")
	}

	var rawFirstCaptureDay string
	err = pool.QueryRow(ctx, `
WITH daily_captures AS (
  SELECT (created_at AT TIME ZONE $2)::date AS capture_day,
         SUM(command_amount)::numeric AS amount
  FROM `+consumptionread.Transactions(4)+` AS captures
  WHERE billing_account_id = $1
    AND command_type = 'capture'
    AND ($3::bigint = 0 OR (actor_type = 'user' AND actor_id = $3::text))
  GROUP BY (created_at AT TIME ZONE $2)::date
)
SELECT COALESCE(SUM(amount), 0)::bigint,
       COALESCE(MAX(amount), 0)::bigint,
       COALESCE(MIN(capture_day)::text, '')
FROM daily_captures`, billingAccountID, location.String(), filter.ActorUserID, filter.WorkspaceID).Scan(
		&totalCredits,
		&peakDailyCredits,
		&rawFirstCaptureDay,
	)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: aggregate capture usage: %w", err)
	}
	if rawFirstCaptureDay == "" {
		return totalCredits, peakDailyCredits, time.Time{}, nil
	}
	firstCaptureDay, err = time.ParseInLocation(time.DateOnly, rawFirstCaptureDay, location)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: parse first capture day: %w", err)
	}
	return totalCredits, peakDailyCredits, firstCaptureDay, nil
}

func GetDailyCaptureUsage(
	ctx context.Context,
	pool *pgxpool.Pool,
	billingAccountID int64,
	filter domain.UsageFilter,
	location *time.Location,
	start time.Time,
	endExclusive time.Time,
) (map[string]int64, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits service: %w", err)
	}
	if billingAccountID <= 0 || !filter.Valid() {
		return nil, domain.ErrInvalidCommand
	}
	if location == nil || !endExclusive.After(start) {
		return nil, fmt.Errorf("credits service: valid daily usage range is required")
	}

	rows, err := pool.Query(ctx, `
SELECT ((created_at AT TIME ZONE $2)::date)::text AS capture_day,
       SUM(command_amount)::bigint AS amount
FROM `+consumptionread.Transactions(6)+` AS captures
WHERE billing_account_id = $1
  AND command_type = 'capture'
  AND ($5::bigint = 0 OR (actor_type = 'user' AND actor_id = $5::text))
  AND created_at >= $3
  AND created_at < $4
GROUP BY (created_at AT TIME ZONE $2)::date
ORDER BY capture_day`, billingAccountID, location.String(), start, endExclusive, filter.ActorUserID, filter.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("credits service: query daily capture usage: %w", err)
	}
	defer rows.Close()

	dailyCredits := make(map[string]int64)
	for rows.Next() {
		var date string
		var amount int64
		if err := rows.Scan(&date, &amount); err != nil {
			return nil, fmt.Errorf("credits service: scan daily capture usage: %w", err)
		}
		dailyCredits[date] = amount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("credits service: read daily capture usage: %w", err)
	}
	return dailyCredits, nil
}
