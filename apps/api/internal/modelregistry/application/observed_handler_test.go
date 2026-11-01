package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	modelregistrydomain "ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

// Observed-handler fakes (domain seam doubles local to this test so the
// application package keeps no fixture outside handler_test.go).
type observedFakeAttemptStore struct {
	obs modelregistrydomain.AttemptObservation
	err error
}

func (f *observedFakeAttemptStore) ListAttemptObservations(context.Context, time.Time) (modelregistrydomain.AttemptObservation, error) {
	return f.obs, f.err
}

type observedFakeSKUCatalog struct {
	cards []modelregistrydomain.SKUCatalogCard
	err   error
}

func (f *observedFakeSKUCatalog) ListSKUCatalog(context.Context) ([]modelregistrydomain.SKUCatalogCard, error) {
	return f.cards, f.err
}

func serveModelsObserved(t *testing.T, role principal.Role, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings/ai-gateway/models-observed", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, role)
		handler.ListModelsObserved(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/models-observed", nil))
	return rec
}

func TestObservedHandlerGatingAndEnvelope(t *testing.T) {
	// Non-admin gating rides the outer Control Plane guard (controlProxy +
	// guard); the handler performs no role check. Guard-level denial is
	// covered by TestControlAuthMatrix in runtime/interfaces/http.
	t.Run("fails closed without service", func(t *testing.T) {
		rec := serveModelsObserved(t, principal.RoleAdmin, NewHandler(&fakeGatewayStore{}, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
	})

	t.Run("returns envelope with rows", func(t *testing.T) {
		handler := NewHandler(&fakeGatewayStore{}, nil)
		handler.SetObservedService(NewObservedService(
			&observedFakeAttemptStore{obs: modelregistrydomain.AttemptObservation{
				Aggregates: []modelregistrydomain.ModelAttemptAggregate{{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 4, Completed: 3, Failed: 1, CacheHits: 1}},
				Failures:   []modelregistrydomain.ModelFailureCount{{Model: "fal/flux-pro", Provider: "fal", FailureStage: "provider", FailureCategory: "timeout", Count: 1}},
			}},
			&observedFakeSKUCatalog{cards: []modelregistrydomain.SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}}}},
		))
		rec := serveModelsObserved(t, principal.RoleAdmin, handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var payload struct {
			Data []modelregistrydomain.ModelObservedRow `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("body is not the envelope: %v", err)
		}
		if len(payload.Data) != 1 {
			t.Fatalf("data = %+v, want 1 row", payload.Data)
		}
		row := payload.Data[0]
		if row.Model != "fal/flux-pro" || !row.HasObservations || row.TopFailureStage != "provider" {
			t.Fatalf("row = %+v", row)
		}
		if row.SKUMargins[0].ActualCostCredits != nil || row.SKUMargins[0].MarginStatus != modelregistrydomain.ObservedMarginUnknownNoCost {
			t.Fatalf("margin = %+v, want unavailable cost marker", row.SKUMargins[0])
		}
	})
}
