package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
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
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return "", err
	}
	return s.uploadWithProfile(ctx, *profile, file, opts)
}

func (s *StorageService) StoreRemote(ctx context.Context, remoteURL string, opts StorageStoreOptions) (string, error) {
	ref, err := s.StoreRemoteRef(ctx, remoteURL, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *StorageService) StoreRemoteRef(ctx context.Context, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.storeRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *StorageService) StoreBytes(ctx context.Context, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	ref, err := s.StoreBytesRef(ctx, filename, data, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
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

// ReadUploadResourceByURL resolves a structured uploads URL/path and reads it
// from the active storage profile. Callers must authorize access before reading.
func (s *StorageService) ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*StoredUploadObject, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	key, err := s.extractObjectKeyFromUserUploadURL(*profile, rawURL)
	if err != nil {
		return nil, err
	}
	if _, err := ParseUploadResource(key); err != nil {
		return nil, err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return nil, err
	}
	return adapter.Read(ctx, profile, key, maxBytes)
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
	ref, err := s.storeRemoteRefWithProfile(ctx, profile, remoteURL, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *StorageService) storeRemoteRefWithProfile(ctx context.Context, profile StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	if strings.HasPrefix(remoteURL, "data:image/") {
		filename, data, err := decodeDataURL(remoteURL)
		if err != nil {
			return nil, err
		}
		if err := validateRemoteImageBytes(filename, data); err != nil {
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

func (s *StorageService) saveUploadedFileLocal(ctx context.Context, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	return s.saveReaderLocal(ctx, file.Filename, src, file.Size, opts)
}

func (s *StorageService) saveBytesLocal(ctx context.Context, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	return s.saveReaderLocal(ctx, filename, bytes.NewReader(data), int64(len(data)), opts)
}

func (s *StorageService) saveBytesLocalRef(ctx context.Context, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error) {
	ref, err := s.saveReaderLocalRef(ctx, filename, inferStoredUploadContentType(filename, data), bytes.NewReader(data), int64(len(data)), opts)
	if err != nil {
		return nil, err
	}
	ref.Width, ref.Height, _ = media.ImageDimensions(data)
	return ref, nil
}

func (s *StorageService) saveReaderLocal(ctx context.Context, filename string, src io.Reader, size int64, opts StorageStoreOptions) (string, error) {
	ref, err := s.saveReaderLocalRef(ctx, filename, "", src, size, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *StorageService) saveReaderLocalRef(ctx context.Context, filename string, contentType string, src io.Reader, size int64, opts StorageStoreOptions) (*StoredUploadRef, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	objectKey, err := buildObjectKey(opts.DirPrefix, ext)
	if err != nil {
		return nil, err
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
	if err := copyExactWithStorageBuffer(dstFile, src, size); err != nil {
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
	}, nil
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

func publicUploadURL(ctx context.Context, objectKey string) string {
	return publicUploadURLWithProvider(ctx, nil, objectKey)
}

func (s *StorageService) publicUploadURL(ctx context.Context, objectKey string) string {
	if s == nil {
		return publicUploadURL(ctx, objectKey)
	}
	return publicUploadURLWithProvider(ctx, s.publicURLs, objectKey)
}

func publicUploadURLWithProvider(ctx context.Context, provider settings.PublicURLSettingProvider, objectKey string) string {
	normalized := "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
	base := resolvePublicUploadBaseURL(ctx, provider)
	if base == nil {
		return normalized
	}
	out := *base
	out.Path = path.Join(base.Path, normalized)
	out.RawQuery = ""
	out.Fragment = ""
	return out.String()
}

func resolvePublicUploadBaseURL(ctx context.Context, provider settings.PublicURLSettingProvider) *url.URL {
	parsed := settings.ResolvePublicAssetBaseURL(ctx, provider)
	if parsed != nil {
		logPublicUploadAuthWarningOnce.Do(func() {
			if isLikelyObjectStorageHost(parsed.Host) {
				slog.Warn("系统设置的素材访问地址指向对象存储域名，上传文件可能绕过应用鉴权被直接访问。建议使用应用自身域名或 CDN 地址。",
					"host", parsed.Host)
			}
		})
	}
	return parsed
}

var logPublicUploadAuthWarningOnce sync.Once

func isLikelyObjectStorageHost(host string) bool {
	host = strings.ToLower(host)
	for _, pattern := range []string{".s3.", ".amazonaws.com", ".digitaloceanspaces.com", ".storage.googleapis.com", ".blob.core.windows.net", ".r2.cloudflarestorage.com"} {
		if strings.Contains(host, pattern) {
			return true
		}
	}
	return false
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
	return nil
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
	if ext := media.DetectImageExtensionFromBytes(data); ext != "" {
		return media.ImageMIMEFromExtension(ext)
	}
	return media.ImageMIMEFromExtension(filepath.Ext(filename))
}

type remoteImageFile struct {
	Filename    string
	ContentType string
	Width       int
	Height      int
	Size        int64

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
		return nil, fmt.Errorf("读取临时文件失败: %w", err)
	}
	width, height, err := imageReaderDimensions(tmpFile)
	if err != nil {
		return nil, err
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("读取临时文件失败: %w", err)
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), ext)
	cleanupTemp = false
	return &remoteImageFile{
		Filename:    filename,
		ContentType: media.ImageMIMEFromExtension(ext),
		Width:       width,
		Height:      height,
		Size:        size,
		file:        tmpFile,
		path:        tmpFile.Name(),
	}, nil
}

func imageReaderDimensions(file *os.File) (int, int, error) {
	if file == nil {
		return 0, 0, fmt.Errorf("文件内容为空")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return 0, 0, fmt.Errorf("读取图片尺寸失败: %w", err)
	}
	width, height, err := media.ImageDimensions(data)
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func validateRemoteImageFile(file *os.File, ext string) error {
	if file == nil {
		return fmt.Errorf("远程文件校验失败: 文件为空")
	}
	if _, ok := media.NormalizeImageExtension(ext); !ok {
		return fmt.Errorf("仅支持 %s 图片", media.SupportedUploadImageLabel())
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("读取临时文件失败: %w", err)
	}
	if err := media.ValidateImageReader(file, ext); err != nil {
		return fmt.Errorf("远程文件校验失败: %w", err)
	}
	return nil
}

func validateRemoteImageBytes(filename string, data []byte) error {
	ext, ok := media.NormalizeImageExtension(filepath.Ext(filename))
	if !ok {
		return fmt.Errorf("仅支持 %s 图片", media.SupportedUploadImageLabel())
	}
	if err := media.ValidateImageBytes(data, ext); err != nil {
		return fmt.Errorf("远程文件校验失败: %w", err)
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
