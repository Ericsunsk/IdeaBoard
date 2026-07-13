package storage

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
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
		url, err := service.publicObjectURL(&S3CompatibleConfig{
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

	t.Run("path style prefixes bucket exactly once", func(t *testing.T) {
		url, err := service.publicObjectURL(&S3CompatibleConfig{
			BucketName:     "feng-assets",
			AccessDomain:   "https://minio.example.com",
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

	t.Run("path style does not duplicate bucket when domain already contains it", func(t *testing.T) {
		url, err := service.publicObjectURL(&S3CompatibleConfig{
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
