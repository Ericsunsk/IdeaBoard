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
	createSKUInput   CreateSKUInput
	updateSKUInput   UpdateSKUInput
	createPriceInput CreatePriceInput
	setPriceInput    SetPriceEnabledInput
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

func (s *handlerServiceStub) CreatePrice(_ context.Context, input CreatePriceInput) (PriceInfo, error) {
	s.createPriceInput = input
	return PriceInfo{PriceVersionID: 1, Version: 1, UnitCredits: input.UnitCredits, Enabled: input.Enabled}, nil
}

func (s *handlerServiceStub) SetPriceEnabled(_ context.Context, input SetPriceEnabledInput) (PriceInfo, error) {
	s.setPriceInput = input
	return PriceInfo{PriceVersionID: 1, Version: input.Version, Enabled: input.Enabled}, nil
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
	handler := NewHandler(service, service)
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
	handler := NewHandler(service, service)
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
				NewHandler(service, service).Quote(c)
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
	handler := NewHandler(service, service)

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
				NewHandler(nil, nil).CreateSKU(c)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/skus", strings.NewReader(`{}`)))
			if recorder.Code != tt.wantStatus || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminBillingWriteHandlersRequireAndTrimReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.PATCH("/skus/:sku", handler.UpdateSKU)
	router.POST("/skus/:sku/prices", handler.CreatePrice)
	router.PATCH("/skus/:sku/prices/:version", handler.SetPriceEnabled)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update missing reason", method: http.MethodPatch, path: "/skus/test:item", body: `{"enabled":false}`},
		{name: "create price blank reason", method: http.MethodPost, path: "/skus/test:item/prices", body: `{"unit_credits":4,"enabled":true,"reason":"  "}`},
		{name: "set state too long Unicode reason", method: http.MethodPatch, path: "/skus/test:item/prices/1", body: `{"enabled":false,"reason":"` + strings.Repeat("点", 1001) + `"}`},
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

	validRequests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPatch, path: "/skus/test:item", body: `{"enabled":false,"reason":"  disable catalog  "}`},
		{method: http.MethodPost, path: "/skus/test:item/prices", body: `{"unit_credits":4,"enabled":true,"reason":"  publish price  "}`},
		{method: http.MethodPatch, path: "/skus/test:item/prices/1", body: `{"enabled":false,"reason":"  pause price  "}`},
	}
	for _, requestCase := range validRequests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(requestCase.method, requestCase.path, strings.NewReader(requestCase.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d body=%s", requestCase.method, requestCase.path, response.Code, response.Body.String())
		}
	}
	if service.updateSKUInput.Action.Reason() != "disable catalog" || service.createPriceInput.Action.Reason() != "publish price" || service.setPriceInput.Action.Reason() != "pause price" {
		t.Fatalf("trimmed reasons update=%q create_price=%q set_price=%q", service.updateSKUInput.Action.Reason(), service.createPriceInput.Action.Reason(), service.setPriceInput.Action.Reason())
	}
}

func TestAdminHandlersRejectUnknownAndInvalidInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 9, principal.RoleAdmin)
		c.Next()
	})
	router.PATCH("/skus/:sku", handler.UpdateSKU)
	router.GET("/skus", handler.ListCatalog)
	router.GET("/skus/:sku/prices", handler.ListPrices)
	router.POST("/skus/:sku/prices", handler.CreatePrice)
	router.PATCH("/skus/:sku/prices/:version", handler.SetPriceEnabled)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update SKU cannot smuggle rename", method: http.MethodPatch, path: "/skus/test:item", body: `{"sku":"test:renamed"}`},
		{name: "create price cannot use provider routing", method: http.MethodPost, path: "/skus/test:item/prices", body: `{"unit_credits":1,"enabled":true,"provider_account_id":2}`},
		{name: "price state cannot mutate unit", method: http.MethodPatch, path: "/skus/test:item/prices/1", body: `{"enabled":true,"unit_credits":2}`},
		{name: "price version must be positive", method: http.MethodPatch, path: "/skus/test:item/prices/0", body: `{"enabled":true}`},
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
