package rewards

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminGeneratesSingleUseCodeBatchWithoutPersistingPlaintext(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	keyring := newTestRewardKeyring(t)
	random := bytes.NewReader(append(bytes.Repeat([]byte{0x00}, 10), bytes.Repeat([]byte{0xff}, 10)...))
	service := NewServiceWithOptions(pool, credits.NewService(pool), keyring, ServiceOptions{
		Random: random,
	})
	var commands AdminCommands = service

	campaign, err := commands.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: "单次码奖励", CreditAmount: 80, Mode: CampaignModeSingleUse,
		MaxRedemptions: 0, ActorUserID: 1, Reason: "创建不限总量的单次码奖励",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	generated, err := commands.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "single-use-batch-001",
		ActorUserID: 1, Reason: "发行首批单次兑换码",
	})
	if err != nil {
		t.Fatalf("GenerateCodeBatch() error = %v", err)
	}
	if generated.Batch.CampaignID != campaign.ID || generated.Batch.CodeCount != 2 ||
		generated.ExportUnavailable || len(generated.Plaintexts) != 2 {
		t.Fatalf("generated Batch = %+v, want metadata and two first-response plaintexts", generated)
	}
	format := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}(?:-[0-9A-HJKMNP-TV-Z]{4}){3}$`)
	if generated.Plaintexts[0] != "0000-0000-0000-0000" ||
		generated.Plaintexts[1] != "ZZZZ-ZZZZ-ZZZZ-ZZZZ" {
		t.Fatalf("plaintexts = %q, want stable Crockford Base32 grouping", generated.Plaintexts)
	}
	for _, plaintext := range generated.Plaintexts {
		if !format.MatchString(plaintext) || strings.ContainsAny(plaintext, "ILOU") {
			t.Fatalf("generated plaintext %q is not human-readable Crockford Base32", plaintext)
		}
	}

	var codeCount int
	var plaintextStored bool
	if err := pool.QueryRow(t.Context(), `
SELECT COUNT(DISTINCT codes.id), COALESCE(bool_or(metadata::TEXT LIKE '%' || $2 || '%'), FALSE)
FROM reward_codes codes
CROSS JOIN reward_admin_actions actions
WHERE codes.batch_id = $1 AND actions.campaign_id = $3`,
		generated.Batch.ID, generated.Plaintexts[0], campaign.ID,
	).Scan(&codeCount, &plaintextStored); err != nil {
		t.Fatalf("read generated Batch persistence: %v", err)
	}
	if codeCount != 2 || plaintextStored {
		t.Fatalf("persisted Batch code_count=%d plaintext_in_journal=%t, want 2 and false", codeCount, plaintextStored)
	}
}

func TestCodeBatchGenerationReplaysMetadataButNeverPlaintext(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewServiceWithOptions(pool, credits.NewService(pool), newTestRewardKeyring(t), ServiceOptions{
		Random: rewardEntropy(0x2a, 0x2b),
	})
	campaign := createSingleUseRewardDraft(t, service, "幂等批次奖励", 10)
	command := GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "idempotent-batch",
		ActorUserID: 1, Reason: "首次生成幂等批次",
	}
	first, err := service.GenerateCodeBatch(t.Context(), command)
	if err != nil {
		t.Fatalf("first GenerateCodeBatch() error = %v", err)
	}
	replay, err := service.GenerateCodeBatch(t.Context(), command)
	if err != nil {
		t.Fatalf("replayed GenerateCodeBatch() error = %v", err)
	}
	if replay.Batch.ID != first.Batch.ID || !replay.Replayed || !replay.ExportUnavailable || len(replay.Plaintexts) != 0 {
		t.Fatalf("replay = %+v, want same metadata without plaintext", replay)
	}
	command.Count = 3
	if _, err := service.GenerateCodeBatch(t.Context(), command); !errors.Is(err, ErrCodeBatchConflict) {
		t.Fatalf("conflicting GenerateCodeBatch() error = %v, want ErrCodeBatchConflict", err)
	}
	var issueActions int
	if err := pool.QueryRow(t.Context(), `
SELECT COUNT(*) FROM reward_admin_actions
WHERE campaign_id = $1 AND action_type = 'issue_code_batch'`, campaign.ID).Scan(&issueActions); err != nil {
		t.Fatal(err)
	}
	if issueActions != 1 {
		t.Fatalf("issue_code_batch Journal count = %d, want 1", issueActions)
	}
}

func TestCodeBatchCollisionRollsBackBatchCodesAndJournal(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewServiceWithOptions(pool, credits.NewService(pool), newTestRewardKeyring(t), ServiceOptions{
		Random: bytes.NewReader(bytes.Repeat([]byte{0x00}, 20)),
	})
	campaign := createSingleUseRewardDraft(t, service, "碰撞回滚奖励", 10)

	_, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "collision-batch",
		ActorUserID: 1, Reason: "验证碰撞整批回滚",
	})
	if !errors.Is(err, ErrCodeCollision) {
		t.Fatalf("GenerateCodeBatch() error = %v, want ErrCodeCollision", err)
	}
	var batches, codes, actions int
	if err := pool.QueryRow(t.Context(), `
