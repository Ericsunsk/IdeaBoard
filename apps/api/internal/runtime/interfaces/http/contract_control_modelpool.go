package httpapi

// Control Model Pool family (issue #849): per-Service Model Pool reads
// and atomic replaces behind control:billing.
//
// The pool is the relational Service Model Pool contract from #853/#848
// (internal/generation/catalog): one Service owns an unordered set of
// {model, enabled, supported_resolutions} entries plus a monotonic
// pool version pinned into routing snapshots. This surface extends the
// billing-catalog boundary (rows are addressed by billing SKU
// `ai-image:<service>`); it never touches the retired single-value
// `feature -> model` settings mapping.
//
// GET returns the complete pool with its current version (0 with empty
// entries when never effectively written). PUT replaces the complete
// pool for one Service transactionally with optimistic concurrency:
// expected_version must match the stored version or the write fails
// with 409 conflict and commits nothing (no silent overwrite). An
// enabled entry must list at least one resolution; an empty entry list
// clears the pool (the Service reads as unavailable until repopulated).

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// controlModelPoolSKUImagePrefix is the only SKU namespace with a Model
// Pool. Pool rows are Services; the billing SKU namespace for Services
// is ai-image:<service> (see skuFeature.ts on the frontend, kept in
// sync by contract test).
const controlModelPoolSKUImagePrefix = "ai-image:"

// ControlModelPoolStore is the narrow pool seam for the Control Plane.
// *catalog.Repository is the production implementation; tests inject a
// fake. Reads take no lock; ReplacePool serializes per Service with a
// version compare-and-swap inside one transaction.
type ControlModelPoolStore interface {
	ListPool(ctx context.Context, service string) (routing.Pool, error)
	ReplacePool(ctx context.Context, service string, expectedVersion uint64, inputs []catalog.PoolEntryInput) (catalog.ReplacePoolResult, error)
}

type controlModelPoolEntry struct {
	ModelID     string   `json:"model_id"`
	Enabled     bool     `json:"enabled"`
	Resolutions []string `json:"resolutions"`
}

type controlModelPoolPayload struct {
	Service string                  `json:"service"`
	SKU     string                  `json:"sku"`
	Version uint64                  `json:"version"`
	Entries []controlModelPoolEntry `json:"entries"`
}

type controlReplaceModelPoolResult struct {
	controlModelPoolPayload
	Changed bool `json:"changed"`
}

type controlReplaceModelPoolRequest struct {
	ExpectedVersion *uint64 `json:"expected_version"`
	Entries         *[]struct {
		ModelID     string   `json:"model_id"`
		Enabled     bool     `json:"enabled"`
		Resolutions []string `json:"resolutions"`
	} `json:"entries"`
	// Reason rides the operator-note convention (header wins; the guard
	// extracts and audits it centrally). Accepted here only so strict
	// decoding does not 400 the documented shape.
	Reason *string `json:"reason,omitempty"`
}

// serviceOfControlPoolSKU maps a billing SKU to its Service slug. Only
// ai-image SKUs own a pool; anything else is a caller bug (400), never
// an availability outcome.
func serviceOfControlPoolSKU(sku string) (string, bool) {
	trimmed := strings.TrimSpace(sku)
	if !strings.HasPrefix(trimmed, controlModelPoolSKUImagePrefix) {
		return "", false
	}
	service := strings.TrimSpace(strings.TrimPrefix(trimmed, controlModelPoolSKUImagePrefix))
	if service == "" {
		return "", false
	}
	return service, true
}

func renderControlModelPool(pool routing.Pool, sku string) controlModelPoolPayload {
	entries := make([]controlModelPoolEntry, 0, len(pool.Entries))
	for _, entry := range pool.Entries {
		resolutions := make([]string, 0, len(entry.SupportedResolutions))
		for _, res := range entry.SupportedResolutions {
			resolutions = append(resolutions, string(res))
		}
		entries = append(entries, controlModelPoolEntry{
			ModelID:     entry.Model,
			Enabled:     entry.Enabled,
			Resolutions: resolutions,
		})
	}
	return controlModelPoolPayload{
		Service: pool.Service,
		SKU:     sku,
		Version: pool.Version,
		Entries: entries,
	}
}

func writeControlModelPoolError(c *gin.Context, err error) {
	var conflict *catalog.PoolVersionConflict
	if errors.As(err, &conflict) {
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, msgConflict7)
		return
	}
	if errors.Is(err, catalog.ErrInvalidPoolInput) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest34)
		return
	}
	contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable55)
}

func (d *ContractControlDeps) getControlModelPool(c *gin.Context, _ controlContext) {
	if d == nil || d.ModelPools == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable55)
		return
	}
	sku := strings.TrimSpace(c.Param("sku"))
	service, ok := serviceOfControlPoolSKU(sku)
	if !ok {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest5)
		return
	}
	pool, err := d.ModelPools.ListPool(c.Request.Context(), service)
	if err != nil {
		writeControlModelPoolError(c, err)
		return
	}
	contract.RenderSuccess(c, renderControlModelPool(pool, sku))
}

func (d *ContractControlDeps) replaceControlModelPool(c *gin.Context, _ controlContext) {
	if d == nil || d.ModelPools == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable55)
		return
	}
	sku := strings.TrimSpace(c.Param("sku"))
	service, ok := serviceOfControlPoolSKU(sku)
	if !ok {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest5)
		return
	}
	var req controlReplaceModelPoolRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	if req.ExpectedVersion == nil || req.Entries == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	inputs := make([]catalog.PoolEntryInput, 0, len(*req.Entries))
	for _, entry := range *req.Entries {
		inputs = append(inputs, catalog.PoolEntryInput{
			ModelID:     entry.ModelID,
			Enabled:     entry.Enabled,
			Resolutions: entry.Resolutions,
		})
	}
	result, err := d.ModelPools.ReplacePool(c.Request.Context(), service, *req.ExpectedVersion, inputs)
	if err != nil {
		writeControlModelPoolError(c, err)
		return
	}
	payload := renderControlModelPool(result.Pool, sku)
	contract.RenderSuccess(c, controlReplaceModelPoolResult{
		controlModelPoolPayload: payload,
		Changed:                 result.Changed,
	})
}
