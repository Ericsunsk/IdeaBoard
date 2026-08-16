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
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/outbound"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3CompatibleService struct {
	clientCache     sync.Map
	capabilityProbe func(context.Context, *S3CompatibleConfig) error
}

type StorageCapability string

const (
	StorageCapabilityListBucket   StorageCapability = "s3:ListBucket"
	StorageCapabilityGetObject    StorageCapability = "s3:GetObject"
	StorageCapabilityPutObject    StorageCapability = "s3:PutObject"
	StorageCapabilityDeleteObject StorageCapability = "s3:DeleteObject"
)

type StorageCapabilityError struct {
	Missing  []StorageCapability
	Endpoint string
}

func (e *StorageCapabilityError) Error() string {
	if e == nil || len(e.Missing) == 0 {
		return "S3 权限探测失败"
	}
	missing := make([]string, 0, len(e.Missing))
	bucketActions := make([]string, 0, 1)
	objectActions := make([]string, 0, 3)
	for _, capability := range e.Missing {
		missing = append(missing, string(capability))
		switch capability {
		case StorageCapabilityListBucket:
			bucketActions = append(bucketActions, string(capability))
		default:
			objectActions = append(objectActions, string(capability))
		}
	}
	scope := make([]string, 0, 2)
	if len(bucketActions) > 0 {
		scope = append(scope, fmt.Sprintf("Bucket ARN 添加 %s", strings.Join(bucketActions, "、")))
	}
	if len(objectActions) > 0 {
		scope = append(scope, fmt.Sprintf("Object ARN 添加 %s", strings.Join(objectActions, "、")))
	}
	if isCloudflareR2Endpoint(e.Endpoint) {
		return fmt.Sprintf("Cloudflare R2 凭证缺少权限：%s。请确认 R2 API Token 使用 Object Read & Write、包含当前 Bucket，且 Access Key ID 与 Secret Access Key 来自同一个 Token", strings.Join(missing, "、"))
	}
	return fmt.Sprintf("S3 凭证缺少权限：%s。请在%s策略中授予对应权限", strings.Join(missing, "、"), strings.Join(scope, "；"))
}

type StorageProbeError struct {
	Capability StorageCapability
	Endpoint   string
	Cause      error
}

func (e *StorageProbeError) Error() string {
	if e == nil || e.Cause == nil {
		return "S3 权限探测失败"
	}
	if code := s3ErrorCode(e.Cause); code != "" {
		return fmt.Sprintf("探测 %s 失败: %s: %v", e.Capability, code, e.Cause)
	}
	return fmt.Sprintf("探测 %s 失败: %v", e.Capability, e.Cause)
}

func (e *StorageProbeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *StorageProbeError) UserMessage() string {
	provider := "S3"
	if isCloudflareR2Endpoint(e.Endpoint) {
		provider = "Cloudflare R2"
	}
	code := s3ErrorCode(e.Cause)
	switch code {
	case "AuthorizationHeaderMalformed":
		if provider == "Cloudflare R2" {
			return "Cloudflare R2 请求配置错误：Region 必须为 auto，Endpoint 必须为 <account-id>.r2.cloudflarestorage.com"
		}
		return "S3 请求配置错误：请检查 Region 与 Endpoint 是否匹配"
	case "SignatureDoesNotMatch", "InvalidAccessKeyId", "InvalidToken":
		return fmt.Sprintf("%s 凭证校验失败：请检查 Endpoint、Region，并确认 Access Key ID 与 Secret Access Key 匹配", provider)
	case "NoSuchBucket":
		return fmt.Sprintf("%s Bucket 不存在或不属于当前账号", provider)
	default:
		return fmt.Sprintf("%s 请求失败（%s），请检查 Endpoint、Region 和凭证配置", provider, e.Capability)
	}
}

type s3CapabilityProbeClient interface {
	ListBucket(ctx context.Context, bucketName, prefix string) error
	PutObject(ctx context.Context, bucketName, objectName string) error
	GetObject(ctx context.Context, bucketName, objectName string) error
	DeleteObject(ctx context.Context, bucketName, objectName string) error
}

type minioS3CapabilityProbeClient struct {
	client *minio.Client
}

func (c minioS3CapabilityProbeClient) ListBucket(ctx context.Context, bucketName, prefix string) error {
	var firstErr error
	for object := range c.client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if firstErr == nil && object.Err != nil {
			firstErr = object.Err
		}
	}
	return firstErr
}

func (c minioS3CapabilityProbeClient) PutObject(ctx context.Context, bucketName, objectName string) error {
	_, err := c.client.PutObject(ctx, bucketName, objectName, strings.NewReader("ideaboard-s3-capability-probe"), int64(len("ideaboard-s3-capability-probe")), minio.PutObjectOptions{ContentType: "text/plain"})
	return err
}

func (c minioS3CapabilityProbeClient) GetObject(ctx context.Context, bucketName, objectName string) error {
	object, err := c.client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer object.Close()
	_, err = object.Stat()
	return err
}

