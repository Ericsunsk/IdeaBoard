package events

// Durable task status outbox (issue #903).
//
// Task state transitions and their delivery facts are written by the same
// PostgreSQL transaction. Publication remains downstream: a River replay
// worker retries pending rows after a crash or a transport failure, while the
// evt_ identity stays stable across every attempt.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTaskStatusOutboxUnavailable = fmt.Errorf("task status outbox unavailable")

const (
	TaskOutboxStatusPending   = "pending"
	TaskOutboxStatusDelivered = "delivered"
)

// TaskStatusOutboxRecord is the immutable transition fact written in the
// state transaction. EventType is empty for internal pending retry events;
// those still fan out on the task status channel but have no v1 workspace
// envelope.
type TaskStatusOutboxRecord struct {
	EventID      string
	WorkspaceID  string
	TaskID       string
	Status       TaskStatus
	EventType    string
	EventVersion int
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	EventData    json.RawMessage
	OccurredAt   time.Time
}

type TaskStatusOutboxRow struct {
	TaskStatusOutboxRecord
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
}

// StableTaskEventID derives one deterministic evt_ id for a transition. The
// execution token distinguishes a retry's pending/running cycle from the
// original submit while redeliveries of the same token converge exactly.
func StableTaskEventID(taskID string, status TaskStatus, executionToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(taskID) + "\x00" + string(status) + "\x00" + strings.TrimSpace(executionToken)))
	return contract.PrefixEvent + hex.EncodeToString(sum[:12])
}

func (r TaskStatusOutboxRecord) valid() error {
	if !contract.IsEventID(strings.TrimSpace(r.EventID)) {
		return fmt.Errorf("events: outbox event id is invalid")
	}
	if strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.TaskID) == "" || strings.TrimSpace(string(r.Status)) == "" {
		return fmt.Errorf("events: outbox task identity and status are required")
	}
	if strings.TrimSpace(r.EventType) != "" {
		if !IsV1EventType(r.EventType) || r.EventVersion <= 0 || len(r.EventData) == 0 {
			return fmt.Errorf("events: outbox workspace event is invalid")
		}
	}
	if r.OccurredAt.IsZero() {
		return fmt.Errorf("events: outbox occurred_at is required")
	}
	return nil
}

// NewTaskStatusOutboxStore builds a durable outbox over the application DB.
type TaskStatusOutboxStore struct{ db *pgxpool.Pool }

type TaskStatusOutboxReader interface {
	Pending(context.Context, int) ([]TaskStatusOutboxRow, error)
	MarkDelivered(context.Context, string) error
	MarkAttempt(context.Context, string, error) error
}

func NewTaskStatusOutboxStore(db *pgxpool.Pool) *TaskStatusOutboxStore {
	return &TaskStatusOutboxStore{db: db}
}

func (s *TaskStatusOutboxStore) valid() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("events: task status outbox database unavailable")
	}
	return nil
}

// InsertTx is idempotent on event_id. It must be called before the caller's
// task state transaction commits; a rollback removes both facts together.
func (s *TaskStatusOutboxStore) InsertTx(ctx context.Context, tx pgx.Tx, record TaskStatusOutboxRecord) error {
	if err := record.valid(); err != nil {
		return err
	}
	if tx == nil {
		return fmt.Errorf("events: task status outbox transaction is nil")
	}
	_, err := tx.Exec(ctx, `
INSERT INTO task_status_event_outbox (
  event_id, workspace_id, task_id, status, event_type, event_version,
  actor_type, actor_id, resource_type, resource_id, event_data, occurred_at,
  status_state, attempts, next_attempt_at
) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,0),$7,$8,$9,$10,$11,$12,'pending',0,$12)
ON CONFLICT (event_id) DO NOTHING`,
		record.EventID, record.WorkspaceID, record.TaskID, record.Status,
		record.EventType, record.EventVersion, record.ActorType, record.ActorID,
		record.ResourceType, record.ResourceID, record.EventData, record.OccurredAt.UTC())
	if err != nil {
		return fmt.Errorf("events: insert task status outbox: %w", err)
	}
	return nil
}

