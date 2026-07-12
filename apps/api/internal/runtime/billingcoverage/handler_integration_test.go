package billingcoverage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubCoverageService struct {
	coverage Coverage
	err      error
}

func (service stubCoverageService) List(context.Context) (Coverage, error) {
	return service.coverage, service.err
}

func TestHandlerReturnsAdminBillingCoverage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/billing/coverage", NewHandler(stubCoverageService{coverage: Coverage{Items: []CoverageItem{{
		Feature: "sketch-to-image", Model: "gpt-image-1", SKU: "ai-image:sketch-to-image.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18",
		SKUConfigured: true, SKUEnabled: true, PriceConfigured: false, PriceEnabled: false,
	}}}}).List)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/billing/coverage", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Code int      `json:"code"`
		Data Coverage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 0 || len(response.Data.Items) != 1 || response.Data.Items[0].Feature != "sketch-to-image" {
		t.Fatalf("response = %+v", response)
	}
}

func TestHandlerFailsClosedWhenCoverageReadFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/billing/coverage", NewHandler(stubCoverageService{err: errors.New("database offline")}).List)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/billing/coverage", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
}
