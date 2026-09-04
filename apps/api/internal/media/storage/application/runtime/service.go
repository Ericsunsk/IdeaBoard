package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/media/storage/application/profiles"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

const maxRemoteFileSize = 50 * 1024 * 1024

type Service struct {
	profiles        *profiles.Service
	drivers         profiles.DriverResolver
	assetLocator    ports.StoredUploadLocator
	publicAPIOrigin string
}

func NewService(
	profiles *profiles.Service,
	drivers profiles.DriverResolver,
	assetLocator ports.StoredUploadLocator,
	publicAPIOrigin string,
) *Service {
	return &Service{
		profiles:        profiles,
		drivers:         drivers,
		assetLocator:    assetLocator,
		publicAPIOrigin: strings.TrimRight(strings.TrimSpace(publicAPIOrigin), "/"),
	}
}

func (s *Service) SetAssetLocator(locator ports.StoredUploadLocator) {
	s.assetLocator = locator
}

func (s *Service) Profiles() *profiles.Service {
	return s.profiles
}

func (s *Service) Upload(ctx context.Context, file *multipart.FileHeader) (string, error) {
	return s.UploadWithOptions(ctx, file, ports.StorageStoreOptions{})
}

func (s *Service) UploadWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (string, error) {
	ref, err := s.UploadRefWithOptions(ctx, file, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *Service) UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.UploadRefWithProfile(ctx, *profile, file, opts)
}

func (s *Service) UploadRefAtProfile(ctx context.Context, profileID string, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.UploadRefWithProfile(ctx, *profile, file, opts)
}

func (s *Service) PrepareUploadRefFromFile(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
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
		key, err = domain.BuildObjectKey(opts.DirPrefix, facts.Extension)
		if err != nil {
			return nil, err
		}
	} else {
		key = domain.CanonicalObjectKeyExtension(key, facts.Extension)
	}
	key, err = domain.NormalizeUploadObjectKey(key)
	if err != nil {
		return nil, fmt.Errorf("上传对象路径无效: %w", err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	publicURL, urlErr := driver.ProviderURL(profile, key)
	if urlErr != nil && profile.Provider == domain.StorageProviderLocal {
		publicURL = s.PublicUploadURL(key)
		urlErr = nil
	}
	if urlErr != nil {
		return nil, urlErr
	}
	return &ports.StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: profile.ID,
		MIMEType:       facts.MIMEType,
		Width:          facts.Width,
		Height:         facts.Height,
		SizeBytes:      file.Size,
		ChecksumSHA256: checksum,
		Animated:       facts.Animated,
	}, nil
}

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
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Service) PrepareUploadRef(ctx context.Context, filename string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(opts.ObjectKey)
	if key == "" {
		key, err = domain.BuildObjectKey(opts.DirPrefix, filepath.Ext(filename))
		if err != nil {
			return nil, err
		}
	}
	key, err = domain.NormalizeUploadObjectKey(key)
	if err != nil {
		return nil, fmt.Errorf("上传对象路径无效: %w", err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	publicURL, urlErr := driver.ProviderURL(profile, key)
	if urlErr != nil && profile.Provider == domain.StorageProviderLocal {
		publicURL = s.PublicUploadURL(key)
		urlErr = nil
	}
	if urlErr != nil {
		return nil, urlErr
	}
	return &ports.StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: profile.ID,
		MIMEType:       inferStoredUploadContentType(filename, nil),
	}, nil
}

func (s *Service) StoreRemoteRef(ctx context.Context, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.StoreRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *Service) StoreMediaRef(ctx context.Context, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.StoreMediaRefWithProfile(ctx, *profile, sourceRef, opts)
}

func (s *Service) StoreMediaRefAtProfile(ctx context.Context, profileID, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.StoreMediaRefWithProfile(ctx, *profile, sourceRef, opts)
}

func (s *Service) StoreMediaRefWithProfile(ctx context.Context, profile domain.StorageProfile, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	trimmed := strings.TrimSpace(sourceRef)
	if strings.HasPrefix(trimmed, "data:") {
		filename, data, err := decodeDataURL(trimmed)
		if err != nil {
			return nil, err
		}
		return s.StoreBytesRefWithProfile(ctx, profile, filename, data, opts)
	}
	return s.StoreRemoteRefWithProfile(ctx, profile, trimmed, opts)
}

func (s *Service) StoreBytesRef(ctx context.Context, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.StoreBytesRefWithProfile(ctx, *profile, filename, data, opts)
}

func (s *Service) StoreBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.StoreBytesRefWithProfile(ctx, *profile, filename, data, opts)
}

