package providerdomain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DefaultTextCacheTTLSeconds is the frozen first cache TTL for deterministic
// text requests (ai-gateway plan). It sits inside the gateway-allowed range.
const DefaultTextCacheTTLSeconds = 3600

// cacheKeyVersion prefixes every cache key so future format changes never
// collide with stored entries.
const cacheKeyVersion = "v1"

// BuildTextCacheKey derives the opaque gateway cache key for one
// deterministic text request. Callers pass the canonical request bytes
// (stable field order, no one-shot fields); feature and model scope the key
// so equivalent prompts under different features or models never share hits.
func BuildTextCacheKey(feature, model string, canonical []byte) (string, error) {
	feature = strings.TrimSpace(feature)
	model = strings.TrimSpace(model)
	if feature == "" {
		return "", fmt.Errorf("cache key requires a feature")
	}
	if model == "" {
		return "", fmt.Errorf("cache key requires a model")
	}
	if len(canonical) == 0 {
		return "", fmt.Errorf("cache key requires canonical request bytes")
	}
	sum := sha256.Sum256([]byte("aigw/" + cacheKeyVersion + "\x00" + feature + "\x00" + model + "\x00"))
	sum = sha256.Sum256(append(sum[:], canonical...))
	return "aigw/" + cacheKeyVersion + "/" + hex.EncodeToString(sum[:]), nil
}
