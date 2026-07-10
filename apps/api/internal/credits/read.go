package credits

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetWallet(ctx context.Context, userID int64) (WalletBalance, error) {
	if s == nil || s.pool == nil {
		return WalletBalance{}, fmt.Errorf("credits service: database pool not configured")
	}
	if userID <= 0 {
		return WalletBalance{}, ErrInvalidCommand
	}
	wallet := WalletBalance{UserID: userID}
	err := s.pool.QueryRow(ctx, `
SELECT
  MAX(balance) FILTER (WHERE account_role = 'user_available'),
  MAX(balance) FILTER (WHERE account_role = 'user_reserved')
FROM credit_accounts
WHERE wallet_user_id = $1
HAVING COUNT(*) FILTER (WHERE account_role IN ('user_available', 'user_reserved')) = 2`, userID).Scan(
		&wallet.Available,
		&wallet.Reserved,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WalletBalance{}, ErrWalletNotFound
	}
	if err != nil {
		return WalletBalance{}, fmt.Errorf("credits service: read wallet: %w", err)
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
	page.Transactions = transactions
	return page, nil
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
