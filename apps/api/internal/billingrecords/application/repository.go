package application

import (
	"context"
	"fmt"
	"time"

	billingrecordsdomain "ideaboard/api/internal/billingrecords/domain"
	billingrecordsports "ideaboard/api/internal/billingrecords/ports"
	"ideaboard/api/internal/consumptionread"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var listRecordsSQL = `
SELECT transactions.id,
       transactions.transaction_id,
       transactions.command_type,
       transactions.command_amount,
       transactions.actor_type,
       transactions.source,
       transactions.business_reference_type,
       CASE WHEN $4::bigint = 0 AND $5::bigint = 0 THEN transactions.available_balance_after ELSE 0 END,
       transactions.created_at,
       transactions.consumption_display_name,
       original.command_type
FROM ` + consumptionread.Transactions(5) + ` AS transactions
LEFT JOIN credit_transactions original
  ON transactions.command_type = 'correction'
 AND original.transaction_id = transactions.related_transaction_id
 AND original.billing_account_id = transactions.billing_account_id
WHERE transactions.billing_account_id = $1
  AND ($2::BIGINT <= 0 OR transactions.id < $2)
  AND transactions.command_type IN ('grant', 'deduction', 'capture', 'correction')
  AND transactions.command_amount > 0
  AND (($4::bigint = 0 AND $5::bigint = 0) OR transactions.command_type = 'capture')
  AND ($4::bigint = 0 OR (transactions.actor_type = 'user' AND transactions.actor_id = $4::text))
  AND (
    transactions.command_type <> 'correction'
    OR original.command_type IN ('grant', 'deduction')
  )
ORDER BY transactions.id DESC
LIMIT $3`

type Repository struct {
	pool *pgxpool.Pool
}

type repositoryQueryer struct {
	ctx  context.Context
	pool *pgxpool.Pool
}

type ledgerRecord struct {
	sequence              int64
	recordID              string
	commandType           billingrecordsdomain.LedgerCommand
	amount                int64
	actorType             billingrecordsdomain.LedgerActorType
	source                billingrecordsdomain.LedgerSource
	businessReferenceType billingrecordsdomain.LedgerReferenceType
	availableAfter        int64
	occurredAt            time.Time
	snapshotDisplayName   pgtype.Text
	originalCommandType   billingrecordsdomain.NullableLedgerCommand
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, pool: r.pool}
}

func (q repositoryQueryer) Query(query string, args ...any) (pgx.Rows, error) {
	return q.pool.Query(q.ctx, query, args...)
}

func (r *Repository) List(ctx context.Context, input billingrecordsports.ListInput, fetchLimit int) ([]ledgerRecord, error) {
	if r == nil || r.pool == nil {
		return nil, billingrecordsports.ErrPoolNotConfigured
	}
	rows, err := r.dbWithContext(ctx).Query(listRecordsSQL, input.BillingAccountID, input.BeforeSequence, fetchLimit, input.ActorUserID, input.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("billing records repository: list records: %w", err)
	}
	defer rows.Close()

	records := make([]ledgerRecord, 0, fetchLimit)
	for rows.Next() {
		var record ledgerRecord
		var commandType, actorType, source, businessReferenceType string
		var originalCommandType pgtype.Text
		if err := rows.Scan(
			&record.sequence,
			&record.recordID,
			&commandType,
			&record.amount,
			&actorType,
			&source,
			&businessReferenceType,
			&record.availableAfter,
			&record.occurredAt,
			&record.snapshotDisplayName,
			&originalCommandType,
		); err != nil {
			return nil, fmt.Errorf("billing records repository: scan record: %w", err)
		}
		record.commandType = billingrecordsdomain.LedgerCommand(commandType)
		record.actorType = billingrecordsdomain.LedgerActorType(actorType)
		record.source = billingrecordsdomain.LedgerSource(source)
		record.businessReferenceType = billingrecordsdomain.LedgerReferenceType(businessReferenceType)
		record.originalCommandType = billingrecordsdomain.NullableLedgerCommand{
			Value: billingrecordsdomain.LedgerCommand(originalCommandType.String),
			Valid: originalCommandType.Valid,
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billing records repository: read records: %w", err)
	}
	return records, nil
}
