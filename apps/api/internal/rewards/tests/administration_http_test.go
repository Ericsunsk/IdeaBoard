package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/rewards"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type repositoryIdentityProjection struct {
	repo  *identitypostgres.Repository
	calls *identityProjectionCalls
}

type identityProjectionCalls struct {
	single int
	batch  int
}

func (p repositoryIdentityProjection) ResolveIdentity(
	ctx context.Context,
	userID int64,
) (rewards.IdentityProjection, error) {
	if p.calls != nil {
		p.calls.single++
	}
	user, err := p.repo.GetByID(ctx, userID)
	if err != nil {
		return rewards.IdentityProjection{}, err
	}
	return rewards.IdentityProjection{PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

func (p repositoryIdentityProjection) ResolveIdentities(
	ctx context.Context,
	userIDs []int64,
) (map[int64]rewards.IdentityProjection, error) {
	if p.calls != nil {
		p.calls.batch++
	}
	usersByID, err := p.repo.GetByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	identities := make(map[int64]rewards.IdentityProjection, len(usersByID))
	for userID, user := range usersByID {
		identities[userID] = rewards.IdentityProjection{PublicID: user.PublicID, DisplayName: user.DisplayName}
	}
	return identities, nil
}

func TestAdminListsSafeRedemptionSummariesScopedToRewardCode(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	userRepo := identitypostgres.NewRepository(db)
	projectionCalls := &identityProjectionCalls{}
	redeemingUser, err := userRepo.GetByID(t.Context(), 3)
	if err != nil {
		t.Fatalf("load redemption user: %v", err)
	}

	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "Admin read target", Code: "ADMIN-READ-TARGET-2026", CreditAmount: 37,
		IdempotencyKey: "admin-read-target", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode() error = %v", err)
	}
	other, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "Admin read other", Code: "ADMIN-READ-OTHER-2026", CreditAmount: 11,
		IdempotencyKey: "admin-read-other", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode(other) error = %v", err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	redeemed, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "ADMIN-READ-TARGET-2026"})
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}

	handler := rewardhttp.NewAdminReadHandler(service, repositoryIdentityProjection{repo: userRepo, calls: projectionCalls})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions", handler.ListAdminRedemptions)

	page, recorder := performRewardRequestWithRecorder[ports.AdminRedemptionPage](
		t, router, http.MethodGet,
		fmt.Sprintf("/admin/rewards/codes/%d/redemptions", created.RewardCode.ID), nil, http.StatusOK,
	)
	if recorder.Header().Get("Cache-Control") != "no-store" || len(page.Redemptions) != 1 {
		t.Fatalf("page = %+v Cache-Control=%q", page, recorder.Header().Get("Cache-Control"))
	}
	item := page.Redemptions[0]
	if item.ID != redeemed.Redemption.ID || item.RewardCodeID != created.RewardCode.ID ||
		item.UserPublicID != redeemingUser.PublicID || item.DisplayName != redeemingUser.DisplayName ||
		item.CreditedAmount != 37 || item.RedeemedAt.IsZero() {
		t.Fatalf("summary = %+v", item)
	}
	if projectionCalls.batch != 1 || projectionCalls.single != 0 {
		t.Fatalf("Identity projection calls = %+v, want one batch and no single reads", *projectionCalls)
	}
	var payload struct {
		Data struct {
			Redemptions []map[string]json.RawMessage `json:"redemptions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantFields := map[string]bool{
		"id": true, "reward_code_id": true, "user_public_id": true, "display_name": true,
		"credited_amount": true, "redeemed_at": true,
	}
	if len(payload.Data.Redemptions) != 1 || len(payload.Data.Redemptions[0]) != len(wantFields) {
		t.Fatalf("summary response fields = %v", payload.Data.Redemptions)
	}
	for field := range payload.Data.Redemptions[0] {
		if !wantFields[field] {
			t.Fatalf("summary response exposed field %q: %s", field, recorder.Body.String())
		}
	}

	otherPage, _ := performRewardRequestWithRecorder[ports.AdminRedemptionPage](
		t, router, http.MethodGet,
		fmt.Sprintf("/admin/rewards/codes/%d/redemptions", other.RewardCode.ID), nil, http.StatusOK,
	)
	if len(otherPage.Redemptions) != 0 {
		t.Fatalf("other RewardCode page = %+v, want empty", otherPage)
	}
}

func TestAdminGetsRedemptionDetailOnlyWithinRewardCodeScope(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	userRepo := identitypostgres.NewRepository(db)
	projectionCalls := &identityProjectionCalls{}
	redeemingUser, err := userRepo.GetByID(t.Context(), 3)
	if err != nil {
		t.Fatalf("load redemption user: %v", err)
	}
	targetCode, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "Admin detail target", Code: "ADMIN-DETAIL-TARGET-2026", CreditAmount: 23,
		IdempotencyKey: "admin-detail-target", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode(target) error = %v", err)
	}
	otherCode, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "Admin detail other", Code: "ADMIN-DETAIL-OTHER-2026", CreditAmount: 17,
		IdempotencyKey: "admin-detail-other", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode(other) error = %v", err)
	}
	provisionRewardWallet(t, pool, creditService, 7)
	provisionRewardWallet(t, pool, creditService, 3)
	targetRedemption, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "ADMIN-DETAIL-TARGET-2026"})
	if err != nil {
		t.Fatalf("Redeem(target) error = %v", err)
	}
	otherRedemption, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 7, Code: "ADMIN-DETAIL-OTHER-2026"})
	if err != nil {
		t.Fatalf("Redeem(other) error = %v", err)
	}

	handler := rewardhttp.NewAdminReadHandler(service, repositoryIdentityProjection{repo: userRepo, calls: projectionCalls})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions/:redemption_id", handler.GetAdminRedemption)

	path := fmt.Sprintf("/admin/rewards/codes/%d/redemptions/%d", targetCode.RewardCode.ID, targetRedemption.Redemption.ID)
	detail, recorder := performRewardRequestWithRecorder[ports.AdminRedemptionDetail](
		t, router, http.MethodGet, path, nil, http.StatusOK,
	)
	if recorder.Header().Get("Cache-Control") != "no-store" ||
		detail.ID != targetRedemption.Redemption.ID || detail.RewardCodeID != targetCode.RewardCode.ID ||
		detail.UserPublicID != redeemingUser.PublicID || detail.DisplayName != redeemingUser.DisplayName ||
		detail.CreditedAmount != 23 || detail.RedeemedAt.IsZero() ||
		detail.CreditTransactionID != targetRedemption.Redemption.CreditTransactionID ||
		detail.MaskedCodeHint != "•••• 2026" {
		t.Fatalf("detail = %+v Cache-Control=%q", detail, recorder.Header().Get("Cache-Control"))
	}
	if projectionCalls.single != 1 || projectionCalls.batch != 0 {
		t.Fatalf("Identity projection calls = %+v, want one single and no batch reads", *projectionCalls)
	}
	var payload struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	wantFields := map[string]bool{
		"id": true, "reward_code_id": true, "user_public_id": true, "display_name": true, "credited_amount": true,
		"redeemed_at": true, "credit_transaction_id": true, "masked_code_hint": true,
	}
	if len(payload.Data) != len(wantFields) {
		t.Fatalf("detail response fields = %v", payload.Data)
	}
	for field := range payload.Data {
		if !wantFields[field] {
			t.Fatalf("detail response exposed field %q: %s", field, recorder.Body.String())
		}
	}

	for _, mismatchPath := range []string{
		fmt.Sprintf("/admin/rewards/codes/%d/redemptions/%d", targetCode.RewardCode.ID, otherRedemption.Redemption.ID),
		fmt.Sprintf("/admin/rewards/codes/%d/redemptions/%d", otherCode.RewardCode.ID, targetRedemption.Redemption.ID),
		fmt.Sprintf("/admin/rewards/codes/%d/redemptions/%d", targetCode.RewardCode.ID, targetRedemption.Redemption.ID+9999),
	} {
		_, mismatchRecorder := performRewardRequestWithRecorder[struct{}](
			t, router, http.MethodGet, mismatchPath, nil, http.StatusNotFound,
		)
		if mismatchRecorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("mismatch Cache-Control=%q", mismatchRecorder.Header().Get("Cache-Control"))
		}
	}
}

func TestAdminRedemptionListUsesBoundedKeysetPagination(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	userRepo := identitypostgres.NewRepository(db)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "Admin pagination", Code: "ADMIN-PAGINATION-2026", CreditAmount: 9,
		IdempotencyKey: "admin-pagination", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode() error = %v", err)
	}
	provisionRewardWallet(t, pool, creditService, 7)
	provisionRewardWallet(t, pool, creditService, 3)
	older, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 7, Code: "ADMIN-PAGINATION-2026"})
	if err != nil {
		t.Fatalf("Redeem(older) error = %v", err)
	}
	newer, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "ADMIN-PAGINATION-2026"})
	if err != nil {
		t.Fatalf("Redeem(newer) error = %v", err)
	}

	handler := rewardhttp.NewAdminReadHandler(service, repositoryIdentityProjection{repo: userRepo})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions", handler.ListAdminRedemptions)
	basePath := fmt.Sprintf("/admin/rewards/codes/%d/redemptions", created.RewardCode.ID)

	first, _ := performRewardRequestWithRecorder[ports.AdminRedemptionPage](
		t, router, http.MethodGet, basePath+"?limit=1", nil, http.StatusOK,
	)
	if len(first.Redemptions) != 1 || first.Redemptions[0].ID != newer.Redemption.ID ||
		first.NextBeforeID != newer.Redemption.ID {
		t.Fatalf("first page = %+v", first)
	}
	second, _ := performRewardRequestWithRecorder[ports.AdminRedemptionPage](
		t, router, http.MethodGet,
		fmt.Sprintf("%s?limit=1&before_id=%d", basePath, first.NextBeforeID), nil, http.StatusOK,
	)
	if len(second.Redemptions) != 1 || second.Redemptions[0].ID != older.Redemption.ID ||
		second.NextBeforeID != 0 || second.Redemptions[0].ID == first.Redemptions[0].ID {
		t.Fatalf("second page = %+v; first page = %+v", second, first)
	}

	for _, query := range []string{
		"before_id=", "before_id=0", "before_id=-1", "before_id=abc",
		"limit=", "limit=0", "limit=101", "limit=abc",
	} {
		_, recorder := performRewardRequestWithRecorder[struct{}](
			t, router, http.MethodGet, basePath+"?"+query, nil, http.StatusBadRequest,
		)
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("query %q Cache-Control=%q", query, recorder.Header().Get("Cache-Control"))
		}
	}
	_, notFound := performRewardRequestWithRecorder[struct{}](
		t, router, http.MethodGet,
		fmt.Sprintf("/admin/rewards/codes/%d/redemptions", created.RewardCode.ID+9999), nil, http.StatusNotFound,
	)
	if notFound.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing RewardCode Cache-Control=%q", notFound.Header().Get("Cache-Control"))
	}
}