func (s *TaskStatusOutboxStore) Pending(ctx context.Context, limit int) ([]TaskStatusOutboxRow, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
SELECT event_id, workspace_id, task_id, status, COALESCE(event_type,''), COALESCE(event_version,0),
       actor_type, actor_id, resource_type, resource_id, event_data, occurred_at,
       attempts, next_attempt_at, last_error
FROM task_status_event_outbox
WHERE status_state = 'pending' AND next_attempt_at <= now()
ORDER BY internal_id
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("events: list pending task status outbox: %w", err)
	}
	defer rows.Close()
	result := make([]TaskStatusOutboxRow, 0, limit)
	for rows.Next() {
		var row TaskStatusOutboxRow
		if err := rows.Scan(&row.EventID, &row.WorkspaceID, &row.TaskID, &row.Status,
			&row.EventType, &row.EventVersion, &row.ActorType, &row.ActorID,
			&row.ResourceType, &row.ResourceID, &row.EventData, &row.OccurredAt,
			&row.Attempts, &row.NextAttemptAt, &row.LastError); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *TaskStatusOutboxStore) MarkDelivered(ctx context.Context, eventID string) error {
	if err := s.valid(); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE task_status_event_outbox SET status_state='delivered', delivered_at=now(), last_error='' WHERE event_id=$1`, strings.TrimSpace(eventID))
	return err
}

func (s *TaskStatusOutboxStore) MarkAttempt(ctx context.Context, eventID string, deliveryErr error) error {
	if err := s.valid(); err != nil {
		return err
	}
	detail := "delivery failed"
	if deliveryErr != nil {
		detail = deliveryErr.Error()
	}
	_, err := s.db.Exec(ctx, `
UPDATE task_status_event_outbox
SET attempts=attempts+1, last_error=left($2, 1000),
    next_attempt_at=now() + LEAST(interval '1 hour', interval '5 seconds' * power(2::double precision, LEAST(attempts, 8)))
WHERE event_id=$1 AND status_state='pending'`, strings.TrimSpace(eventID), detail)
	return err
}

func (r TaskStatusOutboxRow) TaskStatusEvent() TaskStatusEvent {
	return NormalizeTaskStatusEvent(TaskStatusEvent{Type: EventTypeTaskStatusChanged, EventID: r.EventID, TaskID: r.TaskID, Status: r.Status, OccurredAt: r.OccurredAt})
}

// WorkspaceEnvelope reconstructs the frozen v1 envelope without minting a
// new id. The occurred_at from the transition fact is retained verbatim.
func (r TaskStatusOutboxRow) WorkspaceEnvelope() (V1Envelope, bool, error) {
	if strings.TrimSpace(r.EventType) == "" {
		return V1Envelope{}, false, nil
	}
	envelope, err := MintV1EnvelopeWithID(r.EventID, r.EventType, r.WorkspaceID,
		V1Actor{Type: r.ActorType, ID: r.ActorID}, V1Resource{Type: r.ResourceType, ID: r.ResourceID}, r.EventData)
	if err != nil {
		return V1Envelope{}, false, err
	}
	envelope.Version = r.EventVersion
	envelope.OccurredAt = r.OccurredAt.UTC()
	if err := ValidateV1Envelope(envelope); err != nil {
		return V1Envelope{}, false, err
	}
	return envelope, true, nil
}

// TaskStatusOutboxReplayer drains pending rows. Each sink is idempotent on the
// stable event id (WorkspaceEventLog, JetStream MsgID and webhook delivery
// aggregate), so a crash between sinks is safe and at-least-once.
type TaskStatusOutboxReplayer struct {
	Store     TaskStatusOutboxReader
	Status    TaskStatusPublisher
	Workspace *WorkspaceEventLog
	Bus       *NATSJetStreamEventBus
	Webhooks  *WebhookStore
}

func (r *TaskStatusOutboxReplayer) Replay(ctx context.Context, limit int) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("events: task status outbox replayer unavailable")
	}
	rows, err := r.Store.Pending(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := r.deliver(ctx, row); err != nil {
			_ = r.Store.MarkAttempt(ctx, row.EventID, err)
			continue
		}
		if err := r.Store.MarkDelivered(ctx, row.EventID); err != nil {
			return err
		}
	}
	return nil
}

func (r *TaskStatusOutboxReplayer) deliver(ctx context.Context, row TaskStatusOutboxRow) error {
	if r.Status != nil {
		if err := PublishTaskStatusBestEffort(ctx, r.Status, row.TaskStatusEvent()); err != nil {
			return err
		}
	}
	envelope, ok, err := row.WorkspaceEnvelope()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if r.Workspace != nil {
		r.Workspace.Append(envelope)
	}
	if r.Bus != nil {
		if err := r.Bus.PublishWorkspaceEvent(ctx, envelope); err != nil {
			return err
		}
	}
	if r.Webhooks != nil {
		r.Webhooks.Dispatch(ctx, envelope)
	}
	return nil
}
