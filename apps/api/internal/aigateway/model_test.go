package aigateway

import (
	"sync"
	"testing"

	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

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

func TestVisionMappingRoundTrip(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	// #847 removes the single-value admin mapping save; the isolated vision
	// read path stays. The test seeds the isolated key directly through the
	// settings repository (no admin mapping surface remains).
	settingsRepo := settings.NewSystemConfigRepository(db)
	if _, err := repo.ResolveVisionModel(ctx); err == nil {
		t.Fatal("ResolveVisionModel() on empty mapping succeeded, want error")
	}
	if err := settingsRepo.Set(ctx, visionMappingKey, "test-provider/test-model"); err != nil {
		t.Fatalf("seed vision mapping: %v", err)
	}
	got, err := repo.ResolveVisionModel(ctx)
	if err != nil || got != "test-provider/test-model" {
		t.Fatalf("ResolveVisionModel() = %q, %v", got, err)
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
	settingsRepo := settings.NewSystemConfigRepository(db)
	if err := settingsRepo.Set(ctx, visionMappingKey, "openai/a"); err != nil {
		t.Fatalf("seed vision mapping: %v", err)
	}

	// 删除模型 A：指向它的 vision 映射一并移除。
	if err := repo.SaveCatalog(ctx, []CatalogItem{catalogB}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	if _, err := repo.ResolveVisionModel(ctx); err == nil {
		t.Fatal("ResolveVisionModel() succeeded after its model left the catalog, want error")
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
	settingsRepo := settings.NewSystemConfigRepository(db)
	if err := settingsRepo.Set(ctx, visionMappingKey, "openai/a"); err != nil {
		t.Fatalf("seed vision mapping: %v", err)
	}
	// 无删除的保存不得动映射。
	if err := repo.SaveCatalog(ctx, []CatalogItem{{ID: "openai/a", Name: "A2", Provider: "OpenAI"}}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	got, err := repo.ResolveVisionModel(ctx)
	if err != nil || got != "openai/a" {
		t.Fatalf("ResolveVisionModel() = %q, %v", got, err)
	}
}

func TestSaveCatalogConcurrentSavesStayConsistent(t *testing.T) {
	// Concurrent catalog saves must never interleave into partial state:
	// the catalog saves whole and the vision prune agrees with whichever
	// catalog won, regardless of lock acquisition order.
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	settingsRepo := settings.NewSystemConfigRepository(db)
	repo := NewConfigRepository(settingsRepo)
	ctx := t.Context()

	catalogA := CatalogItem{ID: "openai/a", Name: "A", Provider: "OpenAI"}
	catalogB := CatalogItem{ID: "openai/b", Name: "B", Provider: "OpenAI"}
	for i := 0; i < 20; i++ {
		if err := repo.SaveCatalog(ctx, []CatalogItem{catalogA, catalogB}, 42); err != nil {
			t.Fatalf("iter %d: SaveCatalog() error = %v", i, err)
		}
		if err := settingsRepo.Set(ctx, visionMappingKey, "openai/a"); err != nil {
			t.Fatalf("iter %d: seed vision mapping: %v", i, err)
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
			if err := repo.SaveCatalog(ctx, []CatalogItem{catalogA, catalogB}, 42); err != nil {
				errs <- err
			}
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("iter %d: concurrent write error = %v", i, err)
		}

		items, err := repo.ListCatalog(ctx)
		if err != nil || len(items) == 0 {
			t.Fatalf("iter %d: ListCatalog() = %v, %v (catalog must save whole)", i, items, err)
		}
		ids := make(map[string]struct{}, len(items))
		for _, item := range items {
			ids[item.ID] = struct{}{}
		}
		if len(ids) != len(items) {
			t.Fatalf("iter %d: catalog has duplicates: %v", i, items)
		}
		_, visionErr := repo.ResolveVisionModel(ctx)
		if _, ok := ids["openai/a"]; !ok && visionErr == nil {
			t.Fatalf("iter %d: vision mapping survived its model leaving the catalog", i)
		}
	}
}
