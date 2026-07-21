package rewards

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRewardCodeCreateListRedeemAndReplay(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	command := CreateRewardCodeCommand{
		DisplayName: "新用户奖励", Code: "JOIN-IDEA-2026", CreditAmount: 88,
		MaxRedemptions: 1, IdempotencyKey: "reward-code-create-001", ActorUserID: 1,
	}
	created, err := service.CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatalf("CreateRewardCode() error = %v", err)
	}
	if created.Plaintext != "JOINIDEA2026" || created.Replayed ||
		!created.RewardCode.Enabled || created.RewardCode.Status != RewardCodeStatusEnabled ||
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
	page, err := service.ListRewardCodes(t.Context(), ListRewardCodesInput{})
	if err != nil || len(page.RewardCodes) != 1 || page.RewardCodes[0].ID != created.RewardCode.ID {
		t.Fatalf("ListRewardCodes() = %+v error=%v", page, err)
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
	first, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: command.Code})
	if err != nil || first.Replayed || first.CreditedAmount != 88 || first.Redemption.RewardCodeID != created.RewardCode.ID {
		t.Fatalf("first Redeem() = %+v error=%v", first, err)
	}
	sameUserReplay, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: command.Code})
	if err != nil || !sameUserReplay.Replayed || sameUserReplay.Redemption.ID != first.Redemption.ID {
		t.Fatalf("same-user replay = %+v error=%v", sameUserReplay, err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: command.Code}); !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("final-cap Redeem() error = %v, want unified unavailable", err)
	}
	history, err := service.ListRedemptions(t.Context(), ListRedemptionsInput{UserID: 3})
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
	_, pool := testdb.OpenWithPool(t)
	realCredits := credits.NewService(pool)
	service := newTestRewardService(t, pool, realCredits)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "失败回滚奖励", Code: "ROLLBACK-2026", CreditAmount: 9,
		MaxRedemptions: 1, IdempotencyKey: "reward-code-rollback", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, realCredits, 3)
	service.credits = failingGrantCreditService{reader: realCredits}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "ROLLBACK-2026"}); err == nil {
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
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	now := time.Date(2030, 7, 21, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Now: func() time.Time { return now },
	})
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "限时奖励", Code: "EXPIRES-2030", CreditAmount: 5,
		ExpiresAt: &expiresAt, IdempotencyKey: "expires-boundary", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	now = expiresAt
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "EXPIRES-2030"}); !errors.Is(err, ErrCodeUnavailable) {
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

func TestRewardCodeMigrationCatalogAndAppendOnlyGuards(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "Schema Guard", Code: "SCHEMA-GUARD-2026", CreditAmount: 1,
		IdempotencyKey: "schema-guard", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var requiredConstraints, requiredTriggers int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT COUNT(*) FROM pg_constraint
   WHERE connamespace = current_schema()::regnamespace AND conname IN (
     'uq_reward_codes_creator_idempotency',
     'pk_reward_code_digest_aliases',
     'uq_reward_redemptions_user_code',
     'fk_reward_redemptions_credit_transaction'
   )),
  (SELECT COUNT(*) FROM pg_trigger triggers
   JOIN pg_class tables ON tables.oid = triggers.tgrelid
   WHERE tables.relnamespace = current_schema()::regnamespace
     AND NOT triggers.tgisinternal AND triggers.tgname IN (
     'reward_admin_actions_append_only',
     'reward_codes_identity_immutable',
     'set_reward_codes_updated_at'
   ))`).Scan(&requiredConstraints, &requiredTriggers); err != nil {
		t.Fatal(err)
	}
	if requiredConstraints != 4 || requiredTriggers != 3 {
		t.Fatalf("migration catalog constraints=%d triggers=%d, want 4/3", requiredConstraints, requiredTriggers)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE reward_admin_actions SET metadata = '{}'::JSONB WHERE reward_code_id = $1`, created.RewardCode.ID); err == nil {
		t.Fatal("append-only Journal update unexpectedly succeeded")
	}
	if _, err := pool.Exec(t.Context(), `UPDATE reward_codes SET idempotency_key = 'changed' WHERE id = $1`, created.RewardCode.ID); err == nil {
		t.Fatal("immutable RewardCode identity update unexpectedly succeeded")
	}
	if _, err := pool.Exec(t.Context(), `UPDATE reward_codes SET credit_amount = credit_amount + 1 WHERE id = $1`, created.RewardCode.ID); err == nil {
		t.Fatal("immutable RewardCode amount update unexpectedly succeeded")
	}
}

