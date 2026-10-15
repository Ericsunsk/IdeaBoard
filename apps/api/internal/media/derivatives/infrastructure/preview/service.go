package preview

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
)

const (
	capabilityLifetime = 24 * time.Hour
	maxSourceBytes     = 20 * 1024 * 1024
	capabilityPurpose  = "ideaboard.external-preview.v1"
)

var ErrCapability = errors.New("image preview capability is invalid")

type sourceCapability struct {
	Provider string `json:"provider"`
	SourceID string `json:"source_id"`
	URL      string `json:"url"`
	Expires  int64  `json:"expires"`
}

type Service struct {
	aead     cipher.AEAD
	root     string
	renderer *imgproxy.Renderer
	download func(context.Context, string, []string, int64) (*remote.ImageFile, error)
	now      func() time.Time
	slots    chan struct{}
}

func NewService(config imgproxy.Config, root string, renderer *imgproxy.Renderer) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	key, _ := hex.DecodeString(strings.TrimSpace(config.KeyHex))
	salt, _ := hex.DecodeString(strings.TrimSpace(config.SaltHex))
	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	mac.Write([]byte(capabilityPurpose))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(root) == "" || renderer == nil {
		return nil, imgproxy.ErrSource
	}
	return &Service{aead: aead, root: root, renderer: renderer, download: remote.DownloadPublicImage, now: time.Now, slots: make(chan struct{}, 8)}, nil
}

func (s *Service) SearchDisplay(provider, sourceID, sourceURL string) *domain.Display {
	if s == nil || sourceID == "" || len(sourceID) > 255 || !validSource(provider, sourceURL) {
		return nil
	}
	expires := s.now().Add(capabilityLifetime)
	payload, err := json.Marshal(sourceCapability{Provider: provider, SourceID: sourceID, URL: sourceURL, Expires: expires.Unix()})
	if err != nil {
		return nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil
	}
	token := base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, payload, []byte(capabilityPurpose)))
	display := domain.DisplayVariants(func(tier string) string { return "/api/media/previews/" + token + "/" + tier })
	display.ExpiresAt = expires.UTC().Format(time.RFC3339)
	return display
}

func (s *Service) RenderCapability(ctx context.Context, token, tier string) (imgproxy.Image, time.Time, error) {
	if s == nil || len(token) > 16384 {
		return imgproxy.Image{}, time.Time{}, ErrCapability
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return imgproxy.Image{}, time.Time{}, ErrCapability
	}
	payload, err := s.aead.Open(nil, raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():], []byte(capabilityPurpose))
	if err != nil {
		return imgproxy.Image{}, time.Time{}, ErrCapability
	}
	var source sourceCapability
	if json.Unmarshal(payload, &source) != nil || source.SourceID == "" || source.Expires <= s.now().Unix() || !validSource(source.Provider, source.URL) {
		return imgproxy.Image{}, time.Time{}, ErrCapability
	}
	image, err := s.RenderSource(ctx, source.Provider, source.URL, tier)
	return image, time.Unix(source.Expires, 0), err
}

func (s *Service) RenderSource(ctx context.Context, provider, sourceURL, tier string) (imgproxy.Image, error) {
	if s == nil || !validSource(provider, sourceURL) {
		return imgproxy.Image{}, imgproxy.ErrSource
	}
	if _, ok := domain.TierSpecs[tier]; !ok || tier == domain.TierProcessingInput {
		return imgproxy.Image{}, imgproxy.ErrSource
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return imgproxy.Image{}, ctx.Err()
	}
	file, err := s.download(ctx, sourceURL, imageHosts(provider), maxSourceBytes)
	if err != nil {
		return imgproxy.Image{}, imgproxy.ErrUnavailable
	}
	defer file.Cleanup()
	staging, err := os.CreateTemp(s.root, ".preview-*")
	if err != nil {
		return imgproxy.Image{}, imgproxy.ErrUnavailable
	}
	defer os.Remove(staging.Name())
	if err := staging.Chmod(0644); err != nil {
		_ = staging.Close()
		return imgproxy.Image{}, imgproxy.ErrUnavailable
	}
	size, copyErr := io.Copy(staging, io.LimitReader(file, maxSourceBytes+1))
	closeErr := staging.Close()
	if copyErr != nil || closeErr != nil || size > maxSourceBytes {
		return imgproxy.Image{}, imgproxy.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return imgproxy.Image{}, err
	}
	return s.renderer.WorkspaceTier(ctx, filepath.ToSlash(filepath.Join(filepath.Base(s.root), filepath.Base(staging.Name()))), tier)
}

func imageHosts(provider string) []string {
	switch provider {
	case "unsplash":
		return []string{"images.unsplash.com", "plus.unsplash.com"}
	case "pexels":
		return []string{"images.pexels.com"}
	default:
		return nil
	}
}

func validSource(provider, raw string) bool {
	if len(raw) > 4096 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || (parsed.Port() != "" && parsed.Port() != "443") {
		return false
	}
	for _, host := range imageHosts(provider) {
		if parsed.Hostname() == host {
			return true
		}
	}
	return false
}
