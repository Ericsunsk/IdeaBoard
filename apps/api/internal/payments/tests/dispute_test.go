package tests

import (
	"errors"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	"ideaboard/api/internal/testdb"
)

func TestPaymentDisputeDebitsAvailableIdeasAndRestoresOnLateWin(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	_ = createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))
	creditService := credits.NewService(pool)
	spendAmount, err := domain.NewCreditAmount(450)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creditService.Deduct(t.Context(), ports.DeductionCommand{
		UserID: 42, Amount: spendAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "dispute-preexisting-spend", Actor: domain.Actor{Type: domain.ActorAdmin, ID: "42"},
			Source: "admin", Reason: "test preexisting spend",
			BusinessReference: domain.BusinessReference{Type: "test", ID: "dispute"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	provider := paymentports.StripeDispute{
		ID: "dp_partial", Status: "under_review", ChargeID: "ch_refund", PaymentIntentID: "pi_refund",
		AmountMinor: 210, Currency: "usd", CreatedAt: now,
	}
	dispute, changed, err := repo.ConvergePaymentDispute(t.Context(), provider, now)
	if err != nil || !changed {
		t.Fatalf("ConvergePaymentDispute(open) dispute=%+v changed=%v err=%v", dispute, changed, err)
	}
	if dispute.Status != paymentsdomain.PaymentDisputeOpen || dispute.TargetIdeaQuantity != 200 ||
		dispute.DebitedIdeaQuantity != 50 || dispute.UnrecoveredCredits != 150 ||
		dispute.RestrictionID == "" || dispute.DebitTransactionID == "" {
		t.Fatalf("open Dispute = %+v", dispute)
	}
	wallet, err := creditService.GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 0 || len(wallet.ActiveRestrictions) != 1 {
		t.Fatalf("restricted Wallet=%+v err=%v", wallet, err)
	}

	replayed, changed, err := repo.ConvergePaymentDispute(t.Context(), provider, now.Add(time.Minute))
	if err != nil || changed || replayed.DebitTransactionID != dispute.DebitTransactionID {
		t.Fatalf("Dispute replay=%+v changed=%v err=%v", replayed, changed, err)
	}
	provider.Status = "lost"
	lost, changed, err := repo.ConvergePaymentDispute(t.Context(), provider, now.Add(2*time.Minute))
	if err != nil || !changed || lost.Status != paymentsdomain.PaymentDisputeLost || lost.RestoreTransactionID != "" {
		t.Fatalf("lost Dispute=%+v changed=%v err=%v", lost, changed, err)
	}
	provider.Status = "won"
	won, changed, err := repo.ConvergePaymentDispute(t.Context(), provider, now.Add(3*time.Minute))
	if err != nil || !changed || won.Status != paymentsdomain.PaymentDisputeWon || won.RestoreTransactionID == "" {
		t.Fatalf("late-win Dispute=%+v changed=%v err=%v", won, changed, err)
	}
	wallet, err = creditService.GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 50 || len(wallet.ActiveRestrictions) != 0 {
		t.Fatalf("restored Wallet=%+v err=%v", wallet, err)
	}
}

func TestPaymentDisputeFullDebitRetainsRestrictionAfterLoss(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 5, 12, 30, 0, 0, time.UTC)
	_ = createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))
	dispute, changed, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_full_loss", Status: "lost", ChargeID: "ch_refund", PaymentIntentID: "pi_refund",
		AmountMinor: 525, Currency: "usd", CreatedAt: now,
	}, now)
	if err != nil || !changed || dispute.TargetIdeaQuantity != 500 || dispute.DebitedIdeaQuantity != 500 || dispute.UnrecoveredCredits != 0 {
		t.Fatalf("full lost Dispute=%+v changed=%v err=%v", dispute, changed, err)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 0 || len(wallet.ActiveRestrictions) != 1 {
		t.Fatalf("full lost Wallet=%+v err=%v", wallet, err)
	}
}

func TestWonDisputeDiscoveredAfterCloseDoesNotCreateTransientRecovery(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 5, 13, 0, 0, 0, time.UTC)
	_ = createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))

	dispute, changed, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_close_first", Status: "won", ChargeID: "ch_refund", PaymentIntentID: "pi_refund",
		AmountMinor: 105, Currency: "usd", CreatedAt: now.Add(-30 * time.Minute),
	}, now)
	if err != nil || !changed {
		t.Fatalf("ConvergePaymentDispute(won) dispute=%+v changed=%v err=%v", dispute, changed, err)
	}
	if dispute.Status != paymentsdomain.PaymentDisputeWon || dispute.TargetIdeaQuantity != 100 ||
		dispute.DebitedIdeaQuantity != 0 || dispute.UnrecoveredCredits != 0 ||
		dispute.RestrictionID != "" || dispute.DebitTransactionID != "" {
		t.Fatalf("close-first Dispute = %+v", dispute)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 500 || len(wallet.ActiveRestrictions) != 0 {
		t.Fatalf("unchanged Wallet=%+v err=%v", wallet, err)
	}
}

