package tests

import (
	"ideaboard/api/internal/credits/domain"
	payments "ideaboard/api/internal/payments"

	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/db/dbutil"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentcredentials "ideaboard/api/internal/payments/application/credentials"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
)

type refundGatewayFake struct {
	mu             sync.Mutex
	createResponse paymentports.StripeRefund
	beforeCreate   func(paymentports.CreateStripeRefundInput) error
	retrieve       map[string]paymentports.StripeRefund
	createCalls    int
	createKeys     []string
	retrieveCalls  int
}

func (f *refundGatewayFake) CreateRefund(_ context.Context, input paymentports.CreateStripeRefundInput) (paymentports.StripeRefund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	f.createKeys = append(f.createKeys, input.IdempotencyKey)
	if f.beforeCreate != nil {
		if err := f.beforeCreate(input); err != nil {
			return paymentports.StripeRefund{}, err
		}
	}
	result := f.createResponse
	if result.ChargeID == "" {
		result.ChargeID = input.ChargeID
	}
	if result.AmountMinor == 0 {
		result.AmountMinor = input.AmountMinor
	}
	if result.Currency == "" {
		result.Currency = input.Currency
	}
	return result, nil
}

func (f *refundGatewayFake) RetrieveRefund(_ context.Context, _ string, refundID string) (paymentports.StripeRefund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retrieveCalls++
	return f.retrieve[refundID], nil
}

func TestPaymentRefundReservesIdeasIdempotentlyAndReleasesOnFailure(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, pool, repo, now.Add(-13*24*time.Hour))

	refund, replayed, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "refund-integration-1", Now: now,
	})
	if err != nil || replayed {
		t.Fatalf("CreatePendingPaymentRefund() refund=%+v replayed=%v error=%v", refund, replayed, err)
	}
	var refundReason string
	if err := pool.QueryRow(t.Context(), `SELECT reason FROM payment_refunds WHERE id = $1::uuid`, refund.ID).Scan(&refundReason); err != nil || refundReason != "" {
		t.Fatalf("Admin refund reason = %q, %v", refundReason, err)
	}
	if refund.AmountMinor != 200 || refund.Currency != "usd" || refund.Status != paymentsdomain.PaymentRefundPending {
		t.Fatalf("refund = %+v", refund)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 300 || wallet.Reserved != 200 {
		t.Fatalf("reserved Wallet=%+v error=%v", wallet, err)
	}
	_, replayed, err = repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "refund-integration-1", Now: now,
	})
	if err != nil || !replayed {
		t.Fatalf("refund replay replayed=%v error=%v", replayed, err)
	}
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 199,
		IdempotencyKey: "refund-integration-1", Now: now,
	}); !errors.Is(err, paymentcontracts.ErrRefundIdempotency) {
		t.Fatalf("refund idempotency conflict error = %v", err)
	}
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 301,
		IdempotencyKey: "refund-integration-over", Now: now,
	}); !errors.Is(err, paymentcontracts.ErrRefundAmountExceeds) {
		t.Fatalf("over-limit refund error = %v", err)
	}
	failed, err := repo.MarkPaymentRefundFailed(t.Context(), refund.ID, "provider rejected", now.Add(time.Minute))
	if err != nil || failed.Status != paymentsdomain.PaymentRefundFailed {
		t.Fatalf("MarkPaymentRefundFailed() refund=%+v error=%v", failed, err)
	}
	wallet, err = credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 500 || wallet.Reserved != 0 {
		t.Fatalf("released Wallet=%+v error=%v", wallet, err)
	}
	second, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "refund-integration-2", Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	succeeded, err := repo.MarkPaymentRefundSucceeded(t.Context(), second.ID, now.Add(3*time.Minute))
	if err != nil || succeeded.Status != paymentsdomain.PaymentRefundSucceeded || succeeded.CreditTransactionID == "" {
		t.Fatalf("MarkPaymentRefundSucceeded() refund=%+v error=%v", succeeded, err)
	}
	wallet, err = credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 300 || wallet.Reserved != 0 {
		t.Fatalf("settled Wallet=%+v error=%v", wallet, err)
	}
	var commandType string
	if err := pool.QueryRow(t.Context(), `SELECT command_type FROM credit_transactions WHERE transaction_id = $1`, succeeded.CreditTransactionID).Scan(&commandType); err != nil || commandType != "purchase_refund" {
		t.Fatalf("purchase_refund transaction type=%q error=%v", commandType, err)
	}
}

