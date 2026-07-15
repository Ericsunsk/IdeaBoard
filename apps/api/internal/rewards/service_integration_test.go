package rewards

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
	t.Helper()
	keyring, err := NewCodeKeyring(CodeKey{
		Version: "test-v1",
		Secret:  bytes.Repeat([]byte{0x2a}, 32),
	}, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() error = %v", err)
	}
	return NewService(pool, creditService, keyring)
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
