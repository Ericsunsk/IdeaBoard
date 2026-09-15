package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var defaultR2PermissionRetryDelays = []time.Duration{
	250 * time.Millisecond,
	750 * time.Millisecond,
}

const maxRemoteFileSize = 50 * 1024 * 1024

type S3CompatibleService struct {
	clientCache             sync.Map
	capabilityProbe         func(context.Context, *S3CompatibleConfig) error
	deliveryRead            func(context.Context, string, *S3CompatibleConfig, int64) (*ports.StoredUploadObject, error)
	generationRead          func(context.Context, string, *S3CompatibleConfig) (int64, error)
	r2PermissionRetryDelays []time.Duration
}

func NewS3CompatibleService() *S3CompatibleService {
	return &S3CompatibleService{
		r2PermissionRetryDelays: defaultR2PermissionRetryDelays,
	}
}

func (s *S3CompatibleService) SetCapabilityProbe(fn func(context.Context, *S3CompatibleConfig) error) {
	s.capabilityProbe = fn
}

func (s *S3CompatibleService) SetDeliveryRead(fn func(context.Context, string, *S3CompatibleConfig, int64) (*ports.StoredUploadObject, error)) {
	s.deliveryRead = fn
}

func (s *S3CompatibleService) SetGenerationRead(fn func(context.Context, string, *S3CompatibleConfig) (int64, error)) {
	s.generationRead = fn
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

func (s *S3CompatibleService) UploadRefWithConfig(ctx context.Context, file *multipart.FileHeader, cfg *S3CompatibleConfig, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
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

func (s *S3CompatibleService) UploadBytesWithConfigRef(ctx context.Context, filename string, data []byte, cfg *S3CompatibleConfig, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	facts, err := imageformat.InspectDetectedBytes(data)
	if err != nil {
		return nil, fmt.Errorf("读取图片信息失败: %w", err)
	}
	filename = domain.CanonicalStorageFilename(filename, facts.Extension)
	opts.ObjectKey = domain.CanonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)
	ref, err := s.uploadReaderWithConfigRef(ctx, filename, facts.MIMEType, bytes.NewReader(data), int64(len(data)), cfg, opts)
	if err != nil {
		return nil, err
	}
	ref.Width = facts.Width
	ref.Height = facts.Height
	ref.Animated = facts.Animated
	return ref, nil
}

func (s *S3CompatibleService) PutBytesAtKeyWithConfig(ctx context.Context, objectKey string, data []byte, contentType string, cfg *S3CompatibleConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := s.prepareUploadClient(ctx, cfg)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return fmt.Errorf("文件路径不能为空")
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	generation, err := s.nextObjectGeneration(ctx, client, cfg, key)
	if err != nil {
		return fmt.Errorf("读取对象 generation 失败: %w", err)
	}
	body := bytes.NewReader(data)
	if err := s.putObjectWithRetry(ctx, cfg, body, func(attemptCtx context.Context) error {
		_, putErr := client.PutObject(
			attemptCtx,
			strings.TrimSpace(cfg.BucketName),
			key,
			body,
			int64(len(data)),
			minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"object-generation": strconv.FormatInt(generation, 10)}},
		)
		return putErr
	}); err != nil {
		return fmt.Errorf("上传派生图片失败: %w", err)
	}
	return nil
}

func (s *S3CompatibleService) UploadReaderWithConfigRef(ctx context.Context, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.uploadReaderWithConfigRef(ctx, filename, contentType, body, size, cfg, opts)
}