SELECT
  (SELECT COUNT(*) FROM reward_code_batches WHERE campaign_id = $1),
  (SELECT COUNT(*) FROM reward_codes WHERE campaign_id = $1),
  (SELECT COUNT(*) FROM reward_admin_actions WHERE campaign_id = $1 AND action_type = 'issue_code_batch')`,
		campaign.ID,
	).Scan(&batches, &codes, &actions); err != nil {
		t.Fatal(err)
	}
	if batches != 0 || codes != 0 || actions != 0 {
		t.Fatalf("rollback state batches=%d codes=%d actions=%d, want all zero", batches, codes, actions)
	}
}

func TestCodeBatchGenerationRejectsMoreThanSynchronousLimit(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	campaign := createSingleUseRewardDraft(t, service, "批次上限奖励", 0)
	_, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 10001, IdempotencyKey: "too-large",
		ActorUserID: 1, Reason: "验证同步上限",
	})
	if !errors.Is(err, ErrCodeBatchLimit) {
		t.Fatalf("GenerateCodeBatch() error = %v, want ErrCodeBatchLimit", err)
	}
}

func TestSingleUseCodeCanRedeemOnceAndUserCanRedeemCampaignOnce(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Random: bytes.NewReader(append(bytes.Repeat([]byte{0x10}, 10), bytes.Repeat([]byte{0x20}, 10)...)),
	})
	campaign := createSingleUseRewardDraft(t, service, "单次兑换约束", 2)
	generated, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "redeem-once",
		ActorUserID: 1, Reason: "生成单次兑换约束测试码",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活单次兑换活动",
	}); err != nil {
		t.Fatalf("ActivateCampaign() error = %v", err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)

	first, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: generated.Plaintexts[0]})
	if err != nil {
		t.Fatalf("first Redeem() error = %v", err)
	}
	replay, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: generated.Plaintexts[0]})
	if err != nil || !replay.Replayed || replay.Redemption.ID != first.Redemption.ID {
		t.Fatalf("same user replay = %+v error=%v, want original Redemption", replay, err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: generated.Plaintexts[0]}); !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("second user same Code error = %v, want ErrCodeUnavailable", err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: generated.Plaintexts[1]}); !errors.Is(err, ErrAlreadyRedeemed) {
		t.Fatalf("same user different Code error = %v, want ErrAlreadyRedeemed", err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: generated.Plaintexts[1]}); err != nil {
		t.Fatalf("second Code Redeem() error = %v", err)
	}
	campaign, err = service.GetCampaign(t.Context(), campaign.ID)
	if err != nil || campaign.RedemptionCount != 2 || campaign.Status != CampaignStatusExhausted {
		t.Fatalf("Campaign after both Codes = %+v error=%v, want exhausted with 2 Redemptions", campaign, err)
	}
}

func TestSingleUseCampaignCanAppendAfterCodeExhaustionWithinOptionalCap(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Random: bytes.NewReader(append(bytes.Repeat([]byte{0x31}, 10), bytes.Repeat([]byte{0x32}, 10)...)),
	})
	campaign := createSingleUseRewardDraft(t, service, "可追加批次奖励", 0)
	first, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 1, IdempotencyKey: "append-first",
		ActorUserID: 1, Reason: "生成首批单次码",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活不限总量单次码活动",
	}); err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: first.Plaintexts[0]}); err != nil {
		t.Fatal(err)
	}
	exhausted, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil || exhausted.Status != CampaignStatusExhausted || exhausted.ValidUnusedCodeCount != 0 {
		t.Fatalf("Campaign after only Code redeemed = %+v error=%v, want Code-exhausted", exhausted, err)
	}
	second, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 1, IdempotencyKey: "append-second",
		ActorUserID: 1, Reason: "补充已耗尽的单次码",
	})
	if err != nil || len(second.Plaintexts) != 1 {
		t.Fatalf("append GenerateCodeBatch() = %+v error=%v", second, err)
	}
	active, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil || active.Status != CampaignStatusActive || active.ValidUnusedCodeCount != 1 {
		t.Fatalf("Campaign after append = %+v error=%v, want active with one unused Code", active, err)
	}
}

func TestAdminListsAndRevokesOnlyUnusedCodesInBatch(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Random: bytes.NewReader(append(bytes.Repeat([]byte{0x41}, 10), bytes.Repeat([]byte{0x42}, 10)...)),
	})
	campaign := createSingleUseRewardDraft(t, service, "批次撤销奖励", 2)
	generated, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "revoke-unused",
		ActorUserID: 1, Reason: "生成待撤销批次",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活待撤销批次",
	}); err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	redemption, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: generated.Plaintexts[0]})
	if err != nil {
		t.Fatal(err)
	}

	revoked, err := service.RevokeCodeBatch(t.Context(), RevokeCodeBatchCommand{
		CampaignID: campaign.ID, BatchID: generated.Batch.ID, Confirmed: true,
		ActorUserID: 1, Reason: "导出文件丢失，撤销未使用 Code",
	})
	if err != nil {
		t.Fatalf("RevokeCodeBatch() error = %v", err)
	}
	if revoked.RedeemedCount != 1 || revoked.RevokedUnusedCount != 1 || revoked.ValidUnusedCount != 0 {
		t.Fatalf("revoked Batch = %+v, want one redeemed and one revoked-unused", revoked)
	}
	page, err := service.ListCodeBatches(t.Context(), ListCodeBatchesInput{
		CampaignID: campaign.ID, Limit: 20,
	})
	if err != nil || len(page.Batches) != 1 || page.Batches[0].ID != generated.Batch.ID {
		t.Fatalf("ListCodeBatches() = %+v error=%v", page, err)
	}
	if page.Batches[0].RedeemedCount != 1 || page.Batches[0].RevokedUnusedCount != 1 {
		t.Fatalf("listed Batch = %+v, want recovery metadata", page.Batches[0])
	}

	var redeemedRevokedAt, unusedRevokedAt *time.Time
	if err := pool.QueryRow(t.Context(), `
SELECT
  max(revoked_at) FILTER (WHERE id = $2),
  max(revoked_at) FILTER (WHERE id <> $2)
FROM reward_codes
WHERE batch_id = $1`, generated.Batch.ID, redemption.Redemption.CodeID).Scan(
		&redeemedRevokedAt, &unusedRevokedAt,
	); err != nil {
		t.Fatal(err)
	}
	if redeemedRevokedAt != nil || unusedRevokedAt == nil {
		t.Fatalf("revocation state redeemed=%v unused=%v, want redeemed unchanged and unused revoked", redeemedRevokedAt, unusedRevokedAt)
	}
	wallet, err := creditService.GetWallet(t.Context(), 3)
	if err != nil || wallet.Available != campaign.CreditAmount {
		t.Fatalf("Wallet after Batch revoke = %+v error=%v, want existing Grant unchanged", wallet, err)
	}
}

func TestCancelSingleUseCampaignRevokesOnlyUnusedCodes(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Random: bytes.NewReader(append(bytes.Repeat([]byte{0x51}, 10), bytes.Repeat([]byte{0x52}, 10)...)),
	})
	campaign := createSingleUseRewardDraft(t, service, "取消单次码奖励", 2)
	generated, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "cancel-unused",
		ActorUserID: 1, Reason: "生成取消测试批次",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活取消测试批次",
	}); err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)
	first, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: generated.Plaintexts[0]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelCampaign(t.Context(), CancelCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Confirmed: true, Reason: "取消单次码活动",
	}); err != nil {
		t.Fatalf("CancelCampaign() error = %v", err)
	}
	replay, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: generated.Plaintexts[0]})
	if err != nil || !replay.Replayed || replay.Redemption.ID != first.Redemption.ID {
		t.Fatalf("redeemed Code replay after cancel = %+v error=%v", replay, err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: generated.Plaintexts[1]}); !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("unused Code after cancel error = %v, want ErrCodeUnavailable", err)
	}
	var redeemedRevoked, unusedRevoked bool
	if err := pool.QueryRow(t.Context(), `
SELECT
  bool_or(revoked_at IS NOT NULL) FILTER (WHERE id = $2),
  bool_or(revoked_at IS NOT NULL) FILTER (WHERE id <> $2)
