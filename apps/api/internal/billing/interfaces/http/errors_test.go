package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/billing"

	"github.com/gin-gonic/gin"
)

func TestBillingErrorRowsRenderLockedMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		err    error
		status int
		code   string
		msg    string
	}{
		{name: "invalid sku", err: billing.ErrInvalidSKU, status: 400, code: "billing_invalid_input", msg: "参数错误"},
		{name: "invalid quantity", err: billing.ErrInvalidQuantity, status: 400, code: "billing_invalid_input", msg: "参数错误"},
		{name: "invalid catalog entry", err: billing.ErrInvalidCatalogEntry, status: 400, code: "billing_invalid_input", msg: "参数错误"},
		{name: "invalid resolution", err: billing.ErrInvalidOutputResolution, status: 400, code: "billing_invalid_input", msg: "参数错误"},
		{name: "sku not found", err: billing.ErrSKUNotFound, status: 404, code: "billing_sku_not_found", msg: "计费功能不存在"},
		{name: "pricing unavailable", err: billing.ErrPricingUnavailable, status: 422, code: "billing_pricing_unavailable", msg: "计费功能当前不可计价"},
		{name: "item not archived", err: billing.ErrItemNotArchived, status: 409, code: "billing_item_not_archived", msg: "仅已归档功能可以删除"},
		{name: "archived item in use", err: billing.ErrArchivedItemInUse, status: 409, code: "billing_archived_item_in_use", msg: "功能已有历史计费记录，无法删除"},
		{name: "quote total exceeded", err: billing.ErrQuoteTotalExceeded, status: 422, code: "billing_quote_total_exceeded", msg: "报价超过允许上限"},
		{name: "service unavailable", err: billing.ErrServiceUnavailable, status: 503, code: "billing_service_unavailable", msg: "计费服务暂不可用"},
		{name: "unknown", err: errors.New("boom"), status: 500, code: "billing_operation_failed", msg: "计费操作失败"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/err", func(c *gin.Context) { writeError(c, test.err) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			var body struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q", body.Code, test.code)
			}
			if body.Msg != test.msg {
				t.Fatalf("msg = %q, want %q", body.Msg, test.msg)
			}
		})
	}
}

func TestWriteBillingInputErrorRendersMachineCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) { writeBillingInputError(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "billing_invalid_input" || body.Msg != "参数错误" {
		t.Fatalf("body = %+v, want code=billing_invalid_input msg=参数错误", body)
	}
}
