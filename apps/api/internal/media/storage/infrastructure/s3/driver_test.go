package s3

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"

	"github.com/minio/minio-go/v7"
)

type testStorageOutboundPolicyProvider struct{ hosts []string }

func (p testStorageOutboundPolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

type mutableStorageOutboundPolicyProvider struct {
	hosts   []string
	err     error
	seenCtx context.Context
}

func (p *mutableStorageOutboundPolicyProvider) GetOutboundPolicy(ctx context.Context) (settings.OutboundPolicySetting, error) {
	p.seenCtx = ctx
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, p.err
}

type stubS3BucketExistenceChecker struct {
	exists  bool
	err     error
	seenCtx context.Context
}

type stubS3CapabilityProbeClient struct {
	listErr    error
	putErr     error
	deleteErr  error
	listErrors []error
	deleteErrs []error
	listCalls  int
	putKeys    []string
	delKeys    []string
}

func (s *stubS3CapabilityProbeClient) ListBucket(context.Context, string, string) error {
	if s.listCalls < len(s.listErrors) {
		err := s.listErrors[s.listCalls]
		s.listCalls++
		return err
	}
	s.listCalls++
	return s.listErr
}

func (s *stubS3CapabilityProbeClient) PutObject(_ context.Context, _, objectName string) error {
	s.putKeys = append(s.putKeys, objectName)
	return s.putErr
}

func (s *stubS3CapabilityProbeClient) DeleteObject(_ context.Context, _, objectName string) error {
	s.delKeys = append(s.delKeys, objectName)
	if callIndex := len(s.delKeys) - 1; callIndex < len(s.deleteErrs) {
		return s.deleteErrs[callIndex]
	}
	return s.deleteErr
}

func newCapabilityProbeTestService() *S3CompatibleService {
	service := NewS3CompatibleService()
	service.deliveryRead = func(_ context.Context, key string, _ *S3CompatibleConfig, _ int64) (*ports.StoredUploadObject, error) {
		return &ports.StoredUploadObject{ObjectKey: key, Data: []byte(s3CapabilityProbePayload)}, nil
	}
	return service
}

func (s *stubS3BucketExistenceChecker) BucketExists(ctx context.Context, _ string) (bool, error) {
	s.seenCtx = ctx
	return s.exists, s.err
}

func TestParseS3Endpoint(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantHost   string
		wantSecure bool
	}{
		{
			name:       "plain host defaults to https",
			raw:        "s3.us-east-1.amazonaws.com",
			wantHost:   "s3.us-east-1.amazonaws.com",
			wantSecure: true,
		},
		{
			name:       "http endpoint stays insecure",
			raw:        "http://127.0.0.1:9000",
			wantHost:   "127.0.0.1:9000",
			wantSecure: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotSecure, err := parseS3Endpoint(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotHost != tt.wantHost || gotSecure != tt.wantSecure {
				t.Fatalf("got (%q, %v), want (%q, %v)", gotHost, gotSecure, tt.wantHost, tt.wantSecure)
			}
		})
	}
}

