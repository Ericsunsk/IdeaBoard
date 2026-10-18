package events

// Directed Board access signals persist in an outbox for SSE delivery. Callers
// join an access transaction with InsertTx. River retries pending rows; stable
// event identities make live delivery and replay idempotent. Directed signals
// do not reach webhooks.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrBoardAccessOutboxUnavailable = fmt.Errorf("board access outbox unavailable")

const (
	// BoardAccessOutboxRetention bounds delivered-row history to the NATS
	// JetStream MaxAge (7 days): delivered rows are audit trail only;
	// pending rows are never pruned.
	BoardAccessOutboxRetention = 7 * 24 * time.Hour
)

// BoardAccessOutboxRecord identifies a directed revocation at its committed
// access version. MemberPublicID is the recipient's usr_ public identity.
type BoardAccessOutboxRecord struct {
	EventID        string
	WorkspaceID    string
	BoardID        string
	MemberPublicID string
	AccessVersion  int64
	OccurredAt     time.Time
}

type BoardAccessOutboxRow struct {
	BoardAccessOutboxRecord
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
}

func (r BoardAccessOutboxRecord) valid() error {
	if !contract.IsEventID(strings.TrimSpace(r.EventID)) {
		return fmt.Errorf("events: board access outbox event id is invalid")
	}
	if !contract.IsWorkspaceID(strings.TrimSpace(r.WorkspaceID)) {
		return fmt.Errorf("events: board access outbox workspace id must be ws_ opaque")
	}
	if !contract.IsBoardID(strings.TrimSpace(r.BoardID)) {
		return fmt.Errorf("events: board access outbox board id must be board_ opaque")
	}
	if !contract.IsUserID(strings.TrimSpace(r.MemberPublicID)) {
		return fmt.Errorf("events: board access outbox member id must be usr_ opaque")
	}
	if r.AccessVersion < 1 {
		return fmt.Errorf("events: board access outbox access version must be positive")
	}
	if r.OccurredAt.IsZero() {
		return fmt.Errorf("events: board access outbox occurred_at is required")
	}
	return nil
}

// NewBoardAccessOutboxRecord mints the deterministic directed-signal fact
// for one affected user at the committed access version.
func NewBoardAccessOutboxRecord(workspaceID, boardID, memberPublicID string, accessVersion int64, occurredAt time.Time) BoardAccessOutboxRecord {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return BoardAccessOutboxRecord{
		EventID:        StableBoardAccessEventID(boardID, memberPublicID, accessVersion),
		WorkspaceID:    strings.TrimSpace(workspaceID),
		BoardID:        strings.TrimSpace(boardID),
		MemberPublicID: strings.TrimSpace(memberPublicID),
		AccessVersion:  accessVersion,
		OccurredAt:     occurredAt.UTC().Truncate(time.Microsecond),
	}
}

// BoardAccessOutboxStore persists directed-signal facts for durable delivery.
type BoardAccessOutboxStore struct{ db *pgxpool.Pool }

type BoardAccessOutboxReader interface {
	Pending(context.Context, int) ([]BoardAccessOutboxRow, error)
	MarkDelivered(context.Context, string) error
	MarkAttempt(context.Context, string, error) error
}

func NewBoardAccessOutboxStore(db *pgxpool.Pool) *BoardAccessOutboxStore {
	return &BoardAccessOutboxStore{db: db}
}

func (s *BoardAccessOutboxStore) valid() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("events: board access outbox database unavailable")
	}
	return nil
}

// InsertTx stores one directed-signal fact inside the caller's transaction.
// Access and membership changes commit atomically with their directed signals.
func (s *BoardAccessOutboxStore) InsertTx(ctx context.Context, tx pgx.Tx, record BoardAccessOutboxRecord) error {
	if err := record.valid(); err != nil {
		return err
	}
	if tx == nil {
		return fmt.Errorf("events: board access outbox transaction is nil")
	}
	_, err := tx.Exec(ctx, `
INSERT INTO board_access_event_outbox (
  event_id, workspace_id, board_id, member_public_id, access_version, occurred_at,
  status_state, attempts, next_attempt_at
) VALUES ($1,$2,$3,$4,$5,$6,'pending',0,$6)
ON CONFLICT (event_id) DO NOTHING`,
		record.EventID, record.WorkspaceID, record.BoardID, record.MemberPublicID,
		record.AccessVersion, record.OccurredAt.UTC())
	if err != nil {
		return fmt.Errorf("events: insert board access outbox: %w", err)
	}
	return nil
}

