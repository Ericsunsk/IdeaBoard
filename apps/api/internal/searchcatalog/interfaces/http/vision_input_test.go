package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/searchcatalog/domain"

	"github.com/gin-gonic/gin"
)

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
	if err != domain.ErrVisionInput {
		t.Fatalf("Read() error = %v, want domain.ErrVisionInput", err)
	}
}

type visionLibraryAssetReaderStub struct {
	ownerID int64
	assetID int64
	asset   librarydomain.Asset
	err     error
}

func (s *visionLibraryAssetReaderStub) GetAsset(_ context.Context, ownerID, assetID int64) (librarydomain.Asset, error) {
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
	reader := &visionLibraryAssetReaderStub{asset: librarydomain.Asset{OriginalURL: "https://evil.example/image.png"}}

	_, err := (multipartVisionInputReader{materializer: visionReferenceMaterializer{libraryReader: reader}}).Read(request, 42)
	if !errors.Is(err, domain.ErrVisionImageUnavailable) {
		t.Fatalf("Read() error = %v, want domain.ErrVisionImageUnavailable", err)
	}
	if reader.ownerID != 42 || reader.assetID != 9 {
		t.Fatalf("library lookup = owner %d asset %d, want owner 42 asset 9", reader.ownerID, reader.assetID)
	}
}

type visionTestOutboundPolicy struct{ hosts []string }

func (p visionTestOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func TestWriteVisionErrorUsesStableMachineCodes(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{err: domain.ErrVisionInput, wantStatus: http.StatusBadRequest, wantCode: "vision_search_input_invalid"},
		{err: domain.ErrVisionImageInvalid, wantStatus: http.StatusBadRequest, wantCode: "vision_search_image_invalid"},
		{err: domain.ErrVisionImageUnavailable, wantStatus: http.StatusBadRequest, wantCode: "vision_search_image_unavailable"},
		{err: domain.ErrVisionNotConfigured, wantStatus: http.StatusServiceUnavailable, wantCode: "vision_search_not_configured"},
		{err: domain.ErrVisionUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "vision_search_unavailable"},
		{err: domain.ErrVisionSchema, wantStatus: http.StatusBadGateway, wantCode: "vision_search_schema_error"},
		{err: &domain.VisionProviderError{Kind: domain.VisionProviderFailureCredentials, Err: errors.New("unauthorized")}, wantStatus: http.StatusBadGateway, wantCode: "vision_search_credentials"},
		{err: &domain.VisionProviderError{Kind: domain.VisionProviderFailureNetwork, Err: errors.New("connection reset")}, wantStatus: http.StatusBadGateway, wantCode: "vision_search_network"},
		{err: &domain.VisionProviderError{Kind: domain.VisionProviderFailureTimeout, Err: context.DeadlineExceeded}, wantStatus: http.StatusGatewayTimeout, wantCode: "vision_search_timeout"},
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

func validVisionImage() domain.VisionImage {
	return domain.VisionImage{
		Filename:    "fixture.png",
		ContentType: "image/png",
		Data:        []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0, 0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82},
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
	if _, err := parseExternalReferences([]string{`{"provider":"evil","id":"1","preview_url":"https://evil.com/1"}`}); err != domain.ErrVisionInput {
		t.Fatalf("expected domain.ErrVisionInput for evil provider, got %v", err)
	}

	// Malformed JSON
	if _, err := parseExternalReferences([]string{"not-json"}); err != domain.ErrVisionInput {
		t.Fatalf("expected domain.ErrVisionInput for malformed json, got %v", err)
	}

	// Missing fields
	if _, err := parseExternalReferences([]string{`{"provider":"unsplash","id":""}`}); err != domain.ErrVisionInput {
		t.Fatalf("expected domain.ErrVisionInput for missing fields, got %v", err)
	}
}

func TestReadExternalVisionImageDisallowsInvalidHosts(t *testing.T) {
	ref := VisionExternalReference{
		Provider:   "unsplash",
		ID:         "abc",
		PreviewURL: "https://evil.example/image.jpg",
	}
	_, err := readExternalVisionImage(context.Background(), ref)
	if !errors.Is(err, domain.ErrVisionImageUnavailable) {
		t.Fatalf("expected domain.ErrVisionImageUnavailable for evil host, got %v", err)
	}

	// Provider mismatch (unsplash provider with pexels host)
	mismatchRef := VisionExternalReference{
		Provider:   "unsplash",
		ID:         "abc",
		PreviewURL: "https://images.pexels.com/image.jpg",
	}
	_, err = readExternalVisionImage(context.Background(), mismatchRef)
	if !errors.Is(err, domain.ErrVisionImageUnavailable) {
		t.Fatalf("expected domain.ErrVisionImageUnavailable for mismatched provider host, got %v", err)
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