FROM reward_codes WHERE batch_id = $1`, generated.Batch.ID, first.Redemption.CodeID).Scan(
		&redeemedRevoked, &unusedRevoked,
	); err != nil {
		t.Fatal(err)
	}
	if redeemedRevoked || !unusedRevoked {
		t.Fatalf("cancel revocation redeemed=%t unused=%t, want false/true", redeemedRevoked, unusedRevoked)
	}
}

func TestCodeBatchGenerationHonorsCapacityFormula(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Random: rewardEntropy(0x61, 0x62, 0x63),
	})
	campaign := createSingleUseRewardDraft(t, service, "批次容量奖励", 3)
	first, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "capacity-first",
		ActorUserID: 1, Reason: "生成容量首批",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活容量活动",
	}); err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: first.Plaintexts[0]}); err != nil {
		t.Fatal(err)
	}
	_, err = service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "capacity-over",
		ActorUserID: 1, Reason: "验证已完成加有效未使用不超 cap",
	})
	if !errors.Is(err, ErrCampaignExhausted) {
		t.Fatalf("over-cap GenerateCodeBatch() error = %v, want ErrCampaignExhausted", err)
	}
	if _, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 1, IdempotencyKey: "capacity-final",
		ActorUserID: 1, Reason: "补满剩余容量",
	}); err != nil {
		t.Fatalf("within-cap GenerateCodeBatch() error = %v", err)
	}
}

func TestCodeBatchGenerationAllowsLifecycleStatesButRejectsExpiry(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	startsAt := now.Add(time.Hour)
	expiresAt := now.Add(2 * time.Hour)
	service := NewServiceWithOptions(pool, credits.NewService(pool), newTestRewardKeyring(t), ServiceOptions{
		Now: func() time.Time { return now }, Random: rewardEntropy(0x71, 0x72, 0x73),
	})
	campaign, err := service.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: "生命周期追加奖励", CreditAmount: 80, Mode: CampaignModeSingleUse,
		MaxRedemptions: 0, StartsAt: &startsAt, ExpiresAt: &expiresAt,
		ActorUserID: 1, Reason: "创建排期单次码活动",
	})
	if err != nil {
		t.Fatal(err)
	}
	generate := func(key string) error {
		_, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
			CampaignID: campaign.ID, Count: 1, IdempotencyKey: key,
			ActorUserID: 1, Reason: "验证生命周期追加",
		})
		return err
	}
	if err := generate("draft-batch"); err != nil {
		t.Fatalf("Draft generation error = %v", err)
	}
	activated, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活排期活动",
	})
	if err != nil || activated.Status != CampaignStatusScheduled {
		t.Fatalf("scheduled activation = %+v error=%v", activated, err)
	}
	if err := generate("scheduled-batch"); err != nil {
		t.Fatalf("scheduled generation error = %v", err)
	}
	if _, err := service.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "暂停排期活动",
	}); err != nil {
		t.Fatal(err)
	}
	if err := generate("paused-batch"); err != nil {
		t.Fatalf("paused generation error = %v", err)
	}
	now = expiresAt
	if err := generate("expired-batch"); !errors.Is(err, ErrCampaignExpired) {
		t.Fatalf("expired generation error = %v, want ErrCampaignExpired", err)
	}
}

func TestCodeBatchRandomFailureRollsBackAtomically(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewServiceWithOptions(pool, credits.NewService(pool), newTestRewardKeyring(t), ServiceOptions{
		Random: bytes.NewReader(bytes.Repeat([]byte{0x77}, 15)),
	})
	campaign := createSingleUseRewardDraft(t, service, "随机源失败回滚", 2)
	if _, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 2, IdempotencyKey: "rng-failure",
		ActorUserID: 1, Reason: "验证随机源失败回滚",
	}); err == nil {
		t.Fatal("GenerateCodeBatch() succeeded, want entropy failure")
	}
	var batches, codes, actions int
	if err := pool.QueryRow(t.Context(), `
