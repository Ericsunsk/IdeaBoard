package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ResolveProviderPublicImageURL maps one image ref (upload path, public
// URL, or canvas ref) to a public HTTPS URL vendors can fetch.
func ResolveProviderPublicImageURL(baseParsedURL *url.URL, ref, label string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf(msgResolveProviderPublicImageURL, label)
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf(msgResolveProviderPublicImageURL2, label, err)
	}
	if parsed.Scheme == "" && strings.HasPrefix(ref, "/uploads/") {
		if baseParsedURL == nil {
			return "", errors.New(msgResolveProviderPublicImageURL3)
		}
		parsed = baseParsedURL.ResolveReference(parsed)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf(msgResolveProviderPublicImageURL4, label)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf(msgResolveProviderPublicImageURL5, label)
	}
	return parsed.String(), nil
}
