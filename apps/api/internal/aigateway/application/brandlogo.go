package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// Brand logo proxy seam: admin-only read-only passthrough of models.dev
// provider logos (/logos/{provider}.svg). It rides the models.dev read
// module (same host allowlist pre-check, same shared outbound client)
// without a second models.dev client: provider IDs are strictly validated
// strictly validated (lowercase alphanumerics plus hyphen) so the upstream
// path can never escape /logos/*.svg, successful fetches sit in a long-TTL
// memory cache (logos rarely change), and every miss or failure degrades to
// the embedded default mark so admin cards never render a broken image.

// brandLogoContentType is the only content type this seam ever serves.
const brandLogoContentType = "image/svg+xml"

// brandLogoFallbackHeader marks degraded responses carrying the embedded
// default mark so operators can tell fallback from upstream bytes.
const brandLogoFallbackHeader = "X-Brand-Logo-Fallback"

// brandLogoIDPattern strictly validates the provider path segment: lowercase
// alphanumerics with single-hyphen separators, 1-64 chars. Anything else
// (slashes, dots, uppercase, scheme) is rejected before any URL is built,
// which keeps the fetch on the same /logos/*.svg shape (no path traversal,
// no SSRF host smuggling).
var brandLogoIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// defaultBrandLogoSVG is the models.dev generic mark served when the
// provider is unknown to us (allowlist gap) or the upstream fetch fails.
// Byte-identical to what models.dev itself returns for unknown providers,
// so fallback and upstream-miss render the same card.
const defaultBrandLogoSVG = `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
  <path
    shape-rendering="geometricPrecision"
    d="M9.8132 15.9038L9 18.75L8.1868 15.9038C7.75968 14.4089 6.59112 13.2403 5.09619 12.8132L2.25 12L5.09619 11.1868C6.59113 10.7597 7.75968 9.59112 8.1868 8.09619L9 5.25L9.8132 8.09619C10.2403 9.59113 11.4089 10.7597 12.9038 11.1868L15.75 12L12.9038 12.8132C11.4089 13.2403 10.2403 14.4089 9.8132 15.9038Z"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
  />
  <path
    d="M18.2589 8.71454L18 9.75L17.7411 8.71454C17.4388 7.50533 16.4947 6.56117 15.2855 6.25887L14.25 6L15.2855 5.74113C16.4947 5.43883 17.4388 4.49467 17.7411 3.28546L18 2.25L18.2589 3.28546C18.5612 4.49467 19.5053 5.43883 20.7145 5.74113L21.75 6L20.7145 6.25887C19.5053 6.56117 18.5612 7.50533 18.2589 8.71454Z"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
  />
  <path
    d="M16.8942 20.5673L16.5 21.75L16.1058 20.5673C15.8818 19.8954 15.3546 19.3682 14.6827 19.1442L13.5 18.75L14.6827 18.3558C15.3546 18.1318 15.8818 17.6046 16.1058 16.9327L16.5 15.75L16.8942 16.9327C17.1182 17.6046 17.6454 18.1318 18.3173 18.3558L19.5 18.75L18.3173 19.1442C17.6454 19.3682 17.1182 19.8954 16.8942 20.5673Z"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
  />
</svg>
`

type brandLogoEntry struct {
	body      []byte
	fetchedAt time.Time
}

var brandLogoCache struct {
	sync.RWMutex
	entries map[string]brandLogoEntry
}

// resetBrandLogoCache clears the in-memory logo cache (tests only).
func resetBrandLogoCache() {
	brandLogoCache.Lock()
	brandLogoCache.entries = nil
	brandLogoCache.Unlock()
}

// normalizeBrandLogoProvider trims and validates one provider path segment.
// Only invalid input returns an error; every other outcome serves SVG bytes.
func normalizeBrandLogoProvider(raw string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	if !brandLogoIDPattern.MatchString(provider) {
		return "", errors.New(errorrender.MsgNormalizeBrandLogoProvider)
	}
	return provider, nil
}

// loadBrandLogo returns the SVG bytes for one validated provider. Fallback
// is reported explicitly: false means upstream bytes, true means the
// embedded default mark (allowlist gap or upstream failure). Only invalid
// input is an error.
func loadBrandLogo(ctx context.Context, provider string) ([]byte, bool, error) {
	id, err := normalizeBrandLogoProvider(provider)
	if err != nil {
		return nil, false, err
	}

	if !modelsDevAllowlisted(ctx) {
		return []byte(defaultBrandLogoSVG), true, nil
	}

	brandLogoCache.RLock()
	if entry, ok := brandLogoCache.entries[id]; ok && modelsDevCacheFresh(entry.fetchedAt, brandLogoTTL) {
		cached := append([]byte(nil), entry.body...)
		brandLogoCache.RUnlock()
		return cached, false, nil
	}
	brandLogoCache.RUnlock()

	body, ok := fetchBrandLogoUpstream(ctx, id)
	if !ok {
		return []byte(defaultBrandLogoSVG), true, nil
	}

	brandLogoCache.Lock()
	if brandLogoCache.entries == nil {
		brandLogoCache.entries = make(map[string]brandLogoEntry)
	}
	brandLogoCache.entries[id] = brandLogoEntry{body: body, fetchedAt: time.Now()}
	brandLogoCache.Unlock()
	return append([]byte(nil), body...), false, nil
}

// fetchBrandLogoUpstream performs one bounded single-shot GET against
// {modelsDevLogoBaseURL}/{provider}.svg through the shared models.dev
// outbound helper. The provider is pre-validated so the URL shape is fixed;
// the runtime egress policy still gates the host exactly like the reference
// seam. False means "serve the default mark" (no retry: the next card
// render retries upstream naturally since fallbacks are never cached).
func fetchBrandLogoUpstream(ctx context.Context, provider string) ([]byte, bool) {
	rawURL := fmt.Sprintf("%s/%s.svg", strings.TrimSuffix(modelsDevLogoBaseURL, "/"), provider)
	// Logo stays single-shot (MaxAttempts 1): see modelsDevGet.
	resp, err := modelsDevGet(ctx, rawURL, 10*time.Second, 1, brandLogoTarget)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, brandLogoMaxBytes+1))
	if err != nil || len(body) == 0 || int64(len(body)) > brandLogoMaxBytes {
		return nil, false
	}
	if !isBrandLogoSVG(body) {
		return nil, false
	}
	return body, true
}

// isBrandLogoSVG lightly guards against upstream error pages served with a
// 200 status: real marks are XML SVG documents, optionally preceded by a
// prolog or whitespace.
func isBrandLogoSVG(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("<svg")) {
		return true
	}
	if bytes.HasPrefix(trimmed, []byte("<?xml")) {
		if i := bytes.Index(trimmed, []byte("<svg")); i >= 0 && i < 512 {
			return true
		}
	}
	return false
}

// GetBrandLogo serves the admin brand-logo passthrough as image/svg+xml.
// Degraded outcomes (allowlist gap, upstream failure, upstream miss) still
// return HTTP 200 with the default mark and a fallback marker header so
// admin cards never break; only invalid input (bad provider) is a 400.
// Authorization rides the outer Control Plane guard (controlProxyRaw + guard).
func (h *Handler) GetBrandLogo(c *gin.Context) {
	body, fallback, err := loadBrandLogo(c.Request.Context(), c.Query("provider"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	if fallback {
		c.Header(brandLogoFallbackHeader, "models-dev-default")
	}
	c.Data(http.StatusOK, brandLogoContentType, body)
}
