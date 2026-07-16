package billingcoverage

import (
	"context"
	"fmt"
	"sort"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/workflow"
)

type AccountReader interface {
	GetAllEnabled(context.Context) ([]*accounts.ApiAccount, error)
}

type CatalogReader interface {
	GetSKUs(context.Context, []string) ([]billing.CatalogItem, error)
}

type Service struct {
	accounts AccountReader
	catalog  CatalogReader
}

type Coverage struct {
	Items []CoverageItem `json:"items"`
}

type CoverageItem struct {
	Feature         string      `json:"feature"`
	Model           string      `json:"model"`
	SKU             billing.SKU `json:"sku"`
	SKUConfigured   bool        `json:"sku_configured"`
	SKUEnabled      bool        `json:"sku_enabled"`
	PriceConfigured bool        `json:"price_configured"`
	PriceEnabled    bool        `json:"price_enabled"`
	UnitCredits     *int64      `json:"unit_credits,omitempty"`
}

func NewService(accountReader AccountReader, catalogReader CatalogReader) *Service {
	return &Service{accounts: accountReader, catalog: catalogReader}
}

func (service *Service) List(ctx context.Context) (Coverage, error) {
	if service == nil || service.accounts == nil || service.catalog == nil {
		return Coverage{}, fmt.Errorf("billing coverage service unavailable")
	}
	enabledAccounts, err := service.accounts.GetAllEnabled(ctx)
	if err != nil {
		return Coverage{}, fmt.Errorf("list executable provider accounts: %w", err)
	}

	items, skus, err := executableCoverageItems(enabledAccounts)
	if err != nil {
		return Coverage{}, err
	}
	if len(items) == 0 {
		return Coverage{Items: []CoverageItem{}}, nil
	}
	catalogItems, err := service.catalog.GetSKUs(ctx, skus)
	if err != nil {
		return Coverage{}, fmt.Errorf("load billing coverage catalog: %w", err)
	}
	catalogBySKU := make(map[billing.SKU]billing.CatalogItem, len(catalogItems))
	for _, item := range catalogItems {
		catalogBySKU[item.SKU] = item
	}
	for index := range items {
		catalogItem, configured := catalogBySKU[items[index].SKU]
		if !configured {
			continue
		}
		items[index].SKUConfigured = true
		items[index].SKUEnabled = catalogItem.Enabled
		if catalogItem.CurrentPrice == nil {
			continue
		}
		items[index].PriceConfigured = true
		items[index].PriceEnabled = catalogItem.Enabled
		unitCredits := catalogItem.CurrentPrice.UnitCredits
		items[index].UnitCredits = &unitCredits
	}
	return Coverage{Items: items}, nil
}

func executableCoverageItems(enabledAccounts []*accounts.ApiAccount) ([]CoverageItem, []string, error) {
	modelsByFeature := make(map[string]map[string]struct{})
	for _, feature := range workflow.AIImageFeatureSlugs() {
		modelsByFeature[feature] = make(map[string]struct{})
		for _, account := range enabledAccounts {
			if account == nil {
				continue
			}
			for _, model := range account.Models {
				canonicalModel := workflow.NormalizeModelKey(model.Name)
				if !model.Enabled || canonicalModel == "" {
					continue
				}
				if _, executable := accounts.ResolveAccountModel(account, feature, canonicalModel); executable {
					modelsByFeature[feature][canonicalModel] = struct{}{}
				}
			}
		}
	}

	items := make([]CoverageItem, 0)
	skus := make([]string, 0)
	for _, feature := range workflow.AIImageFeatureSlugs() {
		models := make([]string, 0, len(modelsByFeature[feature]))
		for model := range modelsByFeature[feature] {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			request, err := taskbilling.AIImageRequest(feature, model)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve billing coverage product: %w", err)
			}
			sku, err := taskbilling.ResolveSKU(request)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve billing coverage SKU: %w", err)
			}
			items = append(items, CoverageItem{Feature: feature, Model: model, SKU: sku})
			skus = append(skus, string(sku))
		}
	}
	return items, skus, nil
}
