package searchcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

func TestNormalizeVisionExtractionTrimsAndBoundsQuery(t *testing.T) {
	got, err := normalizeVisionExtraction(VisionExtraction{
		Query: "  warm   cafe  ",
	})
	if err != nil {
		t.Fatalf("normalizeVisionExtraction() error = %v", err)
	}
	if got.Query != "warm cafe" {
		t.Fatalf("query = %q", got.Query)
	}
}

func TestNormalizeVisionExtractionRejectsEmptyQuery(t *testing.T) {
	if _, err := normalizeVisionExtraction(VisionExtraction{Query: "   "}); err != ErrVisionSchema {
		t.Fatalf("error = %v, want ErrVisionSchema", err)
	}
	if _, err := normalizeVisionExtraction(VisionExtraction{Query: strings.Repeat("x", MaxSearchQueryRunes+1)}); err != ErrVisionSchema {
		t.Fatalf("overlong query error = %v, want ErrVisionSchema", err)
	}
}

func TestValidateVisionImagesRejectsInvalidAndOversizedInputs(t *testing.T) {
	if err := validateVisionImages([]VisionImage{{Filename: "x.png", ContentType: "image/png", Data: []byte("not an image")}}); err != ErrVisionImageInvalid {
		t.Fatalf("invalid image error = %v, want ErrVisionImageInvalid", err)
	}
	if err := validateVisionImages([]VisionImage{{Filename: "x.png", ContentType: "image/png", Data: make([]byte, visionMaxBytes+1)}}); err != ErrVisionImageInvalid {
		t.Fatalf("oversized image error = %v, want ErrVisionImageInvalid", err)
	}
}

func TestLibraryVisionHostAllowlist(t *testing.T) {
	if !isLibraryImageHost("images.unsplash.com") || !isLibraryImageHost("images.pexels.com") {
		t.Fatal("expected curated image hosts to be allowed")
	}
	if isLibraryImageHost("evil.example") {
		t.Fatal("unexpected host allowed")
	}
}

func TestVisionImageAllowHostsIncludesCuratedCDNs(t *testing.T) {
	restore := outbound.ConfigurePolicyProvider(visionTestOutboundPolicy{hosts: []string{"vision.example"}})
	defer restore()

	got := visionImageAllowHosts(context.Background())
	want := []string{"vision.example", "images.unsplash.com", "plus.unsplash.com", "images.pexels.com"}
	if len(got) != len(want) {
		t.Fatalf("allow hosts = %#v, want %#v", got, want)
	}
	for index, host := range want {
		if got[index] != host {
			t.Fatalf("allow hosts[%d] = %q, want %q", index, got[index], host)
		}
	}
}

type visionConfigStub struct {
	config VisionProviderConfig
	err    error
}

func (s visionConfigStub) VisionConfig(context.Context) (VisionProviderConfig, error) {
	return s.config, s.err
}

type visionClientStub struct {
	input      VisionInput
	extraction VisionExtraction
	err        error
}

func (s *visionClientStub) Extract(_ context.Context, _ VisionProviderConfig, input VisionInput) (VisionExtraction, error) {
	s.input = input
	return s.extraction, s.err
}

func TestVisionSearchServiceOrchestratesTypedPorts(t *testing.T) {
	client := &visionClientStub{extraction: VisionExtraction{Query: " warm cafe "}}
	service := newVisionSearchService(visionConfigStub{config: VisionProviderConfig{EndpointURL: "https://vision.example", Model: "model", APIKey: "secret"}}, client, noopSearchMetrics{})
	got, err := service.Extract(context.Background(), VisionInput{Images: []VisionImage{validVisionImage()}, UserText: "  warm   cafe "})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Query != "warm cafe" || client.input.UserText != "warm cafe" {
		t.Fatalf("unexpected extraction/input: %#v / %#v", got, client.input)
	}
}

