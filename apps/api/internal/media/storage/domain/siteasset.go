package domain

import (
	"path"
	"strings"
)

// SiteAssetLogoDir is the canonical object directory (below the uploads
// root) for the site branding logo: uploads/site/assets/.
const SiteAssetLogoDir = "site/assets"

// SiteAssetLogoDirPrefix returns the canonical object directory for the
// site branding logo: uploads/site/assets/.
func SiteAssetLogoDirPrefix() string {
	return path.Join(UploadObjectRoot, SiteAssetLogoDir) + "/"
}

// SiteAssetLogoObjectKey returns the canonical storage key for one logo
// body: uploads/site/assets/logo_{hash16}{ext}. hash16 is the hex sha256
// prefix, ext the sniffed image extension (leading dot added when
// missing).
func SiteAssetLogoObjectKey(hash16, ext string) (string, error) {
	return contentHashedKey(path.Join(UploadObjectRoot, SiteAssetLogoDir), "logo_", hash16, ext, ErrSiteAssetLogoObjectKeyInvalid)
}

// contentHashedKey builds a canonical content-hashed object key under dir
// with filename stem+digest+extension (digest lower/trim, ext
// lower/default-.png/dot-prefix, path.Join, NormalizeUploadObjectKey).
// Shared skeleton of AvatarObjectKey (stem "") and SiteAssetLogoObjectKey
// (stem "logo_"); blank digests report invalidErr so each caller keeps its
// distinct sentinel (ErrAvatarObjectKeyInvalid /
// ErrSiteAssetLogoObjectKeyInvalid).
func contentHashedKey(dir, stem, hash16, ext string, invalidErr error) (string, error) {
	digest := strings.ToLower(strings.TrimSpace(hash16))
	if digest == "" {
		return "", invalidErr
	}
	extension := strings.ToLower(strings.TrimSpace(ext))
	if extension == "" {
		extension = ".png"
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	return NormalizeUploadObjectKey(path.Join(dir, stem+digest+extension))
}

// IsSiteAssetLogoKey reports whether key lives under the canonical
// site-asset logo keyspace (uploads/site/assets/), excluding traversal
// shapes (no "..", no backslashes, no empty/dot segments).
func IsSiteAssetLogoKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || !strings.HasPrefix(trimmed, SiteAssetLogoDirPrefix()) {
		return false
	}
	if strings.Contains(trimmed, "..") || strings.Contains(trimmed, "\\") {
		return false
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" || segment == "." {
			return false
		}
	}
	return true
}
