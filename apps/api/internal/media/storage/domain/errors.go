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

	// ErrPresignTTLTooLong is returned when the requested TTL exceeds MaxPresignTTL.
	ErrPresignTTLTooLong = errors.New("presign: ttl exceeds maximum cap")

	// ErrPresignUnsupportedProvider is returned when the storage provider does not support provider hand-off.
	ErrPresignUnsupportedProvider = errors.New("presign: storage provider does not support provider hand-off")

	// ErrUploadAccessInvalidPath is returned when the upload path format is invalid.
	ErrUploadAccessInvalidPath = errors.New("upload access invalid path")

	// ErrUploadAccessDenied is returned when access to the upload is denied.
	ErrUploadAccessDenied = errors.New("upload access denied")

	// ErrUploadAccessOwnerMismatch is returned when the user does not own the requested resource.
	ErrUploadAccessOwnerMismatch = errors.New("upload access owner mismatch")

	// ErrUploadAccessDependencyMissing is returned when an authorization dependency is missing.
	ErrUploadAccessDependencyMissing = errors.New("upload access dependency missing")

	// ErrProviderHTTPSUnavailable is returned when the storage profile does not expose a provider HTTPS URL.
	ErrProviderHTTPSUnavailable = errors.New("storage profile does not expose a provider HTTPS URL")
)

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNormalizeStorageUserSegment      = "用户存储标识无效"                          // NormalizeStorageUserSegment @ upload_path.go
	msgNormalizeUploadSegment           = "上传标识无效"                            // NormalizeUploadSegment @ upload_path.go
	msgImageDerivativeObjectKey         = "图片派生对象参数无效"                        // ImageDerivativeObjectKey @ derivative_key.go
	msgStableUploadObjectKey            = "上传文件名无效"                           // StableUploadObjectKey @ object_key.go
	msgTaskOutputUserUploadObjectPrefix = "用户ID无效"                            // TaskOutputUserUploadObjectPrefix @ upload_policy.go
	msgLibraryAssetUploadDirPrefix      = "素材库媒体标识无效"                         // LibraryAssetUploadDirPrefix @ upload_policy.go
	msgParseUploadResourceParts         = "文件路径不能为空"                          // ParseUploadResourceParts @ upload_policy.go
	msgParseUploadResourceParts2        = "不支持的上传资源路径"                        // ParseUploadResourceParts @ upload_policy.go
	msgNormalizeUploadObjectKey         = "仅允许操作 uploads 或 ws-uploads 目录下的文件" // NormalizeUploadObjectKey @ upload_policy.go
	msgNormalizeUploadObjectPrefix      = "文件前缀不能为空"                          // NormalizeUploadObjectPrefix @ upload_policy.go
)