func TestPaymentRefundReservationCommitsBeforeAmbientControlTransaction(t *testing.T) {
	db := testdb.Open(t)
	repo := newTestPaymentStores(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, db, repo, now.Add(-time.Hour))

	carrier, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin control transaction: %v", err)
	}
	carrierCtx := dbutil.ContextWithAmbientTx(t.Context(), carrier)
	refund, replayed, err := repo.CreatePendingPaymentRefund(carrierCtx, paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "refund-ambient-commit", Now: now,
	})
	if err != nil || replayed {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("CreatePendingPaymentRefund() refund=%+v replayed=%v error=%v", refund, replayed, err)
	}
	// A provider call uses a fresh connection. The reservation and its
	// PurchaseRefundHold must already be visible there while the guard tx is
	// still open; otherwise a later Stripe result could not converge safely.
	var status string
	if err := db.QueryRow(t.Context(), `SELECT status FROM payment_refunds WHERE id = $1::uuid`, refund.ID).Scan(&status); err != nil {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("read committed refund reservation: %v", err)
	}
	if status != string(paymentsdomain.PaymentRefundPending) {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("refund status = %q, want pending", status)
	}
	if _, err := repo.MarkPaymentRefundProviderResult(carrierCtx, refund.ID, "re_ambient_commit", "succeeded", now.Add(time.Minute)); err != nil {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("record provider result: %v", err)
	}
	settled, err := repo.MarkPaymentRefundSucceeded(carrierCtx, refund.ID, now.Add(2*time.Minute))
	if err != nil || settled.Status != paymentsdomain.PaymentRefundSucceeded {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("settle refund=%+v error=%v", settled, err)
	}
	if err := carrier.Rollback(t.Context()); err != nil {
		t.Fatalf("rollback control transaction: %v", err)
	}
	if err := db.QueryRow(t.Context(), `SELECT status FROM payment_refunds WHERE id = $1::uuid`, refund.ID).Scan(&status); err != nil {
		t.Fatalf("read settled refund after control rollback: %v", err)
	}
	if status != string(paymentsdomain.PaymentRefundSucceeded) {
		t.Fatalf("refund status after control rollback = %q, want succeeded", status)
	}
	var holds int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE transaction_id = $1 AND command_type = 'hold' AND source = 'stripe_refund_hold' AND business_reference_id = $2`, refund.CreditHoldTransactionID, refund.ID).Scan(&holds); err != nil {
		t.Fatalf("read committed refund hold: %v", err)
	}
	if holds != 1 {
		t.Fatalf("refund hold rows after control rollback = %d, want 1", holds)
	}
}

func TestPaymentRefundServiceChainCommitsBeforeStripeUnderAmbientControlTx(t *testing.T) {
	db := testdb.Open(t)
	repo := newTestPaymentStores(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, db, repo, now.Add(-time.Hour))
	resolver, err := paymentcredentials.NewResolver(repo)
	if err != nil {
		t.Fatalf("credential resolver: %v", err)
	}
	providerSawCommittedReservation := false
	gateway := &refundGatewayFake{
		createResponse: paymentports.StripeRefund{ID: "re_service_chain", Status: "succeeded"},
		beforeCreate: func(input paymentports.CreateStripeRefundInput) error {
			refundID := strings.TrimPrefix(input.IdempotencyKey, "ideaboard-refund-")
			var status string
			if err := db.QueryRow(t.Context(), `SELECT status FROM payment_refunds WHERE id = $1::uuid`, refundID).Scan(&status); err != nil {
				return err
			}
			providerSawCommittedReservation = status == string(paymentsdomain.PaymentRefundPending)
			return nil
		},
	}
	service, err := paymentrefund.NewService(repo, gateway, nil, func() time.Time { return now }, resolver)
	if err != nil {
		t.Fatalf("refund service: %v", err)
	}

	// Production Control wiring carries an open guard transaction while the
	// refund service performs reservation, Stripe I/O, and settlement.
	carrier, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin control transaction: %v", err)
	}
	carrierCtx := dbutil.ContextWithAmbientTx(t.Context(), carrier)
	command := paymentcontracts.CreatePaymentRefundCommand{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "refund-service-chain",
	}
	first, err := service.CreatePaymentRefund(carrierCtx, command)
	if err != nil || first.Status != paymentsdomain.PaymentRefundSucceeded {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("CreatePaymentRefund() refund=%+v error=%v", first, err)
	}
	// The Stripe request must carry the durable refund identity so a retry
	// reuses the same provider-side idempotency key.
	if gateway.createCalls != 1 || len(gateway.createKeys) != 1 || gateway.createKeys[0] != "ideaboard-refund-"+first.ID {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("stripe idempotency keys=%v calls=%d", gateway.createKeys, gateway.createCalls)
	}
	if !providerSawCommittedReservation {
		_ = carrier.Rollback(t.Context())
		t.Fatal("Stripe gateway observed no committed pending refund reservation")
	}
	// The reservation phase audit commits with the hold, before Stripe I/O.
	var reservedAudits int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM payment_admin_actions WHERE target_id = $1 AND metadata->>'phase' = 'reserved'`, first.ID).Scan(&reservedAudits); err != nil || reservedAudits != 1 {
		_ = carrier.Rollback(t.Context())
		t.Fatalf("reserved phase audits=%d error=%v", reservedAudits, err)
	}
	// A later guard audit failure must not roll back cash-side effects.
	if err := carrier.Rollback(t.Context()); err != nil {
		t.Fatalf("rollback control transaction: %v", err)
	}
	settled, err := repo.GetPaymentRefundByID(t.Context(), first.ID)
	if err != nil || settled.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("settled refund after control rollback=%+v error=%v", settled, err)
	}
	// Retry of the same business request replays the durable identity and
	// does not reach Stripe again: no second cash refund is possible.
	retry, err := service.CreatePaymentRefund(t.Context(), command)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("refund retry id=%q want %q error=%v", retry.ID, first.ID, err)
	}
	if gateway.createCalls != 1 {
		t.Fatalf("stripe create calls after retry=%d, want 1", gateway.createCalls)
	}
}

