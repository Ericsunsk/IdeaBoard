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
	createSKUInput CreateSKUInput
	updateSKUInput UpdateSKUInput
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

func (s *handlerServiceStub) CreateSKU(_ context.Context, input CreateSKUInput) (CatalogItem, error) {
	s.createSKUInput = input
	return CatalogItem{SKU: SKU(input.SKU), DisplayName: input.DisplayName, Enabled: input.Enabled}, nil
}

func (s *handlerServiceStub) UpdateSKU(_ context.Context, input UpdateSKUInput) (CatalogItem, error) {
	s.updateSKUInput = input
	return CatalogItem{SKU: SKU(input.SKU)}, nil
}

func (s *handlerServiceStub) ListCatalog(context.Context, ListCatalogInput) (CatalogPage, error) {
	return CatalogPage{Items: []CatalogItem{}}, nil
}

func (s *handlerServiceStub) ListPrices(context.Context, ListPricesInput) (PricePage, error) {
	return PricePage{Prices: []PriceInfo{}}, nil
}

func TestQuoteHandlerRejectsCallerSuppliedPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service, nil)
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
	handler := NewHandler(service, service, nil)
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
	handler := NewHandler(service, service, catalog)
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
				NewHandler(service, service, nil).Quote(c)
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

func TestAdminCreateSKURequiresActorAndExplicitState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service, nil)

	for _, tt := range []struct {
		name       string
		actorID    int64
		role       principal.Role
		body       string
		wantStatus int
	}{
		{name: "missing actor", body: `{"sku":"test:item","display_name":"Item","enabled":true,"reason":"launch pricing"}`, wantStatus: http.StatusUnauthorized},
		{name: "user role", actorID: 9, role: principal.RoleUser, body: `{"sku":"test:item","display_name":"Item","enabled":true,"reason":"launch pricing"}`, wantStatus: http.StatusForbidden},
		{name: "missing explicit enabled", actorID: 9, role: principal.RoleAdmin, body: `{"sku":"test:item","display_name":"Item","reason":"launch pricing"}`, wantStatus: http.StatusBadRequest},
		{name: "missing reason", actorID: 9, role: principal.RoleAdmin, body: `{"sku":"test:item","display_name":"Item","enabled":true}`, wantStatus: http.StatusBadRequest},
		{name: "blank reason", actorID: 9, role: principal.RoleAdmin, body: `{"sku":"test:item","display_name":"Item","enabled":true,"reason":"  "}`, wantStatus: http.StatusBadRequest},
		{name: "valid", actorID: 9, role: principal.RoleAdmin, body: `{"sku":"test:item","display_name":"Item","description":"D","enabled":true,"reason":"  launch pricing  "}`, wantStatus: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/skus", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			if tt.actorID > 0 {
				request.Header.Set("X-Test-Actor", "true")
			}
			routerWithActor := gin.New()
			routerWithActor.Use(func(c *gin.Context) {
				if tt.actorID > 0 {
					testsupport.SetPrincipal(t, c, tt.actorID, tt.role)
				}
				c.Next()
			})
			routerWithActor.POST("/skus", handler.CreateSKU)
			routerWithActor.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", response.Code, response.Body.String(), tt.wantStatus)
			}
		})
	}
	if service.createSKUInput.Action.ActorID() != 9 || !service.createSKUInput.Enabled || service.createSKUInput.Action.Reason() != "launch pricing" {
		t.Fatalf("CreateSKU input = %+v", service.createSKUInput)
	}
}

func TestAdminBillingAuthenticatesBeforeServiceAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name       string
		setup      func(*gin.Context)
		wantStatus int
	}{
		{name: "missing Principal", wantStatus: http.StatusUnauthorized},
		{name: "user Principal", setup: func(c *gin.Context) { testsupport.SetPrincipal(t, c, 9, principal.RoleUser) }, wantStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/skus", func(c *gin.Context) {
				if tt.setup != nil {
					tt.setup(c)
				}
				NewHandler(nil, nil, nil).CreateSKU(c)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/skus", strings.NewReader(`{}`)))
			if recorder.Code != tt.wantStatus || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminBillingWriteHandlerRequiresAndTrimsSKUReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.PATCH("/skus/:sku", handler.UpdateSKU)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update missing reason", method: http.MethodPatch, path: "/skus/test:item", body: `{"enabled":false}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", response.Code, response.Body.String())
			}
		})
	}

	validRequests := []struct{ method, path, body string }{{
		method: http.MethodPatch, path: "/skus/test:item", body: `{"enabled":false,"reason":"  disable catalog  "}`,
	}}
	for _, requestCase := range validRequests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(requestCase.method, requestCase.path, strings.NewReader(requestCase.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d body=%s", requestCase.method, requestCase.path, response.Code, response.Body.String())
		}
	}
	if service.updateSKUInput.Action.Reason() != "disable catalog" {
		t.Fatalf("trimmed update reason=%q", service.updateSKUInput.Action.Reason())
	}
}

func TestAdminPublishPriceUsesAuthenticatedActorWithoutReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := &billableCatalogReaderStub{}
	handler := NewHandler(&handlerServiceStub{}, nil, catalog)
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
	handler := NewHandler(&handlerServiceStub{}, nil, catalog)
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
	service := &handlerServiceStub{}
	handler := NewHandler(service, service, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.PATCH("/skus/:sku", handler.UpdateSKU)
	router.GET("/skus", handler.ListCatalog)
	router.GET("/skus/:sku/prices", handler.ListPrices)
	router.POST("/catalog/:sku/prices", handler.PublishPrice)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update SKU cannot smuggle rename", method: http.MethodPatch, path: "/skus/test:item", body: `{"sku":"test:renamed"}`},
		{name: "publish price cannot use provider routing", method: http.MethodPost, path: "/catalog/test:item/prices", body: `{"unit_credits":1,"provider_account_id":2}`},
		{name: "catalog limit is bounded", method: http.MethodGet, path: "/skus?limit=101"},
		{name: "price cursor is positive", method: http.MethodGet, path: "/skus/test:item/prices?before_version=-1"},
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
