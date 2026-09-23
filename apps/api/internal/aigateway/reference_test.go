package aigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
)

type stubOutboundPolicy struct {
	hosts []string
	err   error
}

func (s stubOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	if s.err != nil {
		return settings.OutboundPolicySetting{}, s.err
	}
	return settings.OutboundPolicySetting{AllowHosts: s.hosts}, nil
}

func stubModelsDevUpstream(t *testing.T, policy []string, handler http.HandlerFunc) *ConfigRepository {
	t.Helper()
	restore := outbound.ConfigurePolicyProvider(stubOutboundPolicy{hosts: policy})
	t.Cleanup(restore)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previousURL := modelsDevCatalogURL
	modelsDevCatalogURL = server.URL
	t.Cleanup(func() { modelsDevCatalogURL = previousURL })
	resetModelsReferenceCache()
	t.Cleanup(resetModelsReferenceCache)
	return &ConfigRepository{}
}

func modelsDevCatalogPayload(input float64, lastUpdated string) string {
	return fmt.Sprintf(`{"testprov":{"id":"testprov","name":"Test","models":{
		"testprov/testmodel":{"id":"testprov/testmodel","name":"Test Model","last_updated":%q,"cost":{"input":%v,"output":10},"limit":{"context":400000}}
	}}}`, lastUpdated, input)
}

func TestGetModelReferenceAllowlistReject(t *testing.T) {
	// models.dev 不在 egress allowlist：直接降级，不发起任何上游请求。
	repo := stubModelsDevUpstream(t, []string{"example.com"}, func(w http.ResponseWriter, r *http.Request) {
		t.Error("models.dev fetch must not run without allowlist")
	})
	ref, err := repo.GetModelReference(context.Background(), "openai/gpt-5", "", "", false)
	if err != nil {
		t.Fatalf("GetModelReference() error = %v", err)
	}
	if ref.Found || ref.ErrorCode != ModelsReferenceCodeAllowlist {
		t.Fatalf("reference = %+v, want allowlist_missing degradation", ref)
	}
	if ref.Source != ModelsReferenceSource || ref.Note == "" {
		t.Fatalf("reference = %+v, want source models.dev with non-settlement note", ref)
	}
}

func TestGetModelReferenceUpstreamDegrades(t *testing.T) {
	// 上游 4xx：空结果 + upstream_error，不阻塞调用方。
	repo := stubModelsDevUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	ref, err := repo.GetModelReference(context.Background(), "testprov/testmodel", "", "", false)
	if err != nil {
		t.Fatalf("GetModelReference() error = %v", err)
	}
	if ref.Found || ref.ErrorCode != ModelsReferenceCodeUpstream {
		t.Fatalf("reference = %+v, want upstream_error degradation", ref)
	}
}

func TestGetModelReferenceCacheHitRefreshAndExpiry(t *testing.T) {
	var payload atomic.Value
	payload.Store(modelsDevCatalogPayload(1.25, "2026-01-01"))
	var hits atomic.Int64
	repo := stubModelsDevUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload.Load().(string)))
	})
	ctx := context.Background()

	first, err := repo.GetModelReference(ctx, "testprov/testmodel", "", "", false)
	if err != nil || !first.Found {
		t.Fatalf("first = %+v, %v", first, err)
	}
	if first.InputUSDPerMTok != 1.25 || first.OutputUSDPerMTok != 10 || first.ContextLimit != 400000 || first.LastUpdated != "2026-01-01" {
		t.Fatalf("first = %+v, want models.dev facts", first)
	}

	// 上游变更但 TTL 内：缓存命中，不再请求。
	payload.Store(modelsDevCatalogPayload(5, "2026-02-02"))
	second, err := repo.GetModelReference(ctx, "testprov/testmodel", "", "", false)
	if err != nil || !second.Found || second.InputUSDPerMTok != 1.25 {
		t.Fatalf("second = %+v, %v, want cached 1.25", second, err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (cache hit)", got)
	}

	// 手动刷新：绕过缓存，拿到新值。
	third, err := repo.GetModelReference(ctx, "testprov/testmodel", "", "", true)
	if err != nil || !third.Found || third.InputUSDPerMTok != 5 || third.LastUpdated != "2026-02-02" {
		t.Fatalf("third = %+v, %v, want refreshed 5.0", third, err)
	}

	// TTL 过期：下一次自动重抓。
	payload.Store(modelsDevCatalogPayload(7.5, "2026-03-03"))
	modelsReferenceCache.Lock()
	modelsReferenceCache.fetchedAt = time.Now().Add(-modelsReferenceTTL - time.Minute)
	modelsReferenceCache.Unlock()
	fourth, err := repo.GetModelReference(ctx, "testprov/testmodel", "", "", false)
	if err != nil || !fourth.Found || fourth.InputUSDPerMTok != 7.5 {
		t.Fatalf("fourth = %+v, %v, want refetch 7.5 after expiry", fourth, err)
	}
}

