package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresWebhookPersistence stores subscriptions, aggregate deliveries and
// per-attempt facts in the event tables owned by the events context.
type PostgresWebhookPersistence struct{ db *pgxpool.Pool }

func NewPostgresWebhookPersistence(db *pgxpool.Pool) *PostgresWebhookPersistence {
	return &PostgresWebhookPersistence{db: db}
}

func (p *PostgresWebhookPersistence) valid() error {
	if p == nil || p.db == nil {
		return fmt.Errorf("events: webhook database unavailable")
	}
	return nil
}

func (p *PostgresWebhookPersistence) CreateSubscription(ctx context.Context, sub *WebhookSubscription) error {
	if err := p.valid(); err != nil {
		return err
	}
	if sub == nil {
		return fmt.Errorf("events: webhook subscription is nil")
	}
	result, err := p.db.Exec(ctx, `
INSERT INTO event_subscriptions (public_id, workspace_id, endpoint_url, signing_secret, event_types, status, created_at, updated_at)
SELECT $1, internal_id, $3, $4, $5, 'active', $6, $6
FROM workspaces WHERE public_id = $2 AND archived_at IS NULL
ON CONFLICT (public_id) DO NOTHING`, sub.ID, sub.WorkspaceID, sub.URL,
		FormatWhsecSecret(sub.Secret), sub.EventTypes, sub.CreatedAt.UTC())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("events: workspace %s not found or subscription %s already exists", sub.WorkspaceID, sub.ID)
	}
	return nil
}

func (p *PostgresWebhookPersistence) DeleteSubscription(ctx context.Context, workspaceID, subscriptionID string) error {
	if err := p.valid(); err != nil {
		return err
	}
	_, err := p.db.Exec(ctx, `DELETE FROM event_subscriptions s USING workspaces w WHERE s.workspace_id = w.internal_id AND w.public_id = $1 AND s.public_id = $2`, workspaceID, subscriptionID)
	return err
}

func (p *PostgresWebhookPersistence) LoadSubscriptions(ctx context.Context) ([]*WebhookSubscription, error) {
	if err := p.valid(); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `
SELECT s.public_id, w.public_id, s.endpoint_url, s.signing_secret, s.event_types,
       s.status, s.created_at, s.updated_at
FROM event_subscriptions s JOIN workspaces w ON w.internal_id = s.workspace_id
ORDER BY s.created_at, s.public_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		var encoded string
		if err := rows.Scan(&sub.ID, &sub.WorkspaceID, &sub.URL, &encoded, &sub.EventTypes, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		secret, ok := ParseWhsecSecret([]byte(encoded))
		if !ok {
			return nil, fmt.Errorf("events: persisted secret for %s is malformed", sub.ID)
		}
		sub.Secret = secret
		sub.Delivered = map[string]struct{}{}
		if err := p.hydrateSubscription(ctx, &sub); err != nil {
			return nil, err
		}
		result = append(result, &sub)
	}
	return result, rows.Err()
}

func (p *PostgresWebhookPersistence) LoadSubscription(ctx context.Context, id string) (*WebhookSubscription, error) {
	if err := p.valid(); err != nil {
		return nil, err
	}
	var sub WebhookSubscription
	var encoded string
	err := p.db.QueryRow(ctx, `
SELECT s.public_id, w.public_id, s.endpoint_url, s.signing_secret, s.event_types,
       s.status, s.created_at, s.updated_at
FROM event_subscriptions s JOIN workspaces w ON w.internal_id = s.workspace_id
	WHERE s.public_id = $1`, id).Scan(&sub.ID, &sub.WorkspaceID, &sub.URL, &encoded, &sub.EventTypes, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	secret, ok := ParseWhsecSecret([]byte(encoded))
	if !ok {
		return nil, fmt.Errorf("events: persisted secret for %s is malformed", id)
	}
	sub.Secret = secret
	sub.Delivered = map[string]struct{}{}
	if err := p.hydrateSubscription(ctx, &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (p *PostgresWebhookPersistence) hydrateSubscription(ctx context.Context, sub *WebhookSubscription) error {
	rows, err := p.db.Query(ctx, `
SELECT e.public_id, e.event_type, d.status, e.event_data
FROM event_deliveries d
JOIN workspace_events e ON e.internal_id = d.event_id
JOIN event_subscriptions s ON s.internal_id = d.subscription_id
WHERE s.public_id = $1
ORDER BY d.created_at DESC`, sub.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var deliveryID, typ, status string
		var payload []byte
		if err := rows.Scan(&deliveryID, &typ, &status, &payload); err != nil {
			return err
		}
		if status == "delivered" {
			sub.Delivered[deliveryID] = struct{}{}
		}
		if status == "dead_letter" && sub.Status != WebhookStatusDeadLetter {
			sub.Status = WebhookStatusDeadLetter
		}
		if status == "failed" && sub.Status == WebhookStatusActive {
			sub.Status = WebhookStatusDegraded
		}
		var envelope V1Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			continue
		}
		if status == "dead_letter" {
			var attempts int
			var lastError string
			_ = p.db.QueryRow(ctx, `SELECT attempt_count, response_body FROM event_deliveries d JOIN event_subscriptions s ON s.internal_id=d.subscription_id JOIN workspace_events e ON e.internal_id=d.event_id WHERE s.public_id=$1 AND e.public_id=$2`, sub.ID, deliveryID).Scan(&attempts, &lastError)
			sub.DeadLetter = append(sub.DeadLetter, WebhookDeadLetter{DeliveryID: deliveryID, EventType: typ, Attempts: attempts, LastError: lastError, At: envelope.OccurredAt})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	attemptRows, err := p.db.Query(ctx, `
SELECT e.public_id, a.attempt_number, a.status_code, a.outcome, a.error_message, a.attempted_at
FROM event_delivery_attempts a
JOIN event_deliveries d ON d.internal_id = a.delivery_id
JOIN workspace_events e ON e.internal_id = d.event_id
JOIN event_subscriptions s ON s.internal_id = d.subscription_id
WHERE s.public_id = $1 ORDER BY a.attempted_at DESC LIMIT $2`, sub.ID, webhookAttemptLogCap)
	if err != nil {
		return err
	}
	defer attemptRows.Close()
	for attemptRows.Next() {
		var attempt WebhookAttempt
		if err := attemptRows.Scan(&attempt.DeliveryID, &attempt.Attempt, &attempt.StatusCode, &attempt.Outcome, &attempt.Error, &attempt.At); err != nil {
			return err
		}
		sub.Attempts = append(sub.Attempts, attempt)
	}
	return attemptRows.Err()
}

func (p *PostgresWebhookPersistence) EnsureDelivery(ctx context.Context, envelope V1Envelope, subscriptionID string) error {
	if err := p.valid(); err != nil {
		return err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `
WITH event_row AS (
  INSERT INTO workspace_events (public_id, workspace_id, event_type, event_version, resource_type, resource_id, event_data, occurred_at)
  SELECT $1, internal_id, $2, $3, $4, $5, $6::jsonb, $7
  FROM workspaces WHERE public_id = $8 AND archived_at IS NULL
  ON CONFLICT (public_id) DO UPDATE SET event_data = EXCLUDED.event_data
  RETURNING internal_id
), sub_row AS (
  SELECT internal_id FROM event_subscriptions WHERE public_id = $9
)
INSERT INTO event_deliveries (event_id, subscription_id, status)
SELECT event_row.internal_id, sub_row.internal_id, 'pending' FROM event_row, sub_row
ON CONFLICT (event_id, subscription_id) DO NOTHING`, envelope.ID, envelope.Type, envelope.Version,
		envelope.Resource.Type, envelope.Resource.ID, payload, envelope.OccurredAt.UTC(), envelope.WorkspaceID, subscriptionID)
	return err
}

func (p *PostgresWebhookPersistence) PendingDeliveries(ctx context.Context) ([]WebhookDeliveryArgs, error) {
	if err := p.valid(); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `
SELECT s.public_id, e.public_id, e.event_data
FROM event_deliveries d
JOIN event_subscriptions s ON s.internal_id=d.subscription_id
JOIN workspace_events e ON e.internal_id=d.event_id
WHERE d.status IN ('pending','retrying')
ORDER BY d.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []WebhookDeliveryArgs
	for rows.Next() {
		var subscriptionID, deliveryID string
		var payload []byte
		if err := rows.Scan(&subscriptionID, &deliveryID, &payload); err != nil {
			return nil, err
		}
		var envelope V1Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, err
		}
		result = append(result, WebhookDeliveryArgs{SubscriptionID: subscriptionID, DeliveryID: deliveryID, Envelope: envelope})
	}
	return result, rows.Err()
}

