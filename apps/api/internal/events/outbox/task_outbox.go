package outbox

// Durable task status outbox (issue #903).
//
// Task state transitions and their delivery facts are written by the same
// PostgreSQL transaction. Publication remains downstream: a River replay
// worker retries pending rows after a crash or a transport failure, while the
// evt_ identity stays stable across every attempt.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/events"
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
	Status       events.TaskStatus
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

func (r TaskStatusOutboxRecord) valid() error {
	if !contract.IsEventID(strings.TrimSpace(r.EventID)) {
		return fmt.Errorf("events: outbox event id is invalid")
	}
	if strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.TaskID) == "" || strings.TrimSpace(string(r.Status)) == "" {
		return fmt.Errorf("events: outbox task identity and status are required")
	}
	if strings.TrimSpace(r.EventType) != "" {
		if !events.IsV1EventType(r.EventType) || r.EventVersion <= 0 || len(r.EventData) == 0 {
			return fmt.Errorf("events: outbox workspace event is invalid")
		}
	}
	if r.OccurredAt.IsZero() {
		return fmt.Errorf("events: outbox occurred_at is required")
	}
	return nil
}

// NewTaskStatusOutboxStore builds a durable outbox over the application DB.
type TaskStatusOutboxStore struct{ genericOutboxStore }

// TaskStatusOutboxReader is the task-queue instantiation of the shared
// OutboxReader seam.
type TaskStatusOutboxReader = OutboxReader[TaskStatusOutboxRow]

func NewTaskStatusOutboxStore(db *pgxpool.Pool) *TaskStatusOutboxStore {
	return &TaskStatusOutboxStore{genericOutboxStore{
		db:          db,
		table:       "task_status_event_outbox",
		unavailable: fmt.Errorf("events: task status outbox database unavailable"),
	}}
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
	if s == nil {
		return nil, fmt.Errorf("events: task status outbox database unavailable")
	}
	if err := s.valid(); err != nil {
		return nil, err
	}
	return queryOutboxPending(ctx, s.db, `
SELECT event_id, workspace_id, task_id, status, COALESCE(event_type,''), COALESCE(event_version,0),
       actor_type, actor_id, resource_type, resource_id, event_data, occurred_at,
       attempts, next_attempt_at, last_error
FROM task_status_event_outbox
WHERE status_state = 'pending' AND next_attempt_at <= now()
ORDER BY internal_id
LIMIT $1`, limit, "events: list pending task status outbox",
		func(rows pgx.Rows, row *TaskStatusOutboxRow) error {
			return rows.Scan(&row.EventID, &row.WorkspaceID, &row.TaskID, &row.Status,
				&row.EventType, &row.EventVersion, &row.ActorType, &row.ActorID,
				&row.ResourceType, &row.ResourceID, &row.EventData, &row.OccurredAt,
				&row.Attempts, &row.NextAttemptAt, &row.LastError)
		})
}

func (s *TaskStatusOutboxStore) MarkDelivered(ctx context.Context, eventID string) error {
	if s == nil {
		return fmt.Errorf("events: task status outbox database unavailable")
	}
	return s.markDelivered(ctx, eventID)
}

func (s *TaskStatusOutboxStore) MarkAttempt(ctx context.Context, eventID string, deliveryErr error) error {
	if s == nil {
		return fmt.Errorf("events: task status outbox database unavailable")
	}
	return s.markAttempt(ctx, eventID, deliveryErr)
}

func (r TaskStatusOutboxRow) TaskStatusEvent() events.TaskStatusEvent {
	return events.NormalizeTaskStatusEvent(events.TaskStatusEvent{Type: events.EventTypeTaskStatusChanged, EventID: r.EventID, TaskID: r.TaskID, Status: r.Status, OccurredAt: r.OccurredAt})
}

// WorkspaceEnvelope reconstructs the frozen v1 envelope without minting a
// new id. The occurred_at from the transition fact is retained verbatim.
func (r TaskStatusOutboxRow) WorkspaceEnvelope() (events.V1Envelope, bool, error) {
	if strings.TrimSpace(r.EventType) == "" {
		return events.V1Envelope{}, false, nil
	}
	envelope, err := events.MintV1EnvelopeWithID(r.EventID, r.EventType, r.WorkspaceID,
		events.V1Actor{Type: r.ActorType, ID: r.ActorID}, events.V1Resource{Type: r.ResourceType, ID: r.ResourceID}, r.EventData)
	if err != nil {
		return events.V1Envelope{}, false, err
	}
	envelope.Version = r.EventVersion
	envelope.OccurredAt = r.OccurredAt.UTC()
	if err := events.ValidateV1Envelope(envelope); err != nil {
		return events.V1Envelope{}, false, err
	}
	return envelope, true, nil
}

// WebhookDispatcher receives durable workspace envelopes for webhook
// fan-out. WebhookStore implements it; the interface keeps the outbox from
// depending on the webhook leaf package.
type WebhookDispatcher interface {
	DispatchErr(context.Context, events.V1Envelope) error
}

// TaskStatusOutboxReplayer drains pending rows. Each sink is idempotent on the
// stable event id (WorkspaceEventLog, JetStream MsgID and webhook delivery
// aggregate), so a crash between sinks is safe and at-least-once.
type TaskStatusOutboxReplayer struct {
	Store     TaskStatusOutboxReader
	Status    events.TaskStatusPublisher
	Workspace *events.WorkspaceEventLog
	Bus       WorkspaceEventPublisher
	Webhooks  WebhookDispatcher
}

func (r *TaskStatusOutboxReplayer) Replay(ctx context.Context, limit int) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("events: task status outbox replayer unavailable")
	}
	drainer := &genericReplayer[TaskStatusOutboxRow]{
		store:       r.Store,
		unavailable: fmt.Errorf("events: task status outbox replayer unavailable"),
		idOf:        func(row TaskStatusOutboxRow) string { return row.EventID },
		deliver:     r.deliver,
	}
	return drainer.replayIsolated(ctx, limit)
}

func (r *TaskStatusOutboxReplayer) deliver(ctx context.Context, row TaskStatusOutboxRow) error {
	if r.Status != nil {
		if err := events.PublishTaskStatusBestEffort(ctx, r.Status, row.TaskStatusEvent()); err != nil {
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
		if err := r.Webhooks.DispatchErr(ctx, envelope); err != nil {
			return err
		}
	}
	return nil
}