func (s *Service) StoreBytesRefWithProfile(ctx context.Context, profile domain.StorageProfile, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.StoreBytesRef(ctx, &profile, filename, data, opts)
}

func (s *Service) StoreRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取媒体存储配置失败: %w", err)
	}
	return s.StoreRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *Service) StoreRemoteRefWithProfile(ctx context.Context, profile domain.StorageProfile, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.StoreRemoteRef(ctx, &profile, remoteURL, opts)
}

func (s *Service) UploadRefWithProfile(ctx context.Context, profile domain.StorageProfile, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	facts, err := canonicalizeMultipartImageWithFacts(file)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.ObjectKey) != "" {
		opts.ObjectKey = domain.CanonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.UploadRef(ctx, &profile, file, opts)
}

func (s *Service) ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*ports.StoredUploadObject, error) {
	profile, key, err := s.ResolveStoredUploadTarget(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.Read(ctx, profile, key, maxBytes)
}

func (s *Service) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return fmt.Errorf("读取存储配置失败: %w", err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.PutObject(ctx, profile, objectKey, data, contentType)
}

func (s *Service) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("读取存储配置失败: %w", err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.Read(ctx, profile, objectKey, maxBytes)
}

func (s *Service) ResolveProviderDerivativeURL(ctx context.Context, assetID int64, tier string) (string, error) {
	if s == nil || s.assetLocator == nil || assetID <= 0 || strings.TrimSpace(tier) == "" {
		return "", fmt.Errorf("媒体衍生图解析器未配置")
	}
	locator, ok := s.assetLocator.(ports.StoredDerivativeLocator)
	if !ok {
		return "", fmt.Errorf("媒体衍生图解析器未配置")
	}
	location, found, err := locator.LocateStoredDerivative(ctx, ports.StoredDerivativeLookup{AssetID: assetID, Tier: strings.TrimSpace(tier)})
	if err != nil {
		return "", fmt.Errorf("定位媒体衍生图失败: %w", err)
	}
	if !found {
		return "", fmt.Errorf("媒体衍生图尚未就绪")
	}
	key, err := domain.NormalizeUploadObjectKey(location.StorageKey)
	if err != nil {
		return "", fmt.Errorf("媒体衍生图存储路径无效: %w", err)
	}
	profile, err := s.profiles.ResolveProfileByID(ctx, location.StorageProfileID)
	if err != nil {
		return "", fmt.Errorf("读取媒体衍生图存储配置失败: %w", err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return "", err
	}
	resolved, err := driver.ProviderURL(profile, key)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(resolved))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", domain.ErrProviderHTTPSUnavailable
	}
	return parsed.String(), nil
}

func (s *Service) DeleteProfileAvatarByURL(ctx context.Context, storageUID string, rawURL string) error {
	return s.deleteUserUploadByURL(ctx, rawURL, func(resource domain.UploadResource) error {
		if resource.Kind != domain.UploadResourceKindProfileAvatar || resource.ProfileAvatar == nil {
			return domain.ErrUploadAccessInvalidPath
		}
		if resource.ProfileAvatar.OwnerSegment != storageUID {
			return domain.ErrUploadAccessOwnerMismatch
		}
		return nil
	})
}

