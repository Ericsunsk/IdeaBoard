package tests

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/rewards/application/redemptions"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRewardCodeCreateSearchRedeemAndReplay(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	command := ports.CreateRewardCodeCommand{
		DisplayName: "新用户奖励", Code: "JOIN-IDEA-2026", CreditAmount: 88,
		MaxRedemptions: 1, IdempotencyKey: "reward-code-create-001", ActorUserID: 1,
	}
	created, err := service.CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatalf("CreateRewardCode() error = %v", err)
	}
	if created.Plaintext != "JOINIDEA2026" || created.Replayed ||
		!created.RewardCode.Enabled || created.RewardCode.Status != ports.RewardCodeStatusEnabled ||
		created.RewardCode.MaskedCodeHint != "•••• 2026" {
		t.Fatalf("created RewardCode = %+v", created)
	}
	replay, err := service.CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatalf("replayed CreateRewardCode() error = %v", err)
	}
	if replay.RewardCode.ID != created.RewardCode.ID || !replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("replayed RewardCode = %+v, want same metadata without plaintext", replay)
	}
	page, err := service.SearchRewardCodes(t.Context(), ports.SearchRewardCodesInput{})
	if err != nil || len(page.RewardCodes) != 1 || page.RewardCodes[0].ID != created.RewardCode.ID {
		t.Fatalf("SearchRewardCodes() = %+v error=%v", page, err)
	}
	var definitions, actions int
	var plaintextPersisted bool
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT COUNT(*) FROM reward_codes),
  (SELECT COUNT(*) FROM reward_admin_actions),
  EXISTS (SELECT 1 FROM reward_admin_actions WHERE metadata::TEXT LIKE '%JOINIDEA2026%')`,
	).Scan(&definitions, &actions, &plaintextPersisted); err != nil {
		t.Fatal(err)
	}
	if definitions != 1 || actions != 1 || plaintextPersisted {
		t.Fatalf("persistence definitions=%d actions=%d plaintext=%t", definitions, actions, plaintextPersisted)
	}

	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)
	first, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: command.Code})
	if err != nil || first.Replayed || first.CreditedAmount != 88 || first.Redemption.RewardCodeID != created.RewardCode.ID {
		t.Fatalf("first Redeem() = %+v error=%v", first, err)
	}
	sameUserReplay, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: command.Code})
	if err != nil || !sameUserReplay.Replayed || sameUserReplay.Redemption.ID != first.Redemption.ID {
		t.Fatalf("same-user replay = %+v error=%v", sameUserReplay, err)
	}
	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 7, Code: command.Code}); !errors.Is(err, rewardsdomain.ErrCodeUnavailable) {
		t.Fatalf("final-cap Redeem() error = %v, want unified unavailable", err)
	}
	history, err := service.ListRedemptions(t.Context(), ports.ListRedemptionsInput{UserID: 3})
	if err != nil || len(history.Redemptions) != 1 || history.Redemptions[0].RewardCodeDisplayName != "新用户奖励" {
		t.Fatalf("ListRedemptions() = %+v error=%v", history, err)
	}
	var redemptionCount, grants int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT redemption_count FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM credit_transactions WHERE business_reference_type = 'reward_code_redemption')`,
		created.RewardCode.ID,
	).Scan(&redemptionCount, &grants); err != nil {
		t.Fatal(err)
	}
	if redemptionCount != 1 || grants != 1 {
		t.Fatalf("atomic facts redemption_count=%d grants=%d, want 1/1", redemptionCount, grants)
	}
}

func TestRewardCodeGrantFailureRollsBackCapacityAndRedemption(t *testing.T) {
	pool := testdb.Open(t)
	realCredits := credits.NewService(pool)
	service := newTestRewardService(t, pool, realCredits)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "失败回滚奖励", Code: "ROLLBACK-2026", CreditAmount: 9,
		MaxRedemptions: 1, IdempotencyKey: "reward-code-rollback", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, realCredits, 3)
	service = rewards.NewServiceWithOptions(pool, failingGrantCreditService{reader: realCredits}, newTestRewardKeyring(t), rewards.ServiceOptions{})
	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "ROLLBACK-2026"}); err == nil {
		t.Fatal("Redeem() succeeded with injected Credit Grant failure")
	}
	var count, redemptions int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT redemption_count FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM reward_redemptions WHERE reward_code_id = $1)`,
		created.RewardCode.ID,
	).Scan(&count, &redemptions); err != nil {
		t.Fatal(err)
	}
	if count != 0 || redemptions != 0 {
		t.Fatalf("rollback facts count=%d redemptions=%d, want 0/0", count, redemptions)
	}
}

func TestRewardCodeExpirationIsEndExclusive(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	expiresAt := time.Now().UTC().Add(time.Hour)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "限时奖励", Code: "EXPIRES-2030", CreditAmount: 5,
		ExpiresAt: &expiresAt, IdempotencyKey: "expires-boundary", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := pool.Exec(t.Context(), `
