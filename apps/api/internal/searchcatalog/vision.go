package searchcatalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
)

const (
	visionSearchTimeout = 30 * time.Second
	visionMaxImages     = 4
	visionMaxBytes      = 10 << 20
)

var (
	ErrVisionUnavailable      = errors.New("vision search provider is unavailable")
	ErrVisionNotConfigured    = errors.New("vision search provider is not configured")
	ErrVisionInput            = errors.New("vision search input is invalid")
	ErrVisionImageInvalid     = errors.New("vision search image is invalid")
	ErrVisionImageUnavailable = errors.New("vision search image is unavailable")
	ErrVisionSchema           = errors.New("vision search response schema is invalid")
)

type VisionExtraction struct {
	Query string `json:"query"`
}

type VisionImage struct {
	Filename    string
	ContentType string
	Data        []byte
}

type VisionInput struct {
	Images   []VisionImage
	UserText string
}

type VisionConfigReader interface {
	VisionConfig(context.Context) (VisionProviderConfig, error)
}

type VisionProviderClient interface {
	Extract(context.Context, VisionProviderConfig, VisionInput) (VisionExtraction, error)
}

type VisionSearchService struct {
	config  VisionConfigReader
	client  VisionProviderClient
	metrics SearchMetrics
}

func NewVisionSearchService(config VisionConfigReader, metrics SearchMetrics) *VisionSearchService {
	return newVisionSearchService(config, openAICompatibleVisionProviderClient{}, metrics)
}

type openAICompatibleVisionProviderClient struct{}

func (openAICompatibleVisionProviderClient) Extract(ctx context.Context, config VisionProviderConfig, input VisionInput) (VisionExtraction, error) {
	if isCloudflareWorkersAI(config.EndpointURL) {
		return extractCloudflareWorkersAIVision(ctx, config, input)
	}
	return extractOpenAICompatibleVision(ctx, config, input)
}

func newVisionSearchService(config VisionConfigReader, client VisionProviderClient, metrics SearchMetrics) *VisionSearchService {
	return &VisionSearchService{config: config, client: client, metrics: metrics}
}

func (s *VisionSearchService) Extract(ctx context.Context, input VisionInput) (VisionExtraction, error) {
	if len(input.Images) < 1 || len(input.Images) > visionMaxImages {
		return VisionExtraction{}, ErrVisionInput
	}
	if err := validateVisionImages(input.Images); err != nil {
		return VisionExtraction{}, err
	}
	input.UserText = strings.Join(strings.Fields(strings.TrimSpace(input.UserText)), " ")
	if utf8RuneCount(input.UserText) > MaxSearchQueryRunes {
		return VisionExtraction{}, fmt.Errorf("搜索关键词不能超过 %d 个字符", MaxSearchQueryRunes)
	}
	if s == nil || s.config == nil || s.client == nil {
		return VisionExtraction{}, ErrVisionUnavailable
	}
	config, err := s.config.VisionConfig(ctx)
	if err != nil {
		if errors.Is(err, ErrProviderUnavailable) {
			return VisionExtraction{}, ErrVisionNotConfigured
		}
		return VisionExtraction{}, fmt.Errorf("%w: load provider config: %v", ErrVisionUnavailable, err)
	}
	if recorder, ok := s.metrics.(VisionSearchMetrics); ok {
		recorder.RecordVisionRequest()
	}
	extraction, err := s.client.Extract(ctx, config, input)
	if err != nil {
		return VisionExtraction{}, err
	}
	return normalizeVisionExtraction(extraction)
}

func validateVisionImages(images []VisionImage) error {
	total := 0
	for _, image := range images {
		if len(image.Data) == 0 || len(image.Data) > visionMaxBytes {
			return ErrVisionImageInvalid
		}
		ext := imageformat.ImageExtensionFromMIME(image.ContentType)
		if ext == "" {
			var ok bool
			ext, ok = imageformat.NormalizeImageExtension(strings.ToLower(pathExt(image.Filename)))
			if !ok {
				ext = imageformat.DetectImageExtensionFromBytes(image.Data)
			}
		}
		if ext == "" || imageformat.ValidateImageBytes(image.Data, ext) != nil || imageformat.ValidateImageDimensions(image.Data) != nil {
			return ErrVisionImageInvalid
		}
		total += len(image.Data)
		if total > visionMaxBytes {
			return ErrVisionImageInvalid
		}
	}
	return nil
}

func normalizeVisionExtraction(value VisionExtraction) (VisionExtraction, error) {
	value.Query = strings.Join(strings.Fields(strings.TrimSpace(value.Query)), " ")
	if value.Query == "" || utf8RuneCount(value.Query) > MaxSearchQueryRunes {
		return VisionExtraction{}, ErrVisionSchema
	}
	return value, nil
}

func utf8RuneCount(value string) int { return len([]rune(value)) }

func testVisionProvider(ctx context.Context, endpoint, model, apiKey string) (FailureCategory, string, error) {
	fixture, err := visionTestFixture()
	if err != nil {
		return FailureUnknown, "Vision Search 连接失败", err
	}
	config := VisionProviderConfig{EndpointURL: endpoint, Model: model, APIKey: apiKey}
	extraction, err := openAICompatibleVisionProviderClient{}.Extract(ctx, config, VisionInput{Images: []VisionImage{fixture}, UserText: "geometric color study"})
	if err != nil {
		category := classifyVisionProviderFailure(err)
		return category, visionProviderTestFailureMessage(category), err
	}
	if _, err := normalizeVisionExtraction(extraction); err != nil {
		return FailureResponse, "Vision Search 返回格式无效", err
	}
	return "", "连接测试通过", nil
}

func visionTestFixture() (VisionImage, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			shade := uint8(72 + (x+y)/2)
			canvas.SetRGBA(x, y, color.RGBA{R: shade, G: 112, B: 168, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, canvas); err != nil {
		return VisionImage{}, fmt.Errorf("encode vision test fixture: %w", err)
	}
	return VisionImage{Filename: "fixture.png", ContentType: "image/png", Data: data.Bytes()}, nil
}

func visionProviderTestFailureMessage(category FailureCategory) string {
	switch category {
	case FailureCredentials:
		return "Vision Search 凭证无效"
	case FailureQuota:
		return "Vision Search 权限或配额不可用"
	case FailureResponse:
		return "Vision Search 返回格式无效"
	default:
		return "Vision Search 连接失败"
	}
}