func (s *S3CompatibleService) uploadReaderWithConfigRef(ctx context.Context, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
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

func (s *S3CompatibleService) putReaderWithConfigRef(ctx context.Context, client *minio.Client, filename string, contentType string, body io.Reader, size int64, cfg *S3CompatibleConfig, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	key := strings.TrimSpace(opts.ObjectKey)
	var err error
	if key == "" {
		key, err = domain.BuildObjectKey(opts.DirPrefix, filepathExt(filename))
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	body, checksum, err := prepareChecksumBody(body)
	if err != nil {
		return nil, fmt.Errorf("计算上传文件 checksum 失败: %w", err)
	}
	generation, err := s.nextObjectGeneration(ctx, client, cfg, key)
	if err != nil {
		return nil, fmt.Errorf("读取对象 generation 失败: %w", err)
	}

	err = s.putObjectWithRetry(ctx, cfg, body, func(attemptCtx context.Context) error {
		_, putErr := client.PutObject(
			attemptCtx,
			strings.TrimSpace(cfg.BucketName),
			key,
			body,
			size,
			minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"object-generation": strconv.FormatInt(generation, 10)}},
		)
		return putErr
	})
	if err != nil {
		return nil, fmt.Errorf("上传失败: %w", err)
	}

	publicURL, err := s.PublicObjectURL(cfg, key)
	if err != nil {
		return nil, err
	}
	return &ports.StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: strings.TrimSpace(cfg.ProfileID),
		MIMEType:       contentType,
		SizeBytes:      size,
		ChecksumSHA256: checksum(),
		Animated:       strings.EqualFold(filepathExt(filename), ".gif"),
	}, nil
}

func prepareChecksumBody(body io.Reader) (io.Reader, func() string, error) {
	seeker, ok := body.(io.Seeker)
	if ok {
		position, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, nil, err
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, body); err != nil {
			return nil, nil, err
		}
		if _, err := seeker.Seek(position, io.SeekStart); err != nil {
			return nil, nil, err
		}
		checksum := hex.EncodeToString(hash.Sum(nil))
		return body, func() string { return checksum }, nil
	}
	capture := &streamChecksumReader{reader: body, hash: sha256.New()}
	return capture, func() string { return hex.EncodeToString(capture.hash.Sum(nil)) }, nil
}

type streamChecksumReader struct {
	reader io.Reader
	hash   hash.Hash
}

func (r *streamChecksumReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		_, _ = r.hash.Write(p[:n])
	}
	return n, err
}

func (s *S3CompatibleService) putObjectWithRetry(
	ctx context.Context,
	cfg *S3CompatibleConfig,
	body io.Reader,
	put func(context.Context) error,
) error {
	seeker, canRetry := body.(io.Seeker)
	if !canRetry {
		return put(ctx)
	}
	return s.retryR2PermissionDenied(ctx, cfg, "object upload", func(attempt int) error {
		if attempt > 0 {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("重试上传前重置文件失败: %w", err)
			}
		}
		return put(ctx)
	})
}

