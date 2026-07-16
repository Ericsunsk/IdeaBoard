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
			SKU:     "ai-image:sketch-to-image",
			Enabled: true,
			CurrentPrice: &billing.PriceInfo{
				UnitCredits: unitCredits,
			},
		},
		{
			SKU:     "ai-image:remove-bg",
			Enabled: false,
		},
		{
			SKU:     "ai-image:style-fusion",
			Enabled: true,
			CurrentPrice: &billing.PriceInfo{
				UnitCredits: 9,
			},
		},
		{
			SKU:     "ai-image:partial-edit",
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
			SKU:           "ai-image:sketch-to-image",
			SKUConfigured: true, SKUEnabled: true,
			PriceConfigured: true, PriceEnabled: true, UnitCredits: &unitCredits,
		},
		{
			Feature:       workflow.FeatureSlugPartialEdit,
			Model:         "gpt-image-1",
			SKU:           "ai-image:partial-edit",
			SKUConfigured: true, SKUEnabled: true,
			PriceConfigured: false, PriceEnabled: false,
		},
		{
			Feature:       workflow.FeatureSlugRemoveBg,
			Model:         "gpt-image-1",
			SKU:           "ai-image:remove-bg",
			SKUConfigured: true, SKUEnabled: false,
			PriceConfigured: false, PriceEnabled: false,
		},
		{
			Feature:       workflow.FeatureSlugStyleFusion,
			Model:         "imagen-4",
			SKU:           "ai-image:style-fusion",
			SKUConfigured: true, SKUEnabled: true,
			PriceConfigured: true, PriceEnabled: true, UnitCredits: int64Pointer(9),
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
