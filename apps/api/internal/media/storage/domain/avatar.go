package domain

import (
	"path"
)

// AvatarObjectRoot scopes profile avatars under uploads/avatars/{uid}/{hash}.ext.
const AvatarObjectRoot = "avatars"

// AvatarUploadObjectPrefix returns the canonical object prefix for one
// caller's avatars: uploads/avatars/{uid}/.
func AvatarUploadObjectPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join(UploadObjectRoot, AvatarObjectRoot, ownerSegment) + "/", nil
}

// AvatarObjectKey returns the canonical storage key for one avatar body:
// uploads/avatars/{uid}/{hash16}{ext}. hash16 is the hex sha256 prefix,
// ext the sniffed image extension (leading dot added when missing).
func AvatarObjectKey(storageUID, hash16, ext string) (string, error) {
	ownerSegment, err := NormalizeStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return contentHashedKey(path.Join(UploadObjectRoot, AvatarObjectRoot, ownerSegment), "", hash16, ext, ErrAvatarObjectKeyInvalid)
}

// DefaultAvatarSVGContentType is the wire type for the bundled fallback.
const DefaultAvatarSVGContentType = "image/svg+xml"

// DefaultAvatarSVG is the bundled no-avatar fallback (Media owns bundled
// business assets). Fixed first-party bytes: no scripts, no event
// handlers, no external references, so it is safe to serve inline.
var DefaultAvatarSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><circle cx="32" cy="32" r="32" fill="#9aa3b2"/><circle cx="32" cy="24" r="11" fill="#e8ecf1"/><path d="M10 54c3-12 12-18 22-18s19 6 22 18c-6 6-13 8-22 8s-16-2-22-8z" fill="#e8ecf1"/></svg>`)
