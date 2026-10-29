package application

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func stubControlPlaneSlugs(t *testing.T, slugs []string, err error) {
	t.Helper()
	previous := controlPlaneSlugsFetcher
	controlPlaneSlugsFetcher = func(ctx context.Context, gateway ResolvedGateway) ([]string, error) {
		return slugs, err
	}
	t.Cleanup(func() { controlPlaneSlugsFetcher = previous })
	// 控制面缓存按 token 哈希隔离：清全局避免跨用例串味。
	configuredSlugsCache.Lock()
	configuredSlugsCache.byToken = make(map[string]configuredSlugsEntry)
	configuredSlugsCache.Unlock()
}

func stubCommunityCatalog(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previousURL := communityCatalogURL
	communityCatalogURL = server.URL
	t.Cleanup(func() { communityCatalogURL = previousURL })
	communityCache.Lock()
	communityCache.entries = nil
	communityCache.fetchedAt = time.Time{}
	communityCache.Unlock()
}

const detectTestGatewayURL = "https://gateway.ai.cloudflare.com/v1/abcdef0123456789abcdef0123456789/detect-test"

type detectOutboundFunc func(context.Context, *http.Request, safehttp.RequestOptions) (*http.Response, error)

func (f detectOutboundFunc) Do(ctx context.Context, req *http.Request, opts safehttp.RequestOptions) (*http.Response, error) {
	return f(ctx, req, opts)
}

