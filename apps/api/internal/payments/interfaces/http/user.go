package http

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/pagination"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type createPaymentCheckoutRequest struct {
	IdempotencyKey      string `json:"idempotency_key"`
	IdeaQuantity        int64  `json:"idea_quantity"`
	TermsAccepted       bool   `json:"terms_accepted"`
	TermsVersion        string `json:"terms_version"`
	RefundPolicyVersion string `json:"refund_policy_version"`
}

func (h *UserCheckoutHandler) GetPurchaseOptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	options, err := h.service.GetPurchaseOptions(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取充值选项失败")
		return
	}
	response.SuccessDTO(c, options)
}

func (h *UserCheckoutHandler) CreateCheckoutSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	var request createPaymentCheckoutRequest
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxPaymentCheckoutRequestBytes, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "充值参数错误")
		return
	}
	returnBaseURL, err := h.checkoutReturnBaseURL()
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	result, err := h.service.CreateCheckout(c.Request.Context(), paymentsdomain.CreateCheckoutCommand{
		UserID: identityPrincipal.UserID(), ClientIdempotencyKey: request.IdempotencyKey,
		IdeaQuantity: request.IdeaQuantity, TermsAccepted: request.TermsAccepted,
		TermsVersion: request.TermsVersion, RefundPolicyVersion: request.RefundPolicyVersion,
		ReturnBaseURL: returnBaseURL,
	})
	if err != nil {
		writeCheckoutError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *UserCheckoutHandler) GetPaymentOrderReturnStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	publicToken, ok := paymentOrderPublicToken(c)
	if !ok {
		return
	}
	result, err := h.service.GetPaymentOrderReturnStatus(c.Request.Context(), identityPrincipal.UserID(), publicToken)
	if errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取充值订单失败")
		return
	}
	response.SuccessDTO(c, result)
}

func (h *UserCheckoutHandler) ListPendingOrders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	orders, err := h.service.ListPendingOrders(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "读取待支付订单失败")
		return
	}
	response.SuccessDTO(c, orders)
}

func (h *UserCheckoutHandler) ContinuePendingOrder(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	if !requireEmptyPaymentActionBody(c) {
		return
	}
	publicToken, ok := paymentOrderPublicToken(c)
	if !ok {
		return
	}
	order, err := h.service.ContinuePendingOrder(c.Request.Context(), identityPrincipal.UserID(), publicToken)
	if errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return
	}
	if errors.Is(err, paymentsdomain.ErrWalletNotRechargeable) {
		response.Error(c, http.StatusLocked, "当前账户暂不可继续充值")
		return
	}
	if errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable) || errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		response.Error(c, http.StatusServiceUnavailable, "支付服务暂不可用")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "继续充值订单失败")
		return
	}
	response.SuccessDTO(c, order)
}

func (h *UserCheckoutHandler) AbandonPendingOrder(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	if !requireEmptyPaymentActionBody(c) {
		return
	}
	publicToken, ok := paymentOrderPublicToken(c)
	if !ok {
		return
	}
	order, err := h.service.AbandonPendingOrder(c.Request.Context(), identityPrincipal.UserID(), publicToken)
	if errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return
	}
	if errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable) || errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		response.Error(c, http.StatusServiceUnavailable, "支付服务暂不可用")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "放弃充值订单失败")
		return
	}
	response.SuccessDTO(c, order)
}

// paymentOrderPublicToken intentionally maps malformed and inaccessible tokens
// to the same response, so the public token is never an ownership oracle.
func paymentOrderPublicToken(c *gin.Context) (string, bool) {
	publicToken := strings.TrimSpace(c.Param("public_token"))
	parsed, err := uuid.Parse(publicToken)
	if err != nil || parsed.String() != strings.ToLower(publicToken) {
		response.Error(c, http.StatusNotFound, "充值订单不存在")
		return "", false
	}
	return publicToken, true
}

// Action routes intentionally have no request schema. Reject a body rather
// than silently accepting undeclared fields and creating a second command API.
func requireEmptyPaymentActionBody(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPaymentCheckoutRequestBytes+1))
	if err != nil || int64(len(body)) > maxPaymentCheckoutRequestBytes || strings.TrimSpace(string(body)) != "" {
		response.Error(c, http.StatusBadRequest, "充值操作参数错误")
		return false
	}
	return true
}

func (h *UserCheckoutHandler) ListRecharges(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "充值服务暂不可用")
		return
	}
	limit, err := pagination.ParseLimit(c.Query("limit"))
	if err != nil {
		writeRechargeHistoryError(c, paymentcontracts.ErrRechargeHistoryLimit)
		return
	}
	page, err := h.service.ListRecharges(c.Request.Context(), identityPrincipal.UserID(), c.Query("cursor"), limit)
	if err != nil {
		writeRechargeHistoryError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}
func (h *UserCheckoutHandler) checkoutReturnBaseURL() (string, error) {
	if h == nil || strings.TrimSpace(h.publicOrigins.Web) == "" {
		return "", paymentsdomain.ErrPurchasesUnavailable
	}
	return h.publicOrigins.WebURL("/user/boards"), nil
}
