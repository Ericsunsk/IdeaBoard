package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/safehttp"
)

const (
	// maxRemoteFileSize caps remote downloads to prevent OOM from unbounded streams.
	maxRemoteFileSize     = 100 * 1024 * 1024 // 100 MB
	storageCopyBufferSize = 32 * 1024
)

var storageCopyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, storageCopyBufferSize)
		return &buf
	},
}

func (s *StorageService) TestProfile(ctx context.Context, id string) error {
	profiles, err := s.loadProfilesWithSecrets(ctx)
	if err != nil {
		return err
	}

	profile, err := s.findProfile(profiles, id)
	if err != nil {
		return err
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.TestProfile(ctx, profile)
}

func (s *StorageService) GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error) {
	profiles, err := s.loadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := s.findProfile(profiles, id)
	if err != nil {
		return nil, err
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, fmt.Errorf("当前存储类型不支持读取 Bucket 信息")
	}
	return adapter.GetBucketInfo(ctx, profile)
}

func (s *StorageService) Upload(ctx context.Context, file *multipart.FileHeader) (string, error) {
	return s.UploadWithOptions(ctx, file, StorageStoreOptions{})
}

func (s *StorageService) UploadWithOptions(ctx context.Context, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	ref, err := s.UploadRefWithOptions(ctx, file, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *StorageService) UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.uploadRefWithProfile(ctx, *profile, file, opts)
}

// UploadRefAtProfile writes an upload through the immutable profile selected
// during ingestion planning. This prevents an operator profile switch from
// splitting the metadata record and object across different backends.
func (s *StorageService) UploadRefAtProfile(ctx context.Context, profileID string, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.uploadRefWithProfile(ctx, *profile, file, opts)
}