// Keep the persisted gateway configuration valid; stub only its outbound
// requests while catalogs continue through the standard safehttp client.
func stubDetectGateway(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	previous := detectOutbound
	detectOutbound = detectOutboundFunc(func(ctx context.Context, req *http.Request, opts safehttp.RequestOptions) (*http.Response, error) {
		if req.URL.Hostname() != "gateway.ai.cloudflare.com" {
			return previous.Do(ctx, req, opts)
		}
		if req.URL.String() != detectTestGatewayURL+"/workers-ai/models/search" {
			t.Fatalf("unexpected gateway probe: %s", req.URL)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})
	t.Cleanup(func() { detectOutbound = previous })
}

func newDetectTestRepo(t *testing.T, token string) *ConfigRepository {
	t.Helper()
	stubDetectGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := context.Background()
	if err := repo.Update(ctx, UpdateInput{
		BaseURL: detectTestGatewayURL,
		Token:   &token,
	}, 42); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	return repo
}

// stubModelsDevCatalog overrides the models.dev upstream for detection
// enrichment: policy allowlists the stub host, the URL points at it, and
// the shared facts cache resets around the test. Reuses the reference
// seam's fetch+cache (no second client); handler controls the four states
// (payload vs 500).
func stubModelsDevCatalog(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previousURL := modelsDevCatalogURL
	modelsDevCatalogURL = server.URL
	t.Cleanup(func() { modelsDevCatalogURL = previousURL })
	resetModelsReferenceCache()
	t.Cleanup(resetModelsReferenceCache)
}

func stubModelsDevCatalogFailure(t *testing.T) {
	t.Helper()
	stubModelsDevCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
}

func TestDetectModelsControlPlaneFailureFailsClosed(t *testing.T) {
	// 网关不可达且 control plane 失败：fail-closed，直接返回 error，
	// 不再回退全量连通性试探。
	stubControlPlaneSlugs(t, nil, errors.New("control plane unavailable"))
	repo := newDetectTestRepo(t, "detect-fail-closed-token")
	if _, err := repo.DetectModels(context.Background(), ""); err == nil {
		t.Fatal("DetectModels() = nil error, want control-plane failure")
	}
}

func TestDetectModelsEmptyConfiguredStaysSilent(t *testing.T) {
	// 已配置列表为空：静默返回零值，且不抓取社区目录（URL 指向黑洞也能成功）。
	stubControlPlaneSlugs(t, []string{}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	repo := newDetectTestRepo(t, "detect-empty-configured-token")
	detection, err := repo.DetectModels(context.Background(), "")
	if err != nil {
		t.Fatalf("DetectModels() error = %v", err)
	}
	if len(detection.Models) != 0 {
		t.Fatalf("Models = %+v, want empty", detection.Models)
	}
	if detection.ConfiguredProviders != 0 {
		t.Errorf("ConfiguredProviders = %d, want 0", detection.ConfiguredProviders)
	}
}

func TestDetectModelsCommunityIntersection(t *testing.T) {
	// 交集语义：control-plane 名单 ∩ 社区目录；ID 一律原生 key（Vertex 不重写）；
	// deprecated/坏条目跳过；未配置 provider 的条目不出现。
	stubControlPlaneSlugs(t, []string{"openai", "fal", "workers-ai", "google-vertex-ai", "cartesia"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"gpt-4o": {"litellm_provider": "openai", "mode": "chat", "max_input_tokens": 131072, "input_cost_per_token": 0.0000025, "output_cost_per_token": 0.00001, "cache_read_input_token_cost": 0.00000125, "supports_prompt_caching": true},
			"claude-sonnet-4-5": {"litellm_provider": "anthropic", "mode": "chat"},
			"vertex_ai/gemini-2.5-flash": {"litellm_provider": "vertex_ai", "mode": "chat", "max_input_tokens": 1048576},
			"gemini-2.0-flash": {"litellm_provider": "vertex_ai-language-models", "mode": "chat"},
			"vertex_ai/old-model": {"litellm_provider": "vertex_ai", "deprecation_date": "2020-01-01"},
			"fal_ai/fal-ai/flux-pro/v1.1": {"litellm_provider": "fal_ai", "mode": "image_generation"},
			"cloudflare/@cf/meta/llama-3.1-8b-instruct-fp8": {"litellm_provider": "cloudflare", "mode": "chat", "max_input_tokens": 32000},
			"vertex_ai/broken": [1, 2, 3],
			"openai/gpt-image-1": {"litellm_provider": "openai", "mode": "image_generation"}
		}`))
	})
	// models.dev 挂：本用例只断言 LiteLLM 交集，降级桩保证 hermetic。
	stubModelsDevCatalogFailure(t)
	repo := newDetectTestRepo(t, "detect-intersection-token")
	detection, err := repo.DetectModels(context.Background(), "")
	if err != nil {
		t.Fatalf("DetectModels() error = %v", err)
	}
	if detection.ConfiguredProviders != 5 {
		t.Errorf("ConfiguredProviders = %d, want 5", detection.ConfiguredProviders)
	}
	byID := make(map[string]DetectedModel, len(detection.Models))
	for _, m := range detection.Models {
		byID[m.ID] = m
	}
	// 7 条命中：openai 2（bare+前缀）+ vertex 2（前缀+bare 变体）+ fal 1 + workers 1 = 6？逐项断言。
	wantIDs := []string{
		"gpt-4o",
		"openai/gpt-image-1",
		"vertex_ai/gemini-2.5-flash",
		"gemini-2.0-flash",
		"fal_ai/fal-ai/flux-pro/v1.1",
		"cloudflare/@cf/meta/llama-3.1-8b-instruct-fp8",
	}
	if len(detection.Models) != len(wantIDs) {
		t.Fatalf("Models = %+v, want %d entries", detection.Models, len(wantIDs))
	}
	for _, id := range wantIDs {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing model %q in %+v", id, detection.Models)
		}
	}
	for _, absent := range []string{"claude-sonnet-4-5", "vertex_ai/old-model", "vertex_ai/broken"} {
		if _, ok := byID[absent]; ok {
			t.Errorf("unexpected model %q present (unconfigured/deprecated/broken must be excluded)", absent)
		}
	}
	// Q4：Vertex 不再重写 compat 全串，原生 key 原样存储。
	if got := byID["vertex_ai/gemini-2.5-flash"]; got.Provider != "Google Vertex AI" {
		t.Errorf("vertex provider = %q, want Google Vertex AI", got.Provider)
	}
	if got := byID["gpt-4o"]; got.Provider != "OpenAI" || got.ContextOrResolution != "128K" {
		t.Errorf("openai entry = %+v, want provider OpenAI + 128K", got)
	} else if !got.SupportsPromptCaching {
		t.Errorf("openai entry = %+v, want supports_prompt_caching true", got)
	}
	if got := byID["fal_ai/fal-ai/flux-pro/v1.1"]; got.Name != "v1.1" || got.Provider != "Fal AI" {
		t.Errorf("fal entry = %+v, want name v1.1 + provider Fal AI", got)
	}
	// 结果按 ID 排序。
	for i := 1; i < len(detection.Models); i++ {
		if detection.Models[i-1].ID > detection.Models[i].ID {
			t.Fatalf("models not sorted by ID: %+v", detection.Models)
		}
	}
}

func TestDetectModelsUnknownFilterErrors(t *testing.T) {
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	repo := newDetectTestRepo(t, "detect-unknown-filter-token")
	if _, err := repo.DetectModels(context.Background(), "no-such-provider"); err == nil {
		t.Fatal("DetectModels(filter) = nil error, want unknown provider error")
	}
}

func TestDetectModelsCommunityFailureSurfacesError(t *testing.T) {
	// Q6/Q9：社区抓取失败显式上浮，不静默空列表。
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	repo := newDetectTestRepo(t, "detect-community-failure-token")
	_, err := repo.DetectModels(context.Background(), "")
	if err == nil {
		t.Fatal("DetectModels() = nil error, want community fetch failure")
	}
}

func TestDetectModelsProviderFilterScopesCatalog(t *testing.T) {
	// provider 过滤：只返回该 provider 的社区切片，configured 计 1。
	stubControlPlaneSlugs(t, []string{"openai", "fal"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"gpt-4o": {"litellm_provider": "openai", "max_input_tokens": 128000},
			"fal_ai/fal-ai/flux-pro/v1.1": {"litellm_provider": "fal_ai"}
		}`))
	})
	stubModelsDevCatalogFailure(t)
	repo := newDetectTestRepo(t, "detect-filter-scope-token")
	detection, err := repo.DetectModels(context.Background(), "fal")
	if err != nil {
		t.Fatalf("DetectModels() error = %v", err)
	}
	if detection.ConfiguredProviders != 1 {
		t.Errorf("ConfiguredProviders = %d, want 1", detection.ConfiguredProviders)
	}
	if len(detection.Models) != 1 || detection.Models[0].ID != "fal_ai/fal-ai/flux-pro/v1.1" {
		t.Fatalf("Models = %+v, want exactly the fal entry", detection.Models)
	}
}

