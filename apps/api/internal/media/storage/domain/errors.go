package domain

import "errors"

var (
	// ErrStoredObjectNotFound is returned when the underlying object is absent.
	ErrStoredObjectNotFound = errors.New("stored object not found")

	// ErrObjectGenerationUnavailable is returned when an object store cannot prove the immutable generation.
	ErrObjectGenerationUnavailable = errors.New("stored object generation unavailable")

	// ErrStorageSecretVersionConflict is returned when a secret rotation version conflict occurs.
	ErrStorageSecretVersionConflict = errors.New("storage secret version rotation conflict: retry the request")

	// ErrStorageProfileLocatorImmutable is returned when attempting to mutate an immutable storage profile locator.
	ErrStorageProfileLocatorImmutable = errors.New("storage profile locator is immutable after media assets reference it")

	// ErrPresignInvalidProfile is returned when the storage profile id is empty or unknown.
	ErrPresignInvalidProfile = errors.New("presign: storage profile id is invalid")

	// ErrPresignInvalidObjectKey is returned when the requested object key is empty or invalid.
	ErrPresignInvalidObjectKey = errors.New("presign: object key is invalid")

	// ErrPresignTTLTooLong is retained for wire compatibility; TTLs now
	// clamp to PresignTTL without an error.
	ErrPresignTTLTooLong = errors.New("presign: ttl exceeds maximum cap")

	// ErrPresignUnsupportedProvider is returned when the storage provider does not support provider hand-off.
	ErrPresignUnsupportedProvider = errors.New("presign: storage provider does not support provider hand-off")

	// ErrUploadAccessDependencyMissing is returned when an authorization dependency is missing.
	ErrUploadAccessDependencyMissing = errors.New("upload access dependency missing")

	// ErrProviderHTTPSUnavailable is returned when the storage profile does not expose a provider HTTPS URL.
	ErrProviderHTTPSUnavailable = errors.New("storage profile does not expose a provider HTTPS URL")

	// ErrAvatarObjectKeyInvalid is returned when an avatar key cannot be
	// built (blank content hash; the upload handler always passes the
	// hex sha256, so this only fires on programmer error).
	ErrAvatarObjectKeyInvalid = errors.New("avatar object key is invalid")

	// ErrSiteAssetLogoObjectKeyInvalid is returned when a branding logo
	// key cannot be built (blank content hash; the control upload
	// handler always passes the hex sha256, so this only fires on
	// programmer error).
	ErrSiteAssetLogoObjectKeyInvalid = errors.New("site asset logo object key is invalid")
)
