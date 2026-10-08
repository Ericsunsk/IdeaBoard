package tests

import (
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentcredentials "ideaboard/api/internal/payments/application/credentials"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/testdb"

	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
)

type countingRefundPublisher struct {
	mu     sync.Mutex
	counts int
}

func (f *countingRefundPublisher) Publish(_ int64, _ paymentports.PaymentEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts++
}

func (f *countingRefundPublisher) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts
}

func TestPaymentRefundPublishDefersToPostCommit(t *testing.T) {
	db, _ := testdb.OpenWithPool(t)
	repo := paymentpostgres.NewRepository(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, db, repo, now.Add(-time.Hour))
	resolver, err := paymentcredentials.NewResolver(repo)
	if err != nil {
		t.Fatalf("credential resolver: %v", err)
	}

	newService := func(events paymentports.PaymentEventPublisher, stripeID string) *paymentrefund.Service {
		gateway := &refundGatewayFake{
			createResponse: paymentports.StripeRefund{ID: stripeID, Status: "succeeded"},
		}
		svc, err := paymentrefund.NewService(repo, gateway, events, func() time.Time { return now }, resolver)
		if err != nil {
			t.Fatalf("refund service: %v", err)
		}
		return svc
	}

	// Without a registry the legacy synchronous behavior stays:
	// reservation + settlement publish immediately.
	syncEvents := &countingRefundPublisher{}
	syncSvc := newService(syncEvents, "re_postcommit_sync")
	syncRefund, err := syncSvc.CreatePaymentRefund(t.Context(), paymentcontracts.CreatePaymentRefundCommand{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "refund-postcommit-sync", Reason: "unused Ideas",
	})
	if err != nil || syncRefund.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("sync CreatePaymentRefund() refund=%+v error=%v", syncRefund, err)
	}
	if got := syncEvents.total(); got != 2 {
		t.Fatalf("sync publishes = %d, want 2 (reservation + settlement)", got)
	}

	deferredEvents := &countingRefundPublisher{}
	deferredSvc := newService(deferredEvents, "re_postcommit_deferred")
	carrier := dbutil.ContextWithAmbientPostCommit(t.Context())
	deferredRefund, err := deferredSvc.CreatePaymentRefund(carrier, paymentcontracts.CreatePaymentRefundCommand{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 50,
		IdempotencyKey: "refund-postcommit-deferred", Reason: "unused Ideas",
	})
	if err != nil {
		// order already has 100 refunded from sync leg (500 total, 100 used),
		// 50 more fits; any error here is a test failure.
		t.Fatalf("deferred CreatePaymentRefund() error=%v", err)
	}
	if deferredRefund.Status != paymentsdomain.PaymentRefundSucceeded && deferredRefund.Status != paymentsdomain.PaymentRefundPending {
		t.Fatalf("deferred refund status = %q", deferredRefund.Status)
	}
	if got := deferredEvents.total(); got != 0 {
		t.Fatalf("deferred publishes before commit = %d, want 0 (nothing may publish before guard commit)", got)
	}
	dbutil.RunAmbientPostCommit(carrier)
	if got := deferredEvents.total(); got != 2 {
		t.Fatalf("deferred publishes after commit = %d, want 2", got)
	}
	// Running hooks twice never double-publishes.
	dbutil.RunAmbientPostCommit(carrier)
	if got := deferredEvents.total(); got != 2 {
		t.Fatalf("publishes after second run = %d, want still 2 (exactly-once)", got)
	}
}