SELECT
  (SELECT COUNT(*) FROM reward_code_batches WHERE campaign_id = $1),
  (SELECT COUNT(*) FROM reward_codes WHERE campaign_id = $1),
  (SELECT COUNT(*) FROM reward_admin_actions WHERE campaign_id = $1 AND action_type = 'issue_code_batch')`,
		campaign.ID,
	).Scan(&batches, &codes, &actions); err != nil {
		t.Fatal(err)
	}
	if batches != 0 || codes != 0 || actions != 0 {
		t.Fatalf("RNG rollback batches=%d codes=%d actions=%d", batches, codes, actions)
	}
}

func TestConcurrentUsersCannotRedeemSingleUseCodeTwice(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Random: rewardEntropy(0x81),
	})
	campaign := createSingleUseRewardDraft(t, service, "并发单次码奖励", 0)
	generated, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: campaign.ID, Count: 1, IdempotencyKey: "concurrent-single-use",
		ActorUserID: 1, Reason: "生成并发测试码",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活并发测试活动",
	}); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{3, 7} {
		provisionRewardWallet(t, pool, creditService, userID)
	}
	start := make(chan struct{})
	errorsByUser := make(chan error, 2)
	var workers sync.WaitGroup
	for _, userID := range []int64{3, 7} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := service.Redeem(t.Context(), RedeemCommand{UserID: userID, Code: generated.Plaintexts[0]})
			errorsByUser <- err
		}()
	}
	close(start)
	workers.Wait()
	close(errorsByUser)
	var succeeded, unavailable int
	for err := range errorsByUser {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrCodeUnavailable):
			unavailable++
		default:
			t.Fatalf("concurrent Redeem() unexpected error = %v", err)
		}
	}
	if succeeded != 1 || unavailable != 1 {
		t.Fatalf("concurrent results success=%d unavailable=%d, want 1/1", succeeded, unavailable)
	}
	campaign, err = service.GetCampaign(t.Context(), campaign.ID)
	if err != nil || campaign.RedemptionCount != 1 || campaign.ValidUnusedCodeCount != 0 {
		t.Fatalf("Campaign after concurrent redemption = %+v error=%v", campaign, err)
	}
}

func TestCampaignCannotMixPublicAndSingleUseCodes(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewServiceWithOptions(pool, credits.NewService(pool), newTestRewardKeyring(t), ServiceOptions{
		Random: rewardEntropy(0x91),
	})
	public := createRewardDraft(t, service, "公共码互斥")
	if _, err := service.GenerateCodeBatch(t.Context(), GenerateCodeBatchCommand{
		CampaignID: public.ID, Count: 1, IdempotencyKey: "public-cannot-batch",
		ActorUserID: 1, Reason: "验证公共码互斥",
	}); !errors.Is(err, ErrCampaignModeMismatch) {
		t.Fatalf("public GenerateCodeBatch() error = %v, want ErrCampaignModeMismatch", err)
	}
	singleUse := createSingleUseRewardDraft(t, service, "单次码互斥", 1)
	if _, err := service.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: singleUse.ID, Code: "NOMIX2026", ActorUserID: 1, Reason: "验证单次码互斥",
	}); !errors.Is(err, ErrCampaignModeMismatch) {
		t.Fatalf("single-use IssuePublicCode() error = %v, want ErrCampaignModeMismatch", err)
	}
}

func TestAdminSchedulesPausesAndResumesPublicCampaign(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	var commands AdminCommands = service
	startsAt := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	expiresAt := startsAt.Add(2 * time.Hour)

	campaign, err := commands.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: "排期奖励", CreditAmount: 80, MaxRedemptions: 3,
		StartsAt: &startsAt, ExpiresAt: &expiresAt,
		ActorUserID: 1, Reason: "创建排期奖励",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	if _, err := commands.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: campaign.ID, Code: "SCHEDULE26", ActorUserID: 1, Reason: "发行排期 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	activated, err := commands.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活排期奖励",
	})
	if err != nil {
		t.Fatalf("ActivateCampaign() error = %v", err)
	}
	if activated.LifecycleStatus != CampaignLifecycleActive || activated.Status != CampaignStatusScheduled {
		t.Fatalf("activated Campaign = %+v, want active lifecycle with scheduled display state", activated)
	}

	paused, err := commands.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "临时暂停排期奖励",
	})
	if err != nil {
		t.Fatalf("PauseCampaign() error = %v", err)
	}
	if paused.LifecycleStatus != CampaignLifecyclePaused || paused.Status != CampaignStatusPaused {
		t.Fatalf("paused Campaign = %+v, want paused lifecycle and display state", paused)
	}

	resumed, err := commands.ResumeCampaign(t.Context(), ResumeCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "恢复排期奖励",
	})
	if err != nil {
		t.Fatalf("ResumeCampaign() error = %v", err)
	}
	if resumed.LifecycleStatus != CampaignLifecycleActive || resumed.Status != CampaignStatusScheduled {
		t.Fatalf("resumed Campaign = %+v, want active lifecycle with scheduled display state", resumed)
	}
}

func TestFirstIssuedCodeFreezesCampaignRulesButNotInternalNote(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	var commands AdminCommands = service
	startsAt := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	expiresAt := startsAt.Add(2 * time.Hour)
	campaign := createRewardDraft(t, service, "冻结规则奖励")

	updated, err := commands.UpdateCampaign(t.Context(), UpdateCampaignCommand{
		CampaignID: campaign.ID, DisplayName: "冻结规则奖励 v2", InternalNote: "发行前可编辑",
		CreditAmount: 90, MaxRedemptions: 4, StartsAt: &startsAt, ExpiresAt: &expiresAt,
		ActorUserID: 1, Reason: "发行前调整规则",
	})
	if err != nil {
		t.Fatalf("UpdateCampaign() before issuance error = %v", err)
	}
	if updated.DisplayName != "冻结规则奖励 v2" || updated.CreditAmount != 90 || updated.MaxRedemptions != 4 {
		t.Fatalf("updated Draft = %+v, want editable rules before issuance", updated)
	}
	if _, err := commands.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: campaign.ID, Code: "FREEZE2026", ActorUserID: 1, Reason: "发行冻结规则 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}

	_, err = commands.UpdateCampaign(t.Context(), UpdateCampaignCommand{
		CampaignID: campaign.ID, DisplayName: updated.DisplayName, InternalNote: "尝试改规则",
		CreditAmount: 100, MaxRedemptions: updated.MaxRedemptions,
		StartsAt: updated.StartsAt, ExpiresAt: updated.ExpiresAt,
		ActorUserID: 1, Reason: "验证首码冻结规则",
	})
	if !errors.Is(err, ErrCampaignRulesFrozen) {
		t.Fatalf("UpdateCampaign() after issuance error = %v, want ErrCampaignRulesFrozen", err)
	}

	noteOnly, err := commands.UpdateCampaign(t.Context(), UpdateCampaignCommand{
		CampaignID: campaign.ID, DisplayName: updated.DisplayName, InternalNote: "发行后仍可维护备注",
		CreditAmount: updated.CreditAmount, MaxRedemptions: updated.MaxRedemptions,
		StartsAt: updated.StartsAt, ExpiresAt: updated.ExpiresAt,
		ActorUserID: 1, Reason: "更新内部备注",
	})
	if err != nil {
		t.Fatalf("UpdateCampaign() note-only error = %v", err)
	}
	if noteOnly.InternalNote != "发行后仍可维护备注" {
		t.Fatalf("InternalNote = %q, want updated note", noteOnly.InternalNote)
	}
}

func TestAdminDeletesOnlyEmptyDraftCampaign(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	var commands AdminCommands = service

	empty := createRewardDraft(t, service, "待删除空 Draft")
	if err := commands.DeleteCampaign(t.Context(), DeleteCampaignCommand{
		CampaignID: empty.ID, ActorUserID: 1, Reason: "删除未使用 Draft",
	}); err != nil {
		t.Fatalf("DeleteCampaign() empty Draft error = %v", err)
	}
	if _, err := service.GetCampaign(t.Context(), empty.ID); !errors.Is(err, ErrCampaignNotFound) {
		t.Fatalf("GetCampaign() deleted Draft error = %v, want ErrCampaignNotFound", err)
	}

	issued := createRewardDraft(t, service, "已发行 Campaign")
	if _, err := commands.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: issued.ID, Code: "NODELETE26", ActorUserID: 1, Reason: "发行不可删除 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	err := commands.DeleteCampaign(t.Context(), DeleteCampaignCommand{
		CampaignID: issued.ID, ActorUserID: 1, Reason: "尝试删除已发行 Campaign",
	})
	if !errors.Is(err, ErrCampaignNotEmpty) {
		t.Fatalf("DeleteCampaign() issued Campaign error = %v, want ErrCampaignNotEmpty", err)
	}
	if _, err := service.GetCampaign(t.Context(), issued.ID); err != nil {
		t.Fatalf("GetCampaign() issued Campaign after rejected delete error = %v", err)
	}
}

func TestAdminLifecycleCommandsPersistAppendOnlyJournalWithoutCodePlaintext(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	campaign := createRewardDraft(t, service, "Journal 奖励")
	updated, err := service.UpdateCampaign(t.Context(), UpdateCampaignCommand{
		CampaignID: campaign.ID, DisplayName: "Journal 奖励", InternalNote: "仅内部可见",
		CreditAmount: campaign.CreditAmount, MaxRedemptions: campaign.MaxRedemptions,
		ActorUserID: 1, Reason: "更新 Journal 奖励备注",
	})
	if err != nil {
		t.Fatalf("UpdateCampaign() error = %v", err)
	}
	const plaintextCode = "JOURNAL26"
	if _, err := service.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: campaign.ID, Code: plaintextCode, ActorUserID: 1, Reason: "发行 Journal Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活 Journal 奖励",
	}); err != nil {
		t.Fatalf("ActivateCampaign() error = %v", err)
	}
	if _, err := service.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "暂停 Journal 奖励",
	}); err != nil {
		t.Fatalf("PauseCampaign() error = %v", err)
	}
	if _, err := service.ResumeCampaign(t.Context(), ResumeCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "恢复 Journal 奖励",
	}); err != nil {
		t.Fatalf("ResumeCampaign() error = %v", err)
	}
	if _, err := service.CancelCampaign(t.Context(), CancelCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Confirmed: true, Reason: "取消 Journal 奖励",
	}); err != nil {
		t.Fatalf("CancelCampaign() error = %v", err)
	}

	wantActions := []string{
		"create_campaign", "update_campaign", "issue_public_code", "activate_campaign",
		"pause_campaign", "resume_campaign", "cancel_campaign",
	}
	rows, err := pool.Query(t.Context(), `
SELECT action_type, actor_user_id, reason, metadata::TEXT
FROM reward_admin_actions
WHERE campaign_id = $1
ORDER BY id`, campaign.ID)
	if err != nil {
		t.Fatalf("read Campaign Journal: %v", err)
	}
	defer rows.Close()
	var gotActions []string
	for rows.Next() {
		var actionType, reason, metadata string
		var actorUserID int64
		if err := rows.Scan(&actionType, &actorUserID, &reason, &metadata); err != nil {
			t.Fatalf("scan Campaign Journal: %v", err)
		}
		if actorUserID != 1 || strings.TrimSpace(reason) == "" {
			t.Fatalf("Journal actor=%d reason=%q, want authenticated actor and non-empty reason", actorUserID, reason)
		}
		if strings.Contains(metadata, plaintextCode) {
			t.Fatalf("Journal metadata leaked Code plaintext: %s", metadata)
		}
		gotActions = append(gotActions, actionType)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Campaign Journal: %v", err)
	}
	if strings.Join(gotActions, ",") != strings.Join(wantActions, ",") {
		t.Fatalf("Journal actions = %v, want %v", gotActions, wantActions)
	}
	if updated.InternalNote != "仅内部可见" {
		t.Fatalf("updated internal note = %q", updated.InternalNote)
	}

	deleted := createRewardDraft(t, service, "删除后保留 Journal")
	if err := service.DeleteCampaign(t.Context(), DeleteCampaignCommand{
		CampaignID: deleted.ID, ActorUserID: 1, Reason: "删除空 Draft 并保留审计",
	}); err != nil {
		t.Fatalf("DeleteCampaign() error = %v", err)
	}
	var deleteActions int
	if err := pool.QueryRow(t.Context(), `