func (c minioS3CapabilityProbeClient) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	return c.client.RemoveObject(ctx, bucketName, objectName, minio.RemoveObjectOptions{})
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
	return s.ProbeCapabilitiesWithConfig(ctx, cfg)
}

func (s *S3CompatibleService) ProbeCapabilitiesWithConfig(ctx context.Context, cfg *S3CompatibleConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validateBaseConfig(ctx, cfg); err != nil {
		return err
	}
	if s.capabilityProbe != nil {
		return s.capabilityProbe(ctx, cfg)
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	return s.probeCapabilities(ctx, cfg, minioS3CapabilityProbeClient{client: client})
}

func (s *S3CompatibleService) probeCapabilities(ctx context.Context, cfg *S3CompatibleConfig, client s3CapabilityProbeClient) error {
	bucketName := strings.TrimSpace(cfg.BucketName)
	missing := make([]StorageCapability, 0, 4)
	if err := client.ListBucket(ctx, bucketName, ".ideaboard-capability-probe/"); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !isS3PermissionDenied(err) {
			return newStorageProbeError(cfg, StorageCapabilityListBucket, err)
		}
		missing = append(missing, StorageCapabilityListBucket)
	}

	deleteProbeKey := fmt.Sprintf(".ideaboard-capability-probe/delete-%d", time.Now().UnixNano())
	if err := client.DeleteObject(ctx, bucketName, deleteProbeKey); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !isS3PermissionDenied(err) {
			return newStorageProbeError(cfg, StorageCapabilityDeleteObject, err)
		}
		missing = append(missing, StorageCapabilityDeleteObject)
	} else {
		probeKey := fmt.Sprintf(".ideaboard-capability-probe/read-write-%d", time.Now().UnixNano())
		putErr := client.PutObject(ctx, bucketName, probeKey)
		if putErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if !isS3PermissionDenied(putErr) {
				return newStorageProbeError(cfg, StorageCapabilityPutObject, putErr)
			}
			missing = append(missing, StorageCapabilityPutObject)
		} else {
			if err := client.GetObject(ctx, bucketName, probeKey); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				if !isS3PermissionDenied(err) {
					return newStorageProbeError(cfg, StorageCapabilityGetObject, err)
				}
				missing = append(missing, StorageCapabilityGetObject)
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cleanupErr := client.DeleteObject(cleanupCtx, bucketName, probeKey)
			cancel()
			if cleanupErr != nil {
				if !isS3PermissionDenied(cleanupErr) {
					return newStorageProbeError(cfg, StorageCapabilityDeleteObject, cleanupErr)
				}
				missing = append(missing, StorageCapabilityDeleteObject)
			}
		}
	}

	if len(missing) > 0 {
		return newStorageCapabilityError(cfg, missing)
	}
	return nil
}

func newStorageCapabilityError(cfg *S3CompatibleConfig, missing []StorageCapability) *StorageCapabilityError {
	endpoint := ""
	if cfg != nil {
		endpoint = cfg.Endpoint
	}
	return &StorageCapabilityError{Missing: missing, Endpoint: endpoint}
}

func newStorageProbeError(cfg *S3CompatibleConfig, capability StorageCapability, cause error) *StorageProbeError {
	endpoint := ""
	if cfg != nil {
		endpoint = cfg.Endpoint
	}
	return &StorageProbeError{Capability: capability, Endpoint: endpoint, Cause: cause}
}

func isS3PermissionDenied(err error) bool {
	switch strings.ToLower(s3ErrorCode(err)) {
	case "accessdenied", "allaccessdisabled", "forbidden", "notauthorized", "operationnotpermitted", "unauthorized":
		return true
	default:
		return false
	}
}

func s3ErrorCode(err error) string {
	var response minio.ErrorResponse
	if errors.As(err, &response) {
		return strings.TrimSpace(response.Code)
	}
	var responsePtr *minio.ErrorResponse
	if errors.As(err, &responsePtr) && responsePtr != nil {
		return strings.TrimSpace(responsePtr.Code)
	}
	return ""
}

func isCloudflareR2Endpoint(endpoint string) bool {
	host := strings.ToLower(parseS3Host(endpoint))
	return strings.HasSuffix(host, ".r2.cloudflarestorage.com")
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
		if isS3PermissionDenied(err) {
			return nil, newStorageCapabilityError(cfg, []StorageCapability{StorageCapabilityListBucket})
		}
		return nil, newStorageProbeError(cfg, StorageCapabilityListBucket, err)
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

func (s *S3CompatibleService) UploadRefWithConfig(ctx context.Context, file *multipart.FileHeader, cfg *S3CompatibleConfig, opts StorageStoreOptions) (*StoredUploadRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, err := s.prepareUploadClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	return s.putReaderWithConfigRef(ctx, client, file.Filename, file.Header.Get("Content-Type"), src, file.Size, cfg, opts)
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
	if err := s.validateBaseConfig(ctx, cfg); err != nil {
		return err
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