func TestFormatContextLength(t *testing.T) {
	tests := []struct {
		tokens int
		want   string
	}{
		{0, ""},
		{-1, ""},
		{512, "512"},
		{4096, "4K"},
		{8192, "8K"},
		{32768, "32K"},
		{131072, "128K"},
		{1000000, "1M"},
		{2000000, "2M"},
	}

	for _, tt := range tests {
		got := formatContextLength(tt.tokens)
		if got != tt.want {
			t.Errorf("formatContextLength(%d) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestResolveControlPlaneIdentityPrefersStoredIDs(t *testing.T) {
	// 回归：自定义 egress BaseURL 无可解析身份时，必须用存储的 account/gateway ID，
	// 而不是直接报 missing identity（此前仅从 URL 正则反解）。
	accountID, gatewayID, err := resolveControlPlaneIdentity(ResolvedGateway{
		BaseURL:   "https://egress.internal.example.com/ai",
		AccountID: "abc123abc123abc123abc123abc123ab",
		GatewayID: "prod",
		Token:     "tok",
	})
	if err != nil {
		t.Fatalf("resolveControlPlaneIdentity() error = %v", err)
	}
	if accountID != "abc123abc123abc123abc123abc123ab" || gatewayID != "prod" {
		t.Errorf("identity = (%q, %q), want stored IDs", accountID, gatewayID)
	}

	// 存储为空时回退 URL 解析。
	accountID, gatewayID, err = resolveControlPlaneIdentity(ResolvedGateway{
		BaseURL: "https://gateway.ai.cloudflare.com/v1/acc123/gw456",
		Token:   "tok",
	})
	if err != nil {
		t.Fatalf("URL fallback error = %v", err)
	}
	if accountID != "acc123" || gatewayID != "gw456" {
		t.Errorf("identity = (%q, %q), want URL-parsed", accountID, gatewayID)
	}

	// 两边都没有：可操作的中文错误，而不是 missing account or gateway identity。
	if _, _, err := resolveControlPlaneIdentity(ResolvedGateway{
		BaseURL: "https://egress.internal.example.com/ai",
		Token:   "tok",
	}); err == nil {
		t.Fatal("expected identity error, got nil")
	} else if got := err.Error(); !strings.Contains(got, "网关设置") {
		t.Errorf("error = %q, want actionable guidance", got)
	}
}

func TestWorkersAIAvailableSignatures(t *testing.T) {
	restorePolicy := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	defer restorePolicy()
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"upstream signature 9106", http.StatusBadRequest, `{"success":false,"errors":[{"code":9106}]}`, true},
		{"upstream signature 7001 spaced", http.StatusBadRequest, `{"code": 7001}`, true},
		{"plain 200", http.StatusOK, `{}`, true},
		{"401 means unavailable", http.StatusUnauthorized, `{}`, false},
		{"code 2008 means unconfigured", http.StatusBadRequest, `{"code":2008}`, false},
		{"unknown 400 means unavailable", http.StatusBadRequest, `{"success":false}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			got, err := workersAIAvailable(context.Background(), ResolvedGateway{
				BaseURL: server.URL,
				Token:   "tok",
			})
			if err != nil {
				t.Fatalf("workersAIAvailable() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("workersAIAvailable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectModelsIncludesWorkersAIByAvailability(t *testing.T) {
	// Workers AI 无 key 不进 control-plane 名单：网关上游存活即纳入，
	// 不要求名单点名。
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"gpt-4o": {"litellm_provider": "openai"},
			"cloudflare/@cf/meta/llama-3.1-8b-instruct-fp8": {"litellm_provider": "cloudflare"}
		}`))
	})
	stubModelsDevCatalogFailure(t)

	repo := newDetectTestRepo(t, "detect-workers-availability-token")
	stubDetectGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9106}]}`))
	})
	ctx := context.Background()
	detection, err := repo.DetectModels(ctx, "")
	if err != nil {
		t.Fatalf("DetectModels() error = %v", err)
	}
	ids := make(map[string]bool, len(detection.Models))
	for _, m := range detection.Models {
		ids[m.ID] = true
	}
	if !ids["gpt-4o"] || !ids["cloudflare/@cf/meta/llama-3.1-8b-instruct-fp8"] {
		t.Fatalf("Models = %+v, want openai + workers-ai by availability", detection.Models)
	}
}

func TestExtractGatewayIdentity(t *testing.T) {
	tests := []struct {
		url      string
		wantAcc  string
		wantGate string
	}{
		{"https://gateway.ai.cloudflare.com/v1/acc123/gw456", "acc123", "gw456"},
		{"https://gateway.ai.cloudflare.com/v1/acc123/gw456/openai/models", "acc123", "gw456"},
		{"https://custom.gateway.com/v1/acc/gw", "", ""},
	}

	for _, tt := range tests {
		acc, gw := extractGatewayIdentity(tt.url)
		if acc != tt.wantAcc || gw != tt.wantGate {
			t.Errorf("extractGatewayIdentity(%q) = (%q, %q), want (%q, %q)", tt.url, acc, gw, tt.wantAcc, tt.wantGate)
		}
	}
}

func modelsDevUnionPayload() string {
	return `{"openai":{"id":"openai","name":"OpenAI","models":{
		"openai/gpt-4o":{"id":"openai/gpt-4o","name":"GPT-4o","knowledge":"2024-05-30",
			"reasoning":true,"tool_call":true,"temperature":false,
			"modalities":{"input":["text","image"],"output":["text"]},
			"cost":{"input":2.5,"output":10},"limit":{"context":128000}},
		"openai/gpt-4o-mini":{"id":"openai/gpt-4o-mini","name":"GPT-4o mini","knowledge":"2023-10-01",
			"modalities":{"input":["text"],"output":["text"]},
			"cost":{"input":1,"output":2},"limit":{"context":128000}},
		"openai/unnamed-only":{"id":"openai/unnamed-only","cost":{"input":1,"output":2}}
	}}}`
}

func TestDetectModelsDualSourceUnion(t *testing.T) {
	// 双源 ok：同 key 合并（models.dev 赢 metadata，社区守 verbatim 与独有字段），
	// models.dev-only 纳入，LiteLLM-only 保留。
	stubControlPlaneSlugs(t, []string{"openai", "deepseek"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"openai/gpt-4o": {"litellm_provider": "openai", "mode": "chat", "max_input_tokens": 131072, "supports_prompt_caching": true},
			"deepseek-chat": {"litellm_provider": "deepseek", "mode": "chat", "max_input_tokens": 64000}
		}`))
	})
	stubModelsDevCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(modelsDevUnionPayload()))
	})
	repo := newDetectTestRepo(t, "detect-dual-source-token")
	detection, err := repo.DetectModels(context.Background(), "")
	if err != nil {
		t.Fatalf("DetectModels() error = %v", err)
	}
	if detection.ConfiguredProviders != 2 {
		t.Errorf("ConfiguredProviders = %d, want 2", detection.ConfiguredProviders)
	}
	byID := make(map[string]DetectedModel, len(detection.Models))
	for _, m := range detection.Models {
		byID[m.ID] = m
	}
	if len(detection.Models) != 4 {
		t.Fatalf("Models = %+v, want 4 entries", detection.Models)
	}

	merged := byID["openai/gpt-4o"]
	if merged.Name != "gpt-4o" || merged.Provider != "OpenAI" {
		t.Errorf("merged identity = %+v, want verbatim name gpt-4o + OpenAI", merged)
	}
	if merged.Mode != "chat" || !merged.SupportsPromptCaching {
		t.Errorf("merged = %+v, want LiteLLM mode chat + prompt caching kept", merged)
	}
	if merged.ContextLimit != 128000 || merged.ContextOrResolution != "125K" {
		t.Errorf("merged = %+v, want models.dev limit 128000/125K winning over 131072", merged)
	}
	if merged.Modalities == nil || len(merged.Modalities.Input) != 2 || merged.Modalities.Input[1] != "image" {
		t.Errorf("merged modalities = %+v, want input[text image]", merged.Modalities)
	}
	if len(merged.Capabilities) != 2 || merged.Capabilities[0] != "reasoning" || merged.Capabilities[1] != "tool_call" {
		t.Errorf("merged capabilities = %+v, want [reasoning tool_call] (true-only, sorted)", merged.Capabilities)
	}
	if merged.Description == "" || !merged.DescriptionEnriched {
		t.Errorf("merged = %+v, want auto description + enriched=true", merged)
	} else if !strings.Contains(merged.Description, "图像") || !strings.Contains(merged.Description, "知识截止") {
		t.Errorf("merged description = %q, want modalities + knowledge signals", merged.Description)
	}

	liteOnly := byID["deepseek-chat"]
	if liteOnly.Provider != "DeepSeek" || liteOnly.ContextOrResolution != "63K" {
		t.Errorf("litellm-only = %+v, want DeepSeek + 63K", liteOnly)
	}
	if liteOnly.ContextLimit != 0 || liteOnly.Modalities != nil || len(liteOnly.Capabilities) != 0 {
		t.Errorf("litellm-only = %+v, want no models.dev enrichment", liteOnly)
	}
	if liteOnly.Description != "" || liteOnly.DescriptionEnriched {
		t.Errorf("litellm-only = %+v, want empty description + enriched=false", liteOnly)
	}

	devOnly := byID["openai/gpt-4o-mini"]
	if devOnly.Provider != "OpenAI" || devOnly.Name != "GPT-4o mini" {
		t.Errorf("models.dev-only = %+v, want OpenAI + upstream display name", devOnly)
	}
	if devOnly.Mode != "" || devOnly.ContextLimit != 128000 {
		t.Errorf("models.dev-only = %+v, want empty mode + limit 128000", devOnly)
	}
	if !devOnly.DescriptionEnriched || !strings.Contains(devOnly.Description, "知识截止") {
		t.Errorf("models.dev-only = %+v, want knowledge description + enriched=true", devOnly)
	}

	unnamed := byID["openai/unnamed-only"]
	if unnamed.Name != "unnamed-only" || unnamed.Description != "" || unnamed.DescriptionEnriched {
		t.Errorf("bare fact = %+v, want segment fallback name + no description fill", unnamed)
	}

	for i := 1; i < len(detection.Models); i++ {
		if detection.Models[i-1].ID > detection.Models[i].ID {
			t.Fatalf("models not sorted by ID: %+v", detection.Models)
		}
	}
}

