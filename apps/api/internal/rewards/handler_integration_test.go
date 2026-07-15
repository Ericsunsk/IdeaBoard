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
	return response.Data
}
