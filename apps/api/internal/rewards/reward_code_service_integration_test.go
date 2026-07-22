package rewards

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRewardCodeCreateSearchRedeemAndReplay(t *testing.T) {
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
	page, err := service.SearchRewardCodes(t.Context(), SearchRewardCodesInput{})
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

func TestSearchRewardCodesFiltersCountsAndPaginates(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	now := time.Date(2030, time.July, 22, 8, 0, 0, 0, time.UTC)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Now: func() time.Time { return now },
	})

	create := func(displayName, code, idempotencyKey string, maxRedemptions int64, expiresAt *time.Time) CreatedRewardCode {
		t.Helper()
		created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
			DisplayName: displayName, Code: code, CreditAmount: 10,
			MaxRedemptions: maxRedemptions, ExpiresAt: expiresAt,
			IdempotencyKey: idempotencyKey, ActorUserID: 1,
		})
		if err != nil {
			t.Fatalf("CreateRewardCode(%q) error = %v", displayName, err)
		}
		return created
	}

	summer := create("夏季迎新奖励", "SUMMER-WELCOME-2030", "search-summer", 0, nil)
	premium := create("夏季高级奖励", "SUMMER-PREMIUM-2030", "search-premium", 0, nil)
	expiresAt := now.Add(time.Hour)
	expired := create("冬季限时奖励", "WINTER-EXPIRED-2030", "search-expired", 1, &expiresAt)
	exhausted := create("容量奖励", "CAPACITY-2030", "search-exhausted", 1, nil)
	if _, err := service.SetRewardCodeEnabled(t.Context(), SetRewardCodeEnabledCommand{
		RewardCodeID: premium.RewardCode.ID, Enabled: false, ActorUserID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "CAPACITY-2030"}); err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 4)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 4, Code: "WINTER-EXPIRED-2030"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRewardCodeEnabled(t.Context(), SetRewardCodeEnabledCommand{
		RewardCodeID: expired.RewardCode.ID, Enabled: false, ActorUserID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)

	assertSearch := func(input SearchRewardCodesInput, wantIDs ...int64) RewardCodePage {
		t.Helper()
		page, err := service.SearchRewardCodes(t.Context(), input)
		if err != nil {
			t.Fatalf("SearchRewardCodes(%+v) error = %v", input, err)
		}
		if page.Total != int64(len(wantIDs)) || len(page.RewardCodes) != len(wantIDs) {
			t.Fatalf("SearchRewardCodes(%+v) = %+v, want IDs %v", input, page, wantIDs)
		}
		for index, wantID := range wantIDs {
			if page.RewardCodes[index].ID != wantID {
				t.Fatalf("SearchRewardCodes(%+v) IDs = %+v, want %v", input, page.RewardCodes, wantIDs)
			}
		}
		return page
	}

	assertSearch(SearchRewardCodesInput{Query: "夏季"}, premium.RewardCode.ID, summer.RewardCode.ID)
	assertSearch(SearchRewardCodesInput{Query: "summer-premium-2030"}, premium.RewardCode.ID)
	previousKey := CodeKey{Version: "test-v1", Secret: bytes.Repeat([]byte{0x2a}, 32)}
	rollingKeyring, err := NewCodeKeyring(
		CodeKey{Version: "test-v2", Secret: bytes.Repeat([]byte{0x2b}, 32)},
		&previousKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	rollingService := NewServiceWithOptions(pool, creditService, rollingKeyring, ServiceOptions{
		Now: func() time.Time { return now },
	})
	rollingPage, err := rollingService.SearchRewardCodes(t.Context(), SearchRewardCodesInput{
		Query: "SUMMER-PREMIUM-2030",
	})
	if err != nil || rollingPage.Total != 1 || len(rollingPage.RewardCodes) != 1 ||
		rollingPage.RewardCodes[0].ID != premium.RewardCode.ID {
		t.Fatalf("rolling HMAC search = %+v error=%v", rollingPage, err)
	}
	assertSearch(SearchRewardCodesInput{Status: RewardCodeStatusDisabled}, premium.RewardCode.ID)
	assertSearch(SearchRewardCodesInput{Status: RewardCodeStatusExpired}, expired.RewardCode.ID)
	assertSearch(SearchRewardCodesInput{Status: RewardCodeStatusExhausted}, exhausted.RewardCode.ID)
	assertSearch(SearchRewardCodesInput{Status: RewardCodeStatusEnabled}, summer.RewardCode.ID)

	first, err := service.SearchRewardCodes(t.Context(), SearchRewardCodesInput{Limit: 2})
	if err != nil || first.Total != 4 || len(first.RewardCodes) != 2 || first.NextBeforeID == 0 {
		t.Fatalf("first keyset page = %+v error=%v", first, err)
	}
	second, err := service.SearchRewardCodes(t.Context(), SearchRewardCodesInput{
		BeforeID: first.NextBeforeID, Limit: 2,
	})
	if err != nil || second.Total != 4 || len(second.RewardCodes) != 2 || second.NextBeforeID != 0 {
		t.Fatalf("second keyset page = %+v error=%v", second, err)
	}
	for _, firstItem := range first.RewardCodes {
		for _, secondItem := range second.RewardCodes {
			if firstItem.ID == secondItem.ID {
				t.Fatalf("keyset pages overlap at RewardCode ID %d", firstItem.ID)
			}
		}
	}

	invalidInputs := []SearchRewardCodesInput{
		{Status: "unknown"},
		{BeforeID: -1},
		{Limit: 101},
		{Query: strings.Repeat("x", 129)},
	}
	for _, input := range invalidInputs {
		if _, err := service.SearchRewardCodes(t.Context(), input); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("SearchRewardCodes(%+v) error = %v, want ErrInvalidCommand", input, err)
		}
	}
}