func TestDetectModelsRelayPartitionsExcluded(t *testing.T) {
	// relay 分区（poe/digitalocean/aihubmix）在 registry 无对应 slug，
	// 归属匹配不上已配 target，不会混入。
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"gpt-4o": {"litellm_provider": "openai", "mode": "chat"}}`))
	})
	stubModelsDevCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"openai":{"id":"openai","name":"OpenAI","models":{
				"openai/gpt-4o-mini":{"id":"openai/gpt-4o-mini","name":"GPT-4o mini","cost":{"input":1,"output":2},"limit":{"context":128000}}}},
			"poe":{"id":"poe","name":"Poe","models":{
				"poe/claude-relay":{"id":"poe/claude-relay","name":"Relay","cost":{"input":1,"output":2},"limit":{"context":8000}}}},
			"digitalocean":{"id":"digitalocean","name":"DigitalOcean","models":{
				"digitalocean/llama-relay":{"id":"digitalocean/llama-relay","name":"Relay","cost":{"input":1,"output":2},"limit":{"context":8000}}}},
			"aihubmix":{"id":"aihubmix","name":"AIHubMix","models":{
				"aihubmix/qwen-relay":{"id":"aihubmix/qwen-relay","name":"Relay","cost":{"input":1,"output":2},"limit":{"context":8000}}}}
		}`))
	})
	repo := newDetectTestRepo(t, "detect-relay-excluded-token")
	detection, err := repo.DetectModels(context.Background(), "")
	if err != nil {
		t.Fatalf("DetectModels() error = %v", err)
	}
	byID := make(map[string]DetectedModel, len(detection.Models))
	for _, m := range detection.Models {
		byID[m.ID] = m
	}
	for _, want := range []string{"gpt-4o", "openai/gpt-4o-mini"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("missing model %q in %+v", want, detection.Models)
		}
	}
	for _, relay := range []string{"poe/claude-relay", "digitalocean/llama-relay", "aihubmix/qwen-relay"} {
		if _, ok := byID[relay]; ok {
			t.Errorf("relay model %q leaked into %+v", relay, detection.Models)
		}
	}
}

