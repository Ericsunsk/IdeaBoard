package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

func TestNormalizeVisionExtractionTrimsAndBoundsQuery(t *testing.T) {
	got, err := normalizeVisionExtraction(domain.VisionExtraction{
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
	if _, err := normalizeVisionExtraction(domain.VisionExtraction{Query: "   "}); err != domain.ErrVisionSchema {
		t.Fatalf("error = %v, want domain.ErrVisionSchema", err)
	}
	if _, err := normalizeVisionExtraction(domain.VisionExtraction{Query: strings.Repeat("x", MaxSearchQueryRunes+1)}); err != domain.ErrVisionSchema {
		t.Fatalf("overlong query error = %v, want domain.ErrVisionSchema", err)
	}
}

func TestValidateVisionImagesRejectsInvalidAndOversizedInputs(t *testing.T) {
	if err := ValidateVisionImages([]domain.VisionImage{{Filename: "x.png", ContentType: "image/png", Data: []byte("not an image")}}); err != domain.ErrVisionImageInvalid {
		t.Fatalf("invalid image error = %v, want domain.ErrVisionImageInvalid", err)
	}
	if err := ValidateVisionImages([]domain.VisionImage{{Filename: "x.png", ContentType: "image/png", Data: make([]byte, domain.VisionMaxBytes+1)}}); err != domain.ErrVisionImageInvalid {
		t.Fatalf("oversized image error = %v, want domain.ErrVisionImageInvalid", err)
	}
}

type visionConfigStub struct {
	config ports.VisionProviderConfig
	err    error
}

func (s visionConfigStub) VisionConfig(context.Context) (ports.VisionProviderConfig, error) {
	return s.config, s.err
}

type visionClientStub struct {
	input      domain.VisionInput
	extraction domain.VisionExtraction
	err        error
}

func (s *visionClientStub) Extract(_ context.Context, _ ports.VisionProviderConfig, input domain.VisionInput) (domain.VisionExtraction, error) {
	s.input = input
	return s.extraction, s.err
}

func TestVisionSearchServiceOrchestratesTypedPorts(t *testing.T) {
	client := &visionClientStub{extraction: domain.VisionExtraction{Query: " warm cafe "}}
	service := newVisionSearchService(visionConfigStub{config: ports.VisionProviderConfig{Model: "model"}}, client)
	firstImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	got, err := service.Extract(context.Background(), domain.VisionInput{Images: []domain.VisionImage{firstImage}, UserText: "  warm   cafe "})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Query != "warm cafe" || client.input.UserText != "warm cafe" {
		t.Fatalf("unexpected extraction/input: %#v / %#v", got, client.input)
	}
}

type visionFakeGateway struct {
	status int
	body   string
	err    error
	got    *ports.VisionGatewayRequest
}

func (f *visionFakeGateway) Do(_ context.Context, req ports.VisionGatewayRequest) (ports.VisionGatewayResponse, error) {
	if f.err != nil {
		return ports.VisionGatewayResponse{}, f.err
	}
	if f.got != nil {
		*f.got = req
	}
	return ports.VisionGatewayResponse{StatusCode: f.status, Body: []byte(f.body)}, nil
}

func TestOpenAICompatibleVisionProviderMapsRequestAndResponse(t *testing.T) {
	secondImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	secondImage.Filename = "second.png"
	var got ports.VisionGatewayRequest
	fake := &visionFakeGateway{status: http.StatusOK, got: &got,
		body: `{"choices":[{"message":{"content":"` + "```json\\n{\\\"query\\\":\\\"warm cafe\\\"}\\n```" + `"}}]}`}

	client := gatewayVisionProviderClient{gateway: fake}
	firstImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	gotExtraction, err := client.Extract(context.Background(), ports.VisionProviderConfig{Model: "model"}, domain.VisionInput{Images: []domain.VisionImage{firstImage, secondImage}, UserText: "warm cafe"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if gotExtraction.Query != "warm cafe" {
		t.Fatalf("response = %#v", gotExtraction)
	}
	if got.Model != "model" {
		t.Fatalf("gateway model = %q, want model", got.Model)
	}
	var payload openAIChatRequest
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if payload.Model != "model" || len(payload.Messages) != 2 || len(payload.Messages[1].Content) != 3 {
		t.Fatalf("request = %#v", payload)
	}
	if payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" || payload.Messages[1].Content[1].Type != "image_url" || payload.Messages[1].Content[2].Type != "image_url" || !strings.HasPrefix(payload.Messages[1].Content[1].ImageURL.URL, "data:image/png;base64,") || !strings.HasPrefix(payload.Messages[1].Content[2].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("image content = %#v", payload.Messages)
	}
}
func TestVisionGatewayClientMissingFailsClosed(t *testing.T) {
	client := gatewayVisionProviderClient{}
	missingImage, err := visionTestFixture()
	if err != nil {
		t.Fatalf("visionTestFixture() error = %v", err)
	}
	if _, err := client.Extract(context.Background(), ports.VisionProviderConfig{Model: "model"}, domain.VisionInput{Images: []domain.VisionImage{missingImage}}); err == nil {
		t.Fatal("Extract() with nil gateway succeeded, want error")
	}
}

func visionTestFixture() (domain.VisionImage, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			shade := uint8(72 + (x+y)/2)
			canvas.SetRGBA(x, y, color.RGBA{R: shade, G: 112, B: 168, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, canvas); err != nil {
		return domain.VisionImage{}, fmt.Errorf("encode vision test fixture: %w", err)
	}
	return domain.VisionImage{Filename: "fixture.png", ContentType: "image/png", Data: data.Bytes()}, nil
}
