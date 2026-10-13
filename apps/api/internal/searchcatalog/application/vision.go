package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

type VisionSearchService struct {
	config ports.VisionConfigReader
	client ports.VisionProviderClient
}

type gatewayVisionProviderClient struct {
	gateway ports.VisionGatewayClient
}

func (c gatewayVisionProviderClient) Extract(ctx context.Context, config ports.VisionProviderConfig, input domain.VisionInput) (domain.VisionExtraction, error) {
	return extractOpenAICompatibleVision(ctx, c.gateway, config.Model, input)
}

func newVisionSearchService(config ports.VisionConfigReader, client ports.VisionProviderClient) *VisionSearchService {
	return &VisionSearchService{config: config, client: client}
}

func (s *VisionSearchService) Extract(ctx context.Context, input domain.VisionInput) (domain.VisionExtraction, error) {
	if err := validateVisionInput(input); err != nil {
		return domain.VisionExtraction{}, err
	}
	input.UserText = strings.Join(strings.Fields(strings.TrimSpace(input.UserText)), " ")
	if s == nil || s.config == nil || s.client == nil {
		return domain.VisionExtraction{}, domain.ErrVisionUnavailable
	}
	config, err := s.config.VisionConfig(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrProviderUnavailable) {
			return domain.VisionExtraction{}, domain.ErrVisionNotConfigured
		}
		return domain.VisionExtraction{}, fmt.Errorf("%w: load provider config: %w", domain.ErrVisionUnavailable, err)
	}
	extraction, err := s.client.Extract(ctx, config, input)
	if err != nil {
		return domain.VisionExtraction{}, err
	}
	return normalizeVisionExtraction(extraction)
}

func ValidateVisionImages(images []domain.VisionImage) error {
	total := 0
	for _, image := range images {
		if len(image.Data) == 0 || len(image.Data) > domain.VisionMaxBytes {
			return domain.ErrVisionImageInvalid
		}
		ext := imageformat.ImageExtensionFromMIME(image.ContentType)
		if ext == "" {
			var ok bool
			ext, ok = imageformat.NormalizeImageExtension(strings.ToLower(pathExt(image.Filename)))
			if !ok {
				ext = imageformat.DetectImageExtensionFromBytes(image.Data)
			}
		}
		if ext == "" {
			return domain.ErrVisionImageInvalid
		}
		if _, err := imageformat.InspectBytes(image.Data, ext); err != nil {
			return domain.ErrVisionImageInvalid
		}
		total += len(image.Data)
		if total > domain.VisionMaxBytes {
			return domain.ErrVisionImageInvalid
		}
	}
	return nil
}

func normalizeVisionExtraction(value domain.VisionExtraction) (domain.VisionExtraction, error) {
	value.Query = strings.Join(strings.Fields(strings.TrimSpace(value.Query)), " ")
	if value.Query == "" || utf8RuneCount(value.Query) > MaxSearchQueryRunes {
		return domain.VisionExtraction{}, domain.ErrVisionSchema
	}
	return value, nil
}
func utf8RuneCount(value string) int { return len([]rune(value)) }

func pathExt(filename string) string {
	if index := strings.LastIndex(filename, "."); index >= 0 {
		return filename[index:]
	}
	return ""
}