func TestDetectModelsModelsDevFailureDegrades(t *testing.T) {
	// models.dev 挂 → 降级纯 LiteLLM，不报错。
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"gpt-4o": {"litellm_provider": "openai", "mode": "chat", "max_input_tokens": 131072}}`))
	})
	stubModelsDevCatalogFailure(t)
	repo := newDetectTestRepo(t, "detect-modelsdev-degraded-token")
	detection, err := repo.DetectModels(context.Background(), "")
	if err != nil {
		t.Fatalf("DetectModels() error = %v, want LiteLLM-only degradation", err)
	}
	if len(detection.Models) != 1 || detection.Models[0].ID != "gpt-4o" {
		t.Fatalf("Models = %+v, want pure LiteLLM result", detection.Models)
	}
	got := detection.Models[0]
	if got.ContextLimit != 0 || got.Modalities != nil || got.Description != "" || got.DescriptionEnriched {
		t.Errorf("degraded = %+v, want no models.dev enrichment", got)
	}
	if got.ContextOrResolution != "128K" {
		t.Errorf("degraded context = %q, want LiteLLM 128K", got.ContextOrResolution)
	}
}

func TestDetectModelsBothSourcesFailClosed(t *testing.T) {
	// 双挂 → 保持 fail-closed：显式 error，不静默空列表。
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	stubCommunityCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	stubModelsDevCatalogFailure(t)
	repo := newDetectTestRepo(t, "detect-both-down-token")
	if _, err := repo.DetectModels(context.Background(), ""); err == nil {
		t.Fatal("DetectModels() = nil error, want fail-closed on dual-source failure")
	}
}

func TestModelsDevCapabilities(t *testing.T) {
	truthy, falsy := true, false
	fact := modelsDevFact{reasoning: &truthy, toolCall: &truthy, temperature: &falsy, openWeights: &truthy}
	got := modelsDevCapabilities(fact)
	if len(got) != 3 || got[0] != "open_weights" || got[1] != "reasoning" || got[2] != "tool_call" {
		t.Fatalf("capabilities = %v, want true-only sorted [open_weights reasoning tool_call]", got)
	}
	if len(modelsDevCapabilities(modelsDevFact{})) != 0 {
		t.Fatal("empty fact capabilities must stay empty")
	}
}

func TestDescribeModelsDevFact(t *testing.T) {
	textOnly := &ModelReferenceModalities{Input: []string{"text"}, Output: []string{"text"}}
	if got := describeModelsDevFact(modelsDevFact{modalities: textOnly}); got != "" {
		t.Errorf("text-only = %q, want silent default", got)
	}
	multi := &ModelReferenceModalities{Input: []string{"text", "image"}, Output: []string{"text"}}
	if got := describeModelsDevFact(modelsDevFact{modalities: multi, knowledge: "2024-05-30"}); got != "输入：文本、图像；输出：文本；知识截止 2024-05-30" {
		t.Errorf("multimodal = %q, want modalities + knowledge placeholder", got)
	}
	if got := describeModelsDevFact(modelsDevFact{knowledge: "2024-01-01"}); got != "知识截止 2024-01-01" {
		t.Errorf("knowledge-only = %q", got)
	}
	unknown := &ModelReferenceModalities{Input: []string{"text", "pdf"}, Output: []string{"text"}}
	if got := describeModelsDevFact(modelsDevFact{modalities: unknown}); got != "输入：文本、pdf；输出：文本" {
		t.Errorf("unknown token = %q, want verbatim passthrough", got)
	}
	if got := describeModelsDevFact(modelsDevFact{}); got != "" {
		t.Errorf("empty fact = %q, want empty", got)
	}
}
