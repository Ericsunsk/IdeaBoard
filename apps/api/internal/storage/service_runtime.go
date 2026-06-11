package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/platform/safehttp"
)

const (
	// maxRemoteFileSize caps remote downloads to prevent OOM from unbounded streams.
	maxRemoteFileSize = 100 * 1024 * 1024 // 100 MB
)

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
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return "", err
	}
	return s.uploadWithProfile(ctx, *profile, file, opts)
}

func (s *StorageService) StoreRemote(ctx context.Context, remoteURL string, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return "", err
	}
	return s.storeRemoteWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *StorageService) StoreBytes(ctx context.Context, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return "", err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	return adapter.StoreBytes(ctx, profile, filename, data, opts)
}

func (s *StorageService) DeleteWorkspaceUploadByURL(ctx context.Context, storageUID string, boardID string, rawURL string) error {
	expectedPrefix, err := WorkspaceBoardUploadObjectPrefix(storageUID, boardID)
	if err != nil {
		return err
	}

	return s.deleteUserUploadByURL(ctx, rawURL, func(resource UploadResource) error {
		if resource.Kind != UploadResourceKindWorkspace || resource.Workspace == nil || !strings.HasPrefix(resource.ObjectKey, expectedPrefix) {
			return fmt.Errorf("仅允许删除当前画布工作区上传文件")
		}
		return nil
	})
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
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return err
	}
	key, err := s.extractObjectKeyFromUserUploadURL(*profile, rawURL)
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

func (s *StorageService) DeleteWorkspaceByStorageUID(ctx context.Context, storageUID string) error {
	prefix, err := WorkspaceUserUploadObjectPrefix(storageUID)
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

func (s *StorageService) DeleteTaskOutputsByUserTask(ctx context.Context, userID int64, taskID string) error {
	prefix, err := TaskOutputUploadObjectPrefix(userID, taskID)
	if err != nil {
		return err
	}
	return s.deleteUploadObjectPrefix(ctx, prefix)
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

func (s *StorageService) DeleteWorkspaceBoardByStorageUID(ctx context.Context, storageUID string, boardID string) error {
	prefix, err := WorkspaceBoardUploadObjectPrefix(storageUID, boardID)
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

func (s *StorageService) ensureProfileReadyForRuntime(profile StorageProfile) error {
	switch profile.Provider {
	case StorageProviderLocal:
		return nil
	case StorageProviderS3Compatible:
		cfg := s.s3CompatibleConfigFromProfile(&profile)
		if !s.s3CompatibleService.IsConfigEnabled(cfg) {
			return fmt.Errorf("请先完整填写并启用 S3 兼容存储")
		}
		return nil
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
		break
	}

	local, err := s.findProfile(profiles, builtinLocalProfileID())
	if err != nil {
		return nil, err
	}
	return local, nil
}

func (s *StorageService) isProfileReady(profile StorageProfile) bool {
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return false
	}
	return adapter.IsProfileReady(profile)
}

func (s *StorageService) uploadWithProfile(ctx context.Context, profile StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	return adapter.Upload(ctx, &profile, file, opts)
}

func (s *StorageService) storeRemoteWithProfile(ctx context.Context, profile StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	if strings.HasPrefix(remoteURL, "data:image/") {
		filename, data, err := decodeDataURL(remoteURL)
		if err != nil {
			return "", err
		}
		if err := validateRemoteImageBytes(filename, data); err != nil {
			return "", err
		}
		adapter, err := s.adapterRegistry.get(profile.Provider)
		if err != nil {
			return "", err
		}
		return adapter.StoreBytes(ctx, &profile, filename, data, opts)
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	return adapter.StoreRemote(ctx, &profile, remoteURL, opts)
}

func (s *StorageService) saveUploadedFileLocal(file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	return s.saveBytesLocal(file.Filename, data, opts)
}

func (s *StorageService) saveBytesLocal(filename string, data []byte, opts StorageStoreOptions) (string, error) {
	return s.saveReaderLocal(filename, bytes.NewReader(data), int64(len(data)), opts)
}

func (s *StorageService) saveReaderLocal(filename string, src io.Reader, size int64, opts StorageStoreOptions) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	objectKey, err := buildObjectKey(opts.DirPrefix, ext)
	if err != nil {
		return "", err
	}
	dst := filepath.FromSlash(objectKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}
	cleanupDst := true
	defer func() {
		if cleanupDst {
			_ = os.Remove(dst)
		}
	}()
	if size >= 0 {
		if _, err := io.CopyN(dstFile, src, size); err != nil {
			_ = dstFile.Close()
			return "", fmt.Errorf("保存文件失败: %v", err)
		}
	} else if _, err := io.Copy(dstFile, src); err != nil {
		_ = dstFile.Close()
		return "", fmt.Errorf("保存文件失败: %v", err)
	}
	if err := dstFile.Close(); err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}
	cleanupDst = false
	return publicUploadURL(objectKey), nil
}

func publicUploadURL(objectKey string) string {
	normalized := "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
	base := resolvePublicUploadBaseURL()
	if base == nil {
		return normalized
	}
	rel, err := url.Parse(normalized)
	if err != nil {
		return normalized
	}
	return base.ResolveReference(rel).String()
}

func resolvePublicUploadBaseURL() *url.URL {
	for _, key := range []string{"APP_PUBLIC_ASSET_BASE_URL", "APP_PUBLIC_BASE_URL"} {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		return parsed
	}
	return nil
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
		return fmt.Errorf("删除文件失败: %v", err)
	}
	return nil
}

