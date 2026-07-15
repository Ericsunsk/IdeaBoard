package rewards

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

func newTestRewardServiceAt(
	t *testing.T,
	pool *pgxpool.Pool,
	creditService *credits.Service,
	now time.Time,
) *Service {
	t.Helper()
	keyring, err := NewCodeKeyring(CodeKey{
		Version: "test-v1",
		Secret:  bytes.Repeat([]byte{0x2a}, 32),
	}, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() error = %v", err)
	}
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