UPDATE reward_codes
SET expires_at = clock_timestamp()
WHERE id = $1`, created.RewardCode.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "EXPIRES-2030"}); !errors.Is(err, rewardsdomain.ErrCodeUnavailable) {
		t.Fatalf("Redeem() at expires_at error = %v, want unified ErrCodeUnavailable", err)
	}
	var count int64
	if err := pool.QueryRow(t.Context(), `SELECT redemption_count FROM reward_codes WHERE id = $1`, created.RewardCode.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("redemption_count=%d at expiry boundary, want 0", count)
	}
}

func TestRewardCodeRedemptionUsesDatabaseTimeWhenApplicationClockIsAhead(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	expiresAt := time.Now().UTC().Add(time.Hour)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "数据库时间奖励", Code: "DB-CLOCK-2030", CreditAmount: 5,
		ExpiresAt: &expiresAt, IdempotencyKey: "database-clock-authority", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	skewedService := newTestRewardServiceAt(t, pool, creditService, expiresAt.Add(time.Hour))
	result, err := skewedService.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "DB-CLOCK-2030"})
	if err != nil || result.CreditedAmount != 5 {
		t.Fatalf("Redeem() with ahead application clock = %+v error=%v", result, err)
	}
	var redemptionCount int64
	if err := pool.QueryRow(t.Context(), `SELECT redemption_count FROM reward_codes WHERE id = $1`,
		created.RewardCode.ID).Scan(&redemptionCount); err != nil {
		t.Fatal(err)
	}
	if redemptionCount != 1 {
		t.Fatalf("redemption_count=%d, want database-authoritative reservation", redemptionCount)
	}
}

func TestRewardCodeConcurrentFinalCapacityAndSameUserReplay(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "并发奖励", Code: "RACE-CODE-2026", CreditAmount: 5,
		MaxRedemptions: 1, IdempotencyKey: "reward-code-race", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)
	users := []int64{3, 7}
	errorsByUser := make([]error, len(users))
	var wait sync.WaitGroup
	for index, userID := range users {
		wait.Add(1)
		go func(index int, userID int64) {
			defer wait.Done()
			_, errorsByUser[index] = service.Redeem(t.Context(), ports.RedeemCommand{UserID: userID, Code: "RACE-CODE-2026"})
		}(index, userID)
	}
	wait.Wait()
	successes, unavailable := 0, 0
	for _, redeemError := range errorsByUser {
		switch {
		case redeemError == nil:
			successes++
		case errors.Is(redeemError, rewardsdomain.ErrCodeUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected concurrent Redeem() error = %v", redeemError)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("concurrent results successes=%d unavailable=%d", successes, unavailable)
	}
	var count int64
	if err := pool.QueryRow(t.Context(), `SELECT redemption_count FROM reward_codes WHERE id = $1`, created.RewardCode.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("redemption_count=%d, want 1", count)
	}

	sameUserCode, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "同用户并发奖励", Code: "SAME-USER-2026", CreditAmount: 6,
		MaxRedemptions: 0, IdempotencyKey: "reward-code-same-user", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make([]ports.RedeemResult, 2)
	sameUserErrors := make([]error, 2)
	var replayWait sync.WaitGroup
	for index := range results {
		replayWait.Add(1)
		go func(index int) {
			defer replayWait.Done()
			results[index], sameUserErrors[index] = service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "SAME-USER-2026"})
		}(index)
	}
	replayWait.Wait()
	replayed := 0
	for index, redeemError := range sameUserErrors {
		if redeemError != nil {
			t.Fatalf("same-user concurrent Redeem() error = %v", redeemError)
		}
		if results[index].Replayed {
			replayed++
		}
	}
	if replayed != 1 || results[0].Redemption.ID != results[1].Redemption.ID {
		t.Fatalf("same-user concurrent results = %+v, want one grant and one replay", results)
	}
	if err := pool.QueryRow(t.Context(), `SELECT redemption_count FROM reward_codes WHERE id = $1`, sameUserCode.RewardCode.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("same-user redemption_count=%d, want 1", count)
	}
}

func TestRewardCodeDeleteAndRedeemSerializeWithoutOrphanFacts(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "Delete Redeem Race", Code: "DELETE-REDEEM-2030", CreditAmount: 9,
		IdempotencyKey: "delete-redeem-race", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)

	lockTx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockTx.Rollback(t.Context()) }()
	if err := lockRewardCodeID(t.Context(), lockTx, created.RewardCode.ID); err != nil {
		t.Fatal(err)
	}

	type deleteOutcome struct {
		deleted ports.DeletedRewardCode
		err     error
	}
	deleteReady := make(chan struct{})
	deleteDone := make(chan deleteOutcome, 1)
	redeemReady := make(chan struct{})
	redeemDone := make(chan error, 1)
	start := make(chan struct{})
	go func() {
		close(deleteReady)
		<-start
		deleted, err := service.DeleteRewardCode(t.Context(), ports.DeleteRewardCodeCommand{
			RewardCodeID: created.RewardCode.ID, ActorUserID: 1,
		})
		deleteDone <- deleteOutcome{deleted: deleted, err: err}
	}()
	go func() {
		close(redeemReady)
		<-start
		_, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "DELETE-REDEEM-2030"})
		redeemDone <- err
	}()
	<-deleteReady
	<-redeemReady
	close(start)

	deadline := time.Now().Add(5 * time.Second)
	for {
		var lockWaiters int
		if err := pool.QueryRow(t.Context(), `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()
  AND wait_event_type = 'Lock'
  AND query LIKE '%pg_advisory_xact_lock%'`).Scan(&lockWaiters); err != nil {
			t.Fatal(err)
		}
		if lockWaiters >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Delete and Redeem did not both wait on held RewardCode lock; waiters=%d", lockWaiters)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := lockTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	deleteResult := <-deleteDone
	redeemErr := <-redeemDone
	var rewardCodes, redemptions, grants, orphanRedemptions, orphanGrants int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT COUNT(*) FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM reward_redemptions WHERE reward_code_id = $1),
  (SELECT COUNT(*) FROM credit_transactions WHERE business_reference_type = 'reward_code_redemption'),
  (SELECT COUNT(*)
   FROM reward_redemptions redemptions
   LEFT JOIN reward_codes codes ON codes.id = redemptions.reward_code_id
   WHERE codes.id IS NULL),
  (SELECT COUNT(*)
   FROM credit_transactions transactions
   LEFT JOIN reward_redemptions redemptions
     ON transactions.business_reference_type = 'reward_code_redemption'
    AND transactions.business_reference_id = 'rc:' || redemptions.id::TEXT
   WHERE transactions.business_reference_type = 'reward_code_redemption'
     AND redemptions.id IS NULL)`, created.RewardCode.ID).Scan(
		&rewardCodes, &redemptions, &grants, &orphanRedemptions, &orphanGrants,
	); err != nil {
		t.Fatal(err)
	}
	if orphanRedemptions != 0 || orphanGrants != 0 {
		t.Fatalf("orphan Reward facts redemptions=%d grants=%d", orphanRedemptions, orphanGrants)
	}
	switch {
	case deleteResult.err == nil && errors.Is(redeemErr, rewardsdomain.ErrCodeUnavailable):
		if deleteResult.deleted.RewardCodeID != created.RewardCode.ID || rewardCodes != 0 || redemptions != 0 || grants != 0 {
			t.Fatalf("delete-first facts deleted=%+v codes=%d redemptions=%d grants=%d", deleteResult.deleted, rewardCodes, redemptions, grants)
		}
	case errors.Is(deleteResult.err, rewardsdomain.ErrRewardCodeRedeemed) && redeemErr == nil:
		if rewardCodes != 1 || redemptions != 1 || grants != 1 {
			t.Fatalf("redeem-first facts codes=%d redemptions=%d grants=%d", rewardCodes, redemptions, grants)
		}
	default:
		t.Fatalf("Delete/Redeem race outcomes delete=%+v error=%v redeemError=%v", deleteResult.deleted, deleteResult.err, redeemErr)
	}
}

type failingGrantCreditService struct {
	reader *credits.Service
}

func (service failingGrantCreditService) Grant(context.Context, pgx.Tx, redemptions.CreditGrantRequest) (redemptions.CreditGrantResult, error) {
	return redemptions.CreditGrantResult{}, fmt.Errorf("injected Credit Grant failure")
}

func (service failingGrantCreditService) ReadWallet(ctx context.Context, userID int64) (redemptions.WalletSnapshot, error) {
	accountID, err := service.reader.ResolvePersonalAccount(ctx, userID)
	if err != nil {
		return redemptions.WalletSnapshot{}, err
	}
	wallet, err := service.reader.GetWallet(ctx, accountID)
	if err != nil {
		return redemptions.WalletSnapshot{}, err
	}
	return redemptions.WalletSnapshot{Available: wallet.Available}, nil
}
