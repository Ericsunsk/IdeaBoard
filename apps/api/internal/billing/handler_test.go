package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type handlerServiceStub struct {
	quoteInput struct {
		sku        string
		resolution OutputResolution
		quantity   int64
	}
}

type billableCatalogReaderStub struct {
	items               []BillableItemCatalogCard
	err                 error
	publishSKU          string
	publishPrices       map[OutputResolution]int64
	publishActorID      int64
	availabilitySKU     string
	availabilityEnabled bool
	availabilityActorID int64
	deleted             ArchivedItemDeleteResult
	deleteActorID       int64
}

func (s *billableCatalogReaderStub) DeleteArchived(_ context.Context, sku string, actorID int64) (ArchivedItemDeleteResult, error) {
	s.deleted = ArchivedItemDeleteResult{SKU: SKU(sku)}
	s.deleteActorID = actorID
	return s.deleted, s.err
}

func (s *billableCatalogReaderStub) List(context.Context) ([]BillableItemCatalogCard, error) {
	return s.items, s.err
}

func (s *billableCatalogReaderStub) PublishPrices(_ context.Context, sku string, prices map[OutputResolution]int64, actorID int64) (map[OutputResolution]PriceInfo, error) {
	s.publishSKU = sku
	s.publishPrices = prices
	s.publishActorID = actorID
	return map[OutputResolution]PriceInfo{OutputResolution1K: {PriceVersionID: 1, Version: 1, OutputResolution: OutputResolution1K, UnitCredits: prices[OutputResolution1K]}}, s.err
}

func (s *billableCatalogReaderStub) SetAvailability(_ context.Context, sku string, enabled bool, actorID int64) (AvailabilityInfo, error) {
	s.availabilitySKU = sku
	s.availabilityEnabled = enabled
	s.availabilityActorID = actorID
	status := BillableItemStatusDisabled
	if enabled {
		status = BillableItemStatusActive
	}
	return AvailabilityInfo{SKU: SKU(sku), Status: status}, s.err
}

func (s *handlerServiceStub) QuoteAtResolution(_ context.Context, sku string, resolution OutputResolution, quantity int64) (Quote, error) {
	s.quoteInput.sku = sku
	s.quoteInput.resolution = resolution
	s.quoteInput.quantity = quantity
	return Quote{
		PriceSnapshot: PriceSnapshot{SKU: "ideaboard:image.generate", DisplayName: "AI 生图", PriceVersionID: 3, PriceVersion: 2, OutputResolution: resolution, UnitCredits: 25, QuotedAt: time.Unix(10, 0).UTC()},
		Quantity:      quantity, TotalCredits: 100,
	}, nil
}

func TestQuoteHandlerRejectsCallerSuppliedPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.POST("/quote", handler.Quote)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{
  "sku":"ideaboard:image.generate","quantity":4,"unit_credits":1
}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", response.Code, response.Body.String())
	}
	if service.quoteInput.sku != "" {
		t.Fatalf("Quote service was called with caller supplied price: %+v", service.quoteInput)
	}
}

func TestQuoteHandlerReturnsServerSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.POST("/quote", handler.Quote)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{"sku":"IDEABOARD:IMAGE.GENERATE","output_resolution":"1k","quantity":4}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"display_name":"AI 生图"`) || !strings.Contains(response.Body.String(), `"total_credits":100`) || !strings.Contains(response.Body.String(), `"price_version_id":3`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.quoteInput.sku != "IDEABOARD:IMAGE.GENERATE" || service.quoteInput.resolution != OutputResolution1K || service.quoteInput.quantity != 4 {
		t.Fatalf("Quote input = %+v", service.quoteInput)
	}
}

func TestQuoteHandlerRequiresExplicitOutputResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.POST("/quote", NewHandler(service, nil).Quote)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{"sku":"ideaboard:image.generate","quantity":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || service.quoteInput.sku != "" {
		t.Fatalf("status=%d quote=%+v body=%s", response.Code, service.quoteInput, response.Body.String())
	}
}

func TestAdminBillableCatalogReturnsFeatureCardContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	price := PriceInfo{PriceVersionID: 1, Version: 1, OutputResolution: OutputResolution1K, UnitCredits: 25}
	catalog := &billableCatalogReaderStub{items: []BillableItemCatalogCard{
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "ai-image:local-inpaint", Namespace: "ai-image", ProductKind: "workflow-feature",
				DisplayName: "局部重绘", Description: "仅重绘图片选定区域。", SortOrder: 10,
			},
			Status: BillableItemStatusUnpriced,
		},
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "ai-image:remove-bg", Namespace: "ai-image", ProductKind: "workflow-feature",
				DisplayName: "抠图", SortOrder: 20,
			},
			Status: BillableItemStatusActive, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: price},
		},
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "ai-image:hd-upscale", Namespace: "ai-image", ProductKind: "workflow-feature",
				DisplayName: "HD", SortOrder: 30,
			},
			Status: BillableItemStatusDisabled, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: price},
		},
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "legacy:removed", Namespace: "legacy", ProductKind: "legacy",
				DisplayName: "已移除功能", SortOrder: 0,
			},
			Status: BillableItemStatusArchived, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: price},
		},
	}}
	handler := NewHandler(service, catalog)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/catalog", handler.ListBillableCatalog)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/catalog", nil))

	body := response.Body.String()
	if response.Code != http.StatusOK ||
		!strings.Contains(body, `"sku":"ai-image:local-inpaint"`) ||
		!strings.Contains(body, `"product_kind":"workflow-feature"`) ||
		!strings.Contains(body, `"display_name":"局部重绘"`) ||
		!strings.Contains(body, `"status":"unpriced"`) ||
		!strings.Contains(body, `"status":"active"`) ||
		!strings.Contains(body, `"status":"disabled"`) ||
		!strings.Contains(body, `"status":"archived"`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestQuoteHandlerRequiresTypedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name  string
		setup func(*gin.Context)
	}{
		{name: "missing"},
		{name: "invalid Principal", setup: func(c *gin.Context) { _ = principal.Set(c, principal.Principal{}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &handlerServiceStub{}
			router := gin.New()
			router.POST("/quote", func(c *gin.Context) {
				if tt.setup != nil {
					tt.setup(c)
				}
				NewHandler(service, nil).Quote(c)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{"sku":"ideaboard:image.generate","quantity":1}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized || service.quoteInput.sku != "" || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d input = %+v body = %s", recorder.Code, service.quoteInput, recorder.Body.String())
			}
		})
	}
}

func TestAdminPublishPriceUsesAuthenticatedActorWithoutReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := &billableCatalogReaderStub{}
	handler := NewHandler(&handlerServiceStub{}, catalog)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/catalog/:sku/prices", handler.PublishPrice)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/catalog/ai-image:local-inpaint/prices", strings.NewReader(`{"prices":{"1k":0,"2k":2}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || catalog.publishSKU != "ai-image:local-inpaint" || catalog.publishPrices[OutputResolution1K] != 0 || catalog.publishPrices[OutputResolution2K] != 2 || catalog.publishActorID != 9 {
		t.Fatalf("status=%d body=%s publish=%+v", recorder.Code, recorder.Body.String(), catalog)
	}

	missingRecorder := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodPost, "/catalog/ai-image:local-inpaint/prices", strings.NewReader(`{}`))
	missingRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing prices status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}
}