func TestSearchRewardCodesUsesSingleTimestampForFilterAndRowStatus(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	createdAt := time.Date(2030, time.July, 22, 8, 0, 0, 0, time.UTC)
	keyring := newTestRewardKeyring(t)
	createService := NewServiceWithOptions(pool, creditService, keyring, ServiceOptions{
		Now: func() time.Time { return createdAt },
	})
	expiresAt := createdAt.Add(time.Hour)
	created, err := createService.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "边界状态奖励", Code: "SNAPSHOT-STATUS-2030", CreditAmount: 10,
		MaxRedemptions: 0, ExpiresAt: &expiresAt, IdempotencyKey: "snapshot-status", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	clockCalls := 0
	searchService := NewServiceWithOptions(pool, creditService, keyring, ServiceOptions{
		Now: func() time.Time {
			clockCalls++
			if clockCalls == 1 {
				return expiresAt.Add(-time.Second)
			}
			return expiresAt.Add(time.Second)
		},
	})

	page, err := searchService.SearchRewardCodes(t.Context(), SearchRewardCodesInput{
		Status: RewardCodeStatusEnabled,
	})
	if err != nil || page.Total != 1 || len(page.RewardCodes) != 1 ||
		page.RewardCodes[0].ID != created.RewardCode.ID || page.RewardCodes[0].Status != RewardCodeStatusEnabled ||
		clockCalls != 1 {
		t.Fatalf("snapshot search = %+v error=%v clockCalls=%d", page, err, clockCalls)
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
	service := newTestRewardService(t, pool, creditService)
	expiresAt := time.Now().UTC().Add(time.Hour)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
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

func TestRewardCodeRedemptionUsesDatabaseTimeWhenApplicationClockIsAhead(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	expiresAt := time.Now().UTC().Add(time.Hour)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "数据库时间奖励", Code: "DB-CLOCK-2030", CreditAmount: 5,
		ExpiresAt: &expiresAt, IdempotencyKey: "database-clock-authority", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	skewedService := newTestRewardServiceAt(t, pool, creditService, expiresAt.Add(time.Hour))
	result, err := skewedService.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "DB-CLOCK-2030"})
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
	if _, err := pool.Exec(t.Context(), `UPDATE reward_codes SET redemption_count = redemption_count + 2 WHERE id = $1`, created.RewardCode.ID); err == nil {
		t.Fatal("invalid RewardCode counter update unexpectedly succeeded")
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
				page, err = oldService.SearchRewardCodes(t.Context(), SearchRewardCodesInput{})
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

func TestRewardCodeManagementChangesOnlyFutureRedemptionsAndEnforcesCap(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	now := time.Date(2030, time.July, 22, 8, 0, 0, 0, time.UTC)
	service := newTestRewardServiceAt(t, pool, creditService, now)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "初始奖励", Code: "MANAGE-CODE-2030", CreditAmount: 10,
		IdempotencyKey: "manage-code", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)
	first, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "MANAGE-CODE-2030"})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "后续奖励", CreditAmount: 20,
		MaxRedemptions: 1, ActorUserID: 1,
	})
	if err != nil || updated.RewardCode.Status != RewardCodeStatusExhausted {
		t.Fatalf("exhausting update = %+v error=%v", updated, err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: "MANAGE-CODE-2030"}); !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("exhausted Redeem() error = %v", err)
	}
	updated, err = service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "后续奖励", CreditAmount: 20,
		MaxRedemptions: 0, ActorUserID: 1,
	})
	if err != nil || updated.RewardCode.Status != RewardCodeStatusEnabled {
		t.Fatalf("unlimited update = %+v error=%v", updated, err)
	}
	second, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: "MANAGE-CODE-2030"})
	if err != nil || second.CreditedAmount != 20 {
		t.Fatalf("future redemption = %+v error=%v", second, err)
	}
	if _, err := service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "后续奖励", CreditAmount: 20,
		MaxRedemptions: 1, ActorUserID: 1,
	}); !errors.Is(err, ErrCapBelowRedemptions) {
		t.Fatalf("cap below redemption count error = %v", err)
	}

	var persistedAmount int64
	var transactionID string
	var redeemedAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT credit_amount, credit_transaction_id, redeemed_at