func TestRewardCodeConcurrentFinalCapacityAndSameUserReplay(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
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
			_, errorsByUser[index] = service.Redeem(t.Context(), RedeemCommand{UserID: userID, Code: "RACE-CODE-2026"})
		}(index, userID)
	}
	wait.Wait()
	successes, unavailable := 0, 0
	for _, redeemError := range errorsByUser {
		switch {
		case redeemError == nil:
			successes++
		case errors.Is(redeemError, ErrCodeUnavailable):
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

	sameUserCode, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "同用户并发奖励", Code: "SAME-USER-2026", CreditAmount: 6,
		MaxRedemptions: 0, IdempotencyKey: "reward-code-same-user", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make([]RedeemResult, 2)
	sameUserErrors := make([]error, 2)
	var replayWait sync.WaitGroup
	for index := range results {
		replayWait.Add(1)
		go func(index int) {
			defer replayWait.Done()
			results[index], sameUserErrors[index] = service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "SAME-USER-2026"})
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

func TestRewardCodeCreateReplaySurvivesHMACRotation(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := CodeKey{Version: "old", Secret: bytes.Repeat([]byte{0x31}, 32)}
	newKey := CodeKey{Version: "new", Secret: bytes.Repeat([]byte{0x32}, 32)}
	oldKeyring, err := NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(pool, creditService, oldKeyring)
	command := CreateRewardCodeCommand{
		DisplayName: "轮换奖励", Code: "ROTATE-CODE-2026", CreditAmount: 12,
		IdempotencyKey: "reward-code-rotation", ActorUserID: 1,
	}
	created, err := service.CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	rotatedService := NewService(pool, creditService, rotatedKeyring)
	replay, err := rotatedService.CreateRewardCode(t.Context(), command)
	if err != nil || replay.RewardCode.ID != created.RewardCode.ID || !replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("rotation replay = %+v error=%v", replay, err)
	}
}

func TestRewardCodeCreateReplaySurvivesReverseHMACRotation(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := CodeKey{Version: "reverse-old", Secret: bytes.Repeat([]byte{0x33}, 32)}
	newKey := CodeKey{Version: "reverse-new", Secret: bytes.Repeat([]byte{0x34}, 32)}
	oldKeyring, err := NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	command := CreateRewardCodeCommand{
		DisplayName: "反向轮换奖励", Code: "REVERSE-ROTATE-2026", CreditAmount: 14,
		IdempotencyKey: "reward-code-reverse-rotation", ActorUserID: 1,
	}
	created, err := NewService(pool, creditService, rotatedKeyring).CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := NewService(pool, creditService, oldKeyring).CreateRewardCode(t.Context(), command)
	if err != nil || replay.RewardCode.ID != created.RewardCode.ID || !replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("reverse rotation replay = %+v error=%v", replay, err)
	}
	differentCode := command
	differentCode.Code = "DIFFERENT-ROTATE-2026"
	if _, err := NewService(pool, creditService, oldKeyring).CreateRewardCode(t.Context(), differentCode); !errors.Is(err, ErrRewardCodeConflict) {
		t.Fatalf("same idempotency with different Code error = %v, want conflict", err)
	}
	differentPayload := command
	differentPayload.CreditAmount++
	if _, err := NewService(pool, creditService, rotatedKeyring).CreateRewardCode(t.Context(), differentPayload); !errors.Is(err, ErrRewardCodeConflict) {
		t.Fatalf("same idempotency with different payload error = %v, want conflict", err)
	}
}

func TestRewardCodeCreateConcurrentIdempotencyAcrossRollingKeyrings(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := CodeKey{Version: "concurrent-old", Secret: bytes.Repeat([]byte{0x35}, 32)}
	newKey := CodeKey{Version: "concurrent-new", Secret: bytes.Repeat([]byte{0x36}, 32)}
	oldKeyring, err := NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	services := []*Service{
		NewService(pool, creditService, oldKeyring),
		NewService(pool, creditService, rotatedKeyring),
	}
	command := CreateRewardCodeCommand{
		DisplayName: "并发轮换奖励", Code: "CONCURRENT-ROTATE-2026", CreditAmount: 16,
		IdempotencyKey: "reward-code-concurrent-rotation", ActorUserID: 1,
	}
	results := make([]CreatedRewardCode, len(services))
	errorsByService := make([]error, len(services))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *Service) {
			defer wait.Done()
			<-start
			results[index], errorsByService[index] = service.CreateRewardCode(t.Context(), command)
		}(index, service)
	}
	close(start)
	wait.Wait()

	replayed, plaintextResponses := 0, 0
	for index, createError := range errorsByService {
		if createError != nil {
			t.Fatalf("concurrent rotation create %d error = %v", index, createError)
		}
		if results[index].Replayed {
			replayed++
		}
		if results[index].Plaintext != "" {
			plaintextResponses++
		}
	}
	if results[0].RewardCode.ID != results[1].RewardCode.ID || replayed != 1 || plaintextResponses != 1 {
		t.Fatalf("concurrent rotation results = %+v, want one create and one replay", results)
	}
}

