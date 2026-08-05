package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const PaymentCheckoutShutdownQueue = "payment_checkout_shutdown"

type PaymentCheckoutShutdownJobArgs struct {
	ShutdownID string `json:"shutdown_id"`
}

func (PaymentCheckoutShutdownJobArgs) Kind() string { return "payment_checkout_shutdown" }

type PaymentCheckoutShutdownScheduleJobArgs struct{}

func (PaymentCheckoutShutdownScheduleJobArgs) Kind() string {
	return "payment_checkout_shutdown_schedule"
}

type paymentCheckoutShutdownService interface {
	RunPaymentCheckoutShutdown(context.Context, string) error
	RunActivePaymentCheckoutShutdowns(context.Context) error
}

type PaymentCheckoutShutdownWorker struct {
	river.WorkerDefaults[PaymentCheckoutShutdownJobArgs]
	service paymentCheckoutShutdownService
}

type PaymentCheckoutShutdownScheduleWorker struct {
	river.WorkerDefaults[PaymentCheckoutShutdownScheduleJobArgs]
	service paymentCheckoutShutdownService
}

func NewPaymentCheckoutShutdownWorker(service paymentCheckoutShutdownService) *PaymentCheckoutShutdownWorker {
	return &PaymentCheckoutShutdownWorker{service: service}
}

func NewPaymentCheckoutShutdownScheduleWorker(service paymentCheckoutShutdownService) *PaymentCheckoutShutdownScheduleWorker {
	return &PaymentCheckoutShutdownScheduleWorker{service: service}
}

func (w *PaymentCheckoutShutdownWorker) Work(ctx context.Context, job *river.Job[PaymentCheckoutShutdownJobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.ShutdownID == "" {
		return fmt.Errorf("invalid payment Checkout shutdown job")
	}
	return w.service.RunPaymentCheckoutShutdown(ctx, job.Args.ShutdownID)
}

func (w *PaymentCheckoutShutdownScheduleWorker) Work(ctx context.Context, job *river.Job[PaymentCheckoutShutdownScheduleJobArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return fmt.Errorf("invalid payment Checkout shutdown schedule job")
	}
	return w.service.RunActivePaymentCheckoutShutdowns(ctx)
}

func PaymentCheckoutShutdownPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(paymentReconciliationInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PaymentCheckoutShutdownScheduleJobArgs{}, &river.InsertOpts{Queue: PaymentCheckoutShutdownQueue, MaxAttempts: 8, Priority: 1}
		},
		&river.PeriodicJobOpts{ID: "payment-checkout-shutdown-schedule", RunOnStart: true},
	)}
}

type checkoutShutdownEnqueuer struct {
	client *river.Client[pgx.Tx]
}

func NewPaymentCheckoutShutdownEnqueuer(client *river.Client[pgx.Tx]) PaymentCheckoutShutdownEnqueuer {
	return checkoutShutdownEnqueuer{client: client}
}

func (e checkoutShutdownEnqueuer) EnqueuePaymentCheckoutShutdown(ctx context.Context, shutdownID string) error {
	if e.client == nil || shutdownID == "" {
		return fmt.Errorf("payment Checkout shutdown queue unavailable")
	}
	_, err := e.client.Insert(ctx, PaymentCheckoutShutdownJobArgs{ShutdownID: shutdownID}, &river.InsertOpts{
		Queue: PaymentCheckoutShutdownQueue, MaxAttempts: 8, Priority: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	})
	return err
}

func (*PaymentCheckoutShutdownWorker) Timeout(*river.Job[PaymentCheckoutShutdownJobArgs]) time.Duration {
	return paymentReconciliationWorkerTimeout
}

func (*PaymentCheckoutShutdownScheduleWorker) Timeout(*river.Job[PaymentCheckoutShutdownScheduleJobArgs]) time.Duration {
	return paymentReconciliationWorkerTimeout
}

var _ PaymentCheckoutShutdownEnqueuer = checkoutShutdownEnqueuer{}
