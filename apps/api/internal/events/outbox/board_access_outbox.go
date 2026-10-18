package outbox

// Directed Board access signals persist in an outbox for SSE delivery. Callers
// join an access transaction with InsertTx. River retries pending rows; stable
// event identities make live delivery and replay idempotent. Directed signals
// do not reach webhooks.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/events"
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
		EventID:        events.StableEventID(boardID, memberPublicID, strconv.FormatInt(accessVersion, 10)),
		WorkspaceID:    strings.TrimSpace(workspaceID),
		BoardID:        strings.TrimSpace(boardID),
		MemberPublicID: strings.TrimSpace(memberPublicID),
		AccessVersion:  accessVersion,
		OccurredAt:     occurredAt.UTC().Truncate(time.Microsecond),
	}
}

// BoardAccessOutboxStore persists directed-signal facts for durable delivery.
type BoardAccessOutboxStore struct{ genericOutboxStore }

// BoardAccessOutboxReader is the board-queue instantiation of the shared
// OutboxReader seam.
type BoardAccessOutboxReader = OutboxReader[BoardAccessOutboxRow]

func NewBoardAccessOutboxStore(db *pgxpool.Pool) *BoardAccessOutboxStore {
	return &BoardAccessOutboxStore{genericOutboxStore{
		db:          db,
		table:       "board_access_event_outbox",
		unavailable: fmt.Errorf("events: board access outbox database unavailable"),
	}}
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
	if s == nil {
		return nil, fmt.Errorf("events: board access outbox database unavailable")
	}
	if err := s.valid(); err != nil {
		return nil, err
	}
	return queryOutboxPending(ctx, s.db, `
SELECT event_id, workspace_id, board_id, member_public_id, access_version, occurred_at,
       attempts, next_attempt_at, last_error
FROM board_access_event_outbox
WHERE status_state = 'pending' AND next_attempt_at <= now()
ORDER BY internal_id
LIMIT $1`, limit, "events: list pending board access outbox",
		func(rows pgx.Rows, row *BoardAccessOutboxRow) error {
			return rows.Scan(&row.EventID, &row.WorkspaceID, &row.BoardID, &row.MemberPublicID,
				&row.AccessVersion, &row.OccurredAt,
				&row.Attempts, &row.NextAttemptAt, &row.LastError)
		})
}

func (s *BoardAccessOutboxStore) MarkDelivered(ctx context.Context, eventID string) error {
	if s == nil {
		return fmt.Errorf("events: board access outbox database unavailable")
	}
	return s.markDelivered(ctx, eventID)
}

func (s *BoardAccessOutboxStore) MarkAttempt(ctx context.Context, eventID string, deliveryErr error) error {
	if s == nil {
		return fmt.Errorf("events: board access outbox database unavailable")
	}
	return s.markAttempt(ctx, eventID, deliveryErr)
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
func (r BoardAccessOutboxRecord) AccessChangedEnvelope() (events.V1Envelope, error) {
	envelope, err := events.MintV1EnvelopeWithID(r.EventID, events.EventV1BoardAccessChanged, r.WorkspaceID,
		events.V1Actor{Type: events.V1ActorService, ID: "board-access"},
		events.V1Resource{Type: events.V1ResourceBoard, ID: r.BoardID},
		events.V1BoardAccessChangedData{BoardID: r.BoardID, MemberID: r.MemberPublicID, AccessVersion: r.AccessVersion})
	if err != nil {
		return events.V1Envelope{}, err
	}
	envelope.OccurredAt = r.OccurredAt.UTC()
	if err := events.ValidateV1Envelope(envelope); err != nil {
		return events.V1Envelope{}, err
	}
	return envelope, nil
}

type WorkspaceEventPublisher interface {
	PublishWorkspaceEvent(context.Context, events.V1Envelope) error
}

// BoardAccessSignals delivers committed directed signals immediately and on replay.
// Stable event IDs make hot-log and JetStream delivery idempotent across retries.
// Directed signals never reach webhooks.
type BoardAccessSignals struct {
	Store BoardAccessOutboxReader
	Log   *events.WorkspaceEventLog
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
	rows := make([]BoardAccessOutboxRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, BoardAccessOutboxRow{BoardAccessOutboxRecord: record})
	}
	drainer := &genericReplayer[BoardAccessOutboxRow]{
		store:       r.Store,
		unavailable: ErrBoardAccessOutboxUnavailable,
		idOf:        func(row BoardAccessOutboxRow) string { return row.EventID },
		deliver: func(ctx context.Context, row BoardAccessOutboxRow) error {
			return r.deliver(ctx, row.BoardAccessOutboxRecord)
		},
	}
	return drainer.deliverJoined(ctx, rows)
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