func TestMultipartVisionInputReaderIsAllOrNothing(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("library_asset_ids[]", "invalid"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	_, err := (multipartVisionInputReader{}).Read(request, 1)
	if err != ErrVisionInput {
		t.Fatalf("Read() error = %v, want ErrVisionInput", err)
	}
}

type visionLibraryAssetReaderStub struct {
	ownerID int64
	assetID int64
	asset   library.Asset
	err     error
}

func (s *visionLibraryAssetReaderStub) GetAsset(_ context.Context, ownerID, assetID int64) (library.Asset, error) {
	s.ownerID = ownerID
	s.assetID = assetID
	return s.asset, s.err
}

func TestMultipartVisionInputReaderResolvesLibraryAssetForOwner(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("library_asset_ids[]", "9"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	reader := &visionLibraryAssetReaderStub{asset: library.Asset{OriginalURL: "https://evil.example/image.png"}}

	_, err := (multipartVisionInputReader{materializer: visionReferenceMaterializer{libraryReader: reader}}).Read(request, 42)
	if !errors.Is(err, ErrVisionImageUnavailable) {
		t.Fatalf("Read() error = %v, want ErrVisionImageUnavailable", err)
	}
	if reader.ownerID != 42 || reader.assetID != 9 {
		t.Fatalf("library lookup = owner %d asset %d, want owner 42 asset 9", reader.ownerID, reader.assetID)
	}
}

func TestOpenAICompatibleVisionProviderMapsRequestAndResponse(t *testing.T) {
	secondImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	secondImage.Filename = "second.png"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		var payload openAIChatRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "model" || len(payload.Messages) != 2 || len(payload.Messages[1].Content) != 3 {
			t.Fatalf("request = %#v", payload)
		}
		if payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" || payload.Messages[1].Content[1].Type != "image_url" || payload.Messages[1].Content[2].Type != "image_url" || !strings.HasPrefix(payload.Messages[1].Content[1].ImageURL.URL, "data:image/png;base64,") || !strings.HasPrefix(payload.Messages[1].Content[2].ImageURL.URL, "data:image/png;base64,") {
			t.Fatalf("image content = %#v", payload.Messages)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "```json\n{\"query\":\"warm cafe\"}\n```"}}}})
	}))
	defer server.Close()
	configureVisionTestOutbound(t, server.URL)

	got, err := (openAICompatibleVisionProviderClient{}).Extract(context.Background(), VisionProviderConfig{EndpointURL: server.URL, Model: "model", APIKey: "secret"}, VisionInput{Images: []VisionImage{validVisionImage(), secondImage}, UserText: "warm cafe"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Query != "warm cafe" {
		t.Fatalf("response = %#v", got)
	}
}

func TestOpenAICompatibleVisionProviderClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		want       FailureCategory
		wantSchema bool
	}{
		{name: "credentials", status: http.StatusUnauthorized, want: FailureCredentials},
		{name: "quota", status: http.StatusTooManyRequests, want: FailureQuota},
		{name: "provider response", status: http.StatusInternalServerError, want: FailureResponse},
		{name: "malformed json", status: http.StatusOK, body: "not-json", want: FailureResponse, wantSchema: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			configureVisionTestOutbound(t, server.URL)

			_, err := (openAICompatibleVisionProviderClient{}).Extract(context.Background(), VisionProviderConfig{EndpointURL: server.URL, Model: "model"}, VisionInput{Images: []VisionImage{validVisionImage()}})
			if err == nil {
				t.Fatal("Extract() error = nil")
			}
			if got := classifyVisionProviderFailure(err); got != test.want {
				t.Fatalf("failure category = %q, want %q", got, test.want)
			}
			if test.wantSchema && !errors.Is(err, ErrVisionSchema) {
				t.Fatalf("error = %v, want ErrVisionSchema", err)
			}
		})
	}
}

type visionTestOutboundPolicy struct{ hosts []string }