FROM reward_redemptions WHERE id = $1`, first.Redemption.ID).Scan(
		&persistedAmount, &transactionID, &redeemedAt,
	); err != nil {
		t.Fatal(err)
	}
	if persistedAmount != first.Redemption.CreditAmount || transactionID != first.Redemption.CreditTransactionID ||
		!redeemedAt.Equal(first.Redemption.RedeemedAt) {
		t.Fatalf("historical Redemption changed amount=%d transaction=%q time=%v", persistedAmount, transactionID, redeemedAt)
	}

	expiredAt := now.Add(-time.Minute)
	updated, err = service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "后续奖励", CreditAmount: 20,
		MaxRedemptions: 0, ExpiresAt: &expiredAt, ActorUserID: 1,
	})
	if err != nil || updated.RewardCode.Status != RewardCodeStatusExpired {
		t.Fatalf("expired update = %+v error=%v", updated, err)
	}
	enabledUpdated, err := service.SetRewardCodeEnabled(t.Context(), SetRewardCodeEnabledCommand{
		RewardCodeID: created.RewardCode.ID, Enabled: false, ActorUserID: 1,
	})
	if err != nil || enabledUpdated.Enabled || enabledUpdated.Status != RewardCodeStatusExpired {
		t.Fatalf("expired state takes precedence over disabled intent = %+v error=%v", enabledUpdated, err)
	}
	var actionsBefore int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM reward_admin_actions`).Scan(&actionsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRewardCodeEnabled(t.Context(), SetRewardCodeEnabledCommand{
		RewardCodeID: created.RewardCode.ID, Enabled: false, ActorUserID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	var actionsAfter int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM reward_admin_actions`).Scan(&actionsAfter); err != nil {
		t.Fatal(err)
	}
	if actionsAfter != actionsBefore {
		t.Fatalf("set-state replay appended Journal actions before=%d after=%d", actionsBefore, actionsAfter)
	}
}

func TestRewardCodeRedemptionSnapshotsSurviveRenameAndRotation(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "兑换时名称", Code: "SNAPSHOT-OLD-2030", CreditAmount: 10,
		IdempotencyKey: "redemption-snapshot", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)
	first, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "SNAPSHOT-OLD-2030"})
	if err != nil || first.HistoryItem.RewardCodeDisplayName != "兑换时名称" || first.HistoryItem.MaskedCodeHint != "•••• 2030" {
		t.Fatalf("first redemption = %+v error=%v", first, err)
	}
	if _, err := service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "未来名称", CreditAmount: 20,
		MaxRedemptions: 0, Code: rewardCodeString("SNAPSHOT-NEW-2040"), ActorUserID: 1,
	}); err != nil {
		t.Fatal(err)
	}

	replay, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "SNAPSHOT-NEW-2040"})
	if err != nil || !replay.Replayed || replay.HistoryItem != first.HistoryItem {
		t.Fatalf("replayed redemption = %+v error=%v, want original snapshot %+v", replay, err, first.HistoryItem)
	}
	oldHistory, err := service.ListRedemptions(t.Context(), ListRedemptionsInput{UserID: 3})
	if err != nil || len(oldHistory.Redemptions) != 1 || oldHistory.Redemptions[0] != first.HistoryItem {
		t.Fatalf("old user history = %+v error=%v, want original snapshot %+v", oldHistory, err, first.HistoryItem)
	}

	second, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: "SNAPSHOT-NEW-2040"})
	if err != nil || second.Replayed || second.HistoryItem.RewardCodeDisplayName != "未来名称" || second.HistoryItem.MaskedCodeHint != "•••• 2040" {
		t.Fatalf("future redemption = %+v error=%v", second, err)
	}
	newHistory, err := service.ListRedemptions(t.Context(), ListRedemptionsInput{UserID: 7})
	if err != nil || len(newHistory.Redemptions) != 1 || newHistory.Redemptions[0] != second.HistoryItem {
		t.Fatalf("new user history = %+v error=%v, want future snapshot %+v", newHistory, err, second.HistoryItem)
	}
}

func TestRewardCodeDeleteAndRedeemSerializeWithoutOrphanFacts(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
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
	var lockedID int64
	if err := lockTx.QueryRow(t.Context(), `SELECT id FROM reward_codes WHERE id = $1 FOR UPDATE`, created.RewardCode.ID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	type deleteOutcome struct {
		deleted DeletedRewardCode
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
		deleted, err := service.DeleteRewardCode(t.Context(), DeleteRewardCodeCommand{
			RewardCodeID: created.RewardCode.ID, ActorUserID: 1,
		})
		deleteDone <- deleteOutcome{deleted: deleted, err: err}
	}()
	go func() {
		close(redeemReady)
		<-start
		_, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "DELETE-REDEEM-2030"})
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
  AND query LIKE '%reward_codes%'
  AND query LIKE '%FOR UPDATE%'`).Scan(&lockWaiters); err != nil {
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
	case deleteResult.err == nil && errors.Is(redeemErr, ErrCodeUnavailable):
		if deleteResult.deleted.RewardCodeID != created.RewardCode.ID || rewardCodes != 0 || redemptions != 0 || grants != 0 {
			t.Fatalf("delete-first facts deleted=%+v codes=%d redemptions=%d grants=%d", deleteResult.deleted, rewardCodes, redemptions, grants)
		}
	case errors.Is(deleteResult.err, ErrRewardCodeRedeemed) && redeemErr == nil:
		if rewardCodes != 1 || redemptions != 1 || grants != 1 {
			t.Fatalf("redeem-first facts codes=%d redemptions=%d grants=%d", rewardCodes, redemptions, grants)
		}
	default:
		t.Fatalf("Delete/Redeem race outcomes delete=%+v error=%v redeemError=%v", deleteResult.deleted, deleteResult.err, redeemErr)
	}
}

func TestRewardCodeRotationIsAtomicReplaySafeAndRevokesOldCode(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "轮换奖励", Code: "OLD-ROTATE-2030", CreditAmount: 12,
		IdempotencyKey: "rotate-management", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "其他奖励", Code: "OTHER-ROTATE-2030", CreditAmount: 1,
		IdempotencyKey: "rotate-other", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	rotations := make([]UpdatedRewardCode, 2)
	rotationErrors := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range rotations {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			rotations[index], rotationErrors[index] = service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
				RewardCodeID: created.RewardCode.ID, DisplayName: "轮换奖励", CreditAmount: 12,
				MaxRedemptions: 0, Code: rewardCodeString("NEW-ROTATE-2030"), ActorUserID: 1,
			})
		}(index)
	}
	close(start)
	wait.Wait()
	replayed, plaintextResponses := 0, 0
	for index, rotationError := range rotationErrors {
		if rotationError != nil {
			t.Fatalf("concurrent UpdateRewardCode() %d error = %v", index, rotationError)
		}
		if rotations[index].Replayed != nil && *rotations[index].Replayed {
			replayed++
		}
		if rotations[index].Plaintext == "NEWROTATE2030" {
			plaintextResponses++
		}
		if rotations[index].RewardCode.MaskedCodeHint != "•••• 2030" {
			t.Fatalf("concurrent rotation %d = %+v", index, rotations[index])
		}
	}
	if replayed != 1 || plaintextResponses != 1 {
		t.Fatalf("concurrent rotations = %+v, want one rotation and one replay", rotations)
	}
	replay, err := service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "轮换奖励", CreditAmount: 12,
		MaxRedemptions: 0, Code: rewardCodeString("NEW-ROTATE-2030"), ActorUserID: 1,
	})
	if err != nil || replay.Replayed == nil || !*replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("rotation replay = %+v error=%v", replay, err)
	}
	if _, err := service.UpdateRewardCode(t.Context(), UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "不应提交", CreditAmount: 99,
		MaxRedemptions: 0, Code: rewardCodeString("OTHER-ROTATE-2030"), ActorUserID: 1,
	}); !errors.Is(err, ErrCodeCollision) {
		t.Fatalf("rotation collision error = %v", err)
	}
	var displayName string
	var creditAmount int64
	if err := pool.QueryRow(t.Context(), `SELECT display_name, credit_amount FROM reward_codes WHERE id = $1`,
		created.RewardCode.ID).Scan(&displayName, &creditAmount); err != nil {
		t.Fatal(err)
	}
	if displayName != "轮换奖励" || creditAmount != 12 {
		t.Fatalf("collision committed rule update display_name=%q credit_amount=%d", displayName, creditAmount)
	}
	if other.RewardCode.ID == created.RewardCode.ID {
		t.Fatal("rotation collision fixture reused RewardCode")
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "OLD-ROTATE-2030"}); !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("old Code Redeem() error = %v", err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "NEW-ROTATE-2030"}); err != nil {
		t.Fatalf("new Code Redeem() error = %v", err)
	}
	var totalActions, rotationActions int
	var sensitiveJournal bool
	if err := pool.QueryRow(t.Context(), `SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE action_type = 'rotate_reward_code'),
  bool_or((metadata || target_snapshot)::TEXT LIKE '%NEWROTATE2030%'
    OR (metadata || target_snapshot)::TEXT LIKE '%OLDROTATE2030%')
FROM reward_admin_actions WHERE reward_code_id_snapshot = $1`, created.RewardCode.ID).Scan(
		&totalActions, &rotationActions, &sensitiveJournal,
	); err != nil {
		t.Fatal(err)
	}
	if totalActions != 2 || rotationActions != 1 || sensitiveJournal {
		t.Fatalf("rotation Journal total=%d rotations=%d sensitive=%t", totalActions, rotationActions, sensitiveJournal)
	}
	var fullDigest, journalText string
	if err := pool.QueryRow(t.Context(), `SELECT encode(code_digest, 'hex') FROM reward_codes WHERE id = $1`,
		created.RewardCode.ID).Scan(&fullDigest); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT string_agg((metadata || target_snapshot)::TEXT, '')
