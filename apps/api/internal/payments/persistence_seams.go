package payments

import (
	"context"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

// StripeConfigurationStore owns active settings and immutable credential generations.
// Orders resolve provider access by generation, so publication never serializes
// unrelated Stripe work behind a process-wide lock.
type StripeConfigurationStore interface {
	GetStripeConfiguration(context.Context) (StoredStripeConfiguration, error)
	GetStripeCredential(context.Context, string) (StoredStripeConfiguration, error)
	GetStripeCredentialForOrder(context.Context, string) (StoredStripeConfiguration, error)
	GetStripeCredentialForOrderNumber(context.Context, string) (StoredStripeConfiguration, error)
	ListStripeCredentials(context.Context) ([]StoredStripeConfiguration, error)
	ReplaceStripeConfiguration(context.Context, int64, StoredStripeConfiguration, ConfigurationAudit) (StoredStripeConfiguration, *PaymentCheckoutShutdown, error)
	GetPaymentCheckoutShutdown(context.Context, string) (PaymentCheckoutShutdown, error)
	ListActivePaymentCheckoutShutdowns(context.Context) ([]PaymentCheckoutShutdown, error)
	ListPaymentOrdersForCheckoutShutdown(context.Context, PaymentCheckoutShutdown, int32) ([]PaymentOrder, error)
	CompletePaymentCheckoutShutdown(context.Context, string, time.Time) error
	RecordConfigurationAudit(context.Context, ConfigurationAudit) error
}

// StripeWebhookInboxStore is the durable, idempotent inbox for verified Stripe
// events. It intentionally contains no Checkout or configuration operations.
type StripeWebhookInboxStore interface {
	InsertStripeWebhookEvent(context.Context, StripeWebhookEvent, string, *time.Time) (bool, error)
	MarkStripeWebhookEventProcessed(context.Context, string, string, string, time.Time) error
}

// PaymentCheckoutStore owns Checkout intent, customer, order lookup, and
// session-bound per-user serialization. It does not transition paid balances.
type PaymentCheckoutStore interface {
	WithUserCheckoutLock(context.Context, int64, func(context.Context) error) error
	CreatePaymentOrderIntent(context.Context, CreatePaymentOrderIntentInput) (PaymentOrder, bool, error)
	GetPaymentCustomer(context.Context, int64, string, StripeMode) (PaymentCustomer, error)
	UpsertPaymentCustomer(context.Context, PaymentCustomer) (PaymentCustomer, error)
	BindPaymentOrderCheckout(context.Context, BindPaymentOrderCheckoutInput) (PaymentOrder, error)
	GetPaymentOrderByPublicToken(context.Context, int64, string) (PaymentOrder, error)
	GetPaymentOrderByID(context.Context, string) (PaymentOrder, error)
	GetPaymentOrderByCheckoutSession(context.Context, string, string) (PaymentOrder, error)
	ListActivePaymentOrders(context.Context, int64) ([]PaymentOrder, error)
	ListUserPendingPaymentOrders(context.Context, int64) ([]PaymentOrder, error)
	ListUnsettledPaymentOrders(context.Context) ([]PaymentOrder, error)
	listPaymentRechargeHistory(context.Context, int64, time.Time, *rechargeHistoryCursor, int) ([]dbsqlc.ListPaymentRechargeHistoryRow, error)
	listAdminPaymentOrders(context.Context, adminPaymentOrderFilters) (AdminPaymentOrderPage, error)
	getAdminPaymentOrder(context.Context, string) (AdminPaymentOrderDetail, []dbsqlc.ListAdminPaymentOrderWebhookEventsRow, error)
}

// PaymentOrderConvergenceStore is the minimum seam required by the Checkout
// lifecycle. It keeps order state transitions and Credit convergence together
// without exposing refund, dispute, or reconciliation persistence.
type PaymentOrderConvergenceStore interface {
	MarkPaymentOrderAsyncPending(context.Context, string) (PaymentOrder, error)
	MarkPaymentOrderFailed(context.Context, string) (PaymentOrder, error)
	MarkPaymentOrderExpired(context.Context, string) (PaymentOrder, bool, error)
	MarkPaymentOrderCancelled(context.Context, int64, string) (PaymentOrder, bool, error)
	ConvergePaidPaymentOrder(context.Context, PaidCheckoutFacts, time.Time) (PaymentConvergenceResult, error)
	CreditPaidPaymentOrder(context.Context, string, time.Time) (PaymentConvergenceResult, error)
}

// PaymentConvergenceStore owns monotonic order, refund, dispute, and recovery
// transitions. Its operations may write Credits, so callers must preserve the
// repository's transaction and replay semantics.
type PaymentConvergenceStore interface {
	PaymentOrderConvergenceStore
	ConvergePaymentDispute(context.Context, StripeDispute, time.Time) (PaymentDispute, bool, error)
	ListDuePaymentDisputes(context.Context, time.Time, int) ([]PaymentDispute, error)
	ListPaymentDisputesForOrder(context.Context, string) ([]PaymentDispute, error)
	ClaimDuePaymentOrderReconciliations(context.Context, time.Time) ([]PaymentReconciliationClaim, error)
	CompletePaymentOrderReconciliationClaim(context.Context, PaymentReconciliationClaim, PaymentReconciliationOutcome) (bool, error)
	ListPaymentReconciliationAnomalies(context.Context, int32) ([]PaymentReconciliationAnomaly, error)
	CreateExternalPaymentRefund(context.Context, StripeRefund, time.Time) (PaymentRefund, error)
	RecordPaymentRefundAnomaly(context.Context, StripeRefund, string, time.Time) (int64, error)
	CreatePendingPaymentRefund(context.Context, CreatePendingPaymentRefundInput) (PaymentRefund, bool, error)
	MarkPaymentRefundProviderResult(context.Context, string, string, string, time.Time) (PaymentRefund, error)
	GetPaymentRefundByStripeID(context.Context, string) (PaymentRefund, error)
	ListPendingPaymentRefunds(context.Context, time.Time, int) ([]PaymentRefund, error)
	ListPaymentRefundsForOrder(context.Context, string) ([]PaymentRefund, error)
	MarkPaymentRefundFailed(context.Context, string, string, time.Time) (PaymentRefund, error)
	MarkPaymentRefundSucceeded(context.Context, string, time.Time) (PaymentRefund, error)
}

var (
	_ StripeConfigurationStore     = (*Repository)(nil)
	_ StripeWebhookInboxStore      = (*Repository)(nil)
	_ PaymentCheckoutStore         = (*Repository)(nil)
	_ PaymentOrderConvergenceStore = (*Repository)(nil)
	_ PaymentConvergenceStore      = (*Repository)(nil)
)
