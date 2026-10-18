package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) ReplaceStripeConfiguration(ctx context.Context, expectedRevision int64, value paymentconfiguration.StoredStripeConfiguration, audit paymentconfiguration.Audit) (paymentconfiguration.StoredStripeConfiguration, *paymentconfiguration.PaymentCheckoutShutdown, error) {
	if r == nil || r.db == nil {
		return paymentconfiguration.StoredStripeConfiguration{}, nil, fmt.Errorf("payments repository: database unavailable")
	}
	var stored paymentconfiguration.StoredStripeConfiguration
	var shutdown *paymentconfiguration.PaymentCheckoutShutdown
	err := r.DBWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ pgx.Tx) error {
		previous, previousErr := q.GetPaymentStripeConfiguration(ctx)
		if err := validateConfigurationRevision(previous, previousErr, expectedRevision); err != nil {
			return err
		}
		credentialID, err := r.ensureStripeCredential(ctx, q, &value)
		if err != nil {
			return err
		}
		row, err := q.ReplacePaymentStripeConfiguration(ctx, dbsqlc.ReplacePaymentStripeConfigurationParams{
			ActiveCredentialID: pgtype.UUID{Bytes: credentialID, Valid: true}, ProductTaxCode: value.ProductTaxCode,
			PurchasesEnabled: value.PurchasesEnabled, UpdatedBy: value.UpdatedBy, ExpectedRevision: expectedRevision,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return paymentcontracts.ErrStripeConfigurationChanged
			}
			return fmt.Errorf("replace Stripe configuration: %w", err)
		}
		if previousErr == nil && previous.PurchasesEnabled && !row.PurchasesEnabled {
			previousCredential, err := q.GetPaymentStripeCredential(ctx, previous.ActiveCredentialID)
			if err != nil {
				return fmt.Errorf("load previous Stripe credential for Checkout shutdown: %w", err)
			}
			shutdownRow, err := q.InsertPaymentCheckoutShutdown(ctx, dbsqlc.InsertPaymentCheckoutShutdownParams{
				StripeCredentialID: previous.ActiveCredentialID, StripeAccountID: previousCredential.AccountID,
			})
			if err != nil {
				return fmt.Errorf("create Checkout shutdown: %w", err)
			}
			value := paymentCheckoutShutdown(shutdownRow)
			shutdown = &value
		}
		if err := insertConfigurationAudit(ctx, q, audit); err != nil {
			return err
		}
		stored = value
		stored.ProductTaxCode = row.ProductTaxCode
		stored.PurchasesEnabled = row.PurchasesEnabled
		stored.UpdatedBy = row.UpdatedBy
		stored.Revision = row.Revision
		stored.UpdatedAt = dbutil.TimeValue(row.UpdatedAt)
		return nil
	})
	return stored, shutdown, err
}

func validateConfigurationRevision(previous dbsqlc.PaymentStripeConfiguration, previousErr error, expectedRevision int64) error {
	if previousErr != nil && !errors.Is(previousErr, pgx.ErrNoRows) {
		return fmt.Errorf("load previous Stripe configuration: %w", previousErr)
	}
	if previousErr == nil && previous.Revision != expectedRevision {
		return paymentcontracts.ErrStripeConfigurationChanged
	}
	if errors.Is(previousErr, pgx.ErrNoRows) && expectedRevision != 0 {
		return paymentcontracts.ErrStripeConfigurationChanged
	}
	return nil
}