// PrepareUploadRefFromFile inspects the actual multipart bytes before
// generating an object key. It also canonicalizes the filename and MIME header
// so every storage adapter receives metadata derived from the image itself.
// Callers that need to register an ingestion before writing the object (for
// example board uploads) should use this method instead of PrepareUploadRef.
func (s *StorageService) PrepareUploadRefFromFile(ctx context.Context, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	facts, err := canonicalizeMultipartImageWithFacts(file)
	if err != nil {
		return nil, err
	}
	checksum, err := checksumMultipartFile(file)
	if err != nil {
		return nil, fmt.Errorf("读取上传文件 checksum 失败: %w", err)
	}
	key := strings.TrimSpace(opts.ObjectKey)
	if key == "" {
		key, err = buildObjectKey(opts.DirPrefix, facts.Extension)
		if err != nil {
			return nil, err
		}
	} else {
		key = canonicalObjectKeyExtension(key, facts.Extension)
	}
	key, err = normalizeUploadObjectKey(key)
	if err != nil {
		return nil, fmt.Errorf("上传对象路径无效: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	publicURL, urlErr := adapter.ProviderURL(profile, key)
	if urlErr != nil && profile.Provider == StorageProviderLocal {
		publicURL = s.publicUploadURL(ctx, key)
		urlErr = nil
	}
	if urlErr != nil {
		return nil, urlErr
	}
	return &StoredUploadRef{
		URL: publicURL, StorageKey: key, StorageProfile: profile.ID,
		MIMEType: facts.MIMEType, Width: facts.Width, Height: facts.Height,
		SizeBytes: file.Size, ChecksumSHA256: checksum, Animated: facts.Animated,
	}, nil
}

// checksumMultipartFile hashes the source before any object is written. It
// opens an independent multipart reader, leaving adapter upload state intact.
func checksumMultipartFile(file *multipart.FileHeader) (string, error) {
	if file == nil {
		return "", fmt.Errorf("文件内容不合法")
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if err := copyExactWithStorageBuffer(hash, src, file.Size); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *StorageService) PrepareUploadRef(ctx context.Context, filename string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(opts.ObjectKey)
	if key == "" {
		key, err = buildObjectKey(opts.DirPrefix, filepathExt(filename))
		if err != nil {
			return nil, err
		}
	}
	key, err = normalizeUploadObjectKey(key)
	if err != nil {
		return nil, fmt.Errorf("上传对象路径无效: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	publicURL, urlErr := adapter.ProviderURL(profile, key)
	if urlErr != nil && profile.Provider == StorageProviderLocal {
		publicURL = s.publicUploadURL(ctx, key)
		urlErr = nil
	}
	if urlErr != nil {
		return nil, urlErr
	}
	return &StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: profile.ID,
		MIMEType:       inferStoredUploadContentType(filename, nil),
	}, nil
}

func (s *StorageService) StoreRemoteRef(ctx context.Context, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.storeRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

// StoreMediaRef materializes an authorized internal upload or an external image
// into a new storage object. Internal refs are copied through the active storage
// adapter instead of being routed through the HTTP downloader.
func (s *StorageService) StoreMediaRef(ctx context.Context, sourceRef string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.storeMediaRefWithProfile(ctx, *profile, sourceRef, opts)
}

// StoreMediaRefAtProfile materializes a source into the immutable profile/key
// selected when its Asset ingestion began. Retries must not follow a newly
// activated storage profile.
func (s *StorageService) StoreMediaRefAtProfile(ctx context.Context, profileID, sourceRef string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.storeMediaRefWithProfile(ctx, *profile, sourceRef, opts)
}

func (s *StorageService) storeMediaRefWithProfile(ctx context.Context, profile StorageProfile, sourceRef string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	trimmed := strings.TrimSpace(sourceRef)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return s.storeRemoteRefWithProfile(ctx, profile, trimmed, opts)
	}

	obj, err := s.ReadUploadResourceByURL(ctx, trimmed, maxRemoteFileSize)
	if err != nil {
		return nil, err
	}
	if err := validateImageBytes(obj.Filename, obj.Data); err != nil {
		return nil, err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.StoreBytesRef(ctx, &profile, obj.Filename, obj.Data, opts)
}

func (s *StorageService) StoreBytesRef(ctx context.Context, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.StoreBytesRef(ctx, profile, filename, data, opts)
}

func (s *StorageService) storeBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.StoreBytesRef(ctx, profile, filename, data, opts)
}

func (s *StorageService) storeRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.storeRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

// ReadUploadResourceByURL resolves a structured uploads URL/path and reads it
// from the profile that owns the asset. Callers must authorize access before reading.
func (s *StorageService) ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*StoredUploadObject, error) {
	profile, key, err := s.resolveStoredUploadTarget(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.Read(ctx, profile, key, maxBytes)
}

// PutObjectAtProfile writes a deterministic derivative through the profile
// that owns the source asset. It intentionally bypasses the active profile.
func (s *StorageService) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.PutObject(ctx, profile, objectKey, data, contentType)
}

// ReadObjectAtProfile reads a derivative from its canonical storage profile.
func (s *StorageService) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*StoredUploadObject, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.Read(ctx, profile, objectKey, maxBytes)
}

func (s *StorageService) ResolveSignedURLAtProfile(ctx context.Context, profileID, objectKey string, expiry time.Duration) (string, bool, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return "", false, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	if canonicalizeStorageProvider(profile.Provider) == StorageProviderLocal {
		return "", false, nil
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", false, err
	}
	url, err := adapter.SignedURL(ctx, profile, objectKey, expiry)
	if err != nil {
		return "", true, err
	}
	return url, true, nil
}

func (s *StorageService) ResolveProviderURLAtProfile(ctx context.Context, profileID, objectKey string) (string, error) {
	profile, err := s.resolveProfileByID(ctx, profileID)
	if err != nil {
		return "", fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	return adapter.ProviderURL(profile, objectKey)
}

// ResolveProviderAssetURL resolves an immutable Asset fact through its owning
// Storage Profile. It never follows the current profile selected by operators.
func (s *StorageService) ResolveProviderAssetURL(ctx context.Context, assetID int64) (string, error) {
	if s == nil || s.assetLocator == nil || assetID <= 0 {
		return "", fmt.Errorf("媒体资产解析器未配置")
	}
	location, found, err := s.assetLocator.LocateStoredUpload(ctx, StoredUploadLookup{AssetID: assetID})
	if err != nil {
		return "", fmt.Errorf("定位媒体资产失败: %w", err)
	}
	if !found {
		return "", fmt.Errorf("媒体资产不存在或已不可用")
	}
	key, err := normalizeUploadObjectKey(location.StorageKey)
	if err != nil {
		return "", fmt.Errorf("媒体资产存储路径无效: %w", err)
	}
	profile, err := s.resolveProfileByID(ctx, location.StorageProfileID)
	if err != nil {
		return "", fmt.Errorf("读取媒体资产存储配置失败: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	resolved, err := adapter.ProviderURL(profile, key)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(resolved))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("媒体资产未配置 Provider 可访问的 HTTPS URL")
	}
	return parsed.String(), nil
}

// ResolveProviderDerivativeURL resolves a ready derivative through the
// immutable storage profile recorded on that derivative. There is deliberately
// no fallback to the original Asset: processing endpoints must receive the
// bounded processing-input representation.
func (s *StorageService) ResolveProviderDerivativeURL(ctx context.Context, assetID int64, tier string) (string, error) {
	if s == nil || s.assetLocator == nil || assetID <= 0 || strings.TrimSpace(tier) == "" {
		return "", fmt.Errorf("媒体衍生图解析器未配置")
	}
	locator, ok := s.assetLocator.(StoredDerivativeLocator)
	if !ok {
		return "", fmt.Errorf("媒体衍生图解析器未配置")
	}
	location, found, err := locator.LocateStoredDerivative(ctx, StoredDerivativeLookup{AssetID: assetID, Tier: strings.TrimSpace(tier)})
	if err != nil {
		return "", fmt.Errorf("定位媒体衍生图失败: %w", err)
	}
	if !found {
		return "", fmt.Errorf("媒体衍生图尚未就绪")
	}
	key, err := normalizeUploadObjectKey(location.StorageKey)
	if err != nil {
		return "", fmt.Errorf("媒体衍生图存储路径无效: %w", err)
	}
	profile, err := s.resolveProfileByID(ctx, location.StorageProfileID)
	if err != nil {
		return "", fmt.Errorf("读取媒体衍生图存储配置失败: %w", err)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	resolved, err := adapter.ProviderURL(profile, key)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(resolved))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("媒体衍生图未配置 Provider 可访问的 HTTPS URL")
	}
	return parsed.String(), nil
}

func (s *StorageService) DeleteProfileAvatarByURL(ctx context.Context, storageUID string, rawURL string) error {
	expectedPrefix, err := ProfileAvatarUploadObjectPrefix(storageUID)
	if err != nil {
		return err
	}

	return s.deleteUserUploadByURL(ctx, rawURL, func(resource UploadResource) error {
		if resource.Kind != UploadResourceKindProfileAvatar || resource.ProfileAvatar == nil || !strings.HasPrefix(resource.ObjectKey, expectedPrefix) {
			return fmt.Errorf("仅允许删除当前用户头像文件")
		}
		return nil
	})
}

func (s *StorageService) deleteUserUploadByURL(ctx context.Context, rawURL string, validateResource func(UploadResource) error) error {
	profile, key, err := s.resolveStoredUploadTarget(ctx, rawURL)
	if err != nil {
		return err
	}
	resource, err := ParseUploadResource(key)
	if err != nil {
		return err
	}
	if err := validateResource(resource); err != nil {
		return err
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.Delete(ctx, profile, key)
}

func (s *StorageService) DeleteCanvasByStorageUID(ctx context.Context, storageUID string) error {
	prefix, err := CanvasUserUploadObjectPrefix(storageUID)
	if err != nil {
		return err
	}
	return s.deleteUploadObjectPrefix(ctx, prefix)
}

func (s *StorageService) DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error {
	prefixes, err := userUploadObjectPrefixes(userID, storageUID)
	if err != nil {
		return err
	}
	return s.deleteUploadObjectPrefixes(ctx, prefixes)
}

func (s *StorageService) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.Delete(ctx, profile, objectKey)
}

// DeleteObjectByStorageProfile removes an object from the profile recorded with
// the asset. Reclamation must not follow the current profile because operators
// can switch storage after an asset is created.
func (s *StorageService) DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error {
	profile, err := s.loadProfileForReclamation(ctx, strings.TrimSpace(storageProfileID))
	if err != nil {
		return err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.Delete(ctx, profile, objectKey)
}

func (s *StorageService) ListObjectKeysByStorageProfile(ctx context.Context, storageProfileID string, objectKeyPrefix string) ([]string, error) {
	profile, err := s.loadProfileForReclamation(ctx, strings.TrimSpace(storageProfileID))
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.ListObjectKeysByPrefix(ctx, profile, objectKeyPrefix)
}

func (s *StorageService) deleteUploadObjectPrefix(ctx context.Context, prefix string) error {
	return s.deleteUploadObjectPrefixes(ctx, []string{prefix})
}

func (s *StorageService) deleteUploadObjectPrefixes(ctx context.Context, prefixes []string) error {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return err
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	for _, prefix := range prefixes {
		normalizedPrefix, err := normalizeUploadObjectPrefix(prefix)
		if err != nil {
			return err
		}
		if err := adapter.DeleteByPrefix(ctx, profile, normalizedPrefix); err != nil {
			return err
		}
	}
	return nil
}

func (s *StorageService) DeleteCanvasBoardByStorageUID(ctx context.Context, storageUID string, boardID string) error {
	prefix, err := CanvasBoardUploadObjectPrefix(storageUID, boardID)
	if err != nil {
		return err
	}

	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return err
	}

	normalizedPrefix, err := normalizeUploadObjectPrefix(prefix)
	if err != nil {
		return err
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.DeleteByPrefix(ctx, profile, normalizedPrefix)
}

func (s *StorageService) UsesObjectStorage(ctx context.Context) bool {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil || profile == nil {
		return false
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return false
	}
	return adapter.UsesObjectStorage()
}

func (s *StorageService) ensureProfileReadyForRuntime(ctx context.Context, profile StorageProfile) error {
	switch profile.Provider {
	case StorageProviderLocal:
		return nil
	case StorageProviderS3Compatible:
		cfg := s.s3CompatibleConfigFromProfile(&profile)
		if !s.s3CompatibleService.IsConfigEnabled(cfg) {
			return fmt.Errorf("请先完整填写并启用 S3 兼容存储")
		}
		return s.s3CompatibleService.ProbeCapabilitiesWithConfig(ctx, cfg)
	default:
		return fmt.Errorf("不支持的存储类型")
	}
}

func (s *StorageService) resolveActiveProfile(ctx context.Context) (*StorageProfile, error) {
	profiles, err := s.loadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}

	for _, profile := range profiles {
		if !profile.IsCurrent {
			continue
		}
		if s.isProfileReady(profile) {
			return &profile, nil
		}
		return nil, fmt.Errorf("当前存储 %q 的 active secret version 不可用或配置不完整", profile.ID)
	}

	local, err := s.findProfile(profiles, builtinLocalProfileID())
	if err != nil {
		return nil, err
	}
	return local, nil
}

func (s *StorageService) resolveStoredUploadTarget(ctx context.Context, rawURL string) (*StorageProfile, string, error) {
	key, err := uploadObjectKeyFromURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	if _, err := ParseUploadResource(key); err != nil {
		return nil, "", err
	}

	if s.assetLocator != nil {
		location, found, err := s.assetLocator.LocateStoredUpload(ctx, StoredUploadLookup{
			URL:       strings.TrimSpace(rawURL),
			ObjectKey: key,
		})
		if err != nil {
			return nil, "", fmt.Errorf("定位媒体资产失败: %w", err)
		}
		if found {
			locatedKey, err := normalizeUploadObjectKey(location.StorageKey)
			if err != nil {
				return nil, "", fmt.Errorf("媒体资产存储路径无效: %w", err)
			}
			if locatedKey != key {
				return nil, "", fmt.Errorf("媒体资产存储路径与请求不一致")
			}
			profile, err := s.resolveProfileByID(ctx, location.StorageProfileID)
			if err != nil {
				return nil, "", fmt.Errorf("读取媒体资产存储配置失败: %w", err)
			}
			return profile, locatedKey, nil
		}
	}

	profile, err := s.resolveProfileFromUploadURL(ctx, rawURL, key)
	if err != nil {
		return nil, "", err
	}
	return profile, key, nil
}

func (s *StorageService) resolveProfileByID(ctx context.Context, profileID string) (*StorageProfile, error) {
	profiles, err := s.loadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}
	return s.findProfile(profiles, strings.TrimSpace(profileID))
}

func (s *StorageService) resolveProfileFromUploadURL(ctx context.Context, rawURL string, key string) (*StorageProfile, error) {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "/") {
		return s.resolveProfileByID(ctx, builtinLocalProfileID())
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("图片地址格式错误")
	}
	profiles, err := s.loadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}
	var matched *StorageProfile
	for index := range profiles {
		profile := &profiles[index]
		if profile.Provider != StorageProviderS3Compatible {
			continue
		}
		candidate, candidateErr := s.extractObjectKeyFromUserUploadURL(*profile, trimmed)
		if candidateErr != nil || candidate != key {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("图片地址匹配多个存储配置")
		}
		matched = profile
	}
	if matched == nil {
		return nil, fmt.Errorf("未找到图片所属的存储配置")
	}
	return matched, nil
}

func uploadObjectKeyFromURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("图片地址不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("图片地址格式错误")
	}
	if parsed.Scheme != "" {
		scheme := strings.ToLower(parsed.Scheme)
		if (scheme != "http" && scheme != "https") || parsed.Host == "" {
			return "", fmt.Errorf("图片地址格式错误")
		}
	}
	pathValue := "/" + strings.TrimLeft(parsed.Path, "/")
	markerIndex := strings.Index(pathValue, "/uploads/")
	if markerIndex < 0 {
		return "", fmt.Errorf("不支持的上传资源路径")
	}
	return normalizeUploadObjectKey(strings.TrimPrefix(pathValue[markerIndex:], "/"))
}

func (s *StorageService) isProfileReady(profile StorageProfile) bool {
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return false
	}
	return adapter.IsProfileReady(profile)
}

func (s *StorageService) uploadRefWithProfile(ctx context.Context, profile StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	facts, err := canonicalizeMultipartImageWithFacts(file)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.ObjectKey) != "" {
		opts.ObjectKey = canonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.UploadRef(ctx, &profile, file, opts)
}

func canonicalObjectKeyExtension(objectKey, ext string) string {
	trimmed := strings.TrimSpace(objectKey)
	if trimmed == "" || ext == "" {
		return trimmed
	}
	base := path.Base(trimmed)
	if base == "." || base == "" || base == ".." {
		return trimmed
	}
	base = strings.TrimSuffix(base, path.Ext(base)) + ext
	return path.Join(path.Dir(trimmed), base)
}

