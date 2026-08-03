package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
)

const PaymentReconciliationQueue = "payment_reconciliation"

const paymentReconciliationWorkerTimeout = 4 * time.Minute

type PaymentReconciliationScheduleJobArgs struct{}

func (PaymentReconciliationScheduleJobArgs) Kind() string { return "payment_reconciliation_schedule" }

type paymentReconciliationService interface {
	ReconcileDuePaymentOrders(context.Context) error
}

type PaymentReconciliationWorker struct {
	river.WorkerDefaults[PaymentReconciliationScheduleJobArgs]
	service paymentReconciliationService
}

func NewPaymentReconciliationWorker(service paymentReconciliationService) *PaymentReconciliationWorker {
	return &PaymentReconciliationWorker{service: service}
}

func (w *PaymentReconciliationWorker) Work(ctx context.Context, job *river.Job[PaymentReconciliationScheduleJobArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return fmt.Errorf("invalid payment reconciliation job")
	}
	return w.service.ReconcileDuePaymentOrders(ctx)
}

func PaymentReconciliationPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(paymentReconciliationInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PaymentReconciliationScheduleJobArgs{}, &river.InsertOpts{
				Queue: PaymentReconciliationQueue, MaxAttempts: 8, Priority: 4,
			}
		},
		&river.PeriodicJobOpts{ID: "payment-reconciliation-schedule", RunOnStart: true},
	)}
}

func (*PaymentReconciliationWorker) Timeout(*river.Job[PaymentReconciliationScheduleJobArgs]) time.Duration {
	return paymentReconciliationWorkerTimeout
}
