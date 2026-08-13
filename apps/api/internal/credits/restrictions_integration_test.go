package credits

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/testdb"
)

func TestWalletRestrictionsAreConcurrentIdempotentAndIndependentlyReleased(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	userID := int64(91)
	if _, err := service.Grant(ctx, GrantCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 100),
		Metadata: testMetadata("wallet-restrictions:grant", "wallet-restrictions-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	command := RestrictWalletCommand{
		UserID:        userID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: BusinessReference{Type: "finding", ID: "finding-91-a"},
		Reason:        "projection mismatch",
		Actor:         Actor{Type: ActorSystem, ID: "reconciliation"},
	}
	const attempts = 16
	results := make(chan WalletRestrictionChange, attempts)
	errorsSeen := make(chan error, attempts)
	var wait sync.WaitGroup
	wait.Add(attempts)
	for range attempts {
		go func() {
			defer wait.Done()
			result, err := service.RestrictWallet(ctx, command)
			if err != nil {
				errorsSeen <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("RestrictWallet() concurrent error = %v", err)
	}

	var restrictionID string
	createdCount := 0
	resultCount := 0
	for result := range results {
		resultCount++
		if restrictionID == "" {
			restrictionID = result.RestrictionID
		}
		if result.RestrictionID != restrictionID {
			t.Fatalf("concurrent restriction IDs differ: %q and %q", restrictionID, result.RestrictionID)
		}
		if !result.Replayed {
			createdCount++
		}
	}
	if resultCount != attempts || createdCount != 1 {
		t.Fatalf("restriction results=%d created=%d, want %d/1", resultCount, createdCount, attempts)
	}

	independent, err := service.RestrictWallet(ctx, RestrictWalletCommand{
		UserID:        userID,
		Type:          "risk_control",
		Source:        "identity_risk",
		RelatedObject: BusinessReference{Type: "review", ID: "review-91"},
		Reason:        "manual risk review",
		Actor:         Actor{Type: ActorAdmin, ID: "7"},
	})
	if err != nil {
		t.Fatalf("RestrictWallet(independent) error = %v", err)
	}
	if independent.RestrictionID == restrictionID || independent.Replayed {
		t.Fatalf("independent restriction = %+v, first ID=%s", independent, restrictionID)
	}

	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet(restricted) error = %v", err)
	}
	if len(wallet.ActiveRestrictions) != 2 {
		t.Fatalf("active restrictions = %+v, want 2", wallet.ActiveRestrictions)
	}

	releaseResults := make(chan WalletRestrictionChange, attempts)
	releaseErrors := make(chan error, attempts)
	wait = sync.WaitGroup{}
	wait.Add(attempts)
	for range attempts {
		go func() {
			defer wait.Done()
			result, err := service.ReleaseWalletRestriction(ctx, ReleaseWalletRestrictionCommand{
				RestrictionID: restrictionID,
				Reason:        "finding resolved",
				Actor:         Actor{Type: ActorSystem, ID: "reconciliation"},
			})
			if err != nil {
				releaseErrors <- err
				return
			}
			releaseResults <- result
		}()
	}
	wait.Wait()
	close(releaseResults)
	close(releaseErrors)
	for err := range releaseErrors {
		t.Fatalf("ReleaseWalletRestriction() concurrent error = %v", err)
	}
	releasedCount := 0
	releaseResultCount := 0
	for result := range releaseResults {
		releaseResultCount++
		if result.RestrictionID != restrictionID {
			t.Fatalf("released restriction ID = %q, want %q", result.RestrictionID, restrictionID)
		}
		if !result.Replayed {
			releasedCount++
		}
	}
	if releaseResultCount != attempts || releasedCount != 1 {
		t.Fatalf("release results=%d released=%d, want %d/1", releaseResultCount, releasedCount, attempts)
	}

	wallet, err = service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet(after one release) error = %v", err)
	}
	if len(wallet.ActiveRestrictions) != 1 || wallet.ActiveRestrictions[0].RestrictionID != independent.RestrictionID {
		t.Fatalf("active restrictions after one release = %+v", wallet.ActiveRestrictions)
	}
	if _, err := service.Hold(ctx, HoldCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 1),
		Metadata: testMetadata("wallet-restrictions:still-blocked", "wallet-restrictions-still-blocked"),
	}); !errors.Is(err, ErrWalletRestricted) {
		t.Fatalf("Hold(with independent restriction) error = %v, want ErrWalletRestricted", err)
	}

	if _, err := service.ReleaseWalletRestriction(ctx, ReleaseWalletRestrictionCommand{
		RestrictionID: independent.RestrictionID,
		Reason:        "review completed",
		Actor:         Actor{Type: ActorAdmin, ID: "7"},
	}); err != nil {
		t.Fatalf("ReleaseWalletRestriction(independent) error = %v", err)
	}
	if _, err := service.Hold(ctx, HoldCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 1),
		Metadata: testMetadata("wallet-restrictions:unblocked", "wallet-restrictions-unblocked"),
	}); err != nil {
		t.Fatalf("Hold(after all releases) error = %v", err)
	}

	reopened, err := service.RestrictWallet(ctx, command)
	if err != nil {
		t.Fatalf("RestrictWallet(after release) error = %v", err)
	}
	if reopened.Replayed || reopened.RestrictionID == restrictionID {
		t.Fatalf("reopened restriction = %+v, previous ID=%s", reopened, restrictionID)
	}
	var totalRows, releasedRows int
	if err := pool.QueryRow(ctx, `
SELECT count(*), count(*) FILTER (WHERE released_at IS NOT NULL)
FROM credit_wallet_restrictions
WHERE wallet_user_id = $1`, userID).Scan(&totalRows, &releasedRows); err != nil {
		t.Fatalf("read restriction history error = %v", err)
	}
	if totalRows != 3 || releasedRows != 2 {
		t.Fatalf("restriction history total=%d released=%d, want 3/2", totalRows, releasedRows)
	}
}

