package credits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetAvailableBalances(
	ctx context.Context,
	userIDs []int64,
) (map[int64]int64, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("credits service: database pool not configured")
	}
	if len(userIDs) == 0 {
		return map[int64]int64{}, nil
	}
	for _, userID := range userIDs {
		if userID <= 0 {
			return nil, ErrInvalidCommand
		}
	}

	rows, err := s.pool.Query(ctx, `
SELECT requested.user_id,
       wallets.user_id IS NOT NULL AS wallet_exists,
       available.id IS NOT NULL AS available_exists,
       reserved.id IS NOT NULL AS reserved_exists,
       COALESCE(available.balance, 0)
FROM unnest($1::bigint[]) AS requested(user_id)
LEFT JOIN credit_wallets wallets ON wallets.user_id = requested.user_id
LEFT JOIN credit_accounts available
  ON available.wallet_user_id = requested.user_id
 AND available.asset_code = 'credits'
 AND available.account_role = 'user_available'
LEFT JOIN credit_accounts reserved
  ON reserved.wallet_user_id = requested.user_id
 AND reserved.asset_code = 'credits'
 AND reserved.account_role = 'user_reserved'
ORDER BY requested.user_id`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("credits service: read available balances: %w", err)
	}
	defer rows.Close()

	balances := make(map[int64]int64, len(userIDs))
	for rows.Next() {
		var (
			userID          int64
			walletExists    bool
			availableExists bool
			reservedExists  bool
			available       int64
		)
		if err := rows.Scan(
			&userID,
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
				ErrWalletBalanceIntegrity,
				userID,
				walletExists,
				availableExists,
				reservedExists,
			)
		}
		balances[userID] = available
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("credits service: iterate available balances: %w", err)
	}
	return balances, nil
}

func (s *Service) GetWallet(ctx context.Context, userID int64) (WalletDetails, error) {
	if s == nil || s.pool == nil {
		return WalletDetails{}, fmt.Errorf("credits service: database pool not configured")
	}
	if userID <= 0 {
		return WalletDetails{}, ErrInvalidCommand
	}
	wallet := WalletDetails{
		WalletBalance:      WalletBalance{UserID: userID},
		ActiveRestrictions: make([]WalletRestriction, 0),
	}
	var activeRestrictionsJSON []byte
	err := s.pool.QueryRow(ctx, `
	WITH wallet_balance AS (
	  SELECT wallets.user_id,
	         wallets.status,
	         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available') AS available_credits,
	         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved') AS reserved_credits
	  FROM credit_wallets wallets
	  JOIN credit_accounts accounts ON accounts.wallet_user_id = wallets.user_id
	  WHERE wallets.user_id = $1
	  GROUP BY wallets.user_id, wallets.status
	  HAVING COUNT(*) FILTER (WHERE accounts.account_role IN ('user_available', 'user_reserved')) = 2
	)
	SELECT wallet_balance.status,
	       wallet_balance.available_credits,
	       wallet_balance.reserved_credits,
	       COALESCE((
	         SELECT jsonb_agg(
	           jsonb_build_object(
	             'restriction_id', restrictions.restriction_id,
	             'user_id', restrictions.wallet_user_id,
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
	         WHERE restrictions.wallet_user_id = wallet_balance.user_id
	           AND restrictions.released_at IS NULL
	       ), '[]'::jsonb)
	FROM wallet_balance`, userID).Scan(
		&wallet.Status,
		&wallet.Available,
		&wallet.Reserved,
		&activeRestrictionsJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WalletDetails{}, ErrWalletNotFound
	}
	if err != nil {
		return WalletDetails{}, fmt.Errorf("credits service: read wallet: %w", err)
	}
	if err := json.Unmarshal(activeRestrictionsJSON, &wallet.ActiveRestrictions); err != nil {
		return WalletDetails{}, fmt.Errorf("credits service: decode active Wallet restrictions: %w", err)
	}
	return wallet, nil
}

func (s *Service) ListLedger(ctx context.Context, input ListLedgerInput) (LedgerPage, error) {
	if s == nil || s.pool == nil {
		return LedgerPage{}, fmt.Errorf("credits service: database pool not configured")
	}
	if input.UserID <= 0 {
		return LedgerPage{}, ErrInvalidCommand
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
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
WHERE wallet_user_id = $1
  AND ($2::BIGINT <= 0 OR id < $2)
ORDER BY id DESC
LIMIT $3`, input.UserID, input.BeforeSequence, limit+1)
	if err != nil {
		return LedgerPage{}, fmt.Errorf("credits service: list ledger: %w", err)
	}
	defer rows.Close()
	transactions := make([]LedgerTransaction, 0, limit+1)
	for rows.Next() {
		var transaction LedgerTransaction
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
			return LedgerPage{}, fmt.Errorf("credits service: scan ledger transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return LedgerPage{}, fmt.Errorf("credits service: read ledger transactions: %w", err)
	}
	page := LedgerPage{}
	if len(transactions) > limit {
		page.NextBeforeSequence = transactions[limit-1].SequenceID
		transactions = transactions[:limit]
	}
	if err := s.loadPostings(ctx, transactions); err != nil {
		return LedgerPage{}, err
	}
	if err := s.loadReferences(ctx, transactions); err != nil {
		return LedgerPage{}, err
	}
	page.Transactions = transactions
	return page, nil
}

func (s *Service) loadReferences(ctx context.Context, transactions []LedgerTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	transactionIDs, byID := collectReferenceTransactionIDs(transactions)
	holdsByTransactionID, err := s.loadHoldReferences(ctx, transactionIDs, byID)
	if err != nil {
		return err
	}
	chargesByTransactionID, err := s.loadChargeReferences(ctx, transactionIDs, byID)
	if err != nil {
		return err
	}
	attachReferenceFallbacks(transactions, holdsByTransactionID, chargesByTransactionID)
	return nil
}

func collectReferenceTransactionIDs(transactions []LedgerTransaction) ([]string, map[string]*LedgerTransaction) {
	transactionIDs := make([]string, 0, len(transactions)*2)
	seenTransactionIDs := make(map[string]struct{}, len(transactions)*2)
	byID := make(map[string]*LedgerTransaction, len(transactions))
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

func (s *Service) loadHoldReferences(
	ctx context.Context,
	transactionIDs []string,
	byID map[string]*LedgerTransaction,
) (map[string]*HoldReference, error) {
	holds, err := s.pool.Query(ctx, `
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
	holdsByTransactionID := make(map[string]*HoldReference, len(byID))
	for holds.Next() {
		var holdTransactionID, captureTransactionID, releaseTransactionID string
		reference := HoldReference{}
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

func (s *Service) loadChargeReferences(
	ctx context.Context,
	transactionIDs []string,
	byID map[string]*LedgerTransaction,
) (map[string]*ChargeReference, error) {
	charges, err := s.pool.Query(ctx, `
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
	chargesByTransactionID := make(map[string]*ChargeReference, len(byID))
	for charges.Next() {
		var captureTransactionID, refundTransactionID string
		reference := ChargeReference{}
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
	transactions []LedgerTransaction,
	holdsByTransactionID map[string]*HoldReference,
	chargesByTransactionID map[string]*ChargeReference,
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

func (s *Service) loadPostings(ctx context.Context, transactions []LedgerTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	transactionIDs := make([]string, len(transactions))
	byID := make(map[string]*LedgerTransaction, len(transactions))
	for index := range transactions {
		transactionIDs[index] = transactions[index].TransactionID
		byID[transactions[index].TransactionID] = &transactions[index]
	}
	rows, err := s.pool.Query(ctx, `
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
		var posting Posting
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
