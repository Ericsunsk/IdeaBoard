package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

type Repository struct {
	db *sql.DB
}

type paymentContextDB struct {
	ctx context.Context
	db  *sql.DB
}

type StoredStripeConfiguration struct {
	AccountID               string
	Mode                    StripeMode
	ChargesEnabled          bool
	ProductTaxCode          string
	PurchasesEnabled        bool
	StripeProductID         string
	StripeWebhookEndpointID string
	APIKeyCiphertext        string
	WebhookSecretCiphertext string
	SecretMaterialKind      string
	CreatedBy               int64
	UpdatedBy               int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type ConfigurationAudit struct {
	ActorID      int64
	Outcome      string
	ErrorCode    string
	ErrorSummary string
	Metadata     json.RawMessage
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) dbWithContext(ctx context.Context) paymentContextDB {
	return paymentContextDB{ctx: ctx, db: r.db}
}

func (db paymentContextDB) queries() *dbsqlc.Queries {
	return dbsqlc.New(db.db)
}

func (db paymentContextDB) withTx(run func(*dbsqlc.Queries, *sql.Tx) error) error {
	return dbutil.WithTxDBTX(db.ctx, db.db, run)
}

func (r *Repository) GetStripeConfiguration(ctx context.Context) (StoredStripeConfiguration, error) {
	if r == nil || r.db == nil {
		return StoredStripeConfiguration{}, fmt.Errorf("payments repository: database unavailable")
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentStripeConfiguration(ctx)
	if err != nil {
		return StoredStripeConfiguration{}, err
	}
	return storedStripeConfiguration(row), nil
}

func (r *Repository) ReplaceStripeConfiguration(ctx context.Context, value StoredStripeConfiguration, audit ConfigurationAudit) (StoredStripeConfiguration, error) {
	if r == nil || r.db == nil {
		return StoredStripeConfiguration{}, fmt.Errorf("payments repository: database unavailable")
	}
	var stored StoredStripeConfiguration
	err := r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ *sql.Tx) error {
		row, err := q.ReplacePaymentStripeConfiguration(ctx, dbsqlc.ReplacePaymentStripeConfigurationParams{
			AccountID: value.AccountID, Mode: string(value.Mode), ChargesEnabled: value.ChargesEnabled,
			ProductTaxCode: value.ProductTaxCode, PurchasesEnabled: value.PurchasesEnabled,
			StripeProductID: value.StripeProductID, StripeWebhookEndpointID: value.StripeWebhookEndpointID,
			ApiKeyCiphertext: value.APIKeyCiphertext, WebhookSecretCiphertext: value.WebhookSecretCiphertext,
			SecretMaterialKind: value.SecretMaterialKind, CreatedBy: value.UpdatedBy,
		})
		if err != nil {
			return fmt.Errorf("replace Stripe configuration: %w", err)
		}
		if err := insertConfigurationAudit(ctx, q, audit); err != nil {
			return err
		}
		stored = storedStripeConfiguration(row)
		return nil
	})
	return stored, err
}

func (r *Repository) RecordConfigurationAudit(ctx context.Context, audit ConfigurationAudit) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	return insertConfigurationAudit(ctx, r.dbWithContext(ctx).queries(), audit)
}

func (r *Repository) InsertStripeWebhookEvent(ctx context.Context, event StripeWebhookEvent, result string, processedAt *time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().InsertPaymentWebhookEvent(ctx, dbsqlc.InsertPaymentWebhookEventParams{
		StripeEventID: event.ID, EventType: event.Type, StripeObjectID: event.ObjectID,
		Livemode: event.Livemode, StripeCreatedAt: event.CreatedAt, ProcessingResult: result,
		ProcessedAt: dbutil.NullTime(processedAt),
	})
	if err != nil {
		return false, fmt.Errorf("insert Stripe webhook event: %w", err)
	}
	return rows == 0, nil
}

func insertConfigurationAudit(ctx context.Context, q *dbsqlc.Queries, audit ConfigurationAudit) error {
	metadata := audit.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if err := q.InsertPaymentAdminAction(ctx, dbsqlc.InsertPaymentAdminActionParams{
		ActionType: "configure_stripe", ActorID: audit.ActorID, Outcome: audit.Outcome,
		ErrorCode: audit.ErrorCode, ErrorSummary: audit.ErrorSummary, Metadata: metadata,
	}); err != nil {
		return fmt.Errorf("insert payment admin action: %w", err)
	}
	return nil
}

func storedStripeConfiguration(row dbsqlc.PaymentStripeConfiguration) StoredStripeConfiguration {
	return StoredStripeConfiguration{
		AccountID: row.AccountID, Mode: StripeMode(row.Mode), ChargesEnabled: row.ChargesEnabled,
		ProductTaxCode: row.ProductTaxCode, PurchasesEnabled: row.PurchasesEnabled,
		StripeProductID: row.StripeProductID, StripeWebhookEndpointID: row.StripeWebhookEndpointID,
		APIKeyCiphertext: row.ApiKeyCiphertext, WebhookSecretCiphertext: row.WebhookSecretCiphertext,
		SecretMaterialKind: row.SecretMaterialKind, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func isConfigurationNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
