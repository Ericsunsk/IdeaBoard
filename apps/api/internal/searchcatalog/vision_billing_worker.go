package searchcatalog

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

const VisionBillingQueue = "vision_billing"

type visionBillingReconciliationArgs struct{}

func (visionBillingReconciliationArgs) Kind() string { return "vision_billing_reconciliation" }

type VisionBillingReconciliationWorker struct {
	river.WorkerDefaults[visionBillingReconciliationArgs]
	service *VisionBillingService
}

func NewVisionBillingReconciliationWorker(service *VisionBillingService) *VisionBillingReconciliationWorker {
	return &VisionBillingReconciliationWorker{service: service}
}

func (w *VisionBillingReconciliationWorker) Work(ctx context.Context, job *river.Job[visionBillingReconciliationArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return ErrVisionUnavailable
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
			return visionBillingReconciliationArgs{}, &river.InsertOpts{Queue: VisionBillingQueue, MaxAttempts: 5, Priority: 4}
		},
		&river.PeriodicJobOpts{ID: "vision-billing-reconciliation", RunOnStart: true},
	)}
}