func TestExternalDashboardRefundUsesTypedHoldUnderRestriction(t *testing.T) {
	db := testdb.Open(t)
	repo := newTestPaymentStores(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	createCreditedRefundOrder(t, db, repo, now.Add(-time.Hour))
	// A spending restriction blocks the generic spending Hold but must not
	// block legitimate Dashboard refund recovery via PurchaseRefundHold.
	if _, err := credits.NewService(db).RestrictWallet(t.Context(), ports.RestrictWalletCommand{
		UserID: 42, Type: "payment_dispute", Source: "stripe_dispute",
		RelatedObject: domain.BusinessReference{Type: "payment_dispute", ID: "dispute-897"},
		Reason:        "open payment dispute", Actor: domain.Actor{Type: domain.ActorService, ID: "payments"},
	}); err != nil {
		t.Fatalf("RestrictWallet: %v", err)
	}
	provider := paymentports.StripeRefund{ID: "re_dash_897", ChargeID: "ch_refund", AmountMinor: 100, Currency: "usd", Status: "succeeded"}
	refund, err := repo.CreateExternalPaymentRefund(t.Context(), provider, now)
	if err != nil || refund.Status != paymentsdomain.PaymentRefundPending {
		t.Fatalf("CreateExternalPaymentRefund() refund=%+v error=%v", refund, err)
	}
	var typedHolds int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE transaction_id = $1 AND command_type = 'hold' AND source = 'stripe_refund_hold' AND business_reference_id = $2`, refund.CreditHoldTransactionID, refund.ID).Scan(&typedHolds); err != nil || typedHolds != 1 {
		t.Fatalf("typed refund hold rows=%d error=%v", typedHolds, err)
	}
	// A duplicate provider delivery replays the durable fact without a
	// second reservation.
	duplicate, err := repo.CreateExternalPaymentRefund(t.Context(), provider, now.Add(time.Minute))
	if err != nil || duplicate.ID != refund.ID {
		t.Fatalf("duplicate dashboard refund id=%q want %q error=%v", duplicate.ID, refund.ID, err)
	}
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE business_reference_type = 'payment_refund' AND business_reference_id = $1 AND command_type = 'hold'`, refund.ID).Scan(&typedHolds); err != nil || typedHolds != 1 {
		t.Fatalf("hold rows after duplicate=%d error=%v", typedHolds, err)
	}
	settled, err := repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now.Add(2*time.Minute))
	if err != nil || settled.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("MarkPaymentRefundSucceeded() refund=%+v error=%v", settled, err)
	}
	wallet, err := credits.NewService(db).GetWallet(t.Context(), 42)
	if err != nil || wallet.Reserved != 0 {
		t.Fatalf("settled wallet=%+v error=%v", wallet, err)
	}
}

