package billing

import (
	"context"
	"fmt"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const (
	PaymentReconciliationQueue     = "payment_reconciliation"
	PaymentCheckoutShutdownQueue   = "payment_checkout_shutdown"
	paymentBackgroundWorkerTimeout = 4 * time.Minute
)

type paymentReconciliationScheduleJobArgs struct{}

func (paymentReconciliationScheduleJobArgs) Kind() string { return "payment_reconciliation_schedule" }

type paymentReconciliationService interface {
	ReconcileDuePaymentOrders(context.Context) error
}

type PaymentReconciliationWorker struct {
	river.WorkerDefaults[paymentReconciliationScheduleJobArgs]
	service paymentReconciliationService
}

func NewPaymentReconciliationWorker(service paymentReconciliationService) *PaymentReconciliationWorker {
	return &PaymentReconciliationWorker{service: service}
}

func (w *PaymentReconciliationWorker) Work(ctx context.Context, job *river.Job[paymentReconciliationScheduleJobArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return fmt.Errorf("invalid payment reconciliation job")
	}
	return w.service.ReconcileDuePaymentOrders(ctx)
}

func PaymentReconciliationPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(paymentreconciliation.Interval),
		func() (river.JobArgs, *river.InsertOpts) {
			return paymentReconciliationScheduleJobArgs{}, &river.InsertOpts{
				Queue: PaymentReconciliationQueue, MaxAttempts: 8, Priority: 4,
			}
		},
		&river.PeriodicJobOpts{ID: "payment-reconciliation-schedule", RunOnStart: true},
	)}
}

func (*PaymentReconciliationWorker) Timeout(*river.Job[paymentReconciliationScheduleJobArgs]) time.Duration {
	return paymentBackgroundWorkerTimeout
}

type paymentCheckoutShutdownJobArgs struct {
	ShutdownID string `json:"shutdown_id"`
}

func (paymentCheckoutShutdownJobArgs) Kind() string { return "payment_checkout_shutdown" }

type paymentCheckoutShutdownScheduleJobArgs struct{}

func (paymentCheckoutShutdownScheduleJobArgs) Kind() string {
	return "payment_checkout_shutdown_schedule"
}

type paymentCheckoutShutdownService interface {
	RunPaymentCheckoutShutdown(context.Context, string) error
	RunActivePaymentCheckoutShutdowns(context.Context) error
}

type PaymentCheckoutShutdownWorker struct {
	river.WorkerDefaults[paymentCheckoutShutdownJobArgs]
	service paymentCheckoutShutdownService
}

type PaymentCheckoutShutdownScheduleWorker struct {
	river.WorkerDefaults[paymentCheckoutShutdownScheduleJobArgs]
	service paymentCheckoutShutdownService
}

func NewPaymentCheckoutShutdownWorker(service paymentCheckoutShutdownService) *PaymentCheckoutShutdownWorker {
	return &PaymentCheckoutShutdownWorker{service: service}
}

func NewPaymentCheckoutShutdownScheduleWorker(service paymentCheckoutShutdownService) *PaymentCheckoutShutdownScheduleWorker {
	return &PaymentCheckoutShutdownScheduleWorker{service: service}
}

func (w *PaymentCheckoutShutdownWorker) Work(ctx context.Context, job *river.Job[paymentCheckoutShutdownJobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.ShutdownID == "" {
		return fmt.Errorf("invalid payment Checkout shutdown job")
	}
	return w.service.RunPaymentCheckoutShutdown(ctx, job.Args.ShutdownID)
}

func (w *PaymentCheckoutShutdownScheduleWorker) Work(ctx context.Context, job *river.Job[paymentCheckoutShutdownScheduleJobArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return fmt.Errorf("invalid payment Checkout shutdown schedule job")
	}
	return w.service.RunActivePaymentCheckoutShutdowns(ctx)
}

func PaymentCheckoutShutdownPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(paymentreconciliation.Interval),
		func() (river.JobArgs, *river.InsertOpts) {
			return paymentCheckoutShutdownScheduleJobArgs{}, &river.InsertOpts{
				Queue: PaymentCheckoutShutdownQueue, MaxAttempts: 8, Priority: 1,
			}
		},
		&river.PeriodicJobOpts{ID: "payment-checkout-shutdown-schedule", RunOnStart: true},
	)}
}

type PaymentCheckoutShutdownDispatcher struct {
	client *river.Client[pgx.Tx]
}

func NewPaymentCheckoutShutdownDispatcher(client *river.Client[pgx.Tx]) (paymentconfiguration.ShutdownEnqueuer, error) {
	if client == nil {
		return nil, fmt.Errorf("configure Payment Checkout shutdown dispatcher: River client is nil")
	}
	return PaymentCheckoutShutdownDispatcher{client: client}, nil
}

func (d PaymentCheckoutShutdownDispatcher) EnqueuePaymentCheckoutShutdown(ctx context.Context, shutdownID string) error {
	if d.client == nil || shutdownID == "" {
		return fmt.Errorf("payment Checkout shutdown queue unavailable")
	}
	_, err := d.client.Insert(ctx, paymentCheckoutShutdownJobArgs{ShutdownID: shutdownID}, &river.InsertOpts{
		Queue: PaymentCheckoutShutdownQueue, MaxAttempts: 8, Priority: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	})
	return err
}

func (*PaymentCheckoutShutdownWorker) Timeout(*river.Job[paymentCheckoutShutdownJobArgs]) time.Duration {
	return paymentBackgroundWorkerTimeout
}

func (*PaymentCheckoutShutdownScheduleWorker) Timeout(*river.Job[paymentCheckoutShutdownScheduleJobArgs]) time.Duration {
	return paymentBackgroundWorkerTimeout
}
