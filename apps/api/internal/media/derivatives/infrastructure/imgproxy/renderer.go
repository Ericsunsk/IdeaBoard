package imgproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/imageformat"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/infrastructure/s3"
)

const (
	renderRequestTTL = 5 * time.Minute
	maxImageBytes    = 100 * 1024 * 1024
)

var (
	ErrSource      = errors.New("image rendering source is unavailable")
	ErrNotFound    = errors.New("rendered image was not found")
	ErrUnavailable = errors.New("image renderer is unavailable")
)

type ProfileResolver interface {
	ResolveActiveProfile(context.Context) (*storagedomain.StorageProfile, error)
}

type Image struct {
	Data     []byte
	MIMEType string
}

type Renderer struct {
	config   Config
	profiles ProfileResolver
	client   *http.Client
}

func NewRenderer(config Config, profiles ProfileResolver) *Renderer {
	return &Renderer{
		config: config, profiles: profiles,
		client: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (r *Renderer) WorkspaceTier(ctx context.Context, storageKey, tier string) (Image, error) {
	if r == nil || tier == domain.TierProcessingInput {
		return Image{}, ErrSource
	}
	source, err := LocalSource(storageKey)
	if err != nil {
		return Image{}, ErrSource
	}
	return r.tier(ctx, source, tier)
}

func (r *Renderer) Avatar(ctx context.Context, storageKey string, size int) (Image, error) {
	source, err := r.activeSource(ctx, storageKey)
	if err != nil {
		return Image{}, err
	}
	target, err := r.config.AvatarURL(size, source, time.Now().Add(renderRequestTTL))
	if err != nil {
		return Image{}, ErrSource
	}
	return r.fetch(ctx, target)
}

func (r *Renderer) Logo(ctx context.Context, storageKey string) (Image, error) {
	source, err := r.activeSource(ctx, storageKey)
	if err != nil {
		return Image{}, err
	}
	return r.tier(ctx, source, domain.TierPreview)
}

func (r *Renderer) tier(ctx context.Context, source, tier string) (Image, error) {
	target, err := r.config.TierURL(tier, source, time.Now().Add(renderRequestTTL))
	if err != nil {
		return Image{}, ErrSource
	}
	return r.fetch(ctx, target)
}

func (r *Renderer) activeSource(ctx context.Context, storageKey string) (string, error) {
	if r == nil || r.profiles == nil {
		return "", ErrSource
	}
	profile, err := r.profiles.ResolveActiveProfile(ctx)
	if err != nil || profile == nil || !profile.Enabled {
		return "", ErrSource
	}
	var source string
	switch profile.Provider {
	case storagedomain.StorageProviderLocal:
		source, err = LocalSource(storageKey)
	case storagedomain.StorageProviderS3Compatible:
		cfg := s3.S3CompatibleConfigFromProfile(profile)
		source, err = S3Source(cfg.BucketName, storageKey)
	default:
		return "", ErrSource
	}
	if err != nil {
		return "", ErrSource
	}
	return source, nil
}

func (r *Renderer) fetch(ctx context.Context, target string) (Image, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Image{}, ErrUnavailable
	}
	response, err := r.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return Image{}, ctx.Err()
		}
		return Image{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Image{}, ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return Image{}, ErrUnavailable
	}
	mime, valid := imageformat.NormalizeImageMIME(response.Header.Get("Content-Type"))
	if !valid || response.ContentLength > maxImageBytes {
		return Image{}, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return Image{}, ctx.Err()
		}
		return Image{}, ErrUnavailable
	}
	if len(data) == 0 || len(data) > maxImageBytes {
		return Image{}, ErrUnavailable
	}
	return Image{Data: data, MIMEType: mime}, nil
}
