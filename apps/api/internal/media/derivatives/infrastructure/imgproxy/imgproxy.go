package imgproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/derivatives/domain"
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

// TierURL builds a signed pull URL that renders the source object at the
// tier geometry as webp. Source is an imgproxy source URL such as
// local:///<storage-key> or s3://<bucket>/<key>.
func (c Config) TierURL(tier, source string) (string, error) {
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
	signature, err := c.SignPath(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/") + "/" + signature + path, nil
}

// LocalSource maps a builtin-local storage key to an imgproxy source URL.
//
// Storage keys are relative to the app working directory and always start
// with the uploads root (see UploadObjectRoot). Compose mounts that root at
// /data/uploads, while IMGPROXY_LOCAL_FILESYSTEM_ROOT points at /data, so the
// uploads segment must remain part of the source path.
//
// The local:// scheme treats the segment between the slashes as a host and the
// rest as a path relative to IMGPROXY_LOCAL_FILESYSTEM_ROOT. An absolute path
// therefore needs the empty host form — "local:///<key>" — or imgproxy resolves
// the key against the wrong directory and 404s.
func LocalSource(storageKey string) (string, error) {
	key := strings.TrimPrefix(strings.TrimSpace(storageKey), "/")
	if key == "" {
		return "", fmt.Errorf("imgproxy local source key is required")
	}
	// Storage keys are relative to the uploads mount. Requiring the canonical
	// root and rejecting empty/dot segments prevents directory resolution and
	// path traversal even if a malformed key reaches this boundary.
	segments := strings.Split(key, "/")
	if len(segments) < 2 || segments[0] != "uploads" {
		return "", fmt.Errorf("imgproxy local source key must be below the uploads root")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("imgproxy local source key contains an invalid path segment")
		}
	}
	return "local:///" + key, nil
}
