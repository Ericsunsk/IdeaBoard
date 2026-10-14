package tests

import (
	"bytes"
	"errors"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
	"strings"
	"testing"
	"time"
)

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
	provisionRewardWallet(t, pool, creditService, 7)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: "WINTER-EXPIRED-2030"}); err != nil {
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