func (s *BoardAccessOutboxStore) Pending(ctx context.Context, limit int) ([]BoardAccessOutboxRow, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
SELECT event_id, workspace_id, board_id, member_public_id, access_version, occurred_at,
       attempts, next_attempt_at, last_error
FROM board_access_event_outbox
WHERE status_state = 'pending' AND next_attempt_at <= now()
ORDER BY internal_id
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("events: list pending board access outbox: %w", err)
	}
	defer rows.Close()
	result := make([]BoardAccessOutboxRow, 0, limit)
	for rows.Next() {
		var row BoardAccessOutboxRow
		if err := rows.Scan(&row.EventID, &row.WorkspaceID, &row.BoardID, &row.MemberPublicID,
			&row.AccessVersion, &row.OccurredAt,
			&row.Attempts, &row.NextAttemptAt, &row.LastError); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *BoardAccessOutboxStore) MarkDelivered(ctx context.Context, eventID string) error {
	if err := s.valid(); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE board_access_event_outbox SET status_state='delivered', delivered_at=now(), last_error='' WHERE event_id=$1`, strings.TrimSpace(eventID))
	return err
}

func (s *BoardAccessOutboxStore) MarkAttempt(ctx context.Context, eventID string, deliveryErr error) error {
	if err := s.valid(); err != nil {
		return err
	}
	detail := "delivery failed"
	if deliveryErr != nil {
		detail = deliveryErr.Error()
	}
	_, err := s.db.Exec(ctx, `
UPDATE board_access_event_outbox
SET attempts=attempts+1, last_error=left($2, 1000),
    next_attempt_at=now() + LEAST(interval '1 hour', interval '5 seconds' * power(2::double precision, LEAST(attempts, 8)))
WHERE event_id=$1 AND status_state='pending'`, strings.TrimSpace(eventID), detail)
	return err
}

// PruneDelivered drops delivered audit rows past retention. Pending rows
// are never pruned: an undelivered revocation stays retryable.
func (s *BoardAccessOutboxStore) PruneDelivered(ctx context.Context, olderThan time.Time) (int64, error) {
	if err := s.valid(); err != nil {
		return 0, err
	}
	result, err := s.db.Exec(ctx, `DELETE FROM board_access_event_outbox WHERE status_state='delivered' AND delivered_at < $1`, olderThan.UTC())
	if err != nil {
		return 0, fmt.Errorf("events: prune board access outbox: %w", err)
	}
	return result.RowsAffected(), nil
}

// AccessChangedEnvelope reconstructs the directed v1 envelope without
// minting a new id. The payload carries board identity + recipient +
// version only: roles, roster, prompts, and asset identifiers can never
// cross this seam (the struct has no fields for them).
func (r BoardAccessOutboxRecord) AccessChangedEnvelope() (V1Envelope, error) {
	envelope, err := MintV1EnvelopeWithID(r.EventID, EventV1BoardAccessChanged, r.WorkspaceID,
		V1Actor{Type: V1ActorService, ID: "board-access"},
		V1Resource{Type: V1ResourceBoard, ID: r.BoardID},
		V1BoardAccessChangedData{BoardID: r.BoardID, MemberID: r.MemberPublicID, AccessVersion: r.AccessVersion})
	if err != nil {
		return V1Envelope{}, err
	}
	envelope.OccurredAt = r.OccurredAt.UTC()
	if err := ValidateV1Envelope(envelope); err != nil {
		return V1Envelope{}, err
	}
	return envelope, nil
}

type WorkspaceEventPublisher interface {
	PublishWorkspaceEvent(context.Context, V1Envelope) error
}

// BoardAccessSignals delivers committed directed signals immediately and on replay.
// Stable event IDs make hot-log and JetStream delivery idempotent across retries.
// Directed signals never reach webhooks.
type BoardAccessSignals struct {
	Store BoardAccessOutboxReader
	Log   *WorkspaceEventLog
	Bus   WorkspaceEventPublisher
}

func (r *BoardAccessSignals) Replay(ctx context.Context, limit int) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("events: board access outbox replayer unavailable")
	}
	rows, err := r.Store.Pending(ctx, limit)
	if err != nil {
		return err
	}
	records := make([]BoardAccessOutboxRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, row.BoardAccessOutboxRecord)
	}
	return r.Deliver(ctx, records)
}

// Deliver consumes accepted facts without re-reading membership or minting event identities.
func (r *BoardAccessSignals) Deliver(ctx context.Context, records []BoardAccessOutboxRecord) error {
	if r == nil {
		return ErrBoardAccessOutboxUnavailable
	}
	var failures []error
	for _, record := range records {
		if err := r.deliver(ctx, record); err != nil {
			if r.Store != nil {
				_ = r.Store.MarkAttempt(ctx, record.EventID, err)
			}
			failures = append(failures, err)
			continue
		}
		if r.Store != nil {
			if err := r.Store.MarkDelivered(ctx, record.EventID); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func (r *BoardAccessSignals) deliver(ctx context.Context, record BoardAccessOutboxRecord) error {
	envelope, err := record.AccessChangedEnvelope()
	if err != nil {
		return err
	}
	if r.Log != nil {
		r.Log.Append(envelope)
	}
	if r.Bus != nil {
		if err := r.Bus.PublishWorkspaceEvent(ctx, envelope); err != nil {
			return err
		}
	}
	return nil
}