func (s *S3CompatibleService) retryR2PermissionDenied(
	ctx context.Context,
	cfg *S3CompatibleConfig,
	operation string,
	run func(attempt int) error,
) error {
	retryDelays := s.r2PermissionRetryDelays
	if cfg == nil || !isCloudflareR2Endpoint(cfg.Endpoint) {
		retryDelays = nil
	}
	for attempt := 0; ; attempt++ {
		err := run(attempt)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !isS3PermissionDenied(err) || attempt >= len(retryDelays) {
			return err
		}

		if err := waitForStorageProbeRetry(ctx, retryDelays[attempt]); err != nil {
			return err
		}
	}
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
	err = s.retryR2PermissionDenied(ctx, cfg, "object delete", func(int) error {
		return client.RemoveObject(ctx, strings.TrimSpace(cfg.BucketName), key, minio.RemoveObjectOptions{})
	})
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

func (s *S3CompatibleService) ReadWithConfig(ctx context.Context, objectKey string, cfg *S3CompatibleConfig, maxBytes int64) (*ports.StoredUploadObject, error) {
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
	return s.readDeliveryObject(ctx, key, cfg, maxBytes)
}

func (s *S3CompatibleService) readDeliveryObject(ctx context.Context, key string, cfg *S3CompatibleConfig, maxBytes int64) (*ports.StoredUploadObject, error) {
	if s.deliveryRead != nil {
		return s.deliveryRead(ctx, key, cfg, maxBytes)
	}
	publicURL, err := s.PublicObjectURL(cfg, key)
	if err != nil {
		return nil, err
	}
	response, err := safehttp.GetWithAllowHosts(ctx, publicURL, 30*time.Second, outbound.AllowHosts(ctx)...)
	if err != nil || response == nil || response.StatusCode != http.StatusOK {
		if response != nil {
			_ = response.Body.Close()
		}
		signedURL, signedErr := s.SignedURLWithConfig(ctx, key, time.Minute, cfg)
		if signedErr != nil {
			if err != nil {
				return nil, fmt.Errorf("读取文件失败: %w", err)
			}
			return nil, fmt.Errorf("读取文件失败: %w", signedErr)
		}
		response, err = safehttp.GetWithAllowHosts(ctx, signedURL, 30*time.Second, outbound.AllowHosts(ctx)...)
		if err != nil {
			return nil, fmt.Errorf("读取 signed 文件失败: %w", err)
		}
	}
	if response == nil {
		return nil, fmt.Errorf("读取文件失败: 空响应")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, domain.ErrStoredObjectNotFound
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("读取文件失败: 存储服务返回 HTTP %d", response.StatusCode)
	}

	data, err := readStorageObject(response.Body, maxBytes)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = inferStoredUploadContentType(path.Base(key), data)
	}
	return &ports.StoredUploadObject{
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
	err = s.retryR2PermissionDenied(ctx, cfg, "prefix delete", func(int) error {
		objectCh := client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		})
		for object := range objectCh {
			if err := ctx.Err(); err != nil {
				return err
			}
			if object.Err != nil {
				return object.Err
			}
			if err := client.RemoveObject(ctx, bucketName, object.Key, minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("删除前缀对象失败: %w", err)
	}
	return nil
}

func (s *S3CompatibleService) ListObjectKeysWithConfig(ctx context.Context, objectKeyPrefix string, cfg *S3CompatibleConfig) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.IsConfigEnabled(cfg) {
		return nil, fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	prefix := strings.Trim(strings.TrimSpace(objectKeyPrefix), "/")
	if prefix == "" {
		return nil, fmt.Errorf("文件前缀不能为空")
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	for object := range client.ListObjects(ctx, strings.TrimSpace(cfg.BucketName), minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if object.Err != nil {
			return nil, object.Err
		}
		keys = append(keys, object.Key)
	}
	return keys, nil
}

func (s *S3CompatibleService) SignedURLWithConfig(ctx context.Context, objectKey string, expiry time.Duration, cfg *S3CompatibleConfig) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !s.IsConfigEnabled(cfg) {
		return "", fmt.Errorf("S3 兼容存储配置未启用或不完整")
	}
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	if expiry <= 0 {
		expiry = time.Hour
	}
	client, err := s.clientWithConfig(ctx, cfg)
	if err != nil {
		return "", err
	}
	signed, err := client.PresignedGetObject(ctx, strings.TrimSpace(cfg.BucketName), key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("生成 S3 signed URL 失败: %w", err)
	}
	return signed.String(), nil
}

func (s *S3CompatibleService) VerifyObjectGeneration(ctx context.Context, objectKey string, expected int64, cfg *S3CompatibleConfig) error {
	if expected <= 0 {
		return fmt.Errorf("%w: expected generation must be positive", domain.ErrObjectGenerationUnavailable)
	}
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return fmt.Errorf("%w: object key is empty", domain.ErrObjectGenerationUnavailable)
	}
	if !s.IsConfigEnabled(cfg) {
		return fmt.Errorf("%w: S3 兼容存储配置未启用或不完整", domain.ErrObjectGenerationUnavailable)
	}
	var actual int64
	var err error
	if s.generationRead != nil {
		actual, err = s.generationRead(ctx, key, cfg)
	} else {
		client, clientErr := s.clientWithConfig(ctx, cfg)
		if clientErr != nil {
			return fmt.Errorf("%w: %w", domain.ErrObjectGenerationUnavailable, clientErr)
		}
		info, statErr := client.StatObject(ctx, strings.TrimSpace(cfg.BucketName), key, minio.StatObjectOptions{})
		if statErr != nil {
			return fmt.Errorf("%w: stat object: %w", domain.ErrObjectGenerationUnavailable, statErr)
		}
		raw := strings.TrimSpace(info.Metadata.Get("X-Amz-Meta-Object-Generation"))
		if raw == "" {
			raw = strings.TrimSpace(info.Metadata.Get("object-generation"))
		}
		if raw == "" {
			return fmt.Errorf("%w: object metadata is missing x-amz-meta-object-generation", domain.ErrObjectGenerationUnavailable)
		}
		actual, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || actual <= 0 {
			return fmt.Errorf("%w: invalid object generation metadata %q", domain.ErrObjectGenerationUnavailable, raw)
		}
	}
	if err != nil {
		return fmt.Errorf("%w: read object generation: %w", domain.ErrObjectGenerationUnavailable, err)
	}
	if actual <= 0 || actual != expected {
		return fmt.Errorf("%w: got %d, want %d", domain.ErrObjectGenerationUnavailable, actual, expected)
	}
	return nil
}

func (s *S3CompatibleService) nextObjectGeneration(ctx context.Context, client *minio.Client, cfg *S3CompatibleConfig, key string) (int64, error) {
	info, err := client.StatObject(ctx, strings.TrimSpace(cfg.BucketName), key, minio.StatObjectOptions{})
	if err != nil {
		code := strings.ToLower(s3ErrorCode(err))
		if code == "nosuchkey" || code == "notfound" || code == "nosuchobject" {
			return 1, nil
		}
		return 0, err
	}
	raw := strings.TrimSpace(info.Metadata.Get("X-Amz-Meta-Object-Generation"))
	if raw == "" {
		raw = strings.TrimSpace(info.Metadata.Get("object-generation"))
	}
	if raw == "" {
		return 0, domain.ErrObjectGenerationUnavailable
	}
	current, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || current <= 0 {
		return 0, fmt.Errorf("invalid object generation metadata %q", raw)
	}
	if current == int64(^uint64(0)>>1) {
		return 0, fmt.Errorf("object generation overflow")
	}
	return current + 1, nil
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
	transport, err := s3ClientTransport(cfg, secure)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(endpointHost, &minio.Options{
		Creds:        credentials.NewStaticV4(strings.TrimSpace(cfg.AccessKeyID), cfg.SecretAccessKey, ""),
		Secure:       secure,
		Region:       strings.TrimSpace(cfg.RegionID),
		BucketLookup: bucketLookup,
		Transport:    transport,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 S3 客户端失败: %w", err)
	}

	actual, _ := s.clientCache.LoadOrStore(cacheKey, client)
	return actual.(*minio.Client), nil
}

func s3ClientTransport(cfg *S3CompatibleConfig, secure bool) (http.RoundTripper, error) {
	if cfg == nil || !isCloudflareR2Endpoint(cfg.Endpoint) {
		return nil, nil
	}
	transport, err := minio.DefaultTransport(secure)
	if err != nil {
		return nil, fmt.Errorf("初始化 R2 HTTP 传输失败: %w", err)
	}
	transport.DisableKeepAlives = true
	return transport, nil
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

func (s *S3CompatibleService) PublicObjectURL(cfg *S3CompatibleConfig, key string) (string, error) {
	base, err := normalizePublicDomain(cfg.AccessDomain)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", base, key), nil
}

func addS3DomainCandidate(domainSet map[string]struct{}, raw string) {
	host := parseS3Host(raw)
	if host == "" {
		return
	}
	domainSet[host] = struct{}{}
}

func filepathExt(filename string) string {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx >= len(filename)-1 {
		return ".png"
	}
	return strings.ToLower(filename[idx:])
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
