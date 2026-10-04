package persistence

import (
	"context"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type StripeConfigurationStore interface {
	GetStripeConfiguration(context.Context) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredential(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredentialForOrder(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredentialForOrderNumber(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	ListStripeCredentials(context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error)
	ReplaceStripeConfiguration(context.Context, int64, paymentconfiguration.StoredStripeConfiguration, paymentconfiguration.Audit) (paymentconfiguration.StoredStripeConfiguration, *paymentconfiguration.PaymentCheckoutShutdown, error)
	GetPaymentCheckoutShutdown(context.Context, string) (paymentconfiguration.PaymentCheckoutShutdown, error)
	ListActivePaymentCheckoutShutdowns(context.Context) ([]paymentconfiguration.PaymentCheckoutShutdown, error)
	ListPaymentOrdersForCheckoutShutdown(context.Context, paymentconfiguration.PaymentCheckoutShutdown, int32) ([]paymentsdomain.PaymentOrder, error)
	CompletePaymentCheckoutShutdown(context.Context, string, time.Time) error
	RecordConfigurationAudit(context.Context, paymentconfiguration.Audit) error
}

type StripeWebhookInboxStore interface {
	GetStripeWebhookEvent(context.Context, string) (string, error)
	ClaimStripeWebhookEvent(context.Context, string) (bool, error)
	InsertStripeWebhookEvent(context.Context, paymentports.StripeWebhookEvent, string, *time.Time) (bool, error)
	MarkStripeWebhookEventProcessed(context.Context, string, string, string, time.Time) error
}

type PaymentCheckoutStore interface {
	WithUserCheckoutLock(context.Context, int64, func(context.Context) error) error
	CreatePaymentOrderIntent(context.Context, paymentsdomain.CreatePaymentOrderIntentInput) (paymentsdomain.PaymentOrder, bool, error)
	GetPaymentCustomer(context.Context, int64, string, paymentports.StripeMode) (paymentcheckout.PaymentCustomer, error)
	UpsertPaymentCustomer(context.Context, paymentcheckout.PaymentCustomer) (paymentcheckout.PaymentCustomer, error)
	BindPaymentOrderCheckout(context.Context, paymentsdomain.BindPaymentOrderCheckoutInput) (paymentsdomain.PaymentOrder, error)
	GetPaymentOrderByPublicToken(context.Context, int64, string) (paymentsdomain.PaymentOrder, error)
	GetPaymentOrderByID(context.Context, string) (paymentsdomain.PaymentOrder, error)
	GetPaymentOrderByCheckoutSession(context.Context, string, string) (paymentsdomain.PaymentOrder, error)
	ListActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
	ListUserPendingPaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
	ListUnsettledPaymentOrders(context.Context) ([]paymentsdomain.PaymentOrder, error)
	ListPaymentRechargeHistory(context.Context, int64, time.Time, *paymentreadmodel.RechargeHistoryCursor, int) ([]dbsqlc.ListPaymentRechargeHistoryRow, error)
	ListAdminPaymentOrders(context.Context, paymentreadmodel.AdminPaymentOrderFilters) (paymentcontracts.AdminPaymentOrderPage, error)
	GetAdminPaymentOrder(context.Context, string) (paymentreadmodel.AdminPaymentOrderDetailRecord, []dbsqlc.ListAdminPaymentOrderWebhookEventsRow, error)
}

type PaymentOrderConvergenceStore interface {
	MarkPaymentOrderAsyncPending(context.Context, string) (paymentsdomain.PaymentOrder, error)
	MarkPaymentOrderFailed(context.Context, string) (paymentsdomain.PaymentOrder, error)
	MarkPaymentOrderExpired(context.Context, string) (paymentsdomain.PaymentOrder, bool, error)
	MarkPaymentOrderCancelled(context.Context, int64, string) (paymentsdomain.PaymentOrder, bool, error)
	ConvergePaidPaymentOrder(context.Context, paymentsdomain.PaidCheckoutFacts, time.Time) (paymentsdomain.PaymentConvergenceResult, error)
	CreditPaidPaymentOrder(context.Context, string, time.Time) (paymentsdomain.PaymentConvergenceResult, error)
}

type PaymentConvergenceStore interface {
	PaymentOrderConvergenceStore
	ConvergePaymentDispute(context.Context, paymentports.StripeDispute, time.Time) (paymentsdomain.PaymentDispute, bool, error)
	ListDuePaymentDisputes(context.Context, time.Time, int) ([]paymentsdomain.PaymentDispute, error)
	ListPaymentDisputesForOrder(context.Context, string) ([]paymentsdomain.PaymentDispute, error)
	ClaimDuePaymentOrderReconciliations(context.Context, time.Time) ([]paymentreconciliation.Claim, error)
	CompletePaymentOrderReconciliationClaim(context.Context, paymentreconciliation.Claim, paymentreconciliation.Outcome) (bool, error)
	ListPaymentReconciliationAnomalies(context.Context, int32) ([]paymentreconciliation.Anomaly, error)
	CreateExternalPaymentRefund(context.Context, paymentports.StripeRefund, time.Time) (paymentsdomain.PaymentRefund, error)
	RecordPaymentRefundAnomaly(context.Context, paymentports.StripeRefund, string, time.Time) (int64, error)
	CreatePendingPaymentRefund(context.Context, paymentrefund.CreatePendingPaymentRefundInput) (paymentsdomain.PaymentRefund, bool, error)
	MarkPaymentRefundProviderResult(context.Context, string, string, string, time.Time) (paymentsdomain.PaymentRefund, error)
	GetPaymentRefundByStripeID(context.Context, string) (paymentsdomain.PaymentRefund, error)
	ListPendingPaymentRefunds(context.Context, time.Time, int) ([]paymentsdomain.PaymentRefund, error)
	ListPaymentRefundsForOrder(context.Context, string) ([]paymentsdomain.PaymentRefund, error)
	MarkPaymentRefundFailed(context.Context, string, string, time.Time) (paymentsdomain.PaymentRefund, error)
	MarkPaymentRefundSucceeded(context.Context, string, time.Time) (paymentsdomain.PaymentRefund, error)
}
