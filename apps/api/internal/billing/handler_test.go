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
		sku      string
		quantity int64
	}
}

type billableCatalogReaderStub struct {
	items               []BillableItemCatalogCard
	err                 error
	publishSKU          string
	publishCredits      int64
	publishActorID      int64
	availabilitySKU     string
	availabilityEnabled bool
	availabilityActorID int64
	historySKU          string
	historyBefore       int64
	historyLimit        int
	pricePage           PricePage
	deleted             ArchivedItemDeleteResult
	deleteActorID       int64
}

func (s *billableCatalogReaderStub) ListPriceHistory(_ context.Context, sku string, before int64, limit int) (PricePage, error) {
	s.historySKU = sku
	s.historyBefore = before
	s.historyLimit = limit
	return s.pricePage, s.err
}

func (s *billableCatalogReaderStub) DeleteArchived(_ context.Context, sku string, actorID int64) (ArchivedItemDeleteResult, error) {
	s.deleted = ArchivedItemDeleteResult{SKU: SKU(sku)}
	s.deleteActorID = actorID
	return s.deleted, s.err
}

func (s *billableCatalogReaderStub) List(context.Context) ([]BillableItemCatalogCard, error) {
	return s.items, s.err
}

func (s *billableCatalogReaderStub) PublishPrice(_ context.Context, sku string, unitCredits, actorID int64) (PriceInfo, error) {
	s.publishSKU = sku
	s.publishCredits = unitCredits
	s.publishActorID = actorID
	return PriceInfo{PriceVersionID: 1, Version: 1, UnitCredits: unitCredits}, s.err
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

func (s *handlerServiceStub) Quote(_ context.Context, sku string, quantity int64) (Quote, error) {
	s.quoteInput.sku = sku
	s.quoteInput.quantity = quantity
	return Quote{
		PriceSnapshot: PriceSnapshot{SKU: "ideaboard:image.generate", DisplayName: "AI 生图", PriceVersionID: 3, PriceVersion: 2, UnitCredits: 25, QuotedAt: time.Unix(10, 0).UTC()},
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
	request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{"sku":"IDEABOARD:IMAGE.GENERATE","quantity":4}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"display_name":"AI 生图"`) || !strings.Contains(response.Body.String(), `"total_credits":100`) || !strings.Contains(response.Body.String(), `"price_version_id":3`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.quoteInput.sku != "IDEABOARD:IMAGE.GENERATE" || service.quoteInput.quantity != 4 {
		t.Fatalf("Quote input = %+v", service.quoteInput)
	}
}

func TestAdminBillableCatalogReturnsFeatureCardContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	price := &PriceInfo{PriceVersionID: 1, Version: 1, UnitCredits: 25}
	catalog := &billableCatalogReaderStub{items: []BillableItemCatalogCard{
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "ai-image:partial-edit", Namespace: "ai-image", ProductKind: "workflow-feature",
				DisplayName: "局部重绘", Description: "仅重绘图片选定区域。", SortOrder: 10,
			},
			Status: BillableItemStatusUnpriced,
		},
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "ai-image:remove-bg", Namespace: "ai-image", ProductKind: "workflow-feature",
				DisplayName: "抠图", SortOrder: 20,
			},
			Status: BillableItemStatusActive, CurrentPrice: price,
		},
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "ai-image:hd-upscale", Namespace: "ai-image", ProductKind: "workflow-feature",
				DisplayName: "HD", SortOrder: 30,
			},
			Status: BillableItemStatusDisabled, CurrentPrice: price,
		},
		{
			BillableItemDefinition: BillableItemDefinition{
				SKU: "legacy:removed", Namespace: "legacy", ProductKind: "legacy",
				DisplayName: "已移除功能", SortOrder: 0,
			},
			Status: BillableItemStatusArchived, CurrentPrice: price,
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
		!strings.Contains(body, `"sku":"ai-image:partial-edit"`) ||
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
	request := httptest.NewRequest(http.MethodPost, "/catalog/ai-image:partial-edit/prices", strings.NewReader(`{"unit_credits":0}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || catalog.publishSKU != "ai-image:partial-edit" || catalog.publishCredits != 0 || catalog.publishActorID != 9 {
		t.Fatalf("status=%d body=%s publish=%+v", recorder.Code, recorder.Body.String(), catalog)
	}

	missingRecorder := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodPost, "/catalog/ai-image:partial-edit/prices", strings.NewReader(`{}`))
	missingRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing unit_credits status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
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
	request := httptest.NewRequest(http.MethodPut, "/catalog/ai-image:partial-edit/availability", strings.NewReader(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || catalog.availabilitySKU != "ai-image:partial-edit" || catalog.availabilityEnabled || catalog.availabilityActorID != 9 || !strings.Contains(recorder.Body.String(), `"status":"disabled"`) {
		t.Fatalf("status=%d body=%s availability=%+v", recorder.Code, recorder.Body.String(), catalog)
	}

	missingRecorder := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodPut, "/catalog/ai-image:partial-edit/availability", strings.NewReader(`{}`))
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
	router.GET("/catalog/:sku/prices", handler.ListBillableItemPrices)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "publish price cannot use provider routing", method: http.MethodPost, path: "/catalog/test:item/prices", body: `{"unit_credits":1,"provider_account_id":2}`},
		{name: "price cursor is positive", method: http.MethodGet, path: "/catalog/test:item/prices?before_version=-1"},
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

func TestAdminBillableItemHistoryAndDeleteRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []struct {
		name, method, path string
		handler            func(*Handler) gin.HandlerFunc
	}{
		{name: "history", method: http.MethodGet, path: "/catalog/test:item/prices", handler: func(h *Handler) gin.HandlerFunc { return h.ListBillableItemPrices }},
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

func TestAdminBillableItemPriceHistoryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := &billableCatalogReaderStub{pricePage: PricePage{
		Prices: []PriceInfo{{PriceVersionID: 8, Version: 2, UnitCredits: 4, CreatedBy: 9}},
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/catalog/:sku/prices", NewHandler(nil, catalog).ListBillableItemPrices)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/catalog/legacy:removed/prices?before_version=3&limit=10", nil))
	if recorder.Code != http.StatusOK || catalog.historySKU != "legacy:removed" || catalog.historyBefore != 3 || catalog.historyLimit != 10 || !strings.Contains(recorder.Body.String(), `"price_version_id":8`) {
		t.Fatalf("status=%d body=%s history=%+v", recorder.Code, recorder.Body.String(), catalog)
	}

	for _, query := range []string{"?before_version=-1", "?before_version=bad", "?limit=101", "?limit=bad"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/catalog/legacy:removed/prices"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("query=%q status=%d body=%s", query, recorder.Code, recorder.Body.String())
		}
	}

	catalog.err = ErrSKUNotFound
	notFound := httptest.NewRecorder()
	router.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/catalog/missing:item/prices", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d body=%s", notFound.Code, notFound.Body.String())
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