func canonicalizeMultipartImageWithFacts(file *multipart.FileHeader) (imageformat.Facts, error) {
	facts, err := imageformat.InspectFileHeader(file)
	if err != nil {
		return imageformat.Facts{}, fmt.Errorf("图片校验失败: %w", err)
	}
	if file == nil {
		return imageformat.Facts{}, fmt.Errorf("文件内容不合法")
	}
	file.Filename = canonicalStorageFilename(file.Filename, facts.Extension)
	if file.Header == nil {
		file.Header = make(textproto.MIMEHeader)
	}
	file.Header.Set("Content-Type", facts.MIMEType)
	return facts, nil
}

func canonicalStorageFilename(filename, ext string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "image"
	}
	oldExt := filepath.Ext(base)
	if oldExt != "" {
		base = strings.TrimSuffix(base, oldExt)
	}
	if base == "" {
		base = "image"
	}
	return base + ext
}

func (s *StorageService) storeRemoteRefWithProfile(ctx context.Context, profile StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	if strings.HasPrefix(remoteURL, "data:image/") {
		filename, data, err := decodeDataURL(remoteURL)
		if err != nil {
			return nil, err
		}
		if err := validateImageBytes(filename, data); err != nil {
			return nil, err
		}
		adapter, err := s.adapterRegistry.get(profile.Provider)
		if err != nil {
			return nil, err
		}
		return adapter.StoreBytesRef(ctx, &profile, filename, data, opts)
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.StoreRemoteRef(ctx, &profile, remoteURL, opts)
}

func (s *StorageService) saveUploadedFileLocalRef(ctx context.Context, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	return s.saveReaderLocalRef(ctx, file.Filename, file.Header.Get("Content-Type"), src, file.Size, opts)
}

func (s *StorageService) saveBytesLocalRef(ctx context.Context, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error) {
	facts, err := imageformat.InspectDetectedBytes(data)
	if err != nil {
		return nil, fmt.Errorf("读取图片信息失败: %w", err)
	}
	filename = canonicalStorageFilename(filename, facts.Extension)
	opts.ObjectKey = canonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)
	ref, err := s.saveReaderLocalRef(ctx, filename, facts.MIMEType, bytes.NewReader(data), int64(len(data)), opts)
	if err != nil {
		return nil, err
	}
	ref.Width = facts.Width
	ref.Height = facts.Height
	ref.Animated = facts.Animated
	return ref, nil
}