func TestExternalDashboardRefundInsufficientIdeasSurfacesRecoverableError(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))
	// Spend 400 of 500 Ideas so the Dashboard refund cannot be recovered.
	spendAmount, err := domain.NewCreditAmount(400)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credits.NewService(pool).Deduct(t.Context(), ports.DeductionCommand{UserID: 42, Amount: spendAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "refund-external-shortage-spend", Actor: domain.Actor{Type: domain.ActorAdmin, ID: "42"}, Source: "admin", Reason: "consume Ideas", BusinessReference: domain.BusinessReference{Type: "test", ID: "shortage"},
	}}); err != nil {
		t.Fatalf("spend Ideas: %v", err)
	}
	// 300 minor needs ~285 Ideas but only 200 remain available.
	if _, err := repo.CreateExternalPaymentRefund(t.Context(), paymentports.StripeRefund{ID: "re_dash_short", ChargeID: "ch_refund", AmountMinor: 300, Currency: "usd", Status: "succeeded"}, now); !errors.Is(err, domain.ErrInsufficientCredits) {
		t.Fatalf("short dashboard refund error = %v, want ErrInsufficientCredits", err)
	}
	if _, anomalyErr := repo.RecordPaymentRefundAnomaly(t.Context(), paymentports.StripeRefund{ID: "re_dash_short", ChargeID: "ch_refund", AmountMinor: 300, Currency: "usd", Status: "succeeded"}, "manual recovery", now); anomalyErr != nil {
		t.Fatalf("RecordPaymentRefundAnomaly: %v", anomalyErr)
	}
}

func TestExternalDashboardRefundBlockedOnFrozenWallet(t *testing.T) {
	db := testdb.Open(t)
	repo := newTestPaymentStores(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	createCreditedRefundOrder(t, db, repo, now.Add(-time.Hour))
	if _, err := db.Exec(t.Context(), `SELECT set_credit_wallet_status(42, 'frozen')`); err != nil {
		t.Fatalf("freeze wallet: %v", err)
	}
	if _, err := repo.CreateExternalPaymentRefund(t.Context(), paymentports.StripeRefund{ID: "re_dash_frozen", ChargeID: "ch_refund", AmountMinor: 100, Currency: "usd", Status: "succeeded"}, now); !errors.Is(err, domain.ErrWalletFrozen) {
		t.Fatalf("frozen dashboard refund error = %v, want ErrWalletFrozen", err)
	}
}

func TestProportionalRefundAmountUsesCumulativeRounding(t *testing.T) {
	first := paymentsdomain.ProportionalRefundAmount(10, 3, 0, 1)
	second := paymentsdomain.ProportionalRefundAmount(10, 3, 1, 1)
	third := paymentsdomain.ProportionalRefundAmount(10, 3, 2, 1)
	if first != 3 || second != 3 || third != 4 || first+second+third != 10 {
		t.Fatalf("cumulative refund amounts = %d, %d, %d", first, second, third)
	}
}

func TestPaymentRefundRejectsExactFourteenDayBoundary(t *testing.T) {
	db := testdb.Open(t)
	repo := newTestPaymentStores(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, db, repo, now.Add(-paymentsdomain.RefundEligibilityWindow))
	_, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 1,
		IdempotencyKey: "refund-integration-expired", Now: now,
	})
	if !errors.Is(err, paymentcontracts.ErrRefundExpired) {
		t.Fatalf("exact boundary error = %v", err)
	}
}