func TestWalletRestrictionBlocksNewExposureButAllowsConvergenceAndRepair(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	userID := int64(92)
	grantCommand := GrantCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 100),
		Metadata: testMetadata("wallet-restriction-gate:grant", "wallet-restriction-gate-grant"),
	}
	grant, err := service.Grant(ctx, grantCommand)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	captureHold, err := service.Hold(ctx, HoldCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 20),
		Metadata: testMetadata("wallet-restriction-gate:capture-hold", "wallet-restriction-gate-capture-hold"),
	})
	if err != nil {
		t.Fatalf("Hold(capture) error = %v", err)
	}
	releaseHold, err := service.Hold(ctx, HoldCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 10),
		Metadata: testMetadata("wallet-restriction-gate:release-hold", "wallet-restriction-gate-release-hold"),
	})
	if err != nil {
		t.Fatalf("Hold(release) error = %v", err)
	}
	if _, err := service.RestrictWallet(ctx, RestrictWalletCommand{
		UserID:        userID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: BusinessReference{Type: "finding", ID: "finding-92"},
		Reason:        "ledger mismatch",
		Actor:         Actor{Type: ActorSystem, ID: "reconciliation"},
	}); err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}

	replay, err := service.Grant(ctx, grantCommand)
	if err != nil || !replay.Replayed || replay.TransactionID != grant.TransactionID {
		t.Fatalf("Grant(idempotency replay after restriction) = %+v, %v", replay, err)
	}
	blockedCommands := []struct {
		name string
		run  func() error
	}{
		{
			name: "grant",
			run: func() error {
				_, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("wallet-restriction-gate:new-grant", "wallet-restriction-gate-new-grant")})
				return err
			},
		},
		{
			name: "deduction",
			run: func() error {
				_, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("wallet-restriction-gate:deduction", "wallet-restriction-gate-deduction")})
				return err
			},
		},
		{
			name: "hold",
			run: func() error {
				_, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("wallet-restriction-gate:new-hold", "wallet-restriction-gate-new-hold")})
				return err
			},
		},
	}
	for _, testCase := range blockedCommands {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.run(); !errors.Is(err, ErrWalletRestricted) {
				t.Fatalf("command error = %v, want ErrWalletRestricted", err)
			}
		})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(freeze) error = %v", err)
	}
	if err := service.LifecycleInTx(tx).SetWalletFrozen(ctx, userID, true); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SetWalletFrozen() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit freeze error = %v", err)
	}

	capture, err := service.Capture(ctx, CaptureCommand{
		UserID:            userID,
		HoldTransactionID: captureHold.TransactionID,
		Amount:            mustCreditAmount(t, 5),
		Metadata:          testMetadata("wallet-restriction-gate:capture", "wallet-restriction-gate-capture"),
	})
	if err != nil {
		t.Fatalf("Capture(restricted frozen Wallet) error = %v", err)
	}
	if _, err := service.Release(ctx, ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: releaseHold.TransactionID,
		Amount:            mustCreditAmount(t, 5),
		Metadata:          testMetadata("wallet-restriction-gate:release", "wallet-restriction-gate-release"),
	}); err != nil {
		t.Fatalf("Release(restricted frozen Wallet) error = %v", err)
	}
	if _, err := service.Refund(ctx, RefundCommand{
		UserID:              userID,
		ChargeTransactionID: capture.TransactionID,
		Amount:              mustCreditAmount(t, 5),
		Metadata:            testMetadata("wallet-restriction-gate:refund", "wallet-restriction-gate-refund"),
	}); err != nil {
		t.Fatalf("Refund(restricted frozen Wallet) error = %v", err)
	}
	if _, err := service.Correct(ctx, CorrectionCommand{
		UserID:                userID,
		OriginalTransactionID: grant.TransactionID,
		Amount:                mustCreditAmount(t, 5),
		Metadata:              testMetadata("wallet-restriction-gate:correction", "wallet-restriction-gate-correction"),
	}); err != nil {
		t.Fatalf("Correct(restricted frozen Wallet) error = %v", err)
	}

	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.Status != WalletStatusFrozen || len(wallet.ActiveRestrictions) != 1 {
		t.Fatalf("Wallet lifecycle/restrictions = %+v", wallet)
	}
	if wallet.ActiveRestrictions[0].RelatedObject.ID != "finding-92" {
		t.Fatalf("Wallet restriction read model = %+v", wallet.ActiveRestrictions[0])
	}

	var unsettled int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_holds WHERE wallet_user_id = $1 AND status = 'held'`, userID).Scan(&unsettled); err != nil {
		t.Fatalf("count unsettled holds error = %v", err)
	}
	if unsettled != 0 {
		t.Fatalf("unsettled holds = %d, want 0", unsettled)
	}
}

func TestWalletRestrictionDatabaseConstraintsRejectUserActorsAndInvalidReleaseTimeline(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	userID := int64(93)
	if _, err := service.Grant(ctx, GrantCommand{
		UserID:   userID,
		Amount:   mustCreditAmount(t, 10),
		Metadata: testMetadata("wallet-restriction-constraints:grant", "wallet-restriction-constraints-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if _, err := service.RestrictWallet(ctx, RestrictWalletCommand{
		UserID:        userID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: BusinessReference{Type: "finding", ID: "invalid-user-actor"},
		Reason:        "must reject user actor",
		Actor:         Actor{Type: ActorUser, ID: "93"},
	}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("RestrictWallet(user actor) error = %v, want ErrInvalidCommand", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO credit_wallet_restrictions (
  wallet_user_id, restriction_type, source, related_object_type, related_object_id,
  reason, created_by_actor_type, created_by_actor_id
) VALUES ($1, 'ledger_integrity', 'reconciliation', 'finding', 'direct-user-actor',
          'must fail at database boundary', 'user', '93')`, userID); err == nil {
		t.Fatal("database accepted a user-created Wallet Restriction")
	}

	created, err := service.RestrictWallet(ctx, RestrictWalletCommand{
		UserID:        userID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: BusinessReference{Type: "finding", ID: "release-timeline"},
		Reason:        "verify release timeline constraint",
		Actor:         Actor{Type: ActorSystem, ID: "reconciliation"},
	})
	if err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE credit_wallet_restrictions
SET released_at = created_at - interval '1 second',
    released_by_actor_type = 'system',
    released_by_actor_id = 'reconciliation',
    release_reason = 'invalid timeline'
WHERE restriction_id = $1::uuid`, created.RestrictionID); err == nil {
		t.Fatal("database accepted a release before Restriction creation")
	}
}