func (s *Service) deleteUserUploadByURL(ctx context.Context, rawURL string, validateResource func(domain.UploadResource) error) error {
	profile, key, err := s.ResolveStoredUploadTarget(ctx, rawURL)
	if err != nil {
		return err
	}
	resource, err := domain.ParseUploadResource(key)
	if err != nil {
		return err
	}
	if validateResource != nil {
		if err := validateResource(resource); err != nil {
			return err
		}
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.Delete(ctx, profile, key)
}

func (s *Service) DeleteCanvasByStorageUID(ctx context.Context, storageUID string) error {
	prefix, err := domain.CanvasUserUploadObjectPrefix(storageUID)
	if err != nil {
		return err
	}
	return s.DeleteUploadObjectPrefix(ctx, prefix)
}

func (s *Service) DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error {
	prefixes, err := domain.UserUploadObjectPrefixes(userID, storageUID)
	if err != nil {
		return err
	}
	return s.DeleteUploadObjectPrefixes(ctx, prefixes)
}

func (s *Service) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.Delete(ctx, profile, objectKey)
}

func (s *Service) DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error {
	profile, err := s.profiles.ResolveProfileByID(ctx, storageProfileID)
	if err != nil {
		return err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.Delete(ctx, profile, objectKey)
}

func (s *Service) ListObjectKeysByStorageProfile(ctx context.Context, storageProfileID string, objectKeyPrefix string) ([]string, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, storageProfileID)
	if err != nil {
		return nil, err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.ListObjectKeysByPrefix(ctx, profile, objectKeyPrefix)
}

func (s *Service) DeleteUploadObjectPrefix(ctx context.Context, prefix string) error {
	return s.DeleteUploadObjectPrefixes(ctx, []string{prefix})
}

func (s *Service) DeleteUploadObjectPrefixes(ctx context.Context, prefixes []string) error {
	profiles, err := s.profiles.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, prefix := range prefixes {
		if strings.TrimSpace(prefix) == "" {
			continue
		}
		for _, profile := range profiles {
			driver, err := s.drivers.GetDriver(profile.Provider)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if err := driver.DeleteByPrefix(ctx, &profile, prefix); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Service) DeleteCanvasBoardByStorageUID(ctx context.Context, storageUID string, boardID string) error {
	prefix, err := domain.CanvasBoardUploadObjectPrefix(storageUID, boardID)
	if err != nil {
		return err
	}
	return s.DeleteUploadObjectPrefix(ctx, prefix)
}

func (s *Service) UsesObjectStorage(ctx context.Context) bool {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return false
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return false
	}
	return driver.UsesObjectStorage()
}

func (s *Service) ResolveStoredUploadTarget(ctx context.Context, rawURL string) (*domain.StorageProfile, string, error) {
	key, err := uploadObjectKeyFromURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	if _, err := domain.ParseUploadResource(key); err != nil {
		return nil, "", err
	}

	if s.assetLocator != nil {
		location, found, err := s.assetLocator.LocateStoredUpload(ctx, ports.StoredUploadLookup{
			URL:       strings.TrimSpace(rawURL),
			ObjectKey: key,
		})
		if err != nil {
			return nil, "", fmt.Errorf("定位媒体资产失败: %w", err)
		}
		if found {
			locatedKey, err := domain.NormalizeUploadObjectKey(location.StorageKey)
			if err != nil {
				return nil, "", fmt.Errorf("媒体资产存储路径无效: %w", err)
			}
			if locatedKey != key {
				return nil, "", fmt.Errorf("媒体资产存储路径与请求不一致")
			}
			profile, err := s.profiles.ResolveProfileByID(ctx, location.StorageProfileID)
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

func (s *Service) resolveProfileFromUploadURL(ctx context.Context, rawURL string, key string) (*domain.StorageProfile, error) {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "/") {
		return s.profiles.ResolveProfileByID(ctx, "builtin-local")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("图片地址格式错误")
	}
	if s.isPublicLocalUploadURL(parsed) {
		return s.profiles.ResolveProfileByID(ctx, "builtin-local")
	}
	profiles, err := s.profiles.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}
	return s.findS3ProfileForUploadURL(profiles, trimmed, key)
}

func (s *Service) isPublicLocalUploadURL(candidate *url.URL) bool {
	if s == nil || s.publicAPIOrigin == "" {
		return false
	}
	if !strings.HasPrefix("/"+strings.TrimPrefix(candidate.Path, "/"), "/uploads/") {
		return false
	}
	publicOrigin, err := url.Parse(s.publicAPIOrigin)
	return err == nil &&
		strings.EqualFold(publicOrigin.Scheme, candidate.Scheme) &&
		strings.EqualFold(publicOrigin.Host, candidate.Host)
}

func (s *Service) findS3ProfileForUploadURL(profiles []domain.StorageProfile, rawURL string, key string) (*domain.StorageProfile, error) {
	var matched *domain.StorageProfile
	for index := range profiles {
		profile := &profiles[index]
		if profile.Provider != domain.StorageProviderS3Compatible {
			continue
		}
		candidate, candidateErr := extractObjectKeyFromUserUploadURL(*profile, rawURL)
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

func extractObjectKeyFromUserUploadURL(profile domain.StorageProfile, rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("图片地址不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("图片地址格式错误")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return domain.NormalizeUploadObjectKey(strings.TrimPrefix(parsed.Path, "/"))
	}
	switch profile.Provider {
	case domain.StorageProviderLocal:
		return domain.NormalizeUploadObjectKey(strings.TrimPrefix(parsed.Path, "/"))
	case domain.StorageProviderS3Compatible:
		accessDomain, _ := profile.Config["access_domain"].(string)
		return extractObjectKeyFromAccessDomainURL(accessDomain, parsed)
	default:
		return "", fmt.Errorf("不支持的存储类型")
	}
}

func extractObjectKeyFromAccessDomainURL(accessDomain string, parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("图片地址格式错误")
	}
	base := strings.TrimSpace(accessDomain)
	if !strings.Contains(base, "://") {
		base = "https://" + base
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
	return domain.NormalizeUploadObjectKey(candidate)
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
	return domain.NormalizeUploadObjectKey(strings.TrimPrefix(pathValue[markerIndex:], "/"))
}

func canonicalizeMultipartImageWithFacts(file *multipart.FileHeader) (imageformat.Facts, error) {
	facts, err := imageformat.InspectFileHeader(file)
	if err != nil {
		return imageformat.Facts{}, fmt.Errorf("图片校验失败: %w", err)
	}
	if file == nil {
		return imageformat.Facts{}, fmt.Errorf("文件内容不合法")
	}
	file.Filename = domain.CanonicalStorageFilename(file.Filename, facts.Extension)
	if file.Header == nil {
		file.Header = make(textproto.MIMEHeader)
	}
	file.Header.Set("Content-Type", facts.MIMEType)
	return facts, nil
}

func (s *Service) PublicUploadURL(objectKey string) string {
	normalized := "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
	if s.publicAPIOrigin != "" {
		return s.publicAPIOrigin + normalized
	}
	return normalized
}

func decodeDataURL(dataURL string) (string, []byte, error) {
	parsed, err := mediarefs.ParseDataImageURL(dataURL, maxRemoteFileSize)
	if err != nil {
		return "", nil, fmt.Errorf("无效的 base64 图片格式")
	}
	return fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), parsed.Ext), parsed.Data, nil
}

func inferStoredUploadContentType(filename string, data []byte) string {
	if len(data) > 0 {
		if ext := imageformat.DetectImageExtensionFromBytes(data); ext != "" {
			return imageformat.ImageMIMEFromExtension(ext)
		}
	}
	return imageformat.ImageMIMEFromExtension(filepath.Ext(filename))
}
