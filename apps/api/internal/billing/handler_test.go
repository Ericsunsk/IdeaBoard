package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type handlerServiceStub struct {
	quoteInput struct {
		sku      string
		quantity int64
	}
	createSKUInput   CreateSKUInput
	createPriceInput CreatePriceInput
}

func (s *handlerServiceStub) Quote(_ context.Context, sku string, quantity int64) (Quote, error) {
	s.quoteInput.sku = sku
	s.quoteInput.quantity = quantity
	return Quote{
		PriceSnapshot: PriceSnapshot{SKU: "ideaboard:image.generate", PriceVersionID: 3, PriceVersion: 2, UnitCredits: 25, QuotedAt: time.Unix(10, 0).UTC()},
		Quantity:      quantity, TotalCredits: 100,
	}, nil
}

func (s *handlerServiceStub) CreateSKU(_ context.Context, input CreateSKUInput) (CatalogItem, error) {
	s.createSKUInput = input
	return CatalogItem{SKU: SKU(input.SKU), DisplayName: input.DisplayName, Enabled: input.Enabled}, nil
}

func (s *handlerServiceStub) UpdateSKU(_ context.Context, input UpdateSKUInput) (CatalogItem, error) {
	return CatalogItem{SKU: SKU(input.SKU)}, nil
}

func (s *handlerServiceStub) CreatePrice(_ context.Context, input CreatePriceInput) (PriceInfo, error) {
	s.createPriceInput = input
	return PriceInfo{PriceVersionID: 1, Version: 1, UnitCredits: input.UnitCredits, Enabled: input.Enabled}, nil
}

func (s *handlerServiceStub) SetPriceEnabled(_ context.Context, input SetPriceEnabledInput) (PriceInfo, error) {
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
	router.POST("/quote", handler.Quote)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{"sku":"IDEABOARD:IMAGE.GENERATE","quantity":4}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total_credits":100`) || !strings.Contains(response.Body.String(), `"price_version_id":3`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.quoteInput.sku != "IDEABOARD:IMAGE.GENERATE" || service.quoteInput.quantity != 4 {
		t.Fatalf("Quote input = %+v", service.quoteInput)
	}
}

func TestAdminCreateSKURequiresActorAndExplicitState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service)

	for _, tt := range []struct {
		name       string
		actorID    int64
		body       string
		wantStatus int
	}{
		{name: "missing actor", body: `{"sku":"test:item","display_name":"Item","enabled":true}`, wantStatus: http.StatusUnauthorized},
		{name: "missing explicit enabled", actorID: 9, body: `{"sku":"test:item","display_name":"Item"}`, wantStatus: http.StatusBadRequest},
		{name: "valid", actorID: 9, body: `{"sku":"test:item","display_name":"Item","description":"D","enabled":true}`, wantStatus: http.StatusOK},
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
					c.Set("user_id", tt.actorID)
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
	if service.createSKUInput.ActorID != 9 || !service.createSKUInput.Enabled {
		t.Fatalf("CreateSKU input = %+v", service.createSKUInput)
	}
}

func TestAdminHandlersRejectUnknownAndInvalidInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &handlerServiceStub{}
	handler := NewHandler(service, service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(9))
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
