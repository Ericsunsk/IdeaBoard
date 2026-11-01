package infrastructure

import (
	"testing"

	"ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func newGatewayTestRepository(t *testing.T) *ConfigRepository {
	t.Helper()
	db := testdb.Open(t)
	return NewConfigRepository(settings.NewSystemConfigRepository(db))
}

func TestSaveCatalogValidation(t *testing.T) {
	secretstest.EnableEncryption(t)
	repo := newGatewayTestRepository(t)
	ctx := t.Context()

	valid := []domain.CatalogItem{{
		ID: "model-1", Name: "Model 1", Provider: "Test",
		Mode: "chat", Description: "d", ContextOrResolution: "c", Hosting: "h", TermsURL: "t", DocsURL: "d",
	}}
	if err := repo.SaveCatalog(ctx, valid, 1); err != nil {
		t.Fatalf("SaveCatalog(valid) error = %v", err)
	}

	dup := append(append([]domain.CatalogItem{}, valid...), domain.CatalogItem{
		ID: "MODEL-1", Name: "Dup", Provider: "Test",
	})
	if err := repo.SaveCatalog(ctx, dup, 1); err == nil {
		t.Fatal("SaveCatalog(duplicate ID) error = nil, want error")
	}

	badJSON := append([]domain.CatalogItem{}, valid[0])
	badJSON[0].CustomParams = "{not-json"
	if err := repo.SaveCatalog(ctx, badJSON, 1); err == nil {
		t.Fatal("SaveCatalog(invalid custom_params) error = nil, want error")
	}

	nonObject := append([]domain.CatalogItem{}, valid[0])
	nonObject[0].CustomParams = "[1,2]"
	if err := repo.SaveCatalog(ctx, nonObject, 1); err == nil {
		t.Fatal("SaveCatalog(non-object custom_params) error = nil, want error")
	}

	if err := repo.SaveCatalog(ctx, valid, 0); err == nil {
		t.Fatal("SaveCatalog(zero actor) error = nil, want error")
	}
}

func TestListCatalogSurfacesCorruption(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	if _, err := repo.ListCatalog(ctx); err != nil {
		t.Fatalf("ListCatalog(unconfigured) error = %v, want empty default", err)
	}
	settingsRepo := settings.NewSystemConfigRepository(db)
	if err := settingsRepo.Set(ctx, catalogConfigKey, "{corrupt"); err != nil {
		t.Fatalf("seed corrupt catalog: %v", err)
	}
	if _, err := repo.ListCatalog(ctx); err == nil {
		t.Fatal("ListCatalog(corrupt) error = nil, want integrity error")
	}
}