func (p visionTestOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func configureVisionTestOutbound(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	restore := outbound.ConfigurePolicyProvider(visionTestOutboundPolicy{hosts: []string{parsed.Host}})
	t.Cleanup(restore)
}

func TestOpenAICompatibleVisionProviderUsesConfiguredOutboundPolicy(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		called = true
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"query\":\"ok\"}"}}]}`))
	}))
	defer server.Close()
	restore := outbound.ConfigurePolicyProvider(nil)
	defer restore()

	_, err := (openAICompatibleVisionProviderClient{}).Extract(context.Background(), VisionProviderConfig{EndpointURL: server.URL, Model: "model"}, VisionInput{Images: []VisionImage{validVisionImage()}})
	if err == nil || called {
		t.Fatalf("Extract() error = %v, called = %v; endpoint must be blocked without policy", err, called)
	}
}

func TestWriteVisionErrorUsesStableMachineCodes(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{err: ErrVisionInput, wantStatus: http.StatusBadRequest, wantCode: "vision_search_input_invalid"},
		{err: ErrVisionImageInvalid, wantStatus: http.StatusBadRequest, wantCode: "vision_search_image_invalid"},
		{err: ErrVisionImageUnavailable, wantStatus: http.StatusBadRequest, wantCode: "vision_search_image_unavailable"},
		{err: ErrVisionNotConfigured, wantStatus: http.StatusServiceUnavailable, wantCode: "vision_search_not_configured"},
		{err: ErrVisionUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "vision_search_unavailable"},
		{err: ErrVisionSchema, wantStatus: http.StatusBadGateway, wantCode: "vision_search_schema_error"},
		{err: &visionProviderError{kind: visionProviderFailureCredentials, cause: errors.New("unauthorized")}, wantStatus: http.StatusBadGateway, wantCode: "vision_search_credentials"},
		{err: &visionProviderError{kind: visionProviderFailureNetwork, cause: errors.New("connection reset")}, wantStatus: http.StatusBadGateway, wantCode: "vision_search_network"},
		{err: &visionProviderError{kind: visionProviderFailureTimeout, cause: context.DeadlineExceeded}, wantStatus: http.StatusGatewayTimeout, wantCode: "vision_search_timeout"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		writeVisionError(ctx, test.err)
		if recorder.Code != test.wantStatus {
			t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
		}
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if payload.Code != test.wantCode {
			t.Fatalf("code = %q, want %q", payload.Code, test.wantCode)
		}
	}
}

func validVisionImage() VisionImage {
	return VisionImage{
		Filename:    "fixture.png",
		ContentType: "image/png",
		Data:        []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0, 0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82},
	}
}

func TestCloudflareEndpointURL(t *testing.T) {
	tests := []struct {
		endpoint string
		model    string
		want     string
	}{
		{
			endpoint: "https://api.cloudflare.com/client/v4/accounts/acc123/ai/v1",
			model:    "@cf/meta/llama-3.2-11b-vision-instruct",
			want:     "https://api.cloudflare.com/client/v4/accounts/acc123/ai/run/@cf/meta/llama-3.2-11b-vision-instruct",
		},
		{
			endpoint: "https://api.cloudflare.com/client/v4/accounts/acc123/ai/run",
			model:    "@cf/meta/llama-3.2-11b-vision-instruct",
			want:     "https://api.cloudflare.com/client/v4/accounts/acc123/ai/run/@cf/meta/llama-3.2-11b-vision-instruct",
		},
		{
			endpoint: "https://api.cloudflare.com/client/v4/accounts/acc123/ai/run/@cf/meta/llama-3.2-11b-vision-instruct",
			model:    "@cf/meta/llama-3.2-11b-vision-instruct",
			want:     "https://api.cloudflare.com/client/v4/accounts/acc123/ai/run/@cf/meta/llama-3.2-11b-vision-instruct",
		},
	}
	for _, test := range tests {
		got, err := cloudflareEndpointURL(test.endpoint, test.model)
		if err != nil {
			t.Fatalf("cloudflareEndpointURL(%q, %q) error = %v", test.endpoint, test.model, err)
		}
		if got != test.want {
			t.Fatalf("cloudflareEndpointURL(%q, %q) = %q, want %q", test.endpoint, test.model, got, test.want)
		}
	}
}

func TestIsCloudflareWorkersAIRequiresExactHostname(t *testing.T) {
	if !isCloudflareWorkersAI("https://api.cloudflare.com/client/v4/accounts/acc/ai/v1") {
		t.Fatal("expected Cloudflare API host to be detected")
	}
	if isCloudflareWorkersAI("https://api.cloudflare.com.evil.example/client/v4/accounts/acc/ai/v1") {
		t.Fatal("unexpected lookalike host detected as Cloudflare")
	}
}

func TestExtractCloudflareWorkersAIVision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		var payload cloudflareAIRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Image) != 70 || payload.Image[0] != 0x89 {
			t.Fatalf("payload image = %#v", payload.Image)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"response": map[string]string{"query": "cafe interior"},
			},
		})
	}))
	defer server.Close()
	configureVisionTestOutbound(t, server.URL)

	extraction, err := extractCloudflareWorkersAIVision(context.Background(), VisionProviderConfig{
		EndpointURL: server.URL + "/client/v4/accounts/acc123/ai/v1",
		Model:       "@cf/meta/llama-3.2-11b-vision-instruct",
		APIKey:      "secret",
	}, VisionInput{
		Images:   []VisionImage{validVisionImage()},
		UserText: "cafe",
	})
	if err != nil {
		t.Fatalf("extractCloudflareWorkersAIVision() error = %v", err)
	}
	if extraction.Query != "cafe interior" {
		t.Fatalf("extraction = %#v, want query 'cafe interior'", extraction)
	}
}

func TestExtractCloudflareWorkersAIVisionCombinesMultipleImages(t *testing.T) {
	firstImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	secondImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload cloudflareAIRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Image) <= len(firstImage.Data) {
			t.Fatalf("payload image length = %d, want a composite larger than the first image", len(payload.Image))
		}
		composite := make([]byte, len(payload.Image))
		for index, value := range payload.Image {
			if value < 0 || value > 255 {
				t.Fatalf("payload image byte %d = %d", index, value)
			}
			composite[index] = byte(value)
		}
		decoded, _, err := image.Decode(bytes.NewReader(composite))
		if err != nil {
			t.Fatalf("decode composite image: %v", err)
		}
		if decoded.Bounds().Dx() <= 64 {
			t.Fatalf("composite width = %d, want multiple reference tiles", decoded.Bounds().Dx())
		}
		if !strings.Contains(payload.Prompt, "contact sheet") || !strings.Contains(payload.Prompt, "distinctive") || !strings.Contains(payload.Prompt, "searchable traits") || !strings.Contains(payload.Prompt, "JSON only") {
			t.Fatalf("prompt does not describe multi-reference synthesis: %q", payload.Prompt)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"response": map[string]string{"query": "cafe interior"},
			},
		})
	}))
	defer server.Close()
	configureVisionTestOutbound(t, server.URL)

	extraction, err := extractCloudflareWorkersAIVision(context.Background(), VisionProviderConfig{
		EndpointURL: server.URL + "/client/v4/accounts/acc123/ai/v1",
		Model:       "@cf/meta/llama-3.2-11b-vision-instruct",
		APIKey:      "secret",
	}, VisionInput{
		Images:   []VisionImage{firstImage, secondImage},
		UserText: "cafe",
	})
	if err != nil {
		t.Fatalf("extractCloudflareWorkersAIVision() error = %v", err)
	}
	if extraction.Query != "cafe interior" {
		t.Fatalf("extraction = %#v, want query 'cafe interior'", extraction)
	}
}

func TestParseExternalReferences(t *testing.T) {
	validRaw := []string{
		`{"provider":"unsplash","id":"abc-1","preview_url":"https://images.unsplash.com/photo-1"}`,
		`{"provider":"pexels","id":"def-2","preview_url":"https://images.pexels.com/photo-2"}`,
		`{"provider":"unsplash","id":"abc-1","preview_url":"https://images.unsplash.com/photo-1"}`, // duplicate
	}
	refs, err := parseExternalReferences(validRaw)
	if err != nil {
		t.Fatalf("parseExternalReferences() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2", len(refs))
	}
	if refs[0].Provider != "unsplash" || refs[0].ID != "abc-1" || refs[1].Provider != "pexels" || refs[1].ID != "def-2" {
		t.Fatalf("unexpected parsed refs: %#v", refs)
	}

	// Invalid provider
	if _, err := parseExternalReferences([]string{`{"provider":"evil","id":"1","preview_url":"https://evil.com/1"}`}); err != ErrVisionInput {
		t.Fatalf("expected ErrVisionInput for evil provider, got %v", err)
	}

	// Malformed JSON
	if _, err := parseExternalReferences([]string{"not-json"}); err != ErrVisionInput {
		t.Fatalf("expected ErrVisionInput for malformed json, got %v", err)
	}

	// Missing fields
	if _, err := parseExternalReferences([]string{`{"provider":"unsplash","id":""}`}); err != ErrVisionInput {
		t.Fatalf("expected ErrVisionInput for missing fields, got %v", err)
	}
}

func TestReadExternalVisionImageDisallowsInvalidHosts(t *testing.T) {
	ref := VisionExternalReference{
		Provider:   "unsplash",
		ID:         "abc",
		PreviewURL: "https://evil.example/image.jpg",
	}
	_, err := readExternalVisionImage(context.Background(), ref)
	if !errors.Is(err, ErrVisionImageUnavailable) {
		t.Fatalf("expected ErrVisionImageUnavailable for evil host, got %v", err)
	}

	// Provider mismatch (unsplash provider with pexels host)
	mismatchRef := VisionExternalReference{
		Provider:   "unsplash",
		ID:         "abc",
		PreviewURL: "https://images.pexels.com/image.jpg",
	}
	_, err = readExternalVisionImage(context.Background(), mismatchRef)
	if !errors.Is(err, ErrVisionImageUnavailable) {
		t.Fatalf("expected ErrVisionImageUnavailable for mismatched provider host, got %v", err)
	}
}

func TestReadExternalVisionImageMaterializesAllowedCDN(t *testing.T) {
	called := false
	previous := visionImageRequest
	visionImageRequest = func(_ context.Context, request *http.Request) (*http.Response, error) {
		called = true
		if request.URL.Hostname() != "images.unsplash.com" {
			t.Fatalf("request host = %q, want images.unsplash.com", request.URL.Hostname())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png; charset=utf-8"}},
			Body:       io.NopCloser(bytes.NewReader(validVisionImage().Data)),
		}, nil
	}
	t.Cleanup(func() { visionImageRequest = previous })

	image, err := readExternalVisionImage(context.Background(), VisionExternalReference{
		Provider:   "unsplash",
		ID:         "abc",
		PreviewURL: "https://images.unsplash.com/photo-abc",
	})
	if err != nil {
		t.Fatalf("readExternalVisionImage() error = %v", err)
	}
	if !called || image.ContentType != "image/png" || !bytes.Equal(image.Data, validVisionImage().Data) {
		t.Fatalf("materialized image = %#v, called = %v", image, called)
	}
}

func TestMultipartVisionInputReaderWithExternalReferences(t *testing.T) {
	previous := visionImageRequest
	visionImageRequest = func(_ context.Context, request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() != "images.unsplash.com" {
			t.Fatalf("request host = %q, want images.unsplash.com", request.URL.Hostname())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(validVisionImage().Data)),
		}, nil
	}
	t.Cleanup(func() { visionImageRequest = previous })

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	refJSON := `{"provider":"unsplash","id":"p1","preview_url":"https://images.unsplash.com/p1.png"}`
	if err := writer.WriteField("external_references[]", refJSON); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("user_text", "warm aesthetic"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	input, err := (multipartVisionInputReader{}).Read(request, 1)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(input.Images) != 1 || input.UserText != "warm aesthetic" {
		t.Fatalf("input = %#v", input)
	}
}
