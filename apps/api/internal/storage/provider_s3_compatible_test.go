package storage

import (
	"context"
	"errors"
	"testing"
)

type stubS3BucketExistenceChecker struct {
	exists bool
	err    error
}

func (s stubS3BucketExistenceChecker) BucketExists(_ context.Context, _ string) (bool, error) {
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
		err := ensureS3BucketAccessible(stubS3BucketExistenceChecker{err: errors.New("access denied")}, "feng-assets")
		if err == nil || err.Error() != "验证 Bucket 失败: access denied" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error when bucket does not exist", func(t *testing.T) {
		err := ensureS3BucketAccessible(stubS3BucketExistenceChecker{exists: false}, "feng-assets")
		if err == nil || err.Error() != "Bucket 不存在或当前凭证无权访问" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns nil when bucket exists", func(t *testing.T) {
		if err := ensureS3BucketAccessible(stubS3BucketExistenceChecker{exists: true}, "feng-assets"); err != nil {
			t.Fatalf("unexpected error: %v", err)
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
		t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "")
		cfg := *base
		cfg.Endpoint = "s3.us-east-1.amazonaws.com"
		if err := service.validateBaseConfig(&cfg); err != nil {
			t.Fatalf("expected plain public host endpoint to pass, got %v", err)
		}
	})

	t.Run("plain localhost endpoint rejected by default", func(t *testing.T) {
		t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "")
		cfg := *base
		cfg.Endpoint = "127.0.0.1:9000"
		if err := service.validateBaseConfig(&cfg); err == nil {
			t.Fatal("expected localhost endpoint to be rejected")
		}
	})

	t.Run("plain localhost endpoint allowed when allowlisted", func(t *testing.T) {
		t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "127.0.0.1")
		cfg := *base
		cfg.Endpoint = "127.0.0.1:9000"
		if err := service.validateBaseConfig(&cfg); err != nil {
			t.Fatalf("expected localhost endpoint to pass with allowlist, got %v", err)
		}
	})
}
