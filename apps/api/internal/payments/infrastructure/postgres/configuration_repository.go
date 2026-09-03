package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/google/uuid"
)

func (r *Repository) ReplaceStripeConfiguration(ctx context.Context, expectedRevision int64, value paymentconfiguration.StoredStripeConfiguration, audit paymentconfiguration.Audit) (paymentconfiguration.StoredStripeConfiguration, *paymentconfiguration.PaymentCheckoutShutdown, error) {
	if r == nil || r.db == nil {
		return paymentconfiguration.StoredStripeConfiguration{}, nil, fmt.Errorf("payments repository: database unavailable")
	}
	var stored paymentconfiguration.StoredStripeConfiguration
	var shutdown *paymentconfiguration.PaymentCheckoutShutdown
	err := r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ *sql.Tx) error {
		previous, previousErr := q.GetPaymentStripeConfiguration(ctx)
		if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
			return fmt.Errorf("load previous Stripe configuration: %w", previousErr)
		}
		if previousErr == nil && previous.Revision != expectedRevision {
			return paymentcontracts.ErrStripeConfigurationChanged
		}
		if errors.Is(previousErr, sql.ErrNoRows) && expectedRevision != 0 {
			return paymentcontracts.ErrStripeConfigurationChanged
		}
		credentialID, err := uuid.Parse(value.CredentialID)
		if err != nil {
			credentialID = uuid.New()
			credential, insertErr := q.InsertPaymentStripeCredential(ctx, dbsqlc.InsertPaymentStripeCredentialParams{
				ID: credentialID, AccountID: value.AccountID, Mode: string(value.Mode), ChargesEnabled: value.ChargesEnabled,
				PaymentMethodConfigurationID: sql.NullString{String: value.PaymentMethodConfigurationID, Valid: true},
				StripeProductID:              value.StripeProductID, StripeWebhookEndpointID: value.StripeWebhookEndpointID,
				ApiKeyCiphertext: value.APIKeyCiphertext, WebhookSecretCiphertext: value.WebhookSecretCiphertext,
				SecretMaterialKind: value.SecretMaterialKind, CreatedBy: value.UpdatedBy,
			})
			if insertErr != nil {
				return fmt.Errorf("insert Stripe credential generation: %w", insertErr)
			}
			value.CredentialID = credential.ID.String()
			value.CreatedBy = credential.CreatedBy
			value.CreatedAt = credential.CreatedAt
		}
		row, err := q.ReplacePaymentStripeConfiguration(ctx, dbsqlc.ReplacePaymentStripeConfigurationParams{
			ActiveCredentialID: credentialID, ProductTaxCode: value.ProductTaxCode,
			PurchasesEnabled: value.PurchasesEnabled, UpdatedBy: value.UpdatedBy, ExpectedRevision: expectedRevision,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
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
		stored.UpdatedAt = row.UpdatedAt
		return nil
	})
	return stored, shutdown, err
}

func (r *Repository) CompletePaymentCheckoutShutdown(ctx context.Context, id string, completedAt time.Time) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid Checkout shutdown ID")
	}
	_, err = r.dbWithContext(ctx).queries().CompletePaymentCheckoutShutdown(ctx, dbsqlc.CompletePaymentCheckoutShutdownParams{
		ID: parsed, CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
	})
	return err
}

func (r *Repository) RecordConfigurationAudit(ctx context.Context, audit paymentconfiguration.Audit) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	return insertConfigurationAudit(ctx, r.dbWithContext(ctx).queries(), audit)
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
	stored.UpdatedAt = settings.UpdatedAt
	return stored
}

func storedStripeCredential(row dbsqlc.PaymentStripeCredential) paymentconfiguration.StoredStripeConfiguration {
	return paymentconfiguration.StoredStripeConfiguration{
		CredentialID: row.ID.String(), AccountID: row.AccountID, Mode: paymentports.StripeMode(row.Mode), ChargesEnabled: row.ChargesEnabled,
		PaymentMethodConfigurationID: row.PaymentMethodConfigurationID.String,
		StripeProductID:              row.StripeProductID, StripeWebhookEndpointID: row.StripeWebhookEndpointID,
		APIKeyCiphertext: row.ApiKeyCiphertext, WebhookSecretCiphertext: row.WebhookSecretCiphertext,
		SecretMaterialKind: row.SecretMaterialKind, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}
}

func paymentCheckoutShutdown(row dbsqlc.PaymentCheckoutShutdown) paymentconfiguration.PaymentCheckoutShutdown {
	value := paymentconfiguration.PaymentCheckoutShutdown{
		ID: row.ID.String(), StripeCredentialID: row.StripeCredentialID.String(),
		StripeAccountID: row.StripeAccountID, CutoffAt: row.CutoffAt, CreatedAt: row.CreatedAt,
	}
	if row.CompletedAt.Valid {
		completedAt := row.CompletedAt.Time
		value.CompletedAt = &completedAt
	}
	return value
}