SELECT count(*)
FROM reward_admin_actions
WHERE campaign_id = $1 AND action_type IN ('create_campaign', 'delete_campaign')`, deleted.ID).Scan(&deleteActions); err != nil {
		t.Fatalf("read deleted Campaign Journal: %v", err)
	}
	if deleteActions != 2 {
		t.Fatalf("deleted Campaign Journal facts = %d, want 2", deleteActions)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE reward_admin_actions SET reason = 'tampered' WHERE campaign_id = $1`, campaign.ID); err == nil {
		t.Fatal("Journal UPDATE unexpectedly succeeded")
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM reward_admin_actions WHERE campaign_id = $1`, campaign.ID); err == nil {
		t.Fatal("Journal DELETE unexpectedly succeeded")
	}
}

func TestCancelCampaignRevokesPublicCodeButPreservesReplay(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	provisionRewardWallet(t, pool, creditService, 3)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "CANCEL2026", 70, 2)

	first, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() before cancellation error = %v", err)
	}
	cancelled, err := service.CancelCampaign(t.Context(), CancelCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Confirmed: true, Reason: "活动配置错误，终止发放",
	})
	if err != nil {
		t.Fatalf("CancelCampaign() error = %v", err)
	}
	if cancelled.LifecycleStatus != CampaignLifecycleCancelled || cancelled.Status != CampaignStatusCancelled {
		t.Fatalf("cancelled Campaign = %+v, want terminal cancelled state", cancelled)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: code}); !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("new Redeem() after cancellation error = %v, want ErrCodeUnavailable", err)
	}
	replayed, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() replay after cancellation error = %v", err)
	}
	if !replayed.Replayed || replayed.Redemption.ID != first.Redemption.ID {
		t.Fatalf("replay after cancellation = %+v, want original Redemption", replayed)
	}
}

func TestCampaignDerivedStateUsesInclusiveStartExclusiveEndAndStablePrecedence(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	service := newTestRewardServiceAt(t, pool, creditService, now)
	expiresAt := now.Add(time.Hour)

	scheduled, err := service.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: "边界排期奖励", CreditAmount: 40, MaxRedemptions: 2,
		StartsAt: &now, ExpiresAt: &expiresAt,
		ActorUserID: 1, Reason: "创建时间边界奖励",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	if _, err := service.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: scheduled.ID, Code: "BOUNDARY26", ActorUserID: 1, Reason: "发行边界 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	active, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: scheduled.ID, ActorUserID: 1, Reason: "激活边界奖励",
	})
	if err != nil {
		t.Fatalf("ActivateCampaign() error = %v", err)
	}
	if active.Status != CampaignStatusActive {
		t.Fatalf("status at starts_at = %q, want active", active.Status)
	}
	if _, err := service.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: scheduled.ID, ActorUserID: 1, Reason: "暂停至过期边界",
	}); err != nil {
		t.Fatalf("PauseCampaign() error = %v", err)
	}
	expiredService := newTestRewardServiceAt(t, pool, creditService, expiresAt)
	expired, err := expiredService.GetCampaign(t.Context(), scheduled.ID)
	if err != nil {
		t.Fatalf("GetCampaign() at expires_at error = %v", err)
	}
	if expired.Status != CampaignStatusExpired {
		t.Fatalf("status at expires_at = %q, want expired before paused", expired.Status)
	}

	exhaustedCampaign, code := createActivePublicCampaign(t, service, "EXHAUST26", 30, 1)
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code}); err != nil {
		t.Fatalf("Redeem() final capacity error = %v", err)
	}
	exhausted, err := service.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: exhaustedCampaign.ID, ActorUserID: 1, Reason: "暂停已耗尽奖励",
	})
	if err != nil {
		t.Fatalf("PauseCampaign() exhausted error = %v", err)
	}
	if exhausted.Status != CampaignStatusExhausted {
		t.Fatalf("exhausted paused status = %q, want exhausted before paused", exhausted.Status)
	}
}

func TestExpiredCampaignCannotActivateOrResume(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	start := time.Date(2026, time.July, 15, 9, 0, 0, 0, time.UTC)
	expiresAt := start.Add(time.Hour)
	beforeExpiry := newTestRewardServiceAt(t, pool, creditService, start)
	campaign, err := beforeExpiry.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: "过期边界奖励", CreditAmount: 50, MaxRedemptions: 2,
		StartsAt: &start, ExpiresAt: &expiresAt,
		ActorUserID: 1, Reason: "创建过期边界奖励",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	if _, err := beforeExpiry.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: campaign.ID, Code: "EXPIRED26", ActorUserID: 1, Reason: "发行过期边界 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	if _, err := beforeExpiry.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活过期边界奖励",
	}); err != nil {
		t.Fatalf("ActivateCampaign() before expiry error = %v", err)
	}
	if _, err := beforeExpiry.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "暂停但不顺延过期时间",
	}); err != nil {
		t.Fatalf("PauseCampaign() error = %v", err)
	}

	afterExpiry := newTestRewardServiceAt(t, pool, creditService, expiresAt)
	_, err = afterExpiry.ResumeCampaign(t.Context(), ResumeCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "尝试恢复已过期奖励",
	})
	if !errors.Is(err, ErrCampaignExpired) {
		t.Fatalf("ResumeCampaign() at expires_at error = %v, want ErrCampaignExpired", err)
	}

	expiredDraft, err := afterExpiry.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: "已过期 Draft", CreditAmount: 20, MaxRedemptions: 1,
		StartsAt: &start, ExpiresAt: &expiresAt,
		ActorUserID: 1, Reason: "创建已过期 Draft 验证激活",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() expired Draft error = %v", err)
	}
	if _, err := afterExpiry.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: expiredDraft.ID, Code: "OLDRAFT26", ActorUserID: 1, Reason: "发行已过期 Draft Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() expired Draft error = %v", err)
	}
	_, err = afterExpiry.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: expiredDraft.ID, ActorUserID: 1, Reason: "尝试激活已过期 Draft",
	})
	if !errors.Is(err, ErrCampaignExpired) {
		t.Fatalf("ActivateCampaign() expired Draft error = %v, want ErrCampaignExpired", err)
	}
}

func TestRevokedPublicCodeCannotActivateOrResumeCampaign(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))

	draft := createRewardDraft(t, service, "撤销后不可激活")
	if _, err := service.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: draft.ID, Code: "REVOKED126", ActorUserID: 1, Reason: "发行后撤销测试 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode() draft error = %v", err)
	}
	revokeRewardCodeForTest(t, pool, draft.ID)
	_, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: draft.ID, ActorUserID: 1, Reason: "尝试激活无可用 Code 的 Campaign",
	})
	if !errors.Is(err, ErrPublicCodeRequired) {
		t.Fatalf("ActivateCampaign() revoked Code error = %v, want ErrPublicCodeRequired", err)
	}

	paused, _ := createActivePublicCampaign(t, service, "REVOKED226", 20, 2)
	if _, err := service.PauseCampaign(t.Context(), PauseCampaignCommand{
		CampaignID: paused.ID, ActorUserID: 1, Reason: "暂停后撤销 Code",
	}); err != nil {
		t.Fatalf("PauseCampaign() error = %v", err)
	}
	revokeRewardCodeForTest(t, pool, paused.ID)
	_, err = service.ResumeCampaign(t.Context(), ResumeCampaignCommand{
		CampaignID: paused.ID, ActorUserID: 1, Reason: "尝试恢复无可用 Code 的 Campaign",
	})
	if !errors.Is(err, ErrPublicCodeRequired) {
		t.Fatalf("ResumeCampaign() revoked Code error = %v, want ErrPublicCodeRequired", err)
	}
}

func TestConcurrentUsersCannotExceedFinalPublicCapacity(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	userIDs := []int64{1, 3, 7, 8, 10, 11, 12, 42}
	for _, userID := range userIDs {
		provisionRewardWallet(t, pool, creditService, userID)
	}
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "FINALCAP26", 25, 1)

	type outcome struct {
		result RedeemResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, len(userIDs))
	var wait sync.WaitGroup
	for _, userID := range userIDs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := service.Redeem(t.Context(), RedeemCommand{UserID: userID, Code: code})
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(outcomes)

	successes := 0
	for outcome := range outcomes {
		if outcome.err == nil {
			successes++
			continue
		}
		if !errors.Is(outcome.err, ErrCampaignExhausted) {
			t.Fatalf("concurrent Redeem() error = %v, want ErrCampaignExhausted", outcome.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful final-capacity Redemptions = %d, want 1", successes)
	}
	exhausted, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil {
		t.Fatalf("GetCampaign() error = %v", err)
	}
	if exhausted.RedemptionCount != 1 || exhausted.Status != CampaignStatusExhausted {
		t.Fatalf("Campaign after race = %+v, want exactly one Redemption and exhausted", exhausted)
	}
}

func TestConcurrentSameUserSameCodeReplaysOneRedemption(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	service := newTestRewardService(t, pool, creditService)
	_, code := createActivePublicCampaign(t, service, "SAMEREPLAY26", 35, 10)

	const attempts = 8
	start := make(chan struct{})
	results := make(chan RedeemResult, attempts)
	errorsSeen := make(chan error, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
			if err != nil {
				errorsSeen <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("concurrent same-user Redeem() error = %v", err)
	}

	var redemptionID int64
	firstResponses := 0
	replayResponses := 0
	for result := range results {
		if redemptionID == 0 {
			redemptionID = result.Redemption.ID
		}
		if result.Redemption.ID != redemptionID {
			t.Fatalf("Redemption IDs differ: got %d want %d", result.Redemption.ID, redemptionID)
		}
		if result.Replayed {
			replayResponses++
		} else {
			firstResponses++
		}
	}
	if firstResponses != 1 || replayResponses != attempts-1 {
		t.Fatalf("responses first=%d replay=%d, want 1 and %d", firstResponses, replayResponses, attempts-1)
	}
	ledger, err := creditService.ListLedger(t.Context(), credits.ListLedgerInput{UserID: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(ledger.Transactions) != 1 {
		t.Fatalf("Ledger transactions = %d, want one Grant", len(ledger.Transactions))
	}
}

func TestPauseRaceCommitsBeforeOrAfterOneRedemptionWithoutSplitState(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "PAUSERACE26", 45, 2)

	start := make(chan struct{})
	pauseResult := make(chan error, 1)
	redeemResult := make(chan error, 1)
	go func() {
		<-start
		_, err := service.PauseCampaign(t.Context(), PauseCampaignCommand{
			CampaignID: campaign.ID, ActorUserID: 1, Reason: "并发暂停奖励",
		})
		pauseResult <- err
	}()
	go func() {
		<-start
		_, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
		redeemResult <- err
	}()
	close(start)
	if err := <-pauseResult; err != nil {
		t.Fatalf("PauseCampaign() race error = %v", err)
	}
	if err := <-redeemResult; err != nil && !errors.Is(err, ErrCodeUnavailable) {
		t.Fatalf("Redeem() race error = %v, want success or ErrCodeUnavailable", err)
	}

	paused, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil {
		t.Fatalf("GetCampaign() after pause race error = %v", err)
	}
	if paused.LifecycleStatus != CampaignLifecyclePaused || paused.RedemptionCount < 0 || paused.RedemptionCount > 1 {
		t.Fatalf("Campaign after pause race = %+v, want paused with zero or one Redemption", paused)
	}
	ledger, err := creditService.ListLedger(t.Context(), credits.ListLedgerInput{UserID: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() after pause race error = %v", err)
	}
	if int64(len(ledger.Transactions)) != paused.RedemptionCount {
		t.Fatalf("Ledger transactions=%d Campaign Redemptions=%d, want atomic agreement", len(ledger.Transactions), paused.RedemptionCount)
	}
}

func TestMissingWalletRollsBackWithoutConsumingCampaignCapacity(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "NOWALLET26", 55, 1)

	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: code}); !errors.Is(err, credits.ErrWalletNotFound) {
		t.Fatalf("Redeem() missing Wallet error = %v, want ErrWalletNotFound", err)
	}
	afterFailure, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil {
		t.Fatalf("GetCampaign() after missing Wallet error = %v", err)
	}
	if afterFailure.RedemptionCount != 0 {
		t.Fatalf("RedemptionCount after missing Wallet = %d, want 0", afterFailure.RedemptionCount)
	}
	provisionRewardWallet(t, pool, creditService, 7)
	result, err := service.Redeem(t.Context(), RedeemCommand{UserID: 7, Code: code})
	if err != nil {
		t.Fatalf("Redeem() after Wallet provision error = %v", err)
	}
	if result.AvailableBalance != 55 {
		t.Fatalf("available balance = %d, want 55", result.AvailableBalance)
	}
}

func TestCreditCorrectionDoesNotReopenCampaignCapacity(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	provisionRewardWallet(t, pool, creditService, 3)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "CORRECT26", 65, 1)
	redemption, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	amount, err := credits.NewCreditAmount(65)
	if err != nil {
		t.Fatalf("NewCreditAmount() error = %v", err)
	}
	if _, err := creditService.Correct(t.Context(), credits.CorrectionCommand{
		UserID:                1,
		OriginalTransactionID: redemption.Redemption.CreditTransactionID,
		Amount:                amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "reward-correction-no-reopen",
			Actor:          credits.Actor{Type: credits.ActorAdmin, ID: "1"},
			Source:         "reward_correction_test",
			Reason:         "验证更正不恢复奖励容量",
			BusinessReference: credits.BusinessReference{
				Type: "reward_test", ID: "reward-correction-no-reopen",
			},
		},
	}); err != nil {
		t.Fatalf("Correct() reward Grant error = %v", err)
	}
	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: code}); !errors.Is(err, ErrCampaignExhausted) {
		t.Fatalf("Redeem() after Correction error = %v, want ErrCampaignExhausted", err)
	}
	afterCorrection, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil {
		t.Fatalf("GetCampaign() after Correction error = %v", err)
	}
	if afterCorrection.RedemptionCount != 1 || afterCorrection.Status != CampaignStatusExhausted {
		t.Fatalf("Campaign after Correction = %+v, want capacity unchanged and exhausted", afterCorrection)
	}
}

func TestAdminCanCreateIssueAndActivatePublicCampaign(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	keyring, err := NewCodeKeyring(CodeKey{
		Version: "test-v1",
		Secret:  bytes.Repeat([]byte{0x2a}, 32),
	}, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() error = %v", err)
	}
	service := NewService(pool, credits.NewService(pool), keyring)
	var commands AdminCommands = service

	campaign, err := commands.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName:    "新用户设计奖励",
		InternalNote:   "Issue #289 tracer campaign",
		CreditAmount:   80,
		MaxRedemptions: 3,
		ActorUserID:    1,
		Reason:         "创建首个公共奖励活动",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	if campaign.Status != CampaignStatusDraft || campaign.Mode != CampaignModePublic {
		t.Fatalf("campaign = %+v, want public draft", campaign)
	}

	issued, err := commands.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID:  campaign.ID,
		Code:        "IDEA2026",
		ActorUserID: 1,
		Reason:      "发行公共兑换码",
	})
	if err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	if issued.Plaintext != "IDEA2026" || issued.Code.Hint != "2026" {
		t.Fatalf("issued = %+v, want one-time plaintext and four-character hint", issued)
	}

	activated, err := commands.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID:  campaign.ID,
		ActorUserID: 1,
		Reason:      "开始公共奖励活动",
	})
	if err != nil {
		t.Fatalf("ActivateCampaign() error = %v", err)
	}
	if activated.Status != CampaignStatusActive {
		t.Fatalf("activated status = %q, want %q", activated.Status, CampaignStatusActive)
	}
}

func TestPublicCodeCannotBeReissuedAcrossActiveHMACVersions(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	previousKey := CodeKey{Version: "v1", Secret: bytes.Repeat([]byte{0x11}, 32)}
	oldKeyring, err := NewCodeKeyring(previousKey, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring(v1) error = %v", err)
	}
	oldService := NewService(pool, creditService, oldKeyring)
	first := createRewardDraft(t, oldService, "旧版本活动")
	if _, err := oldService.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: first.ID, Code: "IDEA2026", ActorUserID: 1, Reason: "发行旧版本 Code",
	}); err != nil {
		t.Fatalf("IssuePublicCode(v1) error = %v", err)
	}

	currentKey := CodeKey{Version: "v2", Secret: bytes.Repeat([]byte{0x22}, 32)}
	rotatedKeyring, err := NewCodeKeyring(currentKey, &previousKey)
	if err != nil {
		t.Fatalf("NewCodeKeyring(v2, v1) error = %v", err)
	}
	rotatedService := NewService(pool, creditService, rotatedKeyring)
	second := createRewardDraft(t, rotatedService, "新版本活动")
	_, err = rotatedService.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: second.ID, Code: "idea-2026", ActorUserID: 1, Reason: "检查轮换期 Code 重用",
	})
	if !errors.Is(err, ErrPublicCodeExists) {
		t.Fatalf("IssuePublicCode() error = %v, want ErrPublicCodeExists", err)
	}
}

func TestUserRedeemCommitsWithCreditGrantAndReplaysWithoutDoubleGrant(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "IDEA2026", 80, 3)
	var userCommands UserCommands = service

	result, err := userCommands.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	if result.CreditedAmount != 80 || result.AvailableBalance != 80 || result.Replayed {
		t.Fatalf("Redeem() result = %+v, want 80 credited and available", result)
	}
	if result.Redemption.CampaignID != campaign.ID || result.Redemption.UserID != 1 {
		t.Fatalf("redemption = %+v, want campaign %d user 1", result.Redemption, campaign.ID)
	}

	ledger, err := creditService.ListLedger(t.Context(), credits.ListLedgerInput{UserID: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(ledger.Transactions) != 1 {
		t.Fatalf("ledger transaction count = %d, want 1", len(ledger.Transactions))
	}
	transaction := ledger.Transactions[0]
	if transaction.Source != "reward_redemption" || transaction.Reason != "兑换奖励" ||
		transaction.BusinessReference.Type != "reward_redemption" ||
		transaction.BusinessReference.ID != strconv.FormatInt(result.Redemption.ID, 10) {
		t.Fatalf("ledger transaction = %+v, want reward redemption metadata", transaction)
	}

	replayed, err := userCommands.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() replay error = %v", err)
	}
	if !replayed.Replayed || replayed.Redemption.ID != result.Redemption.ID || replayed.AvailableBalance != 80 {
		t.Fatalf("Redeem() replay = %+v, want original redemption", replayed)
	}
	ledger, err = creditService.ListLedger(t.Context(), credits.ListLedgerInput{UserID: 1, Limit: 20})
	if err != nil || len(ledger.Transactions) != 1 {
		t.Fatalf("ledger after replay = %+v, error = %v, want one transaction", ledger, err)
	}

	deductionAmount, err := credits.NewCreditAmount(20)
	if err != nil {
		t.Fatalf("NewCreditAmount() error = %v", err)
	}
	if _, err := creditService.Deduct(t.Context(), credits.DeductionCommand{
		UserID: 1,
		Amount: deductionAmount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "reward-replay-balance-deduction",
			Actor:          credits.Actor{Type: credits.ActorAdmin, ID: "1"},
			Source:         "reward_test",
			Reason:         "验证 replay 返回当前余额",
			BusinessReference: credits.BusinessReference{
				Type: "reward_test", ID: "reward-replay-balance-deduction",
			},
		},
	}); err != nil {
		t.Fatalf("Deduct() after redemption error = %v", err)
	}
	currentReplay, err := userCommands.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() after balance change error = %v", err)
	}
	if !currentReplay.Replayed || currentReplay.Redemption.ID != result.Redemption.ID || currentReplay.AvailableBalance != 60 {
		t.Fatalf("Redeem() after balance change = %+v, want original Redemption and current balance 60", currentReplay)
	}
	ledger, err = creditService.ListLedger(t.Context(), credits.ListLedgerInput{UserID: 1, Limit: 20})
	if err != nil || len(ledger.Transactions) != 2 {
		t.Fatalf("ledger after delayed replay = %+v, error = %v, want Grant plus Deduction", ledger, err)
	}
}

func TestUserRedeemRollsBackCampaignAndCodeWhenCreditGrantFails(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 3)
	setRewardWalletFrozen(t, pool, creditService, 3, true)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "FROZEN26", 60, 1)
	var userCommands UserCommands = service

	_, err := userCommands.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: code})
	if !errors.Is(err, credits.ErrWalletFrozen) {
		t.Fatalf("Redeem() error = %v, want frozen Wallet", err)
	}
	afterFailure, err := service.GetCampaign(t.Context(), campaign.ID)
	if err != nil {
		t.Fatalf("GetCampaign() error = %v", err)
	}
	if afterFailure.RedemptionCount != 0 {
		t.Fatalf("redemption count after rollback = %d, want 0", afterFailure.RedemptionCount)
	}

	setRewardWalletFrozen(t, pool, creditService, 3, false)
	result, err := userCommands.Redeem(t.Context(), RedeemCommand{UserID: 3, Code: code})
	if err != nil {
		t.Fatalf("Redeem() after unfreeze error = %v", err)
	}
	if result.AvailableBalance != 60 {
		t.Fatalf("available balance = %d, want 60", result.AvailableBalance)
	}
}

func TestUserRedeemRollsBackCreditGrantWhenRedemptionInsertFails(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 1)
	service := newTestRewardService(t, pool, creditService)
	campaign, code := createActivePublicCampaign(t, service, "ROLLBACK26", 70, 1)

	if _, err := pool.Exec(t.Context(), `
CREATE FUNCTION fail_reward_redemption_insert()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION 'injected reward redemption failure';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog`); err != nil {
		t.Fatalf("create failure function: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
CREATE TRIGGER fail_reward_redemption_insert
BEFORE INSERT ON reward_redemptions
FOR EACH ROW EXECUTE FUNCTION fail_reward_redemption_insert()`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	if _, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code}); err == nil {
		t.Fatal("Redeem() succeeded with injected Redemption failure")
	}
	assertRewardRedemptionRolledBack(t, pool, service, creditService, campaign.ID, 1)

	if _, err := pool.Exec(t.Context(), `DROP TRIGGER fail_reward_redemption_insert ON reward_redemptions`); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	result, err := service.Redeem(t.Context(), RedeemCommand{UserID: 1, Code: code})
	if err != nil {
		t.Fatalf("Redeem() after rollback error = %v", err)
	}
	if result.AvailableBalance != 70 || result.Redemption.CampaignID != campaign.ID {
		t.Fatalf("Redeem() after rollback = %+v, want reusable Code and 70 Ideas", result)
	}
}

