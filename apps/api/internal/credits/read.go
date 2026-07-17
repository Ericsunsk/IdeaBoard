package credits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

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

	holds, err := s.pool.Query(ctx, `
SELECT hold_transaction_id, amount, status, COALESCE(terminal_transaction_id, '')
FROM credit_holds
WHERE hold_transaction_id = ANY($1::VARCHAR[]) OR terminal_transaction_id = ANY($1::VARCHAR[])`, transactionIDs)
	if err != nil {
		return fmt.Errorf("credits service: list hold references: %w", err)
	}
	defer holds.Close()
	holdsByTransactionID := make(map[string]*HoldReference, len(transactions))
	for holds.Next() {
		var holdTransactionID, terminalTransactionID string
		reference := HoldReference{}
		if err := holds.Scan(&holdTransactionID, &reference.Amount, &reference.Status, &terminalTransactionID); err != nil {
			return fmt.Errorf("credits service: scan hold reference: %w", err)
		}
		reference.TerminalTransactionID = terminalTransactionID
		holdsByTransactionID[holdTransactionID] = &reference
		holdsByTransactionID[terminalTransactionID] = &reference
		if transaction := byID[holdTransactionID]; transaction != nil {
			transaction.Hold = &reference
		}
		if transaction := byID[terminalTransactionID]; transaction != nil {
			transaction.Hold = &reference
		}
	}
	if err := holds.Err(); err != nil {
		return fmt.Errorf("credits service: read hold references: %w", err)
	}

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
		return fmt.Errorf("credits service: list charge references: %w", err)
	}
	defer charges.Close()
	chargesByTransactionID := make(map[string]*ChargeReference, len(transactions))
	for charges.Next() {
		var captureTransactionID, refundTransactionID string
		reference := ChargeReference{}
		if err := charges.Scan(&captureTransactionID, &reference.Amount, &reference.RefundedAmount, &refundTransactionID); err != nil {
			return fmt.Errorf("credits service: scan charge reference: %w", err)
		}
		reference.RemainingRefundable = reference.Amount - reference.RefundedAmount
		chargesByTransactionID[captureTransactionID] = &reference
		chargesByTransactionID[refundTransactionID] = &reference
		if transaction := byID[captureTransactionID]; transaction != nil {
			transaction.Charge = &reference
		}
	}
	if err := charges.Err(); err != nil {
		return fmt.Errorf("credits service: read charge references: %w", err)
	}

	// A capture/refund can refer to a transaction outside this page. Query keys
	// include related IDs and this loop attaches those facts without N+1 reads.
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
	return nil
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