func (s *StorageService) saveReaderLocalRef(ctx context.Context, filename string, contentType string, src io.Reader, size int64, opts StorageStoreOptions) (*StoredUploadRef, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	objectKey := strings.TrimSpace(opts.ObjectKey)
	var err error
	if objectKey == "" {
		objectKey, err = buildObjectKey(opts.DirPrefix, ext)
	}
	if err != nil {
		return nil, err
	}
	objectKey, err = normalizeUploadObjectKey(objectKey)
	if err != nil {
		return nil, fmt.Errorf("上传对象路径无效: %w", err)
	}
	dst := filepath.FromSlash(objectKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}
	cleanupDst := true
	defer func() {
		if cleanupDst {
			_ = os.Remove(dst)
		}
	}()
	hash := sha256.New()
	if err := copyExactWithStorageBuffer(io.MultiWriter(dstFile, hash), src, size); err != nil {
		_ = dstFile.Close()
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}
	if err := dstFile.Close(); err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}
	cleanupDst = false
	if strings.TrimSpace(contentType) == "" {
		contentType = inferStoredUploadContentType(filename, nil)
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	return &StoredUploadRef{
		URL:            s.publicUploadURL(ctx, objectKey),
		StorageKey:     objectKey,
		StorageProfile: builtinLocalProfileID(),
		MIMEType:       contentType,
		SizeBytes:      size,
		ChecksumSHA256: hex.EncodeToString(hash.Sum(nil)),
		Animated:       strings.EqualFold(ext, ".gif"),
	}, nil
}

