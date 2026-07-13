package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/outbound"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3CompatibleService struct {
	clientCache sync.Map
}

type s3BucketExistenceChecker interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
}

func NewS3CompatibleService() *S3CompatibleService {
	return &S3CompatibleService{}
}

func (s *S3CompatibleService) IsConfigEnabled(cfg *S3CompatibleConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.Enabled &&
		strings.TrimSpace(cfg.AccessKeyID) != "" &&
		cfg.SecretAccessKey != "" &&
		strings.TrimSpace(cfg.BucketName) != "" &&
		strings.TrimSpace(cfg.Endpoint) != "" &&
		strings.TrimSpace(cfg.AccessDomain) != ""
}

func (s *S3CompatibleService) TestConnectionWithConfig(ctx context.Context, cfg *S3CompatibleConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validateBaseConfig(ctx, cfg); err != nil {
		return err
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	if err := ensureS3BucketAccessible(ctx, client, strings.TrimSpace(cfg.BucketName)); err != nil {
		return err
	}
	return nil
}

func (s *S3CompatibleService) GetBucketInfoWithConfig(ctx context.Context, cfg *S3CompatibleConfig) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.validateBaseConfig(ctx, cfg); err != nil {
		return nil, err
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := ensureS3BucketAccessible(ctx, client, strings.TrimSpace(cfg.BucketName)); err != nil {
		return nil, err
	}

	domainSet := map[string]struct{}{}
	addS3DomainCandidate(domainSet, cfg.AccessDomain)

	if endpointHost, _, err := parseS3Endpoint(cfg.Endpoint); err == nil && endpointHost != "" {
		addS3DomainCandidate(domainSet, endpointHost)
		addS3DomainCandidate(domainSet, fmt.Sprintf("%s.%s", strings.TrimSpace(cfg.BucketName), endpointHost))
	}

	domains := make([]string, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	return map[string]interface{}{
		"domains": domains,
	}, nil
}

func ensureS3BucketAccessible(ctx context.Context, client s3BucketExistenceChecker, bucketName string) error {
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("验证 Bucket 失败: %w", err)
	}
	if !exists {
		return errors.New("bucket 不存在或当前凭证无权访问")
	}
	return nil
}

func (s *S3CompatibleService) UploadWithConfig(ctx context.Context, file *multipart.FileHeader, cfg *S3CompatibleConfig, opts StorageStoreOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	client, err := s.prepareUploadClient(ctx, cfg)
	if err != nil {
		return "", err
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	return s.putReaderWithConfig(ctx, client, file.Filename, file.Header.Get("Content-Type"), src, file.Size, cfg, opts)
}

func (s *S3CompatibleService) uploadBytesWithConfig(ctx context.Context, filename string, data []byte, cfg *S3CompatibleConfig, opts StorageStoreOptions) (string, error) {
	ref, err := s.uploadBytesWithConfigRef(ctx, filename, data, cfg, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *S3CompatibleService) uploadBytesWithConfigRef(ctx context.Context, filename string, data []byte, cfg *S3CompatibleConfig, opts StorageStoreOptions) (*StoredUploadRef, error) {
	contentType := inferStoredUploadContentType(filename, data)
	ref, err := s.uploadReaderWithConfigRef(ctx, filename, contentType, bytes.NewReader(data), int64(len(data)), cfg, opts)
	if err != nil {
		return nil, err
	}
	ref.Width, ref.Height, _ = imageformat.ImageDimensions(data)
	return ref, nil
}

func (s *S3CompatibleService) uploadReaderWithConfig(ctx context.Context, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts StorageStoreOptions) (string, error) {
	ref, err := s.uploadReaderWithConfigRef(ctx, filename, contentType, body, size, cfg, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *S3CompatibleService) uploadReaderWithConfigRef(ctx context.Context, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts StorageStoreOptions) (*StoredUploadRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.IsConfigEnabled(cfg) {
		return nil, fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	if body == nil {
		return nil, fmt.Errorf("文件内容不能为空")
	}
	if size < 0 {
		return nil, fmt.Errorf("文件大小无效")
	}

	client, err := s.prepareUploadClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return s.putReaderWithConfigRef(ctx, client, filename, contentType, body, size, cfg, opts)
}

func (s *S3CompatibleService) prepareUploadClient(ctx context.Context, cfg *S3CompatibleConfig) (*minio.Client, error) {
	if !s.IsConfigEnabled(cfg) {
		return nil, fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	return s.clientWithConfig(ctx, cfg)
}

func (s *S3CompatibleService) putReaderWithConfig(ctx context.Context, client *minio.Client, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts StorageStoreOptions) (string, error) {
	ref, err := s.putReaderWithConfigRef(ctx, client, filename, contentType, body, size, cfg, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *S3CompatibleService) putReaderWithConfigRef(ctx context.Context, client *minio.Client, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts StorageStoreOptions) (*StoredUploadRef, error) {
	key, err := buildObjectKey(opts.DirPrefix, filepathExt(filename))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	_, err = client.PutObject(
		ctx,
		strings.TrimSpace(cfg.BucketName),
		key,
		body,
		size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return nil, fmt.Errorf("上传失败: %w", err)
	}

	publicURL, err := s.publicObjectURL(cfg, key)
	if err != nil {
		return nil, err
	}
	return &StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: strings.TrimSpace(cfg.ProfileID),
		MIMEType:       contentType,
		SizeBytes:      size,
	}, nil
}

func (s *S3CompatibleService) DeleteWithConfig(ctx context.Context, objectKey string, cfg *S3CompatibleConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.IsConfigEnabled(cfg) {
		return fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return fmt.Errorf("文件路径不能为空")
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, strings.TrimSpace(cfg.BucketName), key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

func (s *S3CompatibleService) readWithConfig(ctx context.Context, objectKey string, cfg *S3CompatibleConfig, maxBytes int64) (*StoredUploadObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.IsConfigEnabled(cfg) {
		return nil, fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return nil, fmt.Errorf("文件路径不能为空")
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	object, err := client.GetObject(ctx, strings.TrimSpace(cfg.BucketName), key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	defer object.Close()

	stat, statErr := object.Stat()
	data, err := readStorageObject(object, maxBytes)
	if err != nil {
		return nil, err
	}
	contentType := ""
	if statErr == nil {
		contentType = strings.TrimSpace(stat.ContentType)
	}
	return &StoredUploadObject{
		ObjectKey:   key,
		Filename:    path.Base(key),
		ContentType: contentType,
		Data:        data,
	}, nil
}

func (s *S3CompatibleService) DeletePrefixWithConfig(ctx context.Context, objectKeyPrefix string, cfg *S3CompatibleConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.IsConfigEnabled(cfg) {
		return fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	prefix := strings.TrimSpace(objectKeyPrefix)
	if prefix == "" {
		return fmt.Errorf("文件前缀不能为空")
	}

	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return err
	}

	bucketName := strings.TrimSpace(cfg.BucketName)
	objectCh := client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for object := range objectCh {
		if err := ctx.Err(); err != nil {
			return err
		}
		if object.Err != nil {
			return fmt.Errorf("列举前缀对象失败: %v", object.Err)
		}
		if err := client.RemoveObject(ctx, bucketName, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("删除对象失败: %w", err)
		}
	}
	return nil
}

func (s *S3CompatibleService) validateBaseConfig(ctx context.Context, cfg *S3CompatibleConfig) error {
	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" || cfg.SecretAccessKey == "" || strings.TrimSpace(cfg.BucketName) == "" {
		return fmt.Errorf("请先填写 Access Key ID / Secret Access Key / Bucket 名称")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return fmt.Errorf("请先填写 Endpoint")
	}

	return validateS3EndpointPolicy(ctx, cfg.Endpoint)
}

func validateS3EndpointPolicy(ctx context.Context, endpoint string) error {
	endpointHost, secure, err := parseS3Endpoint(endpoint)
	if err != nil {
		return err
	}
	endpointURL := "https://" + endpointHost
	if !secure {
		endpointURL = "http://" + endpointHost
	}
	if _, err := outbound.ValidateConfiguredEndpoint(endpointURL, outbound.AllowHosts(ctx)...); err != nil {
		return errors.New("endpoint 不可用，仅支持可达的公网 http(s) 地址")
	}
	return nil
}

func (s *S3CompatibleService) clientWithConfig(ctx context.Context, cfg *S3CompatibleConfig) (*minio.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	if err := validateS3EndpointPolicy(ctx, cfg.Endpoint); err != nil {
		return nil, err
	}
	cacheKey := s3ClientCacheKey(cfg)
	if cached, ok := s.clientCache.Load(cacheKey); ok {
		return cached.(*minio.Client), nil
	}

	endpointHost, secure, err := parseS3Endpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	bucketLookup := minio.BucketLookupAuto
	if cfg.ForcePathStyle {
		bucketLookup = minio.BucketLookupPath
	}

	client, err := minio.New(endpointHost, &minio.Options{
		Creds:        credentials.NewStaticV4(strings.TrimSpace(cfg.AccessKeyID), cfg.SecretAccessKey, ""),
		Secure:       secure,
		Region:       strings.TrimSpace(cfg.RegionID),
		BucketLookup: bucketLookup,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 S3 客户端失败: %w", err)
	}

	actual, _ := s.clientCache.LoadOrStore(cacheKey, client)
	return actual.(*minio.Client), nil
}

func s3ClientCacheKey(cfg *S3CompatibleConfig) string {
	h := sha256.New()
	writeS3ClientCacheKeyPart(h, cfg.Endpoint)
	writeS3ClientCacheKeyPart(h, cfg.AccessKeyID)
	writeS3ClientCacheKeyPart(h, cfg.SecretAccessKey)
	writeS3ClientCacheKeyPart(h, cfg.RegionID)
	h.Write([]byte{boolByte(cfg.ForcePathStyle)})
	return hex.EncodeToString(h.Sum(nil))
}

func writeS3ClientCacheKeyPart(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func (s *S3CompatibleService) publicObjectURL(cfg *S3CompatibleConfig, key string) (string, error) {
	base, err := normalizePublicDomain(cfg.AccessDomain)
	if err != nil {
		return "", err
	}
	bucketName := strings.TrimSpace(cfg.BucketName)
	if cfg.ForcePathStyle {
		if strings.Contains(base, "/"+bucketName) {
			return fmt.Sprintf("%s/%s", base, key), nil
		}
		return fmt.Sprintf("%s/%s/%s", base, bucketName, key), nil
	}
	return fmt.Sprintf("%s/%s", base, key), nil
}

func filepathExt(filename string) string {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx >= len(filename)-1 {
		return ".png"
	}
	return strings.ToLower(filename[idx:])
}

func parseS3Endpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true, fmt.Errorf("无效的 Endpoint")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", true, fmt.Errorf("无效的 Endpoint")
	}
	return parsed.Host, parsed.Scheme != "http", nil
}

func normalizePublicDomain(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("未配置访问域名")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("无效的访问域名")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func addS3DomainCandidate(domainSet map[string]struct{}, raw string) {
	host := parseS3Host(raw)
	if host == "" {
		return
	}
	domainSet[host] = struct{}{}
}

func parseS3Host(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.TrimSpace(parsed.Hostname())
}
