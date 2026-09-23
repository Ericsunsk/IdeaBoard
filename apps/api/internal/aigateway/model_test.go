package aigateway

import (
	"sync"
	"testing"

	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestValidateModelMapping(t *testing.T) {
	slug, model, err := validateModelMapping("render", "openai/gpt-image-1")
	if err != nil || slug != "render" || model != "openai/gpt-image-1" {
		t.Fatalf("validate = %q %q %v", slug, model, err)
	}
	for _, tc := range []struct {
		name    string
		feature string
		model   string
	}{
		{"unknown feature", "nope", "openai/gpt-image-1"},
		{"missing provider", "render", "gpt-image-1"},
		{"blank model", "render", "   "},
		{"blank model after separator", "render", "provider/"},
		{"leading separator in model", "render", "provider//model"},
		{"whitespace model", "render", "openai/gpt image"},
	} {
		if _, _, err := validateModelMapping(tc.feature, tc.model); err == nil {
			t.Fatalf("%s accepted, want error", tc.name)
		}
	}
	// Providers stay open-ended: new suppliers must not need code changes.
	if _, _, err := validateModelMapping("render", "newvendor/some-model-v9"); err != nil {
		t.Fatalf("unknown provider rejected: %v", err)
	}
	// Vision rides the same mapping store without joining the task feature
	// contract: it is not a submittable task, only a model id.
	if slug, _, err := validateModelMapping("vision", "test-provider/test-model"); err != nil || slug != "vision" {
		t.Fatalf("validate vision = %q, %v", slug, err)
	}
}

func TestProbeGatewayConnectionRejectsURLMetadata(t *testing.T) {
	for _, baseURL := range []string{
		"https://user:secret@example.com",
		"https://example.com/path?token=secret",
		"https://example.com/path#fragment",
	} {
		t.Run(baseURL, func(t *testing.T) {
			result := ProbeGatewayConnection(t.Context(), baseURL, "", "openai", "gpt-4o", "v1/models")
			if result.Reachable || result.Message != "网关地址必须是 http(s) URL" {
				t.Fatalf("ProbeGatewayConnection() = %+v, want URL validation failure", result)
			}
		})
	}
}

func TestModelMappingRoundTrip(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	if _, err := repo.ResolveModel(ctx, "render"); err == nil {
		t.Fatal("ResolveModel() on empty mapping succeeded, want error")
	}
	if err := repo.SaveModelMapping(ctx, "render", "openai/gpt-image-1", 42); err != nil {
		t.Fatalf("SaveModelMapping() error = %v", err)
	}
	got, err := repo.ResolveModel(ctx, "render")
	if err != nil || got != "openai/gpt-image-1" {
		t.Fatalf("ResolveModel() = %q, %v", got, err)
	}
	listed, err := repo.ListModelMappings(ctx)
	if err != nil || listed["render"] != "openai/gpt-image-1" {
		t.Fatalf("ListModelMappings() = %v, %v", listed, err)
	}
	if err := repo.SaveModelMapping(ctx, "render", "oops-no-slash", 42); err == nil {
		t.Fatal("SaveModelMapping() accepted bad model, want error")
	}
}

func TestVisionMappingRoundTrip(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	if _, err := repo.ResolveVisionModel(ctx); err == nil {
		t.Fatal("ResolveVisionModel() on empty mapping succeeded, want error")
	}
	if err := repo.SaveModelMapping(ctx, "vision", "test-provider/test-model", 42); err != nil {
		t.Fatalf("SaveModelMapping() error = %v", err)
	}
	got, err := repo.ResolveVisionModel(ctx)
	if err != nil || got != "test-provider/test-model" {
		t.Fatalf("ResolveVisionModel() = %q, %v", got, err)
	}
	listed, err := repo.ListModelMappings(ctx)
	if err != nil || listed["vision"] != "test-provider/test-model" {
		t.Fatalf("ListModelMappings() = %v, %v", listed, err)
	}
}

func TestSaveCatalogRemovesStaleMappings(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	catalogA := CatalogItem{ID: "openai/a", Name: "A", Provider: "OpenAI"}
	catalogB := CatalogItem{ID: "openai/b", Name: "B", Provider: "OpenAI"}
	if err := repo.SaveCatalog(ctx, []CatalogItem{catalogA, catalogB}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	if err := repo.SaveModelMapping(ctx, "render", "openai/a", 42); err != nil {
		t.Fatalf("SaveModelMapping() error = %v", err)
	}
	if err := repo.SaveModelMapping(ctx, "generation", "openai/b", 42); err != nil {
		t.Fatalf("SaveModelMapping() error = %v", err)
	}
	if err := repo.SaveModelMapping(ctx, "vision", "openai/a", 42); err != nil {
		t.Fatalf("SaveModelMapping() error = %v", err)
	}

	// 删除模型 A：指向它的 render + vision 映射一并移除，指向 B 的保留。
	if err := repo.SaveCatalog(ctx, []CatalogItem{catalogB}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	mappings, err := repo.ListModelMappings(ctx)
	if err != nil {
		t.Fatalf("ListModelMappings() error = %v", err)
	}
	if len(mappings) != 1 || mappings["generation"] != "openai/b" {
		t.Fatalf("ListModelMappings() = %v, want only generation->openai/b", mappings)
	}
	items, err := repo.ListCatalog(ctx)
	if err != nil || len(items) != 1 || items[0].ID != "openai/b" {
		t.Fatalf("ListCatalog() = %v, %v", items, err)
	}
}

func TestSaveCatalogWithoutRemovalsKeepsMappings(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	if err := repo.SaveCatalog(ctx, []CatalogItem{{ID: "openai/a", Name: "A", Provider: "OpenAI"}}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	if err := repo.SaveModelMapping(ctx, "render", "openai/a", 42); err != nil {
		t.Fatalf("SaveModelMapping() error = %v", err)
	}
	// 无删除的保存不得动映射。
	if err := repo.SaveCatalog(ctx, []CatalogItem{{ID: "openai/a", Name: "A2", Provider: "OpenAI"}}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	mappings, err := repo.ListModelMappings(ctx)
	if err != nil || mappings["render"] != "openai/a" {
		t.Fatalf("ListModelMappings() = %v, %v", mappings, err)
	}
}

func TestSaveCatalogRacesMappingSaveWithoutPartialState(t *testing.T) {
	// Concurrent catalog pruning and mapping updates must preserve a mapping
	// to a model that remains in the catalog regardless of lock acquisition order.
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	catalogA := CatalogItem{ID: "openai/a", Name: "A", Provider: "OpenAI"}
	catalogB := CatalogItem{ID: "openai/b", Name: "B", Provider: "OpenAI"}
	for i := 0; i < 20; i++ {
		if err := repo.SaveCatalog(ctx, []CatalogItem{catalogA, catalogB}, 42); err != nil {
			t.Fatalf("iter %d: SaveCatalog() error = %v", i, err)
		}
		if err := repo.SaveModelMapping(ctx, "render", "openai/a", 42); err != nil {
			t.Fatalf("iter %d: SaveModelMapping() error = %v", i, err)
		}
		if err := repo.SaveModelMapping(ctx, "generation", "openai/b", 42); err != nil {
			t.Fatalf("iter %d: SaveModelMapping() error = %v", i, err)
		}

		var wg sync.WaitGroup
		errs := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := repo.SaveCatalog(ctx, []CatalogItem{catalogB}, 42); err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			if err := repo.SaveModelMapping(ctx, "render", "openai/b", 42); err != nil {
				errs <- err
			}
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("iter %d: concurrent write error = %v", i, err)
		}

		items, err := repo.ListCatalog(ctx)
		if err != nil || len(items) != 1 || items[0].ID != "openai/b" {
			t.Fatalf("iter %d: ListCatalog() = %v, %v (catalog must save whole)", i, items, err)
		}
		mappings, err := repo.ListModelMappings(ctx)
		if err != nil {
			t.Fatalf("iter %d: ListModelMappings() error = %v", i, err)
		}
		if mappings["generation"] != "openai/b" {
			t.Fatalf("iter %d: generation mapping lost: %v", i, mappings)
		}
		if mappings["render"] != "openai/b" {
			t.Fatalf("iter %d: render mapping to retained model was lost: %v", i, mappings)
		}
	}
}
