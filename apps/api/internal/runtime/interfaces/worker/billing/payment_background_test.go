package billing

import (
	"context"
	"errors"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	"testing"

	"github.com/jackc/pgx/v5"

	"ideaboard/api/internal/testdb"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestPaymentReconciliationWorkerPropagatesTransientFailure(t *testing.T) {
	wantErr := errors.New("Stripe unavailable")
	service := &paymentBackgroundServiceStub{reconciliationErr: wantErr}
	worker := NewPaymentReconciliationWorker(service)
	job := &river.Job[paymentReconciliationScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 8},
	}
	if err := worker.Work(t.Context(), job); !errors.Is(err, wantErr) {
		t.Fatalf("Work() error = %v, want reconciliation error", err)
	}
	if service.reconciliationCalls != 1 {
		t.Fatalf("ReconcileDuePaymentOrders() calls = %d, want 1", service.reconciliationCalls)
	}
	if jobs := PaymentReconciliationPeriodicJobs(); len(jobs) != 1 {
		t.Fatalf("periodic job count = %d, want 1", len(jobs))
	}
}

func TestPaymentCheckoutShutdownWorkersDispatchSpecificAndRecoveryWork(t *testing.T) {
	service := &paymentBackgroundServiceStub{}
	worker := NewPaymentCheckoutShutdownWorker(service)
	job := &river.Job[paymentCheckoutShutdownJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 8},
		Args:   paymentCheckoutShutdownJobArgs{ShutdownID: "shutdown-42"},
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("shutdown Work() error = %v", err)
	}
	if service.shutdownID != "shutdown-42" {
		t.Fatalf("RunPaymentCheckoutShutdown() ID = %q", service.shutdownID)
	}

	scheduleWorker := NewPaymentCheckoutShutdownScheduleWorker(service)
	scheduleJob := &river.Job[paymentCheckoutShutdownScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 8},
	}
	if err := scheduleWorker.Work(t.Context(), scheduleJob); err != nil {
		t.Fatalf("schedule Work() error = %v", err)
	}
	if service.activeShutdownCalls != 1 {
		t.Fatalf("RunActivePaymentCheckoutShutdowns() calls = %d, want 1", service.activeShutdownCalls)
	}
	if jobs := PaymentCheckoutShutdownPeriodicJobs(); len(jobs) != 1 {
		t.Fatalf("shutdown periodic job count = %d, want 1", len(jobs))
	}
}

type paymentBackgroundServiceStub struct {
	reconciliationCalls int
	reconciliationErr   error
	shutdownID          string
	activeShutdownCalls int
}

func (s *paymentBackgroundServiceStub) ReconcileDuePaymentOrders(context.Context) error {
	s.reconciliationCalls++
	return s.reconciliationErr
}

func (s *paymentBackgroundServiceStub) RunPaymentCheckoutShutdown(_ context.Context, shutdownID string) error {
	s.shutdownID = shutdownID
	return nil
}

func (s *paymentBackgroundServiceStub) RunActivePaymentCheckoutShutdowns(context.Context) error {
	s.activeShutdownCalls++
	return nil
}

func TestPaymentCheckoutShutdownDispatcherUsesRiverProducer(t *testing.T) {
	pool := testdb.Open(t)
	if err := workerriver.MigrateRiver(t.Context(), pool); err != nil {
		t.Fatalf("MigrateRiver() error = %v", err)
	}
	producer, err := workerriver.NewRiverProducer(pool)
	if err != nil {
		t.Fatalf("NewRiverProducer() error = %v", err)
	}
	dispatcher, err := NewPaymentCheckoutShutdownDispatcher(producer)
	if err != nil {
		t.Fatalf("NewPaymentCheckoutShutdownDispatcher() error = %v", err)
	}
	before := countPaymentBackgroundJobs(t, pool, "payment_checkout_shutdown")
	if err := dispatcher.EnqueuePaymentCheckoutShutdown(t.Context(), "shutdown-integration"); err != nil {
		t.Fatalf("EnqueuePaymentCheckoutShutdown() error = %v", err)
	}
	if got := countPaymentBackgroundJobs(t, pool, "payment_checkout_shutdown"); got != before+1 {
		t.Fatalf("shutdown job count = %d, want %d", got, before+1)
	}
}

func countPaymentBackgroundJobs(t testing.TB, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, kind string) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM river_job WHERE kind = $1`, kind).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
