package rewards

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestAdminCampaignLifecycleHTTPContract(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	now, err := time.Parse(time.RFC3339, "2030-07-16T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	service := newTestRewardServiceAt(t, pool, credits.NewService(pool), now)
	handler := NewHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/admin/rewards/campaigns", handler.CreateCampaign)
	router.PUT("/admin/rewards/campaigns/:campaign_id", handler.UpdateCampaign)
	router.GET("/admin/rewards/campaigns/:campaign_id", handler.GetCampaign)
	router.DELETE("/admin/rewards/campaigns/:campaign_id", handler.DeleteCampaign)
	router.POST("/admin/rewards/campaigns/:campaign_id/public-code", handler.IssuePublicCode)
	router.POST("/admin/rewards/campaigns/:campaign_id/activate", handler.ActivateCampaign)
	router.POST("/admin/rewards/campaigns/:campaign_id/pause", handler.PauseCampaign)
	router.POST("/admin/rewards/campaigns/:campaign_id/resume", handler.ResumeCampaign)
	router.POST("/admin/rewards/campaigns/:campaign_id/cancel", handler.CancelCampaign)

	startsAt, err := time.Parse(time.RFC3339, "2030-07-16T09:00:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := startsAt.Add(2 * time.Hour)
	campaign := performRewardRequest[Campaign](t, router, http.MethodPost, "/admin/rewards/campaigns", map[string]any{
		"display_name": "排期奖励", "internal_note": "待审核", "credit_amount": 80,
		"max_redemptions": 3, "starts_at": startsAt.Format(time.RFC3339),
		"expires_at": expiresAt.Format(time.RFC3339), "reason": "创建排期奖励",
	}, http.StatusOK)
	if campaign.Status != CampaignStatusDraft || campaign.LifecycleStatus != CampaignLifecycleDraft ||
		campaign.StartsAt == nil || !campaign.StartsAt.Equal(startsAt.UTC()) {
		t.Fatalf("created Campaign = %+v, want UTC-backed scheduled Draft", campaign)
	}

	campaignPath := "/admin/rewards/campaigns/" + strconv.FormatInt(campaign.ID, 10)
	updated := performRewardRequest[Campaign](t, router, http.MethodPut, campaignPath, map[string]any{
		"display_name": "排期奖励 v2", "internal_note": "审核通过", "credit_amount": 100,
		"max_redemptions": 5, "starts_at": startsAt.Format(time.RFC3339),
		"expires_at": expiresAt.Format(time.RFC3339), "reason": "发行前调整规则",
	}, http.StatusOK)
	if updated.DisplayName != "排期奖励 v2" || updated.InternalNote != "审核通过" ||
		updated.CreditAmount != 100 || updated.MaxRedemptions != 5 {
		t.Fatalf("updated Campaign = %+v", updated)
	}

	performRewardRequest[IssuedPublicCode](t, router, http.MethodPost, campaignPath+"/public-code", map[string]any{
		"code": "HTTP2030", "reason": "发行公共 Code",
	}, http.StatusOK)
	activated := performRewardRequest[Campaign](t, router, http.MethodPost, campaignPath+"/activate", map[string]any{
		"reason": "激活奖励",
	}, http.StatusOK)
	if activated.LifecycleStatus != CampaignLifecycleActive || activated.Status != CampaignStatusScheduled {
		t.Fatalf("activated Campaign = %+v", activated)
	}
	paused := performRewardRequest[Campaign](t, router, http.MethodPost, campaignPath+"/pause", map[string]any{
		"reason": "临时暂停",
	}, http.StatusOK)
	if paused.LifecycleStatus != CampaignLifecyclePaused || paused.Status != CampaignStatusPaused {
		t.Fatalf("paused Campaign = %+v", paused)
	}
	resumed := performRewardRequest[Campaign](t, router, http.MethodPost, campaignPath+"/resume", map[string]any{
		"reason": "恢复活动",
	}, http.StatusOK)
	if resumed.LifecycleStatus != CampaignLifecycleActive || resumed.Status != CampaignStatusScheduled {
		t.Fatalf("resumed Campaign = %+v", resumed)
	}
	performRewardRequest[Campaign](t, router, http.MethodPost, campaignPath+"/cancel", map[string]any{
		"confirmed": false, "reason": "取消活动",
	}, http.StatusBadRequest)
	cancelled := performRewardRequest[Campaign](t, router, http.MethodPost, campaignPath+"/cancel", map[string]any{
		"confirmed": true, "reason": "活动配置错误，终止发放",
	}, http.StatusOK)
	if cancelled.LifecycleStatus != CampaignLifecycleCancelled || cancelled.Status != CampaignStatusCancelled {
		t.Fatalf("cancelled Campaign = %+v", cancelled)
	}

	emptyDraft := performRewardRequest[Campaign](t, router, http.MethodPost, "/admin/rewards/campaigns", map[string]any{
		"display_name": "废弃 Draft", "credit_amount": 1, "max_redemptions": 1, "reason": "创建待删除 Draft",
	}, http.StatusOK)
	emptyDraftPath := "/admin/rewards/campaigns/" + strconv.FormatInt(emptyDraft.ID, 10)
	performRewardRequest[struct{}](t, router, http.MethodDelete, emptyDraftPath, map[string]any{
		"reason": "删除未发行 Draft",
	}, http.StatusOK)
	performRewardRequest[Campaign](t, router, http.MethodGet, emptyDraftPath, nil, http.StatusNotFound)
}

func TestAdminSingleUseCodeBatchHTTPContract(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewServiceWithOptions(
		pool,
		credits.NewService(pool),
		newTestRewardKeyring(t),
		ServiceOptions{Random: bytes.NewReader(append(bytes.Repeat([]byte{0x00}, 10), bytes.Repeat([]byte{0xff}, 10)...))},
	)
	handler := NewHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/admin/rewards/campaigns", handler.CreateCampaign)
	router.POST("/admin/rewards/campaigns/:campaign_id/batches", handler.GenerateCodeBatch)
	router.GET("/admin/rewards/campaigns/:campaign_id/batches", handler.ListCodeBatches)
	router.POST("/admin/rewards/campaigns/:campaign_id/batches/:batch_id/revoke", handler.RevokeCodeBatch)

	campaign := performRewardRequest[Campaign](t, router, http.MethodPost, "/admin/rewards/campaigns", map[string]any{
		"display_name": "线下单次码奖励", "credit_amount": 30, "mode": "single_use",
		"max_redemptions": 0, "reason": "创建单次码活动",
	}, http.StatusOK)
	if campaign.Mode != CampaignModeSingleUse || campaign.MaxRedemptions != 0 {
		t.Fatalf("created Campaign = %+v, want unlimited single_use", campaign)
	}

	campaignPath := "/admin/rewards/campaigns/" + strconv.FormatInt(campaign.ID, 10)
	generated, recorder := performRewardRequestWithRecorder[GeneratedCodeBatch](
		t,
		router,
		http.MethodPost,
		campaignPath+"/batches",
		map[string]any{
			"count": 2, "idempotency_key": "http-batch-001", "reason": "生成首批单次码",
		},
		http.StatusOK,
	)
	if recorder.Header().Get("Cache-Control") != "no-store" || len(generated.Plaintexts) != 2 || generated.ExportUnavailable {
		t.Fatalf("generated Batch = %+v Cache-Control=%q, want first no-store export", generated, recorder.Header().Get("Cache-Control"))
	}

	replayed, replayRecorder := performRewardRequestWithRecorder[GeneratedCodeBatch](
		t,
		router,
		http.MethodPost,
		campaignPath+"/batches",
		map[string]any{
			"count": 2, "idempotency_key": "http-batch-001", "reason": "生成首批单次码",
		},
		http.StatusOK,
	)
	if replayRecorder.Header().Get("Cache-Control") != "no-store" || replayed.Batch.ID != generated.Batch.ID ||
		!replayed.Replayed || !replayed.ExportUnavailable || len(replayed.Plaintexts) != 0 {
		t.Fatalf("replayed Batch = %+v Cache-Control=%q, want metadata-only no-store replay", replayed, replayRecorder.Header().Get("Cache-Control"))
	}

	page := performRewardRequest[CodeBatchPage](
		t,
		router,
		http.MethodGet,
		campaignPath+"/batches?limit=20",
		nil,
		http.StatusOK,
	)
	if len(page.Batches) != 1 || page.Batches[0].ValidUnusedCount != 2 {
		t.Fatalf("Batch page = %+v, want two valid unused Codes", page)
	}
	batchPath := campaignPath + "/batches/" + strconv.FormatInt(generated.Batch.ID, 10) + "/revoke"
	performRewardRequest[CodeBatch](t, router, http.MethodPost, batchPath, map[string]any{
		"confirmed": false, "reason": "误操作不应撤销",
	}, http.StatusBadRequest)
	revoked := performRewardRequest[CodeBatch](t, router, http.MethodPost, batchPath, map[string]any{
		"confirmed": true, "reason": "导出文件丢失，撤销未使用码",
	}, http.StatusOK)
	if revoked.ValidUnusedCount != 0 || revoked.RevokedUnusedCount != 2 || revoked.RevokedAt == nil {
		t.Fatalf("revoked Batch = %+v, want both unused Codes revoked", revoked)
	}
}

func TestAuthenticatedUserRedemptionHistoryHTTPContract(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	provisionRewardWallet(t, pool, creditService, 3)
	service := newTestRewardService(t, pool, creditService)
	first := redeemHistoryFixture(t, service, 3, "HTTP2921", "首笔 HTTP 奖励", 12)
	second := redeemHistoryFixture(t, service, 3, "HTTP2922", "最新 HTTP 奖励", 24)
	handler := NewHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 3, principal.RoleUser)
		c.Next()
	})
	router.POST("/user/rewards/redeem", handler.Redeem)
	router.GET("/user/rewards/redemptions", handler.ListRedemptions)

	page1 := performRewardRequest[RedemptionHistoryPage](
		t, router, http.MethodGet, "/user/rewards/redemptions?limit=1", nil, http.StatusOK,
	)
	if len(page1.Redemptions) != 1 || page1.Redemptions[0].ID != second.Redemption.ID ||
		page1.Redemptions[0].MaskedCodeHint != "•••• 2922" || page1.NextBeforeID == 0 {
		t.Fatalf("history page 1 = %+v", page1)
	}
	page2 := performRewardRequest[RedemptionHistoryPage](
		t,
		router,
		http.MethodGet,
		"/user/rewards/redemptions?limit=1&before_id="+strconv.FormatInt(page1.NextBeforeID, 10),
		nil,
		http.StatusOK,
	)
	if len(page2.Redemptions) != 1 || page2.Redemptions[0].ID != first.Redemption.ID {
		t.Fatalf("history page 2 = %+v", page2)
	}
	performRewardRequest[RedemptionHistoryPage](
		t, router, http.MethodGet, "/user/rewards/redemptions?before_id=0", nil, http.StatusBadRequest,
	)
	performRewardRequest[RedemptionHistoryPage](
		t, router, http.MethodGet, "/user/rewards/redemptions?limit=101", nil, http.StatusBadRequest,
	)

	_, code := createActivePublicCampaign(t, service, "HTTP2923", 36, 1)
	redeemed := performRewardRequest[RedeemResult](
		t, router, http.MethodPost, "/user/rewards/redeem", map[string]any{"code": code}, http.StatusOK,
	)
	if redeemed.HistoryItem.ID != redeemed.Redemption.ID ||
		redeemed.HistoryItem.CampaignDisplayName != "兑换测试奖励" ||
		redeemed.HistoryItem.CreditedAmount != 36 ||
		redeemed.HistoryItem.MaskedCodeHint != "•••• 2923" {
		t.Fatalf("Redeem HTTP history item = %+v", redeemed.HistoryItem)
	}
	replayed := performRewardRequest[RedeemResult](
		t, router, http.MethodPost, "/user/rewards/redeem", map[string]any{"code": code}, http.StatusOK,
	)
	if !replayed.Replayed || replayed.HistoryItem.ID != redeemed.HistoryItem.ID {
		t.Fatalf("Redeem HTTP replay = %+v", replayed)
	}
}

type rewardHTTPResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

func performRewardRequest[T any](
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body any,
	wantStatus int,
) T {
	data, _ := performRewardRequestWithRecorder[T](t, router, method, path, body, wantStatus)
	return data
}

func performRewardRequestWithRecorder[T any](
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body any,
	wantStatus int,
) (T, *httptest.ResponseRecorder) {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, recorder.Code, wantStatus, recorder.Body.String())
	}
	var response rewardHTTPResponse[T]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, recorder.Body.String())
	}
	if response.Code != 0 && wantStatus == http.StatusOK {
		t.Fatalf("%s %s response code = %d; body=%s", method, path, response.Code, recorder.Body.String())
	}
	return response.Data, recorder
}
