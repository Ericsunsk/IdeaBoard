package searchcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestNormalizeVisionExtractionBoundsAndDeduplicates(t *testing.T) {
	got, err := normalizeVisionExtraction(VisionExtraction{
		Query:          "  warm   cafe  ",
		Palette:        []string{"#D97706", "#d97706", "invalid", "#78350F"},
		AtmosphereTags: []string{" Walnut Wood ", "Walnut Wood", "Studio Lighting", "Night", "Ignored"},
	})
	if err != nil {
		t.Fatalf("normalizeVisionExtraction() error = %v", err)
	}
	if got.Query != "warm cafe" {
		t.Fatalf("query = %q", got.Query)
	}
	if len(got.Palette) != 2 || got.Palette[0] != "#d97706" || got.Palette[1] != "#78350f" {
		t.Fatalf("palette = %#v", got.Palette)
	}
	if len(got.AtmosphereTags) != 3 || got.AtmosphereTags[0] != "Walnut Wood" {
		t.Fatalf("atmosphere_tags = %#v", got.AtmosphereTags)
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

	_, err := (multipartVisionInputReader{libraryReader: reader}).Read(request, 42)
	if !errors.Is(err, ErrVisionImageUnavailable) {
		t.Fatalf("Read() error = %v, want ErrVisionImageUnavailable", err)
	}
	if reader.ownerID != 42 || reader.assetID != 9 {
		t.Fatalf("library lookup = owner %d asset %d, want owner 42 asset 9", reader.ownerID, reader.assetID)
	}
}

func TestMultipartVisionProviderClientMapsRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm() error = %v", err)
		}
		if request.FormValue("model") != "model" || request.FormValue("user_text") != "warm cafe" {
			t.Errorf("form values = model=%q text=%q", request.FormValue("model"), request.FormValue("user_text"))
		}
		response := VisionExtraction{Query: "warm cafe", Palette: []string{"#ffffff"}}
		_ = json.NewEncoder(writer).Encode(response)
	}))
	defer server.Close()
	configureVisionTestOutbound(t, server.URL)

	got, err := (multipartVisionProviderClient{}).Extract(context.Background(), VisionProviderConfig{EndpointURL: server.URL, Model: "model", APIKey: "secret"}, VisionInput{Images: []VisionImage{validVisionImage()}, UserText: "warm cafe"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Query != "warm cafe" || len(got.Palette) != 1 {
		t.Fatalf("response = %#v", got)
	}
}

func TestMultipartVisionProviderClientClassifiesProviderFailures(t *testing.T) {
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

			_, err := (multipartVisionProviderClient{}).Extract(context.Background(), VisionProviderConfig{EndpointURL: server.URL, Model: "model"}, VisionInput{Images: []VisionImage{validVisionImage()}})
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

func TestMultipartVisionProviderClientUsesConfiguredOutboundPolicy(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		called = true
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"query":"ok"}`))
	}))
	defer server.Close()
	restore := outbound.ConfigurePolicyProvider(nil)
	defer restore()

	_, err := (multipartVisionProviderClient{}).Extract(context.Background(), VisionProviderConfig{EndpointURL: server.URL, Model: "model"}, VisionInput{Images: []VisionImage{validVisionImage()}})
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