func TestGetModelReferenceLookupForms(t *testing.T) {
	repo := stubModelsDevUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(modelsDevCatalogPayload(1.25, "2026-01-01")))
	})
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		modelID  string
		provider string
		model    string
		wantID   string
	}{
		{"exact model_id", "testprov/testmodel", "", "", "testprov/testmodel"},
		{"provider plus model", "", "testprov", "testmodel", "testprov/testmodel"},
		{"bare suffix", "testmodel", "", "", "testmodel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := repo.GetModelReference(ctx, tc.modelID, tc.provider, tc.model, false)
			if err != nil || !ref.Found {
				t.Fatalf("reference = %+v, %v", ref, err)
			}
			if ref.ModelID != tc.wantID || ref.Source != ModelsReferenceSource {
				t.Fatalf("reference = %+v", ref)
			}
		})
	}

	if _, err := repo.GetModelReference(ctx, "", "", "", false); err == nil {
		t.Fatal("empty identity succeeded, want error")
	}
	if ref, err := repo.GetModelReference(ctx, "testprov/no-such-model", "", "", false); err != nil || ref.Found || ref.ErrorCode != ModelsReferenceCodeNotFound {
		t.Fatalf("missing = %+v, %v, want not_found", ref, err)
	}
}

func modelsDevCatalogEnrichedPayload() string {
	return `{"testprov":{"id":"testprov","name":"Test","models":{
		"testprov/enriched":{"id":"testprov/enriched","name":"Enriched Model","description":"上游描述（永不透传）",
			"family":"gpt-test","attachment":true,"reasoning":true,"tool_call":false,"structured_output":true,
			"temperature":false,"knowledge":"2024-05-30","release_date":"2025-08-07","last_updated":"2025-08-07",
			"modalities":{"input":["text","image"],"output":["text"]},"open_weights":false,"status":"deprecated",
			"cost":{"input":2.5,"output":10},"limit":{"context":128000}},
		"testprov/bare":{"id":"testprov/bare","name":"Bare Model",
			"cost":{"input":1,"output":2},"limit":{"context":8000}}
	}}}`
}

func TestGetModelReferenceEnrichment(t *testing.T) {
	// 上游 enrichment 全字段：能力 booleans 上游原名直透，modalities 原样，
	// knowledge/release_date/family/status 透传，description 永不透传。
	repo := stubModelsDevUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(modelsDevCatalogEnrichedPayload()))
	})
	ref, err := repo.GetModelReference(context.Background(), "testprov/enriched", "", "", false)
	if err != nil || !ref.Found {
		t.Fatalf("reference = %+v, %v", ref, err)
	}
	for _, check := range []struct {
		name string
		got  *bool
		want bool
	}{
		{"reasoning", ref.Reasoning, true},
		{"tool_call", ref.ToolCall, false},
		{"temperature", ref.Temperature, false},
		{"attachment", ref.Attachment, true},
		{"structured_output", ref.StructuredOutput, true},
		{"open_weights", ref.OpenWeights, false},
	} {
		if check.got == nil || *check.got != check.want {
			t.Fatalf("%s = %v, want %v (ref=%+v)", check.name, check.got, check.want, ref)
		}
	}
	if ref.Modalities == nil || len(ref.Modalities.Input) != 2 || ref.Modalities.Input[1] != "image" ||
		len(ref.Modalities.Output) != 1 || ref.Modalities.Output[0] != "text" {
		t.Fatalf("modalities = %+v, want input[text image] output[text]", ref.Modalities)
	}
	if ref.Knowledge != "2024-05-30" || ref.ReleaseDate != "2025-08-07" || ref.Family != "gpt-test" || ref.Status != "deprecated" {
		t.Fatalf("enrichment strings = %+v", ref)
	}
	// description 永不透传：序列化后不得出现 description 键。
	raw, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal = %v", err)
	}
	if strings.Contains(string(raw), "description") || strings.Contains(string(raw), "上游描述") {
		t.Fatalf("reference leaks description: %s", raw)
	}
}

func TestGetModelReferenceEnrichmentMissingGraceful(t *testing.T) {
	// 上游缺失 enrichment：指针保持 nil、字符串留空、modalities 为 nil，
	// Found 仍 true，前端一律暂无数据。
	repo := stubModelsDevUpstream(t, []string{modelsDevHost, "127.0.0.1"}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(modelsDevCatalogEnrichedPayload()))
	})
	ref, err := repo.GetModelReference(context.Background(), "testprov/bare", "", "", false)
	if err != nil || !ref.Found {
		t.Fatalf("reference = %+v, %v", ref, err)
	}
	if ref.Reasoning != nil || ref.ToolCall != nil || ref.Temperature != nil ||
		ref.Attachment != nil || ref.StructuredOutput != nil || ref.OpenWeights != nil {
		t.Fatalf("capabilities = %+v, want all nil for missing", ref)
	}
	if ref.Modalities != nil || ref.Knowledge != "" || ref.ReleaseDate != "" || ref.Family != "" || ref.Status != "" {
		t.Fatalf("enrichment = %+v, want empty for missing", ref)
	}
}
