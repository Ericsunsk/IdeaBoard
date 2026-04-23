package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"api/internal/platform/media"
	"api/internal/platform/safehttp"
)

const (
	// maxRemoteFileSize caps remote downloads to prevent OOM from unbounded streams.
	maxRemoteFileSize = 100 * 1024 * 1024 // 100 MB
)

func (s *StorageService) TestProfile(id string) error {
	profiles, err := s.loadProfiles()
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

func (s *StorageService) GetBucketInfo(id string) (map[string]interface{}, error) {
	profiles, err := s.loadProfiles()
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

func (s *StorageService) Upload(file *multipart.FileHeader) (string, error) {
	profile, err := s.resolveActiveProfile()
	if err != nil {
		return "", err
	}
	return s.uploadWithProfile(*profile, file, StorageStoreOptions{})
}

func (s *StorageService) StoreRemote(ctx context.Context, remoteURL string, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile()
	if err != nil {
		return "", err
	}
	return s.storeRemoteWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *StorageService) StoreBytes(filename string, data []byte, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile()
	if err != nil {
		return "", err
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return "", err
	}
	return adapter.StoreBytes(profile, filename, data, opts)
}

func (s *StorageService) UsesObjectStorage() bool {
	profile, err := s.resolveActiveProfile()
	if err != nil || profile == nil {
		return false
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return false
	}
	return adapter.UsesObjectStorage()
}

func (s *StorageService) IsDirectFetchEnabled() bool {
	profile, err := s.resolveActiveProfile()
	if err != nil || profile == nil {
		return false
	}
	adapter, err := s.adapterRegistry.get(profile.Provider)
	if err != nil {
		return false
	}
	return adapter.IsDirectFetchEnabled(profile)
}

func (s *StorageService) ensureProfileReadyForRuntime(profile StorageProfile) error {
	switch profile.Provider {
	case StorageProviderLocal:
		return nil
	case StorageProviderKodo:
		cfg := s.kodoConfigFromProfile(&profile)
		if !s.kodoService.IsConfigEnabled(cfg) {
			return fmt.Errorf("请先完整填写并启用 Kodo 存储")
		}
		if strings.TrimSpace(cfg.AccessDomain) == "" {
			return fmt.Errorf("请先配置访问域名")
		}
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

func (s *StorageService) resolveActiveProfile() (*StorageProfile, error) {
	profiles, err := s.loadProfiles()
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
	dirParts := []string{"uploads"}
	if opts.DirPrefix != "" {
		dirParts = append(dirParts, opts.DirPrefix)
	}
	dirParts = append(dirParts, time.Now().Format("2006/01/02"))
	dir := filepath.Join(dirParts...)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}
	return "/" + strings.ReplaceAll(dst, "\\", "/"), nil
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
		ext = imageExtensionFromContentType(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		return nil, "", fmt.Errorf("仅支持 JPG / PNG / GIF / WEBP 图片")
	}
	if err := validateRemoteImageBytes("remote"+ext, data); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), ext)
	return data, filename, nil
}

func imageExtensionFromContentType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch ct {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func validateRemoteImageBytes(filename string, data []byte) error {
	ext, ok := media.NormalizeImageExtension(filepath.Ext(filename))
	if !ok {
		return fmt.Errorf("仅支持 JPG / PNG / GIF / WEBP 图片")
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
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("无效的 base64 图片格式")
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("base64 解码失败: %v", err)
	}

	ext := ".png"
	switch {
	case strings.Contains(parts[0], "jpeg"), strings.Contains(parts[0], "jpg"):
		ext = ".jpg"
	case strings.Contains(parts[0], "webp"):
		ext = ".webp"
	case strings.Contains(parts[0], "gif"):
		ext = ".gif"
	}

	return fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), ext), decoded, nil
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
