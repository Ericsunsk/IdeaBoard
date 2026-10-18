package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/events"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/platform/secrets"
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
	// Signing secrets are encrypted at rest (Security H1): the column
	// holds AES-256-GCM ciphertext over the whsec_ transport string, and
	// the kind/version columns record the envelope for dual-read and
	// rotation detection. Writes fail closed without the master key.
	payload, materialKind, keyVersion, err := encodeStoredWebhookSecret(sub.Secret)
	if err != nil {
		return fmt.Errorf("events: encrypt webhook signing secret: %w", err)
	}
	result, err := p.db.Exec(ctx, `
INSERT INTO event_subscriptions (public_id, workspace_id, endpoint_url, signing_secret, signing_secret_material_kind, signing_secret_key_version, event_types, status, created_at, updated_at)
SELECT $1, internal_id, $3, $4, $5, $6, $7, 'active', $8, $8
FROM workspaces WHERE public_id = $2 AND archived_at IS NULL
ON CONFLICT (public_id) DO NOTHING`, sub.ID, sub.WorkspaceID, sub.URL,
		payload, materialKind, keyVersion, sub.EventTypes, sub.CreatedAt.UTC())
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
	rows, err := p.db.Query(ctx, subscriptionSelectSQL+`
ORDER BY s.created_at, s.public_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		var payload, materialKind, keyVersion string
		if err := rows.Scan(&sub.ID, &sub.WorkspaceID, &sub.URL, &payload, &materialKind, &keyVersion, &sub.EventTypes, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		secret, err := decodeStoredWebhookSecret(payload, materialKind, keyVersion)
		if err != nil {
			return nil, fmt.Errorf("events: persisted secret for %s is malformed", sub.ID)
		}
		sub.Secret = secret
		sub.Delivered = map[string]struct{}{}
		if err := p.hydrateSubscription(ctx, &sub); err != nil {
			return nil, err
		}
		result = append(result, &sub)
		if strings.TrimSpace(materialKind) == webhookSecretMaterialPlaintext {
			p.maybeReencryptStoredSecret(ctx, sub.ID, secret)
		}
	}
	return result, rows.Err()
}

// subscriptionSelectSQL is the single definition of the subscription
// readmodel projection (subscription facts joined to the owning
// workspace), shared by the list and single-load paths.
const subscriptionSelectSQL = `
SELECT s.public_id, w.public_id, s.endpoint_url, s.signing_secret, s.signing_secret_material_kind, s.signing_secret_key_version, s.event_types,
       s.status, s.created_at, s.updated_at
FROM event_subscriptions s JOIN workspaces w ON w.internal_id = s.workspace_id`

func (p *PostgresWebhookPersistence) LoadSubscription(ctx context.Context, id string) (*WebhookSubscription, error) {
	if err := p.valid(); err != nil {
		return nil, err
	}
	var sub WebhookSubscription
	var payload, materialKind, keyVersion string
	err := p.db.QueryRow(ctx, subscriptionSelectSQL+`
	WHERE s.public_id = $1`, id).Scan(&sub.ID, &sub.WorkspaceID, &sub.URL, &payload, &materialKind, &keyVersion, &sub.EventTypes, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	secret, derr := decodeStoredWebhookSecret(payload, materialKind, keyVersion)
	if derr != nil {
		return nil, fmt.Errorf("events: persisted secret for %s is malformed", id)
	}
	sub.Secret = secret
	sub.Delivered = map[string]struct{}{}
	if err := p.hydrateSubscription(ctx, &sub); err != nil {
		return nil, err
	}
	if strings.TrimSpace(materialKind) == webhookSecretMaterialPlaintext {
		p.maybeReencryptStoredSecret(ctx, sub.ID, secret)
	}
	return &sub, nil
}

// maybeReencryptStoredSecret converges a pre-migration plaintext row to
// the encrypted envelope after a successful dual-read. Best-effort by
// design: the caller already holds the correct in-memory secret, so a
// re-encrypt failure only logs (never fails the read, never carries
// secret material). The conditional UPDATE keeps concurrent readers from
// clobbering each other.
func (p *PostgresWebhookPersistence) maybeReencryptStoredSecret(ctx context.Context, subscriptionID string, raw []byte) {
	if !secrets.IsEncryptionEnabled() {
		return
	}
	payload, materialKind, keyVersion, err := encodeStoredWebhookSecret(raw)
	if err != nil {
		slog.Warn("webhook signing key re-encrypt skipped", "subscription", subscriptionID, "err", err)
		return
	}
	result, err := p.db.Exec(ctx, `
UPDATE event_subscriptions
SET signing_secret = $2, signing_secret_material_kind = $3, signing_secret_key_version = $4, updated_at = now()
WHERE public_id = $1 AND signing_secret_material_kind = $5`,
		subscriptionID, payload, materialKind, keyVersion, webhookSecretMaterialPlaintext)
	if err != nil {
		slog.Warn("webhook signing key re-encrypt failed", "subscription", subscriptionID, "err", err)
		return
	}
	if result.RowsAffected() == 1 {
		slog.Info("webhook signing key re-encrypted", "subscription", subscriptionID, "key_version", keyVersion)
	}
}

// deliveryLookupSQL is the single definition of the delivery JOIN spine
// (delivery facts joined to subscription and event identity), shared by
// the dead-letter detail lookup and the delivery internal-ID resolution.
const deliveryLookupSQL = `FROM event_deliveries d JOIN event_subscriptions s ON s.internal_id=d.subscription_id JOIN workspace_events e ON e.internal_id=d.event_id WHERE s.public_id=$1 AND e.public_id=$2`

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
		var deliveryID, typ string
		var status DeliveryStatus
		var payload []byte
		if err := rows.Scan(&deliveryID, &typ, &status, &payload); err != nil {
			return err
		}
		if status == DeliveryStatusDelivered {
			sub.Delivered[deliveryID] = struct{}{}
		}
		if status == DeliveryStatusDeadLetter && sub.Status != WebhookStatusDeadLetter {
			sub.Status = WebhookStatusDeadLetter
		}
		if status == DeliveryStatusFailed && sub.Status == WebhookStatusActive {
			sub.Status = WebhookStatusDegraded
		}
		var envelope events.V1Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			continue
		}
		if status == "dead_letter" {
			var attempts int
			var lastError string
			_ = p.db.QueryRow(ctx, `SELECT attempt_count, response_body `+deliveryLookupSQL, sub.ID, deliveryID).Scan(&attempts, &lastError)
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

func (p *PostgresWebhookPersistence) EnsureDelivery(ctx context.Context, envelope events.V1Envelope, subscriptionID string) error {
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
		var envelope events.V1Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, err
		}
		result = append(result, WebhookDeliveryArgs{SubscriptionID: subscriptionID, DeliveryID: deliveryID, Envelope: envelope})
	}
	return result, rows.Err()
}

func (p *PostgresWebhookPersistence) deliveryID(ctx context.Context, subscriptionID string, envelope events.V1Envelope) (int64, error) {
	var id int64
	err := p.db.QueryRow(ctx, `SELECT d.internal_id `+deliveryLookupSQL, subscriptionID, envelope.ID).Scan(&id)
	return id, err
}

func (p *PostgresWebhookPersistence) RecordAttempt(ctx context.Context, subscriptionID string, envelope events.V1Envelope, attempt WebhookAttempt, final bool) error {
	if err := p.valid(); err != nil {
		return err
	}
	status := DeliveryStatusRetrying
	completedAt := any(nil)
	if attempt.Outcome == AttemptOutcomeDelivered {
		status = DeliveryStatusDelivered
		completedAt = attempt.At
	} else if final {
		status = DeliveryStatusDeadLetter
		completedAt = attempt.At
	}
	return dbutil.WithTxDBTX(ctx, p.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		var deliveryID int64
		if err := tx.QueryRow(ctx, `SELECT d.internal_id `+deliveryLookupSQL, subscriptionID, envelope.ID).Scan(&deliveryID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO event_delivery_attempts (delivery_id, attempt_number, status_code, outcome, error_message, attempted_at)
VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (delivery_id, attempt_number) DO UPDATE SET status_code=EXCLUDED.status_code, outcome=EXCLUDED.outcome, error_message=EXCLUDED.error_message, attempted_at=EXCLUDED.attempted_at`, deliveryID, attempt.Attempt, attempt.StatusCode, string(attempt.Outcome), attempt.Error, attempt.At); err != nil {
			return fmt.Errorf("events: record webhook attempt: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE event_deliveries SET attempt_count=GREATEST(attempt_count,$2), status=$3, last_attempted_at=$4, response_status=$5, response_body=$6, completed_at=$7 WHERE internal_id=$1`, deliveryID, attempt.Attempt, string(status), attempt.At, attempt.StatusCode, attempt.Error, completedAt); err != nil {
			return fmt.Errorf("events: update webhook delivery: %w", err)
		}
		return nil
	})
}

func (p *PostgresWebhookPersistence) MarkDelivered(ctx context.Context, subscriptionID string, envelope events.V1Envelope) error {
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