func (p *PostgresWebhookPersistence) deliveryID(ctx context.Context, subscriptionID string, envelope V1Envelope) (int64, error) {
	var id int64
	err := p.db.QueryRow(ctx, `SELECT d.internal_id FROM event_deliveries d JOIN event_subscriptions s ON s.internal_id=d.subscription_id JOIN workspace_events e ON e.internal_id=d.event_id WHERE s.public_id=$1 AND e.public_id=$2`, subscriptionID, envelope.ID).Scan(&id)
	return id, err
}

func (p *PostgresWebhookPersistence) RecordAttempt(ctx context.Context, subscriptionID string, envelope V1Envelope, attempt WebhookAttempt, final bool) error {
	if err := p.valid(); err != nil {
		return err
	}
	deliveryID, err := p.deliveryID(ctx, subscriptionID, envelope)
	if err != nil {
		return err
	}
	status := "retrying"
	completedAt := any(nil)
	if attempt.Outcome == "delivered" {
		status = "delivered"
		completedAt = attempt.At
	} else if final {
		status = "dead_letter"
		completedAt = attempt.At
	}
	_, err = p.db.Exec(ctx, `
INSERT INTO event_delivery_attempts (delivery_id, attempt_number, status_code, outcome, error_message, attempted_at)
VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (delivery_id, attempt_number) DO UPDATE SET status_code=EXCLUDED.status_code, outcome=EXCLUDED.outcome, error_message=EXCLUDED.error_message, attempted_at=EXCLUDED.attempted_at`, deliveryID, attempt.Attempt, attempt.StatusCode, attempt.Outcome, attempt.Error, attempt.At)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `UPDATE event_deliveries SET attempt_count=GREATEST(attempt_count,$2), status=$3, last_attempted_at=$4, response_status=$5, response_body=$6, completed_at=$7 WHERE internal_id=$1`, deliveryID, attempt.Attempt, status, attempt.At, attempt.StatusCode, attempt.Error, completedAt)
	return err
}

func (p *PostgresWebhookPersistence) MarkDelivered(ctx context.Context, subscriptionID string, envelope V1Envelope) error {
	if err := p.valid(); err != nil {
		return err
	}
	deliveryID, err := p.deliveryID(ctx, subscriptionID, envelope)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `UPDATE event_deliveries SET attempt_count=GREATEST(attempt_count,1), status='delivered', completed_at=COALESCE(completed_at, now()) WHERE internal_id=$1`, deliveryID)
	return err
}

var _ WebhookPersistence = (*PostgresWebhookPersistence)(nil)