func TestPaymentRefundTerminalFactsCannotRegress(t *testing.T) {
	db := testdb.Open(t)
	repo := newTestPaymentStores(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, db, repo, now.Add(-time.Hour))
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 1,
		IdempotencyKey: "refund-terminal-immutable", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE payment_refunds SET status = 'pending', credit_transaction_id = '', succeeded_at = NULL WHERE id = $1::uuid`, refund.ID); err == nil {
		t.Fatal("terminal refund facts unexpectedly regressed")
	}
}

func TestPaymentRefundReconciliationRecoversPendingProviderStateExactlyOnce(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Now().UTC()
	order := createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "refund-reconciliation", Now: now.Add(-3 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundProviderResult(t.Context(), refund.ID, "re_reconcile", "pending", now.Add(-3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	gateway := &refundGatewayFake{retrieve: map[string]paymentports.StripeRefund{
		"re_reconcile": {ID: "re_reconcile", Status: "succeeded", ChargeID: "ch_refund", AmountMinor: 100, Currency: "usd"},
	}}
	runtime := payments.NewPaymentRuntime(NewEventHub(), func() time.Time { return now })
	transitions, err := payments.NewPaymentOrderTransitions(repo, runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := payments.NewService(payments.ServiceDependencies{
		Configuration: repo, ConfigurationGateway: testStripeGateway{}, ShutdownEnqueuer: testShutdownEnqueuer{},
		Store: repo, CheckoutGateway: testCheckoutGateway{}, Wallets: testsupport.FakeMissingWalletReader{}, Convergence: repo,
		Inbox: repo, Verifier: testWebhookVerifier{},
		RefundGateway: gateway, DisputeGateway: testDisputeGateway{}, Transitions: transitions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileDuePaymentRefunds(t.Context(), now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileDuePaymentRefunds(t.Context(), now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	settled, err := repo.GetPaymentRefundByID(t.Context(), refund.ID)
	if err != nil || settled.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("settled refund=%+v error=%v", settled, err)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 400 || wallet.Reserved != 0 {
		t.Fatalf("settled wallet=%+v error=%v", wallet, err)
	}
	if gateway.retrieveCalls != 1 || gateway.createCalls != 0 {
		t.Fatalf("gateway calls create=%d retrieve=%d", gateway.createCalls, gateway.retrieveCalls)
	}
}

func TestPaymentRefundConcurrentRequestsCannotExceedOrderOrWallet(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))
	errs := make(chan error, 2)
	for index := range 2 {
		go func() {
			_, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
				ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 300,
				IdempotencyKey: fmt.Sprintf("refund-concurrent-%d", index), Now: now,
			})
			errs <- err
		}()
	}
	succeeded, rejected := 0, 0
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, paymentcontracts.ErrRefundAmountExceeds), errors.Is(err, domain.ErrInsufficientCredits):
			rejected++
		default:
			t.Fatalf("unexpected concurrent refund error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent refunds succeeded=%d rejected=%d", succeeded, rejected)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 200 || wallet.Reserved != 300 {
		t.Fatalf("concurrent refund wallet=%+v error=%v", wallet, err)
	}
}

func createCreditedRefundOrder(t *testing.T, db *pgxpool.Pool, repo *testPaymentStores, paidAt time.Time) paymentsdomain.PaymentOrder {
	t.Helper()
	publishTestStripeConfiguration(t, repo, "acct_admin_read", "prod_admin_read")
	if _, err := db.Exec(t.Context(), `SELECT provision_credit_wallet(42)`); err != nil {
		t.Fatal(err)
	}
	order := createAdminPaymentOrder(t, repo, 42, "IBP-20260804-REFUND000001", 500)
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_refund", StripeSessionID: "cs_refund",
		CheckoutURL: "https://checkout.stripe.example/refund", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	paid, _, err := repo.RecordPaidPaymentOrder(t.Context(), paymentsdomain.PaidCheckoutFacts{
		StripeAccountID: bound.StripeAccountID, StripeSessionID: bound.StripeCheckoutSessionID,
		ExpiresAt: bound.ExpiresAt, PublicOrderNumber: bound.PublicOrderNumber, PaymentOrderID: bound.ID,
		RequestHash: bound.RequestHash, StripeCustomerID: bound.StripeCustomerID,
		StripePaymentIntentID: "pi_refund", StripeChargeID: "ch_refund", IdeaQuantity: bound.IdeaQuantity,
		PaidSubtotalMinor: 500, PaidTaxMinor: 25, PaidTotalMinor: 525, PaidCurrency: "usd",
		ReceiptURL: "https://pay.stripe.com/receipts/refund", PaidAt: paidAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	credited, err := repo.CreditPaidPaymentOrder(t.Context(), paid.ID, paidAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return credited.Order
}