FROM reward_admin_actions WHERE reward_code_id_snapshot = $1`, created.RewardCode.ID).Scan(&journalText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(journalText, fullDigest) {
		t.Fatal("rotation Journal exposed a full Code digest")
	}
}

func TestRewardCodeDeleteRequiresZeroRedemptionsAndPreservesJournalSnapshot(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	deletable, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "误建奖励", Code: "DELETE-CODE-2030", CreditAmount: 3,
		IdempotencyKey: "delete-code", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := service.DeleteRewardCode(t.Context(), DeleteRewardCodeCommand{
		RewardCodeID: deletable.RewardCode.ID, ActorUserID: 1,
	})
	if err != nil || deleted.RewardCodeID != deletable.RewardCode.ID {
		t.Fatalf("DeleteRewardCode() = %+v error=%v", deleted, err)
	}
	var definitions, aliases int
	var journalCodeID *int64
	var journalSnapshotID int64
	var journalSnapshotName string
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT COUNT(*) FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM reward_code_digest_aliases WHERE reward_code_id = $1),
  reward_code_id,
  reward_code_id_snapshot,
  target_snapshot->>'display_name'
FROM reward_admin_actions
WHERE reward_code_id_snapshot = $1 AND action_type = 'delete_reward_code'`, deletable.RewardCode.ID).Scan(
		&definitions, &aliases, &journalCodeID, &journalSnapshotID, &journalSnapshotName,
	); err != nil {
		t.Fatal(err)
	}
	if definitions != 0 || aliases != 0 || journalCodeID != nil ||
		journalSnapshotID != deletable.RewardCode.ID || journalSnapshotName != "误建奖励" {
		t.Fatalf("delete facts definitions=%d aliases=%d code_id=%v snapshot_id=%d name=%q",
			definitions, aliases, journalCodeID, journalSnapshotID, journalSnapshotName)
	}

	redeemed, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "已兑奖励", Code: "REDEEMED-DELETE-2030", CreditAmount: 4,
		IdempotencyKey: "redeemed-delete", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: "REDEEMED-DELETE-2030"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteRewardCode(t.Context(), DeleteRewardCodeCommand{
		RewardCodeID: redeemed.RewardCode.ID, ActorUserID: 1,
	}); !errors.Is(err, ErrRewardCodeRedeemed) {
		t.Fatalf("redeemed DeleteRewardCode() error = %v", err)
	}
}

type failingGrantCreditService struct {
	reader *credits.Service
}

func rewardCodeString(value string) *string {
	return &value
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