func assertRewardRedemptionRolledBack(
	t *testing.T,
	pool *pgxpool.Pool,
	service *Service,
	creditService *credits.Service,
	campaignID int64,
	userID int64,
) {
	t.Helper()
	var redemptionCount int64
	if err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM reward_redemptions WHERE campaign_id = $1`, campaignID,
	).Scan(&redemptionCount); err != nil {
		t.Fatalf("count Redemptions: %v", err)
	}
	campaign, err := service.GetCampaign(t.Context(), campaignID)
	if err != nil {
		t.Fatalf("GetCampaign() after rollback error = %v", err)
	}
	wallet, err := creditService.GetWallet(t.Context(), userID)
	if err != nil {
		t.Fatalf("GetWallet() after rollback error = %v", err)
	}
	ledger, err := creditService.ListLedger(t.Context(), credits.ListLedgerInput{UserID: userID, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() after rollback error = %v", err)
	}
	if redemptionCount != 0 || campaign.RedemptionCount != 0 || wallet.Available != 0 || len(ledger.Transactions) != 0 {
		t.Fatalf(
			"rollback state: redemptions=%d campaign_count=%d available=%d ledger=%d",
			redemptionCount, campaign.RedemptionCount, wallet.Available, len(ledger.Transactions),
		)
	}
}

func newTestRewardService(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service) *Service {
	return newTestRewardServiceAt(t, pool, creditService, time.Now())
}

func newTestRewardKeyring(t *testing.T) CodeKeyring {
	t.Helper()
	keyring, err := NewCodeKeyring(CodeKey{
		Version: "test-v1",
		Secret:  bytes.Repeat([]byte{0x2a}, 32),
	}, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() error = %v", err)
	}
	return keyring
}

func rewardEntropy(values ...byte) *bytes.Reader {
	payload := make([]byte, 0, len(values)*generatedCodeEntropyBytes)
	for _, value := range values {
		payload = append(payload, bytes.Repeat([]byte{value}, generatedCodeEntropyBytes)...)
	}
	return bytes.NewReader(payload)
}

func newTestRewardServiceAt(
	t *testing.T,
	pool *pgxpool.Pool,
	creditService *credits.Service,
	now time.Time,
) *Service {
	t.Helper()
	keyring := newTestRewardKeyring(t)
	return NewServiceWithOptions(pool, creditService, keyring, ServiceOptions{Now: func() time.Time { return now }})
}

func createActivePublicCampaign(
	t *testing.T,
	service *Service,
	code string,
	creditAmount int64,
	maxRedemptions int64,
) (Campaign, string) {
	t.Helper()
	campaign := createRewardDraftWithAmount(t, service, "兑换测试奖励", creditAmount, maxRedemptions)
	issued, err := service.IssuePublicCode(t.Context(), IssuePublicCodeCommand{
		CampaignID: campaign.ID, Code: code, ActorUserID: 1, Reason: "发行测试公共兑换码",
	})
	if err != nil {
		t.Fatalf("IssuePublicCode() error = %v", err)
	}
	if _, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "激活测试公共奖励活动",
	}); err != nil {
		t.Fatalf("ActivateCampaign() error = %v", err)
	}
	return campaign, issued.Plaintext
}

func createRewardDraft(t *testing.T, service *Service, displayName string) Campaign {
	t.Helper()
	return createRewardDraftWithAmount(t, service, displayName, 80, 3)
}

func createSingleUseRewardDraft(
	t *testing.T,
	service *Service,
	displayName string,
	maxRedemptions int64,
) Campaign {
	t.Helper()
	campaign, err := service.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: displayName, CreditAmount: 80, Mode: CampaignModeSingleUse,
		MaxRedemptions: maxRedemptions, ActorUserID: 1, Reason: "创建测试单次码奖励活动",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() single-use error = %v", err)
	}
	return campaign
}

func createRewardDraftWithAmount(
	t *testing.T,
	service *Service,
	displayName string,
	creditAmount int64,
	maxRedemptions int64,
) Campaign {
	t.Helper()
	campaign, err := service.CreateCampaign(t.Context(), CreateCampaignCommand{
		DisplayName: displayName, CreditAmount: creditAmount, MaxRedemptions: maxRedemptions,
		ActorUserID: 1, Reason: "创建测试公共奖励活动",
	})
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	return campaign
}

func provisionRewardWallet(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service, userID int64) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin wallet transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatalf("ProvisionWallet() error = %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit wallet: %v", err)
	}
}

func revokeRewardCodeForTest(t *testing.T, pool *pgxpool.Pool, campaignID int64) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
UPDATE reward_codes
SET revoked_at = clock_timestamp(), revoked_by = 1
WHERE campaign_id = $1`, campaignID); err != nil {
		t.Fatalf("revoke reward Code fixture: %v", err)
	}
}

func setRewardWalletFrozen(
	t *testing.T,
	pool *pgxpool.Pool,
	creditService *credits.Service,
	userID int64,
	frozen bool,
) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin wallet status transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if err := creditService.LifecycleInTx(tx).SetWalletFrozen(t.Context(), userID, frozen); err != nil {
		t.Fatalf("SetWalletFrozen() error = %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit wallet status: %v", err)
	}
}
