package billingcoverage

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/workflow"
)

type stubAccountReader struct {
	items []*accounts.ApiAccount
	err   error
}

func (reader stubAccountReader) GetAllEnabled(context.Context) ([]*accounts.ApiAccount, error) {
	return reader.items, reader.err
}

type stubCatalogReader struct {
	items         []billing.CatalogItem
	err           error
	requestedSKUs []string
}

func (reader *stubCatalogReader) GetSKUs(_ context.Context, skus []string) ([]billing.CatalogItem, error) {
	reader.requestedSKUs = append([]string(nil), skus...)
	return reader.items, reader.err
}

func TestServiceListsExecutableAIWorkflowModelCoverage(t *testing.T) {
	unitCredits := int64(4)
	reader := &stubCatalogReader{items: []billing.CatalogItem{
		{
			SKU:     "ai-image:sketch-to-image.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18",
			Enabled: true,
			CurrentPrice: &billing.PriceInfo{
				UnitCredits: unitCredits,
				Enabled:     true,
			},
		},
		{
			SKU:     "ai-image:remove-bg.33d88374630408b27e09a1f4f801f2c107d50958a5b00544589021baf4ad87a6",
			Enabled: false,
		},
		{
			SKU:     "ai-image:style-fusion.47966299eececc95808ccc96a5371dfe03ec3fe9dc3b21ac4fe207275cf82b4a",
			Enabled: true,
			CurrentPrice: &billing.PriceInfo{
				UnitCredits: 9,
				Enabled:     false,
			},
		},
		{
			SKU:     "ai-image:partial-edit.123ab0fba0b8a1df883012d03070a94885fd038434c325a802c822c819cdf495",
			Enabled: true,
		},
	}}
	service := NewService(stubAccountReader{items: []*accounts.ApiAccount{
		{ID: 1, Models: []accounts.AccountModel{
			{Name: " GPT-Image-1 ", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugSketchToImage, workflow.FeatureSlugRemoveBg}},
			{Name: "disabled-model", Enabled: false},
		}},
		{ID: 2, Models: []accounts.AccountModel{
			{Name: "gpt-image-1", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugSketchToImage, workflow.FeatureSlugPartialEdit}},
			{Name: "Imagen-4", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugStyleFusion}},
		}},
	}}, reader)

	coverage, err := service.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(coverage.Items) != 4 {
		t.Fatalf("List() items = %+v, want 4 executable feature/model pairs", coverage.Items)
	}

	want := []CoverageItem{
		{
			Feature:       workflow.FeatureSlugSketchToImage,
			Model:         "gpt-image-1",
			SKU:           "ai-image:sketch-to-image.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18",
			SKUConfigured: true, SKUEnabled: true,
			PriceConfigured: true, PriceEnabled: true, UnitCredits: &unitCredits,
		},
		{
			Feature:       workflow.FeatureSlugPartialEdit,
			Model:         "gpt-image-1",
			SKU:           "ai-image:partial-edit.123ab0fba0b8a1df883012d03070a94885fd038434c325a802c822c819cdf495",
			SKUConfigured: true, SKUEnabled: true,
			PriceConfigured: false, PriceEnabled: false,
		},
		{
			Feature:       workflow.FeatureSlugRemoveBg,
			Model:         "gpt-image-1",
			SKU:           "ai-image:remove-bg.33d88374630408b27e09a1f4f801f2c107d50958a5b00544589021baf4ad87a6",
			SKUConfigured: true, SKUEnabled: false,
			PriceConfigured: false, PriceEnabled: false,
		},
		{
			Feature:       workflow.FeatureSlugStyleFusion,
			Model:         "imagen-4",
			SKU:           "ai-image:style-fusion.47966299eececc95808ccc96a5371dfe03ec3fe9dc3b21ac4fe207275cf82b4a",
			SKUConfigured: true, SKUEnabled: true,
			PriceConfigured: true, PriceEnabled: false, UnitCredits: int64Pointer(9),
		},
	}
	for index := range want {
		assertCoverageItem(t, coverage.Items[index], want[index])
	}
	if len(reader.requestedSKUs) != 4 {
		t.Fatalf("GetSKUs() requested %d SKUs, want 4 deduplicated canonical SKUs", len(reader.requestedSKUs))
	}
}

func TestServiceReturnsEmptyCoverageWhenNoProviderModelIsExecutable(t *testing.T) {
	reader := &stubCatalogReader{}
	service := NewService(stubAccountReader{items: []*accounts.ApiAccount{{
		ID:     1,
		Models: []accounts.AccountModel{{Name: "disabled", Enabled: false}},
	}}}, reader)

	coverage, err := service.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if coverage.Items == nil || len(coverage.Items) != 0 {
		t.Fatalf("List() items = %#v, want non-nil empty list", coverage.Items)
	}
	if len(reader.requestedSKUs) != 0 {
		t.Fatalf("GetSKUs() should not be called with empty coverage, got %v", reader.requestedSKUs)
	}
}

func TestServicePropagatesCoverageSourceFailures(t *testing.T) {
	accountErr := errors.New("accounts unavailable")
	if _, err := NewService(stubAccountReader{err: accountErr}, &stubCatalogReader{}).List(t.Context()); !errors.Is(err, accountErr) {
		t.Fatalf("List() account error = %v, want %v", err, accountErr)
	}

	catalogErr := errors.New("catalog unavailable")
	service := NewService(stubAccountReader{items: []*accounts.ApiAccount{{
		ID:     1,
		Models: []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}}}, &stubCatalogReader{err: catalogErr})
	if _, err := service.List(t.Context()); !errors.Is(err, catalogErr) {
		t.Fatalf("List() catalog error = %v, want %v", err, catalogErr)
	}
}

func assertCoverageItem(t *testing.T, got, want CoverageItem) {
	t.Helper()
	if got.Feature != want.Feature || got.Model != want.Model || got.SKU != want.SKU ||
		got.SKUConfigured != want.SKUConfigured || got.SKUEnabled != want.SKUEnabled ||
		got.PriceConfigured != want.PriceConfigured || got.PriceEnabled != want.PriceEnabled {
		t.Fatalf("coverage item = %+v, want %+v", got, want)
	}
	if (got.UnitCredits == nil) != (want.UnitCredits == nil) || (got.UnitCredits != nil && *got.UnitCredits != *want.UnitCredits) {
		t.Fatalf("coverage unit credits = %v, want %v", got.UnitCredits, want.UnitCredits)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