func TestRewardCodeCreateSerializesSamePlaintextAcrossRollingKeyrings(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := CodeKey{Version: "rolling-old", Secret: bytes.Repeat([]byte{0x41}, 32)}
	newKey := CodeKey{Version: "rolling-new", Secret: bytes.Repeat([]byte{0x42}, 32)}
	oldKeyring, err := NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	newKeyring, err := NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	services := []*Service{
		NewService(pool, creditService, oldKeyring),
		NewService(pool, creditService, newKeyring),
	}
	errorsByService := make([]error, len(services))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *Service) {
			defer wait.Done()
			<-start
			_, errorsByService[index] = service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
				DisplayName: "滚动实例奖励", Code: "ROLLING-CODE-2026", CreditAmount: 10,
				IdempotencyKey: fmt.Sprintf("rolling-instance-%d", index), ActorUserID: 1,
			})
		}(index, service)
	}
	close(start)
	wait.Wait()
	successes, collisions := 0, 0
	for _, createError := range errorsByService {
		switch {
		case createError == nil:
			successes++
		case errors.Is(createError, ErrCodeCollision):
			collisions++
		default:
			t.Fatalf("unexpected rolling create error = %v", createError)
		}
	}
	if successes != 1 || collisions != 1 {
		t.Fatalf("rolling creates successes=%d collisions=%d errors=%v", successes, collisions, errorsByService)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM reward_codes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("RewardCode count=%d, want 1", count)
	}
}

func TestRewardCodeKeyRetirementRejectsDisabledAndExpiredDefinitions(t *testing.T) {
	for _, state := range []string{"disabled", "expired"} {
		t.Run(state, func(t *testing.T) {
			_, pool := testdb.OpenWithPool(t)
			creditService := credits.NewService(pool)
			now := time.Date(2030, time.July, 21, 8, 0, 0, 0, time.UTC)
			oldKey := CodeKey{Version: "retiring-old", Secret: bytes.Repeat([]byte{0x51}, 32)}
			newKey := CodeKey{Version: "retiring-new", Secret: bytes.Repeat([]byte{0x52}, 32)}
			oldKeyring, err := NewCodeKeyring(oldKey, nil)
			if err != nil {
				t.Fatal(err)
			}
			var expiresAt *time.Time
			if state == "expired" {
				expires := now.Add(time.Hour)
				expiresAt = &expires
			}
			oldService := NewServiceWithOptions(pool, creditService, oldKeyring, ServiceOptions{
				Now: func() time.Time { return now },
			})
			created, err := oldService.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
				DisplayName: "待恢复奖励", Code: "RETIRE-CODE-2026", CreditAmount: 10,
				ExpiresAt: expiresAt, IdempotencyKey: "retire-key-state", ActorUserID: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if state == "disabled" {
				_, err = pool.Exec(t.Context(), `UPDATE reward_codes SET enabled = FALSE WHERE id = $1`, created.RewardCode.ID)
			} else {
				now = now.Add(2 * time.Hour)
				var page RewardCodePage
				page, err = oldService.ListRewardCodes(t.Context(), ListRewardCodesInput{})
				if err == nil && (len(page.RewardCodes) != 1 || page.RewardCodes[0].Status != RewardCodeStatusExpired) {
					t.Fatalf("expired RewardCode page = %+v", page)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			newKeyring, err := NewCodeKeyring(newKey, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := NewService(pool, creditService, newKeyring).ValidateActiveCodeKeyVersions(t.Context()); err == nil {
				t.Fatalf("ValidateActiveCodeKeyVersions() error = nil for %s RewardCode", state)
			}
		})
	}
}

type failingGrantCreditService struct {
	reader *credits.Service
}

func (service failingGrantCreditService) InTx(pgx.Tx) credits.Commands {
	return failingCreditCommands{}
}

func (service failingGrantCreditService) GetWallet(ctx context.Context, userID int64) (credits.WalletDetails, error) {
	return service.reader.GetWallet(ctx, userID)
}

type failingCreditCommands struct{}

func (failingCreditCommands) Grant(context.Context, credits.GrantCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("injected Credit Grant failure")
}
func (failingCreditCommands) Deduct(context.Context, credits.DeductionCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("unexpected Deduct")
}
func (failingCreditCommands) Correct(context.Context, credits.CorrectionCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("unexpected Correct")
}
func (failingCreditCommands) Hold(context.Context, credits.HoldCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("unexpected Hold")
}
func (failingCreditCommands) Capture(context.Context, credits.CaptureCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("unexpected Capture")
}
func (failingCreditCommands) Release(context.Context, credits.ReleaseCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("unexpected Release")
}
func (failingCreditCommands) Refund(context.Context, credits.RefundCommand) (credits.CommandResult, error) {
	return credits.CommandResult{}, fmt.Errorf("unexpected Refund")
}
