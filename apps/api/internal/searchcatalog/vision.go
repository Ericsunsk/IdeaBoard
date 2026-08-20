package searchcatalog

import (
	"context"
	"errors"
	"fmt"
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
	Query          string   `json:"query"`
	Palette        []string `json:"palette,omitempty"`
	AtmosphereTags []string `json:"atmosphere_tags,omitempty"`
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
	return newVisionSearchService(config, multipartVisionProviderClient{}, metrics)
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
	value.Palette = normalizePalette(value.Palette)
	value.AtmosphereTags = normalizeAtmosphereTags(value.AtmosphereTags)
	return value, nil
}

func utf8RuneCount(value string) int { return len([]rune(value)) }

func normalizeUnique(values []string, limit int, normalize func(string) (string, bool)) []string {
	result := make([]string, 0, limit)
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized, ok := normalize(value)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
		if len(result) == limit {
			break
		}
	}
	return result
}

func testVisionProvider(ctx context.Context, endpoint, model, apiKey string) (FailureCategory, string, error) {
	fixture := VisionImage{
		Filename:    "fixture.png",
		ContentType: "image/png",
		Data:        []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0, 0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82},
	}
	extraction, err := (multipartVisionProviderClient{}).Extract(ctx, VisionProviderConfig{EndpointURL: endpoint, Model: model, APIKey: apiKey}, VisionInput{Images: []VisionImage{fixture}, UserText: "minimal studio still life"})
	if err != nil {
		category := classifyVisionProviderFailure(err)
		return category, visionProviderTestFailureMessage(category), err
	}
	if _, err := normalizeVisionExtraction(extraction); err != nil {
		return FailureResponse, "Vision Search 返回格式无效", err
	}
	return "", "连接测试通过", nil
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
