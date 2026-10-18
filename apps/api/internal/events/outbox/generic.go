package outbox

// Shared outbox primitives (P0-3): the task-status and board-access outboxes
// are the same durable-pending queue over different fact tables. Only the
// table name, the pending-row projection/scan, and the delivery sinks differ.
// Everything else (limit normalization, delivered flip, exponential-backoff
// retry, replay drain) lives here so the two stores stay structurally one.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// OutboxPendingLimitMax bounds one replay sweep; non-positive or
	// oversized limits fall back to OutboxPendingLimitDefault (same rule
	// both outboxes enforced verbatim before the merge).
	OutboxPendingLimitMax     = 500
	OutboxPendingLimitDefault = 100

	// Outbox retry policy (values are unchanged): exponential backoff
	// OutboxRetryBaseDelay * 2^attempts capped at OutboxRetryMaxDelay,
	// with the attempt exponent clamped at OutboxRetryMaxShift and the
	// persisted error detail truncated at OutboxRetryErrorMaxRunes
	// characters. outboxRetryBackoffSQL below renders these exact
	// values; change them here and in that fragment together.
	OutboxRetryBaseDelay     = 5 * time.Second
	OutboxRetryMaxDelay      = time.Hour
	OutboxRetryMaxShift      = 8
	OutboxRetryErrorMaxRunes = 1000
)

// outboxRetryBackoffSQL is the shared retry bookkeeping fragment: attempt
// counter + truncated error + exponential backoff capped at one hour
// (5s * 2^attempts, attempts clamped at 8). Both outboxes ran this exact
// statement before the merge; only the table name varies. Keep the
// literals in sync with OutboxRetryBaseDelay, OutboxRetryMaxDelay,
// OutboxRetryMaxShift, and OutboxRetryErrorMaxRunes (covered by
// TestOutboxRetryBackoffSQLMatchesPolicy).
const outboxRetryBackoffSQL = `attempts=attempts+1, last_error=left($2, 1000),
    next_attempt_at=now() + LEAST(interval '1 hour', interval '5 seconds' * power(2::double precision, LEAST(attempts, 8)))`

func normalizeOutboxLimit(limit int) int {
	if limit <= 0 || limit > OutboxPendingLimitMax {
		return OutboxPendingLimitDefault
	}
	return limit
}

func outboxAttemptDetail(deliveryErr error) string {
	if deliveryErr != nil {
		return deliveryErr.Error()
	}
	return "delivery failed"
}

// OutboxReader is the narrow retry bookkeeping seam one pending queue needs.
// Both stores (and their test fakes) satisfy their instantiation.
type OutboxReader[Row any] interface {
	Pending(context.Context, int) ([]Row, error)
	MarkDelivered(context.Context, string) error
	MarkAttempt(context.Context, string, error) error
}

// genericOutboxStore owns the pool handle, the fact-table name, and the
// unavailable error. Concrete stores embed it and add only their
// table/column mapping (InsertTx/Pending projection).
type genericOutboxStore struct {
	db          *pgxpool.Pool
	table       string
	unavailable error
}

func (s *genericOutboxStore) valid() error {
	if s == nil || s.db == nil {
		if s != nil && s.unavailable != nil {
			return s.unavailable
		}
		return fmt.Errorf("events: outbox database unavailable")
	}
	return nil
}

func (s *genericOutboxStore) markDelivered(ctx context.Context, eventID string) error {
	if err := s.valid(); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE `+s.table+` SET status_state='delivered', delivered_at=now(), last_error='' WHERE event_id=$1`, strings.TrimSpace(eventID))
	return err
}

func (s *genericOutboxStore) markAttempt(ctx context.Context, eventID string, deliveryErr error) error {
	if err := s.valid(); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE `+s.table+`
SET `+outboxRetryBackoffSQL+`
WHERE event_id=$1 AND status_state='pending'`, strings.TrimSpace(eventID), outboxAttemptDetail(deliveryErr))
	return err
}

// queryOutboxPending runs one pending projection and scans rows through
// scanRow. Limit normalization, the due-set error wrap, and the drain loop
// are shared; callers supply only the table-specific SELECT and scan.
func queryOutboxPending[Row any](ctx context.Context, db *pgxpool.Pool, query string, limit int, wrap string, scanRow func(pgx.Rows, *Row) error) ([]Row, error) {
	limit = normalizeOutboxLimit(limit)
	rows, err := db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", wrap, err)
	}
	defer rows.Close()
	result := make([]Row, 0, limit)
	for rows.Next() {
		var row Row
		if err := scanRow(rows, &row); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// genericReplayer drains one pending queue through a caller-supplied sink.
// Task semantics isolate delivery failures (attempt bookkeeping, continue,
// MarkDelivered failure aborts); board semantics join every failure and
// tolerate a nil store (live HTTP delivery without retry bookkeeping).
type genericReplayer[Row any] struct {
	store       OutboxReader[Row]
	unavailable error
	idOf        func(Row) string
	deliver     func(context.Context, Row) error
}

func (r *genericReplayer[Row]) replayIsolated(ctx context.Context, limit int) error {
	if r == nil || r.store == nil {
		if r != nil && r.unavailable != nil {
			return r.unavailable
		}
		return fmt.Errorf("events: outbox replayer unavailable")
	}
	rows, err := r.store.Pending(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := r.deliver(ctx, row); err != nil {
			_ = r.store.MarkAttempt(ctx, r.idOf(row), err)
			continue
		}
		if err := r.store.MarkDelivered(ctx, r.idOf(row)); err != nil {
			return err
		}
	}
	return nil
}

func (r *genericReplayer[Row]) deliverJoined(ctx context.Context, rows []Row) error {
	if r == nil {
		return fmt.Errorf("events: outbox replayer unavailable")
	}
	var failures []error
	for _, row := range rows {
		if err := r.deliver(ctx, row); err != nil {
			if r.store != nil {
				_ = r.store.MarkAttempt(ctx, r.idOf(row), err)
			}
			failures = append(failures, err)
			continue
		}
		if r.store != nil {
			if err := r.store.MarkDelivered(ctx, r.idOf(row)); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