func TestAdminSetAvailabilityUsesAuthenticatedActorWithoutReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := &billableCatalogReaderStub{}
	handler := NewHandler(&handlerServiceStub{}, catalog)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.PUT("/catalog/:sku/availability", handler.SetAvailability)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/catalog/ai-image:local-inpaint/availability", strings.NewReader(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || catalog.availabilitySKU != "ai-image:local-inpaint" || catalog.availabilityEnabled || catalog.availabilityActorID != 9 || !strings.Contains(recorder.Body.String(), `"status":"disabled"`) {
		t.Fatalf("status=%d body=%s availability=%+v", recorder.Code, recorder.Body.String(), catalog)
	}

	missingRecorder := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodPut, "/catalog/ai-image:local-inpaint/availability", strings.NewReader(`{}`))
	missingRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}
}

func TestAdminHandlersRejectUnknownAndInvalidInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := &billableCatalogReaderStub{}
	handler := NewHandler(&handlerServiceStub{}, catalog)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/catalog/:sku/prices", handler.PublishPrice)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "publish price cannot use provider routing", method: http.MethodPost, path: "/catalog/test:item/prices", body: `{"unit_credits":1,"provider_account_id":2}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminArchivedDeleteRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []struct {
		name, method, path string
		handler            func(*Handler) gin.HandlerFunc
	}{
		{name: "delete", method: http.MethodDelete, path: "/catalog/test:item", handler: func(h *Handler) gin.HandlerFunc { return h.DeleteArchivedItem }},
	} {
		for _, identity := range []struct {
			name       string
			role       principal.Role
			wantStatus int
		}{
			{name: "missing", wantStatus: http.StatusUnauthorized},
			{name: "user", role: principal.RoleUser, wantStatus: http.StatusForbidden},
		} {
			t.Run(endpoint.name+"/"+identity.name, func(t *testing.T) {
				router := gin.New()
				router.Handle(endpoint.method, endpoint.path, func(c *gin.Context) {
					if identity.role != "" {
						testsupport.SetPrincipal(t, c, 9, identity.role)
					}
					endpoint.handler(NewHandler(nil, nil))(c)
				})
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(endpoint.method, endpoint.path, nil))
				if recorder.Code != identity.wantStatus {
					t.Fatalf("status=%d body=%s want=%d", recorder.Code, recorder.Body.String(), identity.wantStatus)
				}
			})
		}
	}
}

func TestAdminArchivedDeleteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := &billableCatalogReaderStub{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 12, principal.RoleAdmin)
		c.Next()
	})
	router.DELETE("/catalog/:sku", NewHandler(nil, catalog).DeleteArchivedItem)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/catalog/legacy:removed", nil))
	if recorder.Code != http.StatusOK || catalog.deleted.SKU != "legacy:removed" || catalog.deleteActorID != 12 || !strings.Contains(recorder.Body.String(), `"sku":"legacy:removed"`) {
		t.Fatalf("status=%d body=%s delete=%+v actor=%d", recorder.Code, recorder.Body.String(), catalog.deleted, catalog.deleteActorID)
	}

	for _, conflict := range []error{ErrItemNotArchived, ErrArchivedItemInUse} {
		catalog.err = conflict
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/catalog/legacy:removed", nil))
		if recorder.Code != http.StatusConflict {
			t.Fatalf("error=%v status=%d body=%s", conflict, recorder.Code, recorder.Body.String())
		}
	}
}