func (s *StorageService) deleteLocalByPrefix(prefix string) error {
	normalizedPrefix, err := normalizeUploadObjectPrefix(prefix)
	if err != nil {
		return err
	}
	localPath := filepath.FromSlash(strings.TrimSuffix(normalizedPrefix, "/"))
	if err := os.RemoveAll(localPath); err != nil {
		return fmt.Errorf("按前缀删除本地文件失败: %v", err)
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
		return extractObjectKeyFromAccessDomainURL(cfg.AccessDomain, parsed, strings.TrimSpace(cfg.BucketName), cfg.ForcePathStyle)
	default:
		return "", fmt.Errorf("不支持的存储类型")
	}
}

func extractObjectKeyFromAccessDomainURL(accessDomain string, parsed *url.URL, bucketName string, forcePathStyle bool) (string, error) {
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
	if forcePathStyle && strings.TrimSpace(bucketName) != "" {
		bucketPrefix := strings.Trim(strings.TrimSpace(bucketName), "/") + "/"
		candidate = strings.TrimPrefix(candidate, bucketPrefix)
	}

	return normalizeUploadObjectKey(candidate)
}

type remoteImageFile struct {
	Filename string
	Size     int64

	file *os.File
	path string
}

func (f *remoteImageFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
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
		return nil, fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "ideaboard-remote-image-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %v", err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
	}()

	size, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片数据失败: %v", err)
	}
	if size > maxSize {
		return nil, fmt.Errorf("远程文件超过大小限制 (%d MB)", maxSize/1024/1024)
	}

	ext := strings.ToLower(path.Ext(strings.Split(remoteURL, "?")[0]))
	if normalized, ok := media.NormalizeImageExtension(ext); ok {
		ext = normalized
	} else {
		ext = media.ImageExtensionFromMIME(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		return nil, fmt.Errorf("仅支持 %s 图片", media.SupportedUploadImageLabel())
	}
	if err := validateRemoteImageFile(tmpFile, ext); err != nil {
		return nil, err
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("读取临时文件失败: %v", err)
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), ext)
	cleanupTemp = false
	return &remoteImageFile{
		Filename: filename,
		Size:     size,
		file:     tmpFile,
		path:     tmpFile.Name(),
	}, nil
}

func validateRemoteImageFile(file *os.File, ext string) error {
	if file == nil {
		return fmt.Errorf("远程文件校验失败: 文件为空")
	}
	if _, ok := media.NormalizeImageExtension(ext); !ok {
		return fmt.Errorf("仅支持 %s 图片", media.SupportedUploadImageLabel())
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("读取临时文件失败: %v", err)
	}
	if err := media.ValidateImageReader(file, ext); err != nil {
		return fmt.Errorf("远程文件校验失败: %v", err)
	}
	return nil
}

func validateRemoteImageBytes(filename string, data []byte) error {
	ext, ok := media.NormalizeImageExtension(filepath.Ext(filename))
	if !ok {
		return fmt.Errorf("仅支持 %s 图片", media.SupportedUploadImageLabel())
	}
	if err := media.ValidateImageBytes(data, ext); err != nil {
		return fmt.Errorf("远程文件校验失败: %v", err)
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