func (r *Repository) ensureStripeCredential(ctx context.Context, q *dbsqlc.Queries, value *paymentconfiguration.StoredStripeConfiguration) (uuid.UUID, error) {
	credentialID, err := uuid.Parse(value.CredentialID)
	if err == nil {
		return credentialID, nil
	}
	credentialID = uuid.New()
	credential, insertErr := q.InsertPaymentStripeCredential(ctx, dbsqlc.InsertPaymentStripeCredentialParams{
		ID: pgtype.UUID{Bytes: credentialID, Valid: true}, AccountID: value.AccountID, Mode: string(value.Mode), ChargesEnabled: value.ChargesEnabled,
		PaymentMethodConfigurationID: pgtype.Text{String: value.PaymentMethodConfigurationID, Valid: true},
		StripeProductID:              value.StripeProductID, StripeWebhookEndpointID: value.StripeWebhookEndpointID,
		ApiKeyCiphertext: value.APIKeyCiphertext, WebhookSecretCiphertext: value.WebhookSecretCiphertext,
		SecretMaterialKind: value.SecretMaterialKind, CreatedBy: value.UpdatedBy,
	})
	if insertErr != nil {
		return uuid.Nil, fmt.Errorf("insert Stripe credential generation: %w", insertErr)
	}
	value.CredentialID = pgUUIDString(credential.ID)
	value.CreatedBy = credential.CreatedBy
	value.CreatedAt = dbutil.TimeValue(credential.CreatedAt)
	return credentialID, nil
}

func (r *Repository) CompletePaymentCheckoutShutdown(ctx context.Context, id string, completedAt time.Time) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid Checkout shutdown ID")
	}
	_, err = r.DBWithContext(ctx).Queries().CompletePaymentCheckoutShutdown(ctx, dbsqlc.CompletePaymentCheckoutShutdownParams{
		ID: pgtype.UUID{Bytes: parsed, Valid: true}, CompletedAt: pgtype.Timestamptz{Time: completedAt, Valid: true},
	})
	return err
}

func (r *Repository) RecordConfigurationAudit(ctx context.Context, audit paymentconfiguration.Audit) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	return insertConfigurationAudit(ctx, r.DBWithContext(ctx).Queries(), audit)
}

func insertConfigurationAudit(ctx context.Context, q *dbsqlc.Queries, audit paymentconfiguration.Audit) error {
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

func storedStripeConfiguration(settings dbsqlc.PaymentStripeConfiguration, credential dbsqlc.PaymentStripeCredential) paymentconfiguration.StoredStripeConfiguration {
	stored := storedStripeCredential(credential)
	stored.ProductTaxCode = settings.ProductTaxCode
	stored.PurchasesEnabled = settings.PurchasesEnabled
	stored.Revision = settings.Revision
	stored.UpdatedBy = settings.UpdatedBy
	stored.UpdatedAt = dbutil.TimeValue(settings.UpdatedAt)
	return stored
}

func storedStripeCredential(row dbsqlc.PaymentStripeCredential) paymentconfiguration.StoredStripeConfiguration {
	return paymentconfiguration.StoredStripeConfiguration{
		CredentialID: pgUUIDString(row.ID), AccountID: row.AccountID, Mode: paymentports.StripeMode(row.Mode), ChargesEnabled: row.ChargesEnabled,
		PaymentMethodConfigurationID: PgTextString(row.PaymentMethodConfigurationID),
		StripeProductID:              row.StripeProductID, StripeWebhookEndpointID: row.StripeWebhookEndpointID,
		APIKeyCiphertext: row.ApiKeyCiphertext, WebhookSecretCiphertext: row.WebhookSecretCiphertext,
		SecretMaterialKind: row.SecretMaterialKind, CreatedBy: row.CreatedBy, CreatedAt: dbutil.TimeValue(row.CreatedAt),
	}
}

func paymentCheckoutShutdown(row dbsqlc.PaymentCheckoutShutdown) paymentconfiguration.PaymentCheckoutShutdown {
	value := paymentconfiguration.PaymentCheckoutShutdown{
		ID: pgUUIDString(row.ID), StripeCredentialID: pgUUIDString(row.StripeCredentialID),
		StripeAccountID: row.StripeAccountID, CutoffAt: dbutil.TimeValue(row.CutoffAt), CreatedAt: dbutil.TimeValue(row.CreatedAt),
	}
	if row.CompletedAt.Valid {
		completedAt := row.CompletedAt.Time
		value.CompletedAt = &completedAt
	}
	return value
}
