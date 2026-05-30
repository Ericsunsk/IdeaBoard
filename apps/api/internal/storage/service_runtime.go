package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"api/internal/platform/media"
	"api/internal/platform/mediarefs"
	"api/internal/platform/safehttp"
)

const (
	// maxRemoteFileSize caps remote downloads to prevent OOM from unbounded streams.
	maxRemoteFileSize = 100 * 1024 * 1024 // 100 MB
)

func (s *StorageService) TestProfile(ctx context.Context, id string) error {
	profiles, err := s.loadProfiles(ctx)
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
	return adapter.TestProfile(profile)
}

func (s *StorageService) GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error) {
	profiles, err := s.loadProfiles(ctx)
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
	return adapter.GetBucketInfo(profile)
}

func (s *StorageService) Upload(ctx context.Context, file *multipart.FileHeader) (string, error) {
	return s.UploadWithOptions(ctx, file, StorageStoreOptions{})
}

func (s *StorageService) UploadWithOptions(ctx context.Context, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile(ctx)
	if err != nil {
		return "", err
	}
	return s.uploadWithProfile(*profile, file, opts)
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
	return adapter.StoreBytes(profile, filename, data, opts)
}

func (s *StorageService) DeleteWorkspaceUploadByURL(ctx context.Context, storageUID string, boardID string, rawURL string) error {
	expectedPrefix, err := WorkspaceBoardUploadObjectPrefix(storageUID, boardID)
	if err != nil {
		return err
	}

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
	if resource.Kind != UploadResourceKindWorkspace || resource.Workspace == nil || !strings.HasPrefix(resource.ObjectKey, expectedPrefix) {
		return fmt.Errorf("仅允许删除当前画布工作区上传文件")
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.Delete(profile, key)
}

func (s *StorageService) DeleteProfileAvatarByURL(ctx context.Context, storageUID string, rawURL string) error {
	expectedPrefix, err := ProfileAvatarUploadObjectPrefix(storageUID)
	if err != nil {
		return err
	}

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
	if resource.Kind != UploadResourceKindProfileAvatar || resource.ProfileAvatar == nil || !strings.HasPrefix(resource.ObjectKey, expectedPrefix) {
		return fmt.Errorf("仅允许删除当前用户头像文件")
	}

	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return err
	}
	return adapter.Delete(profile, key)
}

func (s *StorageService) DeleteWorkspaceByStorageUID(ctx context.Context, storageUID string) error {
	prefix, err := WorkspaceUserUploadObjectPrefix(storageUID)
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
	return adapter.DeleteByPrefix(profile, normalizedPrefix)
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
	return adapter.DeleteByPrefix(profile, normalizedPrefix)
}

func (s *StorageService) UsesObjectStorage() bool {
	profile, err := s.resolveActiveProfile(context.Background())
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
	profiles, err := s.loadProfiles(ctx)
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

func (s *StorageService) uploadWithProfile(profile StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	return adapter.Upload(&profile, file, opts)
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
		fileHeader := createMemoryFileHeader(filename, data)
		return s.uploadWithProfile(profile, fileHeader, opts)
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
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	objectKey := buildObjectKey(opts.DirPrefix, ext)
	dst := filepath.FromSlash(objectKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}
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
	for _, key := range []string{"APP_PUBLIC_ASSET_BASE_URL", "APP_ASSET_BASE_URL", "PUBLIC_ASSET_BASE_URL", "APP_PUBLIC_BASE_URL", "APP_BASE_URL", "PUBLIC_BASE_URL"} {
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
		if strings.HasPrefix(candidate, bucketPrefix) {
			candidate = strings.TrimPrefix(candidate, bucketPrefix)
		}
	}

	return normalizeUploadObjectKey(candidate)
}

func (s *StorageService) downloadRemoteFile(ctx context.Context, remoteURL string, allowHosts []string) ([]byte, string, error) {
	resp, err := safehttp.GetWithAllowHosts(ctx, remoteURL, 30*time.Second, allowHosts...)
	if err != nil {
		return nil, "", fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteFileSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("读取图片数据失败: %v", err)
	}
	if len(data) > maxRemoteFileSize {
		return nil, "", fmt.Errorf("远程文件超过大小限制 (%d MB)", maxRemoteFileSize/1024/1024)
	}

	ext := strings.ToLower(path.Ext(strings.Split(remoteURL, "?")[0]))
	if normalized, ok := media.NormalizeImageExtension(ext); ok {
		ext = normalized
	} else {
		ext = media.ImageExtensionFromMIME(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		return nil, "", fmt.Errorf("仅支持 %s 图片", media.SupportedUploadImageLabel())
	}
	if err := validateRemoteImageBytes("remote"+ext, data); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), ext)
	return data, filename, nil
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

func createMemoryFileHeader(filename string, data []byte) *multipart.FileHeader {
	buffer := &multipartBuffer{filename: filename, data: data}
	return buffer.FileHeader()
}

func decodeDataURL(dataURL string) (string, []byte, error) {
	parsed, err := mediarefs.ParseDataImageURL(dataURL, maxRemoteFileSize)
	if err != nil {
		return "", nil, fmt.Errorf("无效的 base64 图片格式")
	}
	return fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), parsed.Ext), parsed.Data, nil
}

type multipartBuffer struct {
	filename string
	data     []byte
}

func (m *multipartBuffer) FileHeader() *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, m.filename))
	h.Set("Content-Type", "application/octet-stream")
	part, _ := writer.CreatePart(h)
	_, _ = part.Write(m.data)
	_ = writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(int64(len(m.data) + 1024))
	if files, ok := form.File["file"]; ok && len(files) > 0 {
		return files[0]
	}
	return nil
}
