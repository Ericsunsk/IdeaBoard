package imgproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"ideaboard/api/internal/media/derivatives/domain"
	storagedomain "ideaboard/api/internal/media/storage/domain"
)

// Config carries the imgproxy deployment binding. Key/Salt are hex-encoded
// per imgproxy docs; BaseURL is the service origin without path.
//
// Tier geometry is not duplicated here: the domain owns the tier catalogue
// (see derivatives/domain.TierSpecs) because the same numbers back both the
// synchronous processing-input render and these on-demand pull URLs.
type Config struct {
	BaseURL string
	KeyHex  string
	SaltHex string
}

// Validate rejects missing or weak bindings before any URL is signed.
func (c Config) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("imgproxy base URL is required")
	}
	lower := strings.ToLower(strings.TrimSpace(c.BaseURL))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("imgproxy base URL must be an http(s) origin")
	}
	for name, value := range map[string]string{"key": c.KeyHex, "salt": c.SaltHex} {
		raw, err := hex.DecodeString(strings.TrimSpace(value))
		if err != nil || len(raw) < 16 {
			return fmt.Errorf("imgproxy %s must be hex-encoded with at least 16 bytes", name)
		}
	}
	return nil
}

// SignPath returns the URL-safe base64 HMAC-SHA256 signature for an imgproxy
// path (including the leading slash), per the imgproxy signing spec.
func (c Config) SignPath(path string) (string, error) {
	key, err := hex.DecodeString(strings.TrimSpace(c.KeyHex))
	if err != nil {
		return "", fmt.Errorf("decode imgproxy key: %w", err)
	}
	salt, err := hex.DecodeString(strings.TrimSpace(c.SaltHex))
	if err != nil {
		return "", fmt.Errorf("decode imgproxy salt: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	mac.Write([]byte(path))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// TierURL signs an expiring webp rendition using the domain tier geometry.
func (c Config) TierURL(tier, source string, expires time.Time) (string, error) {
	spec, ok := domain.TierSpecs[strings.TrimSpace(tier)]
	if !ok {
		return "", fmt.Errorf("unknown imgproxy tier %q", tier)
	}
	side := spec.MaxSide
	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("imgproxy source is required")
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	path := fmt.Sprintf("/rs:fit:%d:%d/plain/%s@webp", side, side, strings.TrimSpace(source))
	return c.signWithExpiry(path, expires)
}

// AvatarSizes is the closed display-size whitelist for profile avatars.
// The frontend maps devicePixelRatio to the next size up; anything else
// is rejected before signing.
var AvatarSizes = []int{32, 64, 128, 256}

// DefaultAvatarSize serves callers that omit the size query.
const DefaultAvatarSize = 64

// AvatarURL builds a signed pull URL that renders the source object as a
// square webp crop with the given expiry at one of AvatarSizes.
func (c Config) AvatarURL(size int, source string, expires time.Time) (string, error) {
	if !slices.Contains(AvatarSizes, size) {
		return "", fmt.Errorf("unknown avatar size %d", size)
	}
	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("imgproxy source is required")
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	path := fmt.Sprintf("/rs:fill:%d:%d/plain/%s@webp", size, size, strings.TrimSpace(source))
	return c.signWithExpiry(path, expires)
}

func (c Config) signWithExpiry(path string, expires time.Time) (string, error) {
	if expires.IsZero() {
		return "", fmt.Errorf("imgproxy expiry is required")
	}
	path = (&url.URL{Path: fmt.Sprintf("/exp:%d%s", expires.Unix(), path)}).EscapedPath()
	signature, err := c.SignPath(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/") + "/" + signature + path, nil
}

// rejectDotSegments reports whether key contains an empty or dot path
// segment. LocalSource and S3Source share this traversal discipline;
// callers keep their own error context.
func rejectDotSegments(key string) bool {
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

// LocalSource maps an uploads key to the imgproxy local filesystem root.
// Compose mounts uploads beneath /data, so the root stays in the source path.
func LocalSource(storageKey string) (string, error) {
	key := strings.TrimSpace(storageKey)
	if key == "" {
		return "", fmt.Errorf("imgproxy local source key is required")
	}
	// Requiring a known root and rejecting empty/dot segments prevents
	// directory resolution and path traversal even if a malformed key
	// reaches this boundary.
	segments := strings.Split(key, "/")
	if len(segments) < 2 || segments[0] != storagedomain.UploadObjectRoot {
		return "", fmt.Errorf("imgproxy local source key must be below a known uploads root")
	}
	if rejectDotSegments(key) {
		return "", fmt.Errorf("imgproxy local source key contains an invalid path segment")
	}
	return "local:///" + key, nil
}

// S3Source maps a canonical key to the active profile's bucket.
// Deployment credentials and IMGPROXY_ALLOWED_SOURCES enforce renderer access.
func S3Source(bucket, storageKey string) (string, error) {
	name := strings.TrimSpace(bucket)
	if name == "" {
		return "", fmt.Errorf("imgproxy s3 source bucket is required")
	}
	if name == "." || name == ".." || strings.Contains(name, "/") {
		return "", fmt.Errorf("imgproxy s3 source bucket contains an invalid path segment")
	}
	key := strings.TrimSpace(storageKey)
	if key == "" {
		return "", fmt.Errorf("imgproxy s3 source key is required")
	}
	if rejectDotSegments(key) {
		return "", fmt.Errorf("imgproxy s3 source key contains an invalid path segment")
	}
	return "s3://" + name + "/" + key, nil
}
