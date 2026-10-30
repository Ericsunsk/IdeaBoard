package providerdomain

import (
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net/url"
	"strings"
)

// ResolveProviderPublicImageURL maps one image ref (upload path, public
// URL, or canvas ref) to a public HTTPS URL vendors can fetch.
func ResolveProviderPublicImageURL(baseParsedURL *url.URL, ref, label string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf(errorrender.MsgResolveProviderPublicImageURL, label)
	}
	if strings.HasPrefix(strings.ToLower(ref), "data:image/") {
		return ref, nil
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf(errorrender.MsgResolveProviderPublicImageURL2, label, err)
	}
	if parsed.Scheme == "" && strings.HasPrefix(ref, "/uploads/") {
		if baseParsedURL == nil {
			return "", errors.New(errorrender.MsgResolveProviderPublicImageURL3)
		}
		parsed = baseParsedURL.ResolveReference(parsed)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf(errorrender.MsgResolveProviderPublicImageURL4, label)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf(errorrender.MsgResolveProviderPublicImageURL5, label)
	}
	return parsed.String(), nil
}
