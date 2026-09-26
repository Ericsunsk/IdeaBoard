package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type ReconciliationHandler struct {
	service      ReconciliationService
	userResolver UserIDResolver
}

func NewReconciliationHandler(service ReconciliationService, resolver UserIDResolver) *ReconciliationHandler {
	return &ReconciliationHandler{service: service, userResolver: resolver}
}

type adminReconciliationRequest struct {
	Reason string `json:"reason"`
}

func (h *ReconciliationHandler) GetCreditHealth(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	health, err := h.service.GetCreditHealth(c.Request.Context())
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, health)
}

func (h *ReconciliationHandler) StartReconciliation(c *gin.Context) {
	adminID, ok := requireAdmin(c, h != nil && h.service != nil)
	if !ok {
		return
	}
	var request adminReconciliationRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	action, err := adminaction.New(adminID, request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	run, err := h.service.StartReconciliation(c.Request.Context(), ports.StartManualReconciliationInput{
		ActorID: action.ActorID(),
		Reason:  action.Reason(),
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichReconciliationRun(c.Request.Context(), h.userResolver, &run); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *ReconciliationHandler) GetReconciliationRun(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	// :run_id shares the control-plane formal exception (no public_id
	// column on credit_reconciliation_runs; rec_ reserved in the kernel).
	// Admin-gated only.
	runID, ok := parsePositiveInt64(c.Param("run_id"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "run_id 无效")
		return
	}
	run, err := h.service.GetReconciliationRun(c.Request.Context(), runID)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichReconciliationRun(c.Request.Context(), h.userResolver, &run); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *ReconciliationHandler) ListReconciliationRuns(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	input, ok := parseReconciliationPage(c)
	if !ok {
		return
	}
	page, err := h.service.ListReconciliationRuns(c.Request.Context(), input)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichReconciliationRuns(c.Request.Context(), h.userResolver, &page); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *ReconciliationHandler) ListGlobalReconciliationFindings(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	input, ok := parseReconciliationPage(c)
	if !ok {
		return
	}
	status, ok := parseReconciliationFindingStatus(c.Query("status"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "status 无效")
		return
	}
	page, err := h.service.ListGlobalReconciliationFindings(c.Request.Context(), status, input)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichReconciliationFindings(c.Request.Context(), h.userResolver, &page); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func parseReconciliationFindingStatus(value string) (ports.ReconciliationFindingStatus, bool) {
	status := ports.ReconciliationFindingStatus(strings.TrimSpace(value))
	return status, status == ports.ReconciliationFindingOpen || status == ports.ReconciliationFindingResolved
}

func enrichReconciliationRun(ctx context.Context, userResolver UserIDResolver, run *ports.ReconciliationRun) error {
	if userResolver == nil || run == nil || run.RequestedBy == nil {
		return nil
	}
	identity, err := userResolver.ResolveID(ctx, *run.RequestedBy)
	if err != nil {
		return fmt.Errorf("resolve reconciliation actor identity: %w", err)
	}
	publicID := identity.PublicID
	run.RequestedByPublicID = &publicID
	run.RequestedByName = identity.DisplayName
	return nil
}

func enrichReconciliationRuns(ctx context.Context, userResolver UserIDResolver, page *ports.ReconciliationRunPage) error {
	if page == nil {
		return nil
	}
	for index := range page.Runs {
		if err := enrichReconciliationRun(ctx, userResolver, &page.Runs[index]); err != nil {
			return err
		}
	}
	return nil
}

func enrichReconciliationFindings(ctx context.Context, userResolver UserIDResolver, page *ports.ReconciliationFindingPage) error {
	if userResolver == nil || page == nil {
		return nil
	}
	for index := range page.Findings {
		finding := &page.Findings[index]
		if finding.WalletUserID == nil {
			continue
		}
		identity, err := userResolver.ResolveID(ctx, *finding.WalletUserID)
		if err != nil {
			return fmt.Errorf("resolve reconciliation wallet identity: %w", err)
		}
		publicID := identity.PublicID
		finding.WalletPublicID = &publicID
		finding.WalletDisplayName = identity.DisplayName
	}
	return nil
}

func parseReconciliationPage(c *gin.Context) (ports.ListReconciliationInput, bool) {
	input := ports.ListReconciliationInput{}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return input, false
		}
		input.Limit = limit
	}
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		before, ok := parsePositiveInt64(raw)
		if !ok {
			response.Error(c, http.StatusBadRequest, "before_id 无效")
			return input, false
		}
		input.BeforeID = before
	}
	return input, true
}
