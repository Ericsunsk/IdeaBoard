package s3

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

type StorageCapability string

const (
	StorageCapabilityListBucket   StorageCapability = "s3:ListBucket"
	StorageCapabilityPutObject    StorageCapability = "s3:PutObject"
	StorageCapabilityDeleteObject StorageCapability = "s3:DeleteObject"
	StorageCapabilityPublicRead   StorageCapability = "public:ReadObject"
)

const s3CapabilityProbePayload = "ideaboard-s3-capability-probe"

type StorageCapabilityError struct {
	Missing  []StorageCapability
	Endpoint string
}

func (e *StorageCapabilityError) Error() string {
	if e == nil || len(e.Missing) == 0 {
		return msgError
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
		scope = append(scope, fmt.Sprintf(msgError2, strings.Join(bucketActions, "、")))
	}
	if len(objectActions) > 0 {
		scope = append(scope, fmt.Sprintf(msgError3, strings.Join(objectActions, "、")))
	}
	if isCloudflareR2Endpoint(e.Endpoint) {
		return fmt.Sprintf(msgError4, strings.Join(missing, "、"))
	}
	return fmt.Sprintf(msgError5, strings.Join(missing, "、"), strings.Join(scope, "；"))
}

func (e *StorageCapabilityError) UserMessage() string {
	return e.Error()
}

type StorageProbeError struct {
	Capability StorageCapability
	Endpoint   string
	Cause      error
}

func (e *StorageProbeError) Error() string {
	if e == nil || e.Cause == nil {
		return msgError
	}
	if code := s3ErrorCode(e.Cause); code != "" {
		return fmt.Sprintf(msgError6, e.Capability, code, e.Cause)
	}
	return fmt.Sprintf(msgError7, e.Capability, e.Cause)
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
	if e.Capability == StorageCapabilityPublicRead {
		return fmt.Sprintf(msgUserMessage, provider)
	}
	code := s3ErrorCode(e.Cause)
	switch code {
	case "AuthorizationHeaderMalformed":
		if provider == "Cloudflare R2" {
			return msgUserMessage2
		}
		return msgUserMessage3
	case "SignatureDoesNotMatch", "InvalidAccessKeyId", "InvalidToken":
		return fmt.Sprintf(msgUserMessage4, provider)
	case "NoSuchBucket":
		return fmt.Sprintf(msgUserMessage5, provider)
	default:
		return fmt.Sprintf(msgUserMessage6, provider, e.Capability)
	}
}

type s3CapabilityProbeClient interface {
	ListBucket(ctx context.Context, bucketName, prefix string) error
	PutObject(ctx context.Context, bucketName, objectName string) error
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
	_, err := c.client.PutObject(ctx, bucketName, objectName, strings.NewReader(s3CapabilityProbePayload), int64(len(s3CapabilityProbePayload)), minio.PutObjectOptions{ContentType: "text/plain"})
	return err
}

func (c minioS3CapabilityProbeClient) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	return c.client.RemoveObject(ctx, bucketName, objectName, minio.RemoveObjectOptions{})
}

func (s *S3CompatibleService) probeCapabilities(ctx context.Context, cfg *S3CompatibleConfig, client s3CapabilityProbeClient) error {
	retryDelays := s.r2PermissionRetryDelays
	if !isCloudflareR2Endpoint(cfg.Endpoint) {
		retryDelays = nil
	}
	for attempt := 0; ; attempt++ {
		err := s.probeCapabilitiesOnce(ctx, cfg, client)
		if err == nil {
			return nil
		}
		var capabilityErr *StorageCapabilityError
		if !errors.As(err, &capabilityErr) || attempt >= len(retryDelays) {
			return err
		}

		slog.WarnContext(ctx, "Cloudflare R2 capability probe denied; retrying before reporting missing permissions",
			"attempt", attempt+1,
			"max_attempts", len(retryDelays)+1,
			"missing", capabilityErr.Missing,
			"endpoint", cfg.Endpoint,
		)
		if err := waitForStorageProbeRetry(ctx, retryDelays[attempt]); err != nil {
			return err
		}
	}
}

func (s *S3CompatibleService) probeCapabilitiesOnce(ctx context.Context, cfg *S3CompatibleConfig, client s3CapabilityProbeClient) error {
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
			delivered, deliveryErr := s.readDeliveryObject(ctx, probeKey, cfg, int64(len(s3CapabilityProbePayload)))
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cleanupErr := client.DeleteObject(cleanupCtx, bucketName, probeKey)
			cancel()
			if cleanupErr != nil {
				if !isS3PermissionDenied(cleanupErr) {
					return newStorageProbeError(cfg, StorageCapabilityDeleteObject, cleanupErr)
				}
				missing = append(missing, StorageCapabilityDeleteObject)
			} else if deliveryErr != nil {
				return newStorageProbeError(cfg, StorageCapabilityPublicRead, deliveryErr)
			} else if delivered == nil || string(delivered.Data) != s3CapabilityProbePayload {
				return newStorageProbeError(cfg, StorageCapabilityPublicRead, errors.New(msgProbeCapabilitiesOnce))
			}
		}
	}

	if len(missing) > 0 {
		return newStorageCapabilityError(cfg, missing)
	}
	return nil
}

type s3BucketExistenceChecker interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
}

func ensureS3BucketAccessible(ctx context.Context, client s3BucketExistenceChecker, bucketName string) error {
	if client == nil {
		return errors.New(msgEnsureS3BucketAccessible)
	}
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf(msgEnsureS3BucketAccessible2, err)
	}
	if !exists {
		return errors.New(msgEnsureS3BucketAccessible3)
	}
	return nil
}

func waitForStorageProbeRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
