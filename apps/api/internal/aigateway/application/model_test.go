package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

type catalogReadTx struct {
	settings.SystemConfigWriteTx
	raw    string
	vision string
	t      *testing.T
}

func (tx catalogReadTx) Get(_ context.Context, key string) (string, error) {
	if key == catalogConfigKey {
		return tx.raw, nil
	}
	if key == visionMappingKey && tx.vision != "" {
		return tx.vision, nil
	}
	tx.t.Fatalf("unexpected config read %q", key)
	return "", nil
}

func TestStaleMappingKeysCorruptCatalogKeepsMappings(t *testing.T) {
	for _, raw := range []string{
		"{corrupt",
		`[{"id":"openai/a"},{"id":42}]`,
	} {
		t.Run(raw, func(t *testing.T) {
			tx := catalogReadTx{raw: raw, t: t}
			keys, err := staleMappingKeysForRemovedModels(t.Context(), tx, nil)
			if err != nil || len(keys) != 0 {
				t.Fatalf("stale mappings = %v, %v; want no pruning", keys, err)
			}
		})
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

func TestProbeGatewayConnectionRootURLReachableOnAnyStatus(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	var capturedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	result := ProbeGatewayConnection(t.Context(), upstream.URL, "", "", "", "")
	if !result.Reachable || result.StatusCode != http.StatusNotFound {
		t.Fatalf("ProbeGatewayConnection() root = %+v, want reachable 404", result)
	}
	if capturedPath != "/" {
		t.Fatalf("root probe path = %q, want /", capturedPath)
	}
}

func TestProbeGatewayConnectionRootURLTransportFailure(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	result := ProbeGatewayConnection(t.Context(), "http://127.0.0.1:9", "", "", "", "")
	if result.Reachable {
		t.Fatalf("ProbeGatewayConnection() root unreachable = %+v, want reachable:false", result)
	}
	if !strings.Contains(result.Message, errorrender.MsgProbeGatewayConnection5) {
		t.Fatalf("ProbeGatewayConnection() root message = %q, want transport-failure prefix", result.Message)
	}
}

func TestProbeGatewayConnectionPartialRouteRejected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		path     string
	}{
		{"provider without path", "openai", ""},
		{"path without provider", "", "v1/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ProbeGatewayConnection(t.Context(), "https://gateway.example.com", "", tc.provider, "", tc.path)
			if result.Reachable || result.Message != errorrender.MsgProbeGatewayConnection3 {
				t.Fatalf("ProbeGatewayConnection() partial = %+v, want %q", result, errorrender.MsgProbeGatewayConnection3)
			}
		})
	}
}

func TestVisionMappingRoundTrip(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	// The test seeds the isolated key directly through the settings repository.
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

func TestNormalizeCatalogProviderModelIdentity(t *testing.T) {
	items, err := normalizeCatalogItems([]CatalogItem{
		{Provider: "openai", ID: "gpt-image-1", Name: "OpenAI"},
		{Provider: "azureopenai", ID: "gpt-image-1", Name: "Azure"},
	})
	if err != nil || len(items) != 2 {
		t.Fatalf("same native name at different providers = %+v, %v; want two models", items, err)
	}
	if _, err := normalizeCatalogItems([]CatalogItem{
		{Provider: " OpenAI ", ID: "gpt-image-1"},
		{Provider: "openai", ID: "GPT-IMAGE-1"},
	}); err == nil {
		t.Fatal("duplicate provider/model pair accepted")
	}
}

func TestStaleMappingRemovalUsesProviderModelPair(t *testing.T) {
	const raw = `[{"provider":"openai","id":"shared"},{"provider":"azureopenai","id":"shared"}]`
	for _, tc := range []struct {
		name        string
		keep        string
		model       string
		vision      string
		wantRemoval bool
	}{
		{"remove mapped provider", "azureopenai", "shared", "openai/shared", true},
		{"keep mapped provider", "openai", "shared", "openai/shared", false},
		{"keep case variant", "openai", "SHARED", "openai/shared", false},
		{"remove case variant mapping", "azureopenai", "shared", "OpenAI/SHARED", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys, err := staleMappingKeysForRemovedModels(t.Context(),
				catalogReadTx{raw: raw, vision: tc.vision, t: t},
				[]CatalogItem{{Provider: tc.keep, ID: tc.model}})
			if err != nil || (len(keys) > 0) != tc.wantRemoval {
				t.Fatalf("removed mappings = %v, %v; want removal %v", keys, err, tc.wantRemoval)
			}
		})
	}
}

func TestSaveCatalogRemovesStaleMappings(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	catalogA := CatalogItem{ID: "a", Name: "A", Provider: "openai"}
	catalogB := CatalogItem{ID: "b", Name: "B", Provider: "openai"}
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
	if err != nil || len(items) != 1 || items[0].ID != "b" {
		t.Fatalf("ListCatalog() = %v, %v", items, err)
	}
}

func TestSaveCatalogWithoutRemovalsKeepsMappings(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	if err := repo.SaveCatalog(ctx, []CatalogItem{{ID: "a", Name: "A", Provider: "openai"}}, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	settingsRepo := settings.NewSystemConfigRepository(db)
	if err := settingsRepo.Set(ctx, visionMappingKey, "openai/a"); err != nil {
		t.Fatalf("seed vision mapping: %v", err)
	}
	// 无删除的保存不得动映射。
	if err := repo.SaveCatalog(ctx, []CatalogItem{{ID: "a", Name: "A2", Provider: "openai"}}, 42); err != nil {
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

	catalogA := CatalogItem{ID: "a", Name: "A", Provider: "openai"}
	catalogB := CatalogItem{ID: "b", Name: "B", Provider: "openai"}
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
			ids[item.Provider+"/"+item.ID] = struct{}{}
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