func TestS3CompatiblePublicObjectURL(t *testing.T) {
	service := NewS3CompatibleService()

	t.Run("virtual host style uses direct access domain", func(t *testing.T) {
		url, err := service.PublicObjectURL(&S3CompatibleConfig{
			BucketName:   "feng-assets",
			AccessDomain: "https://feng-assets.s3.us-east-1.amazonaws.com",
		}, "uploads/2026/04/22/demo.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "https://feng-assets.s3.us-east-1.amazonaws.com/uploads/2026/04/22/demo.png"
		if url != want {
			t.Fatalf("url = %q, want %q", url, want)
		}
	})

	t.Run("endpoint path style does not alter access domain", func(t *testing.T) {
		url, err := service.PublicObjectURL(&S3CompatibleConfig{
			BucketName:     "feng-assets",
			AccessDomain:   "https://minio.example.com",
			ForcePathStyle: true,
		}, "uploads/2026/04/22/demo.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "https://minio.example.com/uploads/2026/04/22/demo.png"
		if url != want {
			t.Fatalf("url = %q, want %q", url, want)
		}
	})

	t.Run("access domain may contain an explicit bucket path", func(t *testing.T) {
		url, err := service.PublicObjectURL(&S3CompatibleConfig{
			BucketName:     "feng-assets",
			AccessDomain:   "https://minio.example.com/feng-assets",
			ForcePathStyle: true,
		}, "uploads/2026/04/22/demo.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "https://minio.example.com/feng-assets/uploads/2026/04/22/demo.png"
		if url != want {
			t.Fatalf("url = %q, want %q", url, want)
		}
	})
}

func TestS3CompatibleSignedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"/>`))
	}))
	t.Cleanup(server.Close)
	restore := outbound.ConfigurePolicyProvider(testStorageOutboundPolicyProvider{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	url, err := NewS3CompatibleService().SignedURLWithConfig(context.Background(), "uploads/asset/preview.webp", time.Hour, &S3CompatibleConfig{
		Enabled:         true,
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
		BucketName:      "ideaboard",
		Endpoint:        server.URL,
		AccessDomain:    server.URL,
		ForcePathStyle:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "X-Amz-Expires=3600") {
		t.Fatalf("signed URL = %q", url)
	}
}

func TestS3CompatibleReadUsesAccessDomainDeliveryPlane(t *testing.T) {
	restore := outbound.ConfigurePolicyProvider(testStorageOutboundPolicyProvider{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	var endpointCalls atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endpointCalls.Add(1)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`))
	}))
	t.Cleanup(endpoint.Close)

	payload := []byte{0x89, 0x50, 0x4e, 0x47}
	var deliveryCalls atomic.Int64
	delivery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveryCalls.Add(1)
		if r.URL.Path != "/uploads/tasks/42/task/content/image.png" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(delivery.Close)

	s3Svc := NewS3CompatibleService()
	driver := NewS3CompatibleStorageDriver(s3Svc, remote.DownloadImage)
	object, err := driver.Read(context.Background(), &domain.StorageProfile{
		ID:       "r2-delivery-test",
		Provider: domain.StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "access-key",
			storageSecretAccessKeyName: "secret-key",
			"bucket_name":              "ideaboard",
			"endpoint":                 endpoint.URL,
			"access_domain":            delivery.URL,
			"force_path_style":         true,
		},
	}, "uploads/tasks/42/task/content/image.png", 50*1024*1024)
	if err != nil {
		t.Fatalf("public delivery read failed: %v", err)
	}
	if !slices.Equal(object.Data, payload) || object.ContentType != "image/png" {
		t.Fatalf("object=%+v, want access-domain payload", object)
	}
	if endpointCalls.Load() != 0 || deliveryCalls.Load() != 1 {
		t.Fatalf("calls endpoint=%d delivery=%d, want endpoint=0 delivery=1", endpointCalls.Load(), deliveryCalls.Load())
	}
}

func TestS3CompatiblePutRetriesTransientR2AccessDenied(t *testing.T) {
	service := NewS3CompatibleService()
	service.r2PermissionRetryDelays = []time.Duration{0}
	cfg := &S3CompatibleConfig{Endpoint: "account-id.r2.cloudflarestorage.com"}
	body := strings.NewReader("payload")
	permissionErr := minio.ErrorResponse{Code: "AccessDenied", StatusCode: 403}
	attempts := 0
	payloads := make([]string, 0, 2)
	err := service.putObjectWithRetry(context.Background(), cfg, body, func(context.Context) error {
		attempts++
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		payloads = append(payloads, string(data))
		if attempts == 1 {
			return permissionErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want transient permission retry", attempts)
	}
	if !slices.Equal(payloads, []string{"payload", "payload"}) {
		t.Fatalf("payloads = %#v, want body rewound before retry", payloads)
	}
}

func TestS3ClientTransportUsesFreshConnectionsForR2(t *testing.T) {
	transport, err := s3ClientTransport(&S3CompatibleConfig{
		Endpoint: "account-id.r2.cloudflarestorage.com",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	r2Transport, ok := transport.(*http.Transport)
	if !ok || !r2Transport.DisableKeepAlives {
		t.Fatalf("transport = %#v, want R2 transport with keep-alives disabled", transport)
	}

	genericTransport, err := s3ClientTransport(&S3CompatibleConfig{
		Endpoint: "s3.us-east-1.amazonaws.com",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if genericTransport != nil {
		t.Fatalf("generic transport = %#v, want MinIO default transport", genericTransport)
	}
}

func TestEnsureS3BucketAccessible(t *testing.T) {
	t.Run("returns error when bucket check fails", func(t *testing.T) {
		err := ensureS3BucketAccessible(context.Background(), &stubS3BucketExistenceChecker{err: errors.New("access denied")}, "feng-assets")
		if err == nil || err.Error() != "验证 Bucket 失败: access denied" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error when bucket does not exist", func(t *testing.T) {
		err := ensureS3BucketAccessible(context.Background(), &stubS3BucketExistenceChecker{exists: false}, "feng-assets")
		if err == nil || err.Error() != "bucket 不存在或当前凭证无权访问" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns nil when bucket exists", func(t *testing.T) {
		if err := ensureS3BucketAccessible(context.Background(), &stubS3BucketExistenceChecker{exists: true}, "feng-assets"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("passes caller context to bucket check", func(t *testing.T) {
		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "trace-id")
		checker := &stubS3BucketExistenceChecker{exists: true}
		if err := ensureS3BucketAccessible(ctx, checker, "feng-assets"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if checker.seenCtx != ctx {
			t.Fatalf("BucketExists ctx was not caller ctx")
		}
	})
}

func TestProbeS3Capabilities(t *testing.T) {
	t.Run("retries transient R2 access denied responses", func(t *testing.T) {
		permissionErr := minio.ErrorResponse{Code: "AccessDenied", Message: "Access Denied", StatusCode: 403}
		client := &stubS3CapabilityProbeClient{
			listErrors: []error{permissionErr, nil},
			deleteErrs: []error{permissionErr, nil},
		}
		service := newCapabilityProbeTestService()
		service.r2PermissionRetryDelays = []time.Duration{0}
		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{
			BucketName: "feng-assets",
			Endpoint:   "account-id.r2.cloudflarestorage.com",
		}, client)
		if err != nil {
			t.Fatalf("transient R2 AccessDenied must not become a permanent permission error: %v", err)
		}
		if client.listCalls < 2 || len(client.delKeys) < 3 {
			t.Fatalf("probe calls = list:%d delete:%d, want retries before accepting a permission failure", client.listCalls, len(client.delKeys))
		}
	})

	t.Run("bounds persistent R2 permission retries", func(t *testing.T) {
		permissionErr := minio.ErrorResponse{Code: "AccessDenied", Message: "Access Denied", StatusCode: 403}
		client := &stubS3CapabilityProbeClient{listErr: permissionErr, deleteErr: permissionErr}
		service := newCapabilityProbeTestService()
		service.r2PermissionRetryDelays = []time.Duration{0, 0}
		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{
			BucketName: "feng-assets",
			Endpoint:   "account-id.r2.cloudflarestorage.com",
		}, client)
		var capabilityErr *StorageCapabilityError
		if !errors.As(err, &capabilityErr) {
			t.Fatalf("error = %v, want StorageCapabilityError after bounded retries", err)
		}
		if client.listCalls != 3 || len(client.delKeys) != 3 {
			t.Fatalf("probe calls = list:%d delete:%d, want exactly three attempts", client.listCalls, len(client.delKeys))
		}
	})

	t.Run("does not report R2 request configuration errors as missing permissions", func(t *testing.T) {
		upstreamErr := minio.ErrorResponse{
			Code:       "AuthorizationHeaderMalformed",
			Message:    "The authorization header is malformed; region must be auto",
			StatusCode: 400,
		}
		client := &stubS3CapabilityProbeClient{listErr: upstreamErr, deleteErr: upstreamErr}
		service := newCapabilityProbeTestService()

		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{
			BucketName: "feng-assets",
			Endpoint:   "account-id.r2.cloudflarestorage.com",
		}, client)
		var capabilityErr *StorageCapabilityError
		if errors.As(err, &capabilityErr) {
			t.Fatalf("error = %v, must preserve the R2 request error instead of reporting missing permissions", err)
		}
		var probeErr *StorageProbeError
		if !errors.As(err, &probeErr) {
			t.Fatalf("error = %v, want StorageProbeError", err)
		}
		if err == nil || !strings.Contains(err.Error(), "AuthorizationHeaderMalformed") {
			t.Fatalf("error = %v, want original R2 request error", err)
		}
		if !strings.Contains(probeErr.UserMessage(), "Region 必须为 auto") {
			t.Fatalf("user message = %q, want R2 region guidance", probeErr.UserMessage())
		}
	})

	t.Run("accepts all capabilities and removes probe object", func(t *testing.T) {
		client := &stubS3CapabilityProbeClient{}
		service := newCapabilityProbeTestService()
		if err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{BucketName: "feng-assets"}, client); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(client.putKeys) != 1 || len(client.delKeys) != 2 {
			t.Fatalf("probe calls = put:%d delete:%d, want put:1 delete:2", len(client.putKeys), len(client.delKeys))
		}
	})

	t.Run("reports missing permissions and cleans up probe object", func(t *testing.T) {
		permissionErr := minio.ErrorResponse{Code: "AccessDenied", Message: "Access Denied", StatusCode: 403}
		client := &stubS3CapabilityProbeClient{listErr: permissionErr}
		service := newCapabilityProbeTestService()
		cfg := &S3CompatibleConfig{BucketName: "feng-assets"}
		err := service.probeCapabilities(context.Background(), cfg, client)
		var capabilityErr *StorageCapabilityError
		if !errors.As(err, &capabilityErr) {
			t.Fatalf("error = %v, want StorageCapabilityError", err)
		}
		want := []StorageCapability{StorageCapabilityListBucket}
		if !slices.Equal(capabilityErr.Missing, want) {
			t.Fatalf("missing = %#v, want %#v", capabilityErr.Missing, want)
		}
		if len(client.putKeys) != 1 || len(client.delKeys) != 2 {
			t.Fatalf("probe calls = put:%d delete:%d, want put:1 delete:2", len(client.putKeys), len(client.delKeys))
		}
		if client.delKeys[0] == client.putKeys[0] || client.delKeys[1] != client.putKeys[0] {
			t.Fatalf("cleanup used unexpected key: put=%#v delete=%#v", client.putKeys, client.delKeys)
		}
	})

	t.Run("reports public delivery failure and cleans up probe object", func(t *testing.T) {
		client := &stubS3CapabilityProbeClient{}
		service := newCapabilityProbeTestService()
		service.deliveryRead = func(context.Context, string, *S3CompatibleConfig, int64) (*ports.StoredUploadObject, error) {
			return nil, errors.New("access domain returned 403")
		}
		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{BucketName: "feng-assets"}, client)
		var probeErr *StorageProbeError
		if !errors.As(err, &probeErr) || probeErr.Capability != StorageCapabilityPublicRead {
			t.Fatalf("error = %v, want public delivery StorageProbeError", err)
		}
		if !strings.Contains(probeErr.UserMessage(), "公开访问域名读取失败") {
			t.Fatalf("user message = %q, want access-domain guidance", probeErr.UserMessage())
		}
		if len(client.putKeys) != 1 || len(client.delKeys) != 2 || client.delKeys[1] != client.putKeys[0] {
			t.Fatalf("probe cleanup = put:%#v delete:%#v", client.putKeys, client.delKeys)
		}
	})

	t.Run("reports cleanup permission before public delivery failure", func(t *testing.T) {
		permissionErr := minio.ErrorResponse{Code: "AccessDenied", Message: "Access Denied", StatusCode: 403}
		client := &stubS3CapabilityProbeClient{deleteErrs: []error{nil, permissionErr}}
		service := newCapabilityProbeTestService()
		service.deliveryRead = func(context.Context, string, *S3CompatibleConfig, int64) (*ports.StoredUploadObject, error) {
			return nil, errors.New("access domain returned 403")
		}
		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{BucketName: "feng-assets"}, client)
		var capabilityErr *StorageCapabilityError
		if !errors.As(err, &capabilityErr) {
			t.Fatalf("error = %v, want StorageCapabilityError", err)
		}
		if !slices.Equal(capabilityErr.Missing, []StorageCapability{StorageCapabilityDeleteObject}) {
			t.Fatalf("missing = %#v, want DeleteObject only", capabilityErr.Missing)
		}
	})

	t.Run("reports put permission after delete probe succeeds", func(t *testing.T) {
		client := &stubS3CapabilityProbeClient{putErr: minio.ErrorResponse{Code: "AccessDenied", StatusCode: 403}}
		service := newCapabilityProbeTestService()
		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{BucketName: "feng-assets"}, client)
		var capabilityErr *StorageCapabilityError
		if !errors.As(err, &capabilityErr) {
			t.Fatalf("error = %v, want StorageCapabilityError", err)
		}
		if !slices.Equal(capabilityErr.Missing, []StorageCapability{StorageCapabilityPutObject}) {
			t.Fatalf("missing = %#v, want PutObject only", capabilityErr.Missing)
		}
		if len(client.delKeys) != 1 {
			t.Fatalf("delete calls = %d, want 1", len(client.delKeys))
		}
	})

	t.Run("does not create probe object when delete is denied", func(t *testing.T) {
		client := &stubS3CapabilityProbeClient{deleteErr: minio.ErrorResponse{Code: "AccessDenied", StatusCode: 403}}
		service := newCapabilityProbeTestService()
		err := service.probeCapabilities(context.Background(), &S3CompatibleConfig{BucketName: "feng-assets"}, client)
		var capabilityErr *StorageCapabilityError
		if !errors.As(err, &capabilityErr) {
			t.Fatalf("error = %v, want StorageCapabilityError", err)
		}
		if !slices.Equal(capabilityErr.Missing, []StorageCapability{StorageCapabilityDeleteObject}) {
			t.Fatalf("missing = %#v, want DeleteObject only", capabilityErr.Missing)
		}
		if len(client.putKeys) != 0 || len(client.delKeys) != 1 {
			t.Fatalf("object operations should not run after denied delete: put=%d delete=%d", len(client.putKeys), len(client.delKeys))
		}
	})
}

func TestStorageCapabilityErrorMessage(t *testing.T) {
	t.Run("uses IAM guidance for generic S3", func(t *testing.T) {
		err := (&StorageCapabilityError{Missing: []StorageCapability{StorageCapabilityListBucket, StorageCapabilityDeleteObject}}).Error()
		if !strings.Contains(err, "s3:ListBucket") || !strings.Contains(err, "s3:DeleteObject") || !strings.Contains(err, "Bucket ARN") {
			t.Fatalf("message = %q, want actionable capability guidance", err)
		}
	})

	t.Run("uses API token guidance for Cloudflare R2", func(t *testing.T) {
		err := (&StorageCapabilityError{
			Missing:  []StorageCapability{StorageCapabilityListBucket, StorageCapabilityDeleteObject},
			Endpoint: "https://account-id.r2.cloudflarestorage.com",
		}).Error()
		if !strings.Contains(err, "Object Read & Write") || !strings.Contains(err, "当前 Bucket") || strings.Contains(err, "Bucket ARN") {
			t.Fatalf("message = %q, want Cloudflare R2 token guidance", err)
		}
	})
}

func TestIsS3PermissionDenied(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "access denied", err: minio.ErrorResponse{Code: "AccessDenied", StatusCode: 403}, want: true},
		{name: "malformed authorization header", err: minio.ErrorResponse{Code: "AuthorizationHeaderMalformed", StatusCode: 400}, want: false},
		{name: "signature mismatch", err: minio.ErrorResponse{Code: "SignatureDoesNotMatch", StatusCode: 403}, want: false},
		{name: "invalid access key", err: minio.ErrorResponse{Code: "InvalidAccessKeyId", StatusCode: 403}, want: false},
		{name: "network or protocol error", err: errors.New("connection reset"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isS3PermissionDenied(tt.err); got != tt.want {
				t.Fatalf("isS3PermissionDenied() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestS3CompatibleValidateBaseConfigEndpointPolicy(t *testing.T) {
	service := NewS3CompatibleService()
	base := &S3CompatibleConfig{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		BucketName:      "bucket",
		AccessDomain:    "https://cdn.example.com",
	}

	t.Run("plain public host endpoint passes", func(t *testing.T) {
		restore := outbound.ConfigurePolicyProvider(testStorageOutboundPolicyProvider{})
		t.Cleanup(restore)
		cfg := *base
		cfg.Endpoint = "s3.us-east-1.amazonaws.com"
		if err := service.validateBaseConfig(context.Background(), &cfg); err != nil {
			t.Fatalf("expected plain public host endpoint to pass, got %v", err)
		}
	})

	t.Run("plain localhost endpoint rejected by default", func(t *testing.T) {
		restore := outbound.ConfigurePolicyProvider(testStorageOutboundPolicyProvider{})
		t.Cleanup(restore)
		cfg := *base
		cfg.Endpoint = "127.0.0.1:9000"
		if err := service.validateBaseConfig(context.Background(), &cfg); err == nil {
			t.Fatal("expected localhost endpoint to be rejected")
		}
	})

	t.Run("plain localhost endpoint allowed when allowlisted", func(t *testing.T) {
		restore := outbound.ConfigurePolicyProvider(testStorageOutboundPolicyProvider{hosts: []string{"127.0.0.1"}})
		t.Cleanup(restore)
		cfg := *base
		cfg.Endpoint = "127.0.0.1:9000"
		if err := service.validateBaseConfig(context.Background(), &cfg); err != nil {
			t.Fatalf("expected localhost endpoint to pass with allowlist, got %v", err)
		}
	})
}

func TestS3CompatibleClientRechecksEndpointPolicyBeforeUsingCachedClient(t *testing.T) {
	service := NewS3CompatibleService()
	provider := &mutableStorageOutboundPolicyProvider{hosts: []string{"127.0.0.1"}}
	restore := outbound.ConfigurePolicyProvider(provider)
	t.Cleanup(restore)

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "request-context")
	cfg := &S3CompatibleConfig{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		BucketName:      "bucket",
		Endpoint:        "127.0.0.1:9000",
		AccessDomain:    "http://127.0.0.1:9000",
	}

	if _, err := service.clientWithConfig(ctx, cfg); err != nil {
		t.Fatalf("initial allowlisted client creation failed: %v", err)
	}
	if provider.seenCtx != ctx {
		t.Fatal("endpoint policy did not receive caller context")
	}

	provider.hosts = nil
	if _, err := service.clientWithConfig(ctx, cfg); err == nil {
		t.Fatal("cached client remained usable after allowlist revocation")
	}

	provider.hosts = []string{"127.0.0.1"}
	provider.err = errors.New("settings database unavailable")
	if _, err := service.clientWithConfig(ctx, cfg); err == nil {
		t.Fatal("cached client remained usable when policy lookup failed")
	}
}

func TestS3CompatibleServiceVerifyObjectGenerationUsesAdapterMetadata(t *testing.T) {
	service := NewS3CompatibleService()
	cfg := &S3CompatibleConfig{Enabled: true, ProfileID: "profile", AccessKeyID: "ak", SecretAccessKey: "sk", BucketName: "bucket", Endpoint: "s3.example.com", AccessDomain: "https://bucket.example.com"}
	service.generationRead = func(context.Context, string, *S3CompatibleConfig) (int64, error) { return 7, nil }
	if err := service.VerifyObjectGeneration(context.Background(), "uploads/image.png", 7, cfg); err != nil {
		t.Fatalf("matching generation rejected: %v", err)
	}
	if err := service.VerifyObjectGeneration(context.Background(), "uploads/image.png", 8, cfg); !errors.Is(err, domain.ErrObjectGenerationUnavailable) {
		t.Fatalf("mismatched generation error = %v, want ErrObjectGenerationUnavailable", err)
	}
	service.generationRead = func(context.Context, string, *S3CompatibleConfig) (int64, error) { return 0, nil }
	if err := service.VerifyObjectGeneration(context.Background(), "uploads/image.png", 7, cfg); !errors.Is(err, domain.ErrObjectGenerationUnavailable) {
		t.Fatalf("missing generation error = %v, want ErrObjectGenerationUnavailable", err)
	}
}