func TestPaymentDisputeAndLaterRefundShareTheOrderIdeaCap(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))

	dispute, _, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_before_refund", Status: "under_review", ChargeID: "ch_refund", PaymentIntentID: "pi_refund",
		AmountMinor: 210, Currency: "usd", CreatedAt: now,
	}, now)
	if err != nil || dispute.TargetIdeaQuantity != 200 || dispute.DebitedIdeaQuantity != 200 {
		t.Fatalf("Dispute=%+v err=%v", dispute, err)
	}
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 301,
		IdempotencyKey: "refund-after-dispute-over", Now: now.Add(time.Minute),
	}); !errors.Is(err, paymentcontracts.ErrRefundAmountExceeds) {
		t.Fatalf("over-cap refund error=%v", err)
	}
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 300,
		IdempotencyKey: "refund-after-dispute-exact", Now: now.Add(time.Minute),
	})
	if err != nil || refund.IdeaQuantity != 300 {
		t.Fatalf("remaining refund=%+v err=%v", refund, err)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 0 || wallet.Reserved != 300 {
		t.Fatalf("Wallet after shared cap=%+v err=%v", wallet, err)
	}
}

func TestPaymentRefundAndLaterDisputeShareTheOrderIdeaCap(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 5, 14, 30, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))

	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 300,
		IdempotencyKey: "refund-before-dispute", Now: now,
	})
	if err != nil || refund.IdeaQuantity != 300 {
		t.Fatalf("pending refund=%+v err=%v", refund, err)
	}
	dispute, changed, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_after_refund", Status: "under_review", ChargeID: "ch_refund", PaymentIntentID: "pi_refund",
		AmountMinor: 525, Currency: "usd", CreatedAt: now.Add(time.Minute),
	}, now.Add(time.Minute))
	if err != nil || !changed || dispute.TargetIdeaQuantity != 200 || dispute.DebitedIdeaQuantity != 200 {
		t.Fatalf("capped Dispute=%+v changed=%v err=%v", dispute, changed, err)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 0 || wallet.Reserved != 300 {
		t.Fatalf("Wallet after refund-first cap=%+v err=%v", wallet, err)
	}
}

func TestPaymentDisputeSerializesWithConcurrentConsumption(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 5, 15, 0, 0, 0, time.UTC)
	_ = createCreditedRefundOrder(t, pool, repo, now.Add(-time.Hour))
	creditService := credits.NewService(pool)
	holdAmount, err := domain.NewCreditAmount(100)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	holdResult := make(chan error, 1)
	disputeResult := make(chan struct {
		dispute paymentsdomain.PaymentDispute
		err     error
	}, 1)
	go func() {
		<-start
		_, holdErr := creditService.Hold(t.Context(), ports.HoldCommand{
			UserID: 42, Amount: holdAmount,
			Metadata: domain.CommandMetadata{
				IdempotencyKey: "dispute-concurrent-hold", Actor: domain.Actor{Type: domain.ActorUser, ID: "42"},
				Source: "task_submission", Reason: "concurrent task consumption",
				BusinessReference: domain.BusinessReference{Type: "task", ID: "dispute-concurrent-task"},
			},
		})
		holdResult <- holdErr
	}()
	go func() {
		<-start
		dispute, _, convergeErr := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
			ID: "dp_concurrent_hold", Status: "under_review", ChargeID: "ch_refund", PaymentIntentID: "pi_refund",
			AmountMinor: 525, Currency: "usd", CreatedAt: now,
		}, now)
		disputeResult <- struct {
			dispute paymentsdomain.PaymentDispute
			err     error
		}{dispute: dispute, err: convergeErr}
	}()
	close(start)

	holdErr := <-holdResult
	converged := <-disputeResult
	if converged.err != nil {
		t.Fatalf("concurrent Dispute error=%v", converged.err)
	}
	if holdErr != nil && !errors.Is(holdErr, domain.ErrWalletRestricted) {
		t.Fatalf("concurrent Hold error=%v", holdErr)
	}
	wallet, err := creditService.GetWallet(t.Context(), 42)
	if err != nil || wallet.Available < 0 || len(wallet.ActiveRestrictions) != 1 {
		t.Fatalf("concurrent Wallet=%+v err=%v", wallet, err)
	}
	if holdErr == nil {
		if converged.dispute.DebitedIdeaQuantity != 400 || converged.dispute.UnrecoveredCredits != 100 || wallet.Reserved != 100 {
			t.Fatalf("Hold-first convergence Dispute=%+v Wallet=%+v", converged.dispute, wallet)
		}
		return
	}
	if converged.dispute.DebitedIdeaQuantity != 500 || converged.dispute.UnrecoveredCredits != 0 || wallet.Reserved != 0 {
		t.Fatalf("Dispute-first convergence Dispute=%+v Wallet=%+v", converged.dispute, wallet)
	}
}
