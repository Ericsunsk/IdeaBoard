package visionbilling

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/searchcatalog/application"
	"ideaboard/api/internal/searchcatalog/domain"
)

// VisionBillingQueue aliases the Platform-owned policy constant so inserts
// track the matrix with no copied literal.
const VisionBillingQueue = riverpolicy.RiverQueueVisionBilling

type visionBillingReconciliationArgs struct{}

func (visionBillingReconciliationArgs) Kind() string { return "vision_billing_reconciliation" }

type VisionBillingReconciliationWorker struct {
	river.WorkerDefaults[visionBillingReconciliationArgs]
	service application.ReconciliationRunner
}

func NewVisionBillingReconciliationWorker(service application.ReconciliationRunner) *VisionBillingReconciliationWorker {
	return &VisionBillingReconciliationWorker{service: service}
}

func (*VisionBillingReconciliationWorker) Timeout(*river.Job[visionBillingReconciliationArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutVisionBilling
}

func (w *VisionBillingReconciliationWorker) Work(ctx context.Context, job *river.Job[visionBillingReconciliationArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return domain.ErrVisionUnavailable
	}
	return w.service.ReconcilePending(ctx)
}

type visionBillingReconciliationSchedule struct{}

func (visionBillingReconciliationSchedule) Next(current time.Time) time.Time {
	return current.Add(5 * time.Minute)
}

func VisionBillingPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		visionBillingReconciliationSchedule{},
		func() (river.JobArgs, *river.InsertOpts) {
			return visionBillingReconciliationArgs{}, &river.InsertOpts{Queue: VisionBillingQueue, MaxAttempts: riverpolicy.RiverMaxAttemptsVisionBilling, Priority: 4}
		},
		&river.PeriodicJobOpts{ID: "vision-billing-reconciliation", RunOnStart: true},
	)}
}