func (s *StorageService) putLocalObject(objectKey string, data []byte) error {
	normalized, err := normalizeUploadObjectKey(objectKey)
	if err != nil {
		return err
	}
	destination := filepath.FromSlash(normalized)
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return fmt.Errorf("创建派生图片目录失败: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".preview-*")
	if err != nil {
		return fmt.Errorf("创建派生图片临时文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置派生图片权限失败: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("保存派生图片失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("保存派生图片失败: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("发布派生图片失败: %w", err)
	}
	return nil
}

type storageCopyWriter struct {
	dst io.Writer
}

func (w storageCopyWriter) Write(p []byte) (int, error) {
	return w.dst.Write(p)
}

func copyWithStorageBuffer(dst io.Writer, src io.Reader) (int64, error) {
	bufPtr := storageCopyBufferPool.Get().(*[]byte)
	defer storageCopyBufferPool.Put(bufPtr)

	return io.CopyBuffer(storageCopyWriter{dst: dst}, src, *bufPtr)
}

func copyExactWithStorageBuffer(dst io.Writer, src io.Reader, size int64) error {
	if size < 0 {
		_, err := copyWithStorageBuffer(dst, src)
		return err
	}

	written, err := copyWithStorageBuffer(dst, io.LimitReader(src, size))
	if err != nil {
		return err
	}
	if written != size {
		return io.EOF
	}
	return nil
}

func (s *StorageService) publicUploadURL(ctx context.Context, objectKey string) string {
	if s == nil {
		return ""
	}
	if s.publicAPIOrigin != "" {
		return s.publicAPIOrigin + publicUploadURL(objectKey)
	}
	return publicUploadURL(objectKey)
}

func publicUploadURL(objectKey string) string {
	normalized := "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
	return normalized
}

func (s *StorageService) deleteLocalByObjectKey(objectKey string) error {
	normalized, err := normalizeUploadObjectKey(objectKey)
	if err != nil {
		return err
	}

	localPath := filepath.FromSlash(normalized)
	if err := os.Remove(localPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("删除文件失败: %w", err)
	}
	cleanupEmptyLocalUploadParentDirs(localPath)
	return nil
}

func cleanupEmptyLocalUploadParentDirs(localPath string) {
	root := filepath.Clean(uploadObjectRoot)
	dir := filepath.Clean(filepath.Dir(localPath))
	for dir != "." && dir != string(filepath.Separator) && dir != root {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func (s *StorageService) readLocalByObjectKey(objectKey string, maxBytes int64) (*StoredUploadObject, error) {
	normalized, err := normalizeUploadObjectKey(objectKey)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.FromSlash(normalized))
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	defer file.Close()
	data, err := readStorageObject(file, maxBytes)
	if err != nil {
		return nil, err
	}
	return &StoredUploadObject{
		ObjectKey:   normalized,
		Filename:    path.Base(normalized),
		ContentType: inferStoredUploadContentType(path.Base(normalized), data),
		Data:        data,
	}, nil
}

func (s *StorageService) deleteLocalByPrefix(prefix string) error {
	normalizedPrefix, err := normalizeUploadObjectPrefix(prefix)
	if err != nil {
		return err
	}
	localPath := filepath.FromSlash(strings.TrimSuffix(normalizedPrefix, "/"))
	if err := os.RemoveAll(localPath); err != nil {
		return fmt.Errorf("按前缀删除本地文件失败: %w", err)
	}
	return nil
}

func (s *StorageService) extractObjectKeyFromUserUploadURL(profile StorageProfile, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("图片地址不能为空")
	}

	if strings.HasPrefix(trimmed, "/") {
		return normalizeUploadObjectKey(strings.TrimPrefix(trimmed, "/"))
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("图片地址格式错误")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("图片地址格式错误")
	}

	switch canonicalizeStorageProvider(profile.Provider) {
	case StorageProviderLocal:
		return normalizeUploadObjectKey(strings.TrimPrefix(parsed.Path, "/"))
	case StorageProviderS3Compatible:
		cfg := s.s3CompatibleConfigFromProfile(&profile)
		return extractObjectKeyFromAccessDomainURL(cfg.AccessDomain, parsed)
	default:
		return "", fmt.Errorf("不支持的存储类型")
	}
}

func extractObjectKeyFromAccessDomainURL(accessDomain string, parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("图片地址格式错误")
	}
	base, err := normalizePublicDomain(accessDomain)
	if err != nil {
		return "", fmt.Errorf("访问域名配置无效")
	}
	baseURL, err := url.Parse(base)
	if err != nil || baseURL.Host == "" {
		return "", fmt.Errorf("访问域名配置无效")
	}
	if !strings.EqualFold(parsed.Hostname(), baseURL.Hostname()) {
		return "", fmt.Errorf("仅允许删除当前存储域名下的文件")
	}

	relativePath := parsed.Path
	basePath := strings.TrimSuffix(baseURL.EscapedPath(), "/")
	if basePath != "" && basePath != "/" {
		basePrefix := basePath + "/"
		if strings.HasPrefix(relativePath, basePrefix) {
			relativePath = strings.TrimPrefix(relativePath, basePrefix)
		} else if relativePath == basePath {
			relativePath = ""
		}
	}

	candidate := strings.TrimPrefix(relativePath, "/")
	return normalizeUploadObjectKey(candidate)
}

func readStorageObject(src io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > maxRemoteFileSize {
		maxBytes = maxRemoteFileSize
	}
	data, err := io.ReadAll(io.LimitReader(src, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("文件超过大小限制 (%d MB)", maxBytes/1024/1024)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("文件内容为空")
	}
	return data, nil
}

func inferStoredUploadContentType(filename string, data []byte) string {
	if ext := imageformat.DetectImageExtensionFromBytes(data); ext != "" {
		return imageformat.ImageMIMEFromExtension(ext)
	}
	return imageformat.ImageMIMEFromExtension(filepath.Ext(filename))
}

type remoteImageFile struct {
	Filename    string
	ContentType string
	Width       int
	Height      int
	Animated    bool
	Size        int64

	file *os.File
	path string
}

func (f *remoteImageFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *remoteImageFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *remoteImageFile) Cleanup() error {
	if f == nil {
		return nil
	}
	var closeErr error
	if f.file != nil {
		closeErr = f.file.Close()
		f.file = nil
	}
	var removeErr error
	if f.path != "" {
		removeErr = os.Remove(f.path)
		f.path = ""
	}
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func (s *StorageService) downloadRemoteFile(ctx context.Context, remoteURL string, allowHosts []string) (*remoteImageFile, error) {
	return s.downloadRemoteFileWithLimit(ctx, remoteURL, allowHosts, maxRemoteFileSize)
}

func (s *StorageService) downloadRemoteFileWithLimit(ctx context.Context, remoteURL string, allowHosts []string, maxSize int64) (*remoteImageFile, error) {
	if maxSize <= 0 {
		maxSize = maxRemoteFileSize
	}
	resp, err := safehttp.GetWithAllowHosts(ctx, remoteURL, 30*time.Second, allowHosts...)
	if err != nil {
		return nil, fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "ideaboard-remote-image-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
	}()

	size, err := copyWithStorageBuffer(tmpFile, io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片数据失败: %w", err)
	}
	if size > maxSize {
		return nil, fmt.Errorf("远程文件超过大小限制 (%d MB)", maxSize/1024/1024)
	}

	// The remote URL and Content-Type are advisory only. Providers commonly
	// return a negotiated format under a stale filename (for example a JPEG at
	// a `.png` URL), so derive the canonical extension from the bytes before
	// constructing the managed object key.
	facts, err := imageformat.InspectDetected(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("远程图片校验失败: %w", err)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("读取临时文件失败: %w", err)
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), facts.Extension)
	cleanupTemp = false
	return &remoteImageFile{
		Filename:    filename,
		ContentType: facts.MIMEType,
		Width:       facts.Width,
		Height:      facts.Height,
		Animated:    facts.Animated,
		Size:        size,
		file:        tmpFile,
		path:        tmpFile.Name(),
	}, nil
}

func validateImageBytes(filename string, data []byte) error {
	ext, ok := imageformat.NormalizeImageExtension(filepath.Ext(filename))
	if !ok {
		return fmt.Errorf("仅支持 %s 图片", imageformat.SupportedUploadImageLabel())
	}
	if _, err := imageformat.InspectBytes(data, ext); err != nil {
		return fmt.Errorf("图片校验失败: %w", err)
	}
	return nil
}

func decodeDataURL(dataURL string) (string, []byte, error) {
	parsed, err := mediarefs.ParseDataImageURL(dataURL, maxRemoteFileSize)
	if err != nil {
		return "", nil, fmt.Errorf("无效的 base64 图片格式")
	}
	return fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), parsed.Ext), parsed.Data, nil
}
