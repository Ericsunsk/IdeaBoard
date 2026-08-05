package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	s3SmokeEndpointEnv        = "API_TEST_S3_ENDPOINT"
	s3SmokeAccessKeyIDEnv     = "API_TEST_S3_ACCESS_KEY_ID"
	s3SmokeSecretAccessKeyEnv = "API_TEST_S3_SECRET_ACCESS_KEY"
	s3SmokeBucketEnv          = "API_TEST_S3_BUCKET"
	s3SmokeAccessDomainEnv    = "API_TEST_S3_ACCESS_DOMAIN"
	s3SmokeRegionEnv          = "API_TEST_S3_REGION"
	s3SmokeForcePathStyleEnv  = "API_TEST_S3_FORCE_PATH_STYLE"
)

func TestS3CompatibleUploadReadPublicDeleteSmoke(t *testing.T) {
	cfg, missing, err := s3CompatibleSmokeConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Skipf("set %s to run S3/R2-compatible storage smoke test", strings.Join(missing, ", "))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	service := NewS3CompatibleService()
	if err := service.TestConnectionWithConfig(ctx, cfg); err != nil {
		t.Fatalf("S3-compatible bucket connection failed: %v", err)
	}

	payload := s3SmokePNGPayload()
	dirPrefix := fmt.Sprintf("smoke/s3-compatible/%d", time.Now().UnixNano())
	ref, err := service.uploadBytesWithConfigRef(ctx, "smoke.png", payload, cfg, StorageStoreOptions{DirPrefix: dirPrefix})
	if err != nil {
		t.Fatalf("upload smoke object failed: %v", err)
	}

	cleanupKey := ref.StorageKey
	t.Cleanup(func() {
		if cleanupKey == "" {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := NewS3CompatibleService().DeleteWithConfig(cleanupCtx, cleanupKey, cfg); err != nil {
			t.Logf("cleanup smoke object %q failed: %v", cleanupKey, err)
		}
	})

	if ref.URL == "" {
		t.Fatal("uploaded ref URL is empty")
	}
	if ref.StorageKey == "" {
		t.Fatal("uploaded ref storage key is empty")
	}
	if !strings.Contains(ref.URL, ref.StorageKey) {
		t.Fatalf("uploaded ref URL %q does not contain storage key %q", ref.URL, ref.StorageKey)
	}
	if ref.SizeBytes != int64(len(payload)) {
		t.Fatalf("uploaded ref size=%d, want %d", ref.SizeBytes, len(payload))
	}

	stored, err := service.readWithConfig(ctx, ref.StorageKey, cfg, int64(len(payload)+1024))
	if err != nil {
		t.Fatalf("read smoke object failed: %v", err)
	}
	assertStoredSmokeObject(t, stored, ref.StorageKey, payload)
	assertPublicSmokeObjectReadable(ctx, t, ref.URL, payload)

	// Recreate the service and config to exercise the credential/profile reload path
	// without reusing the cached MinIO client from the upload step.
	reloadedCfg := *cfg
	reloadedService := NewS3CompatibleService()
	reloadedObject, err := reloadedService.readWithConfig(ctx, ref.StorageKey, &reloadedCfg, int64(len(payload)+1024))
	if err != nil {
		t.Fatalf("read smoke object after profile reload failed: %v", err)
	}
	assertStoredSmokeObject(t, reloadedObject, ref.StorageKey, payload)

	if err := service.DeleteWithConfig(ctx, ref.StorageKey, cfg); err != nil {
		t.Fatalf("delete smoke object failed: %v", err)
	}
	if _, err := reloadedService.readWithConfig(ctx, ref.StorageKey, &reloadedCfg, int64(len(payload)+1024)); err == nil {
		t.Fatal("expected deleted smoke object to be unreadable")
	}
	cleanupKey = ""
}

func s3CompatibleSmokeConfigFromEnv() (*S3CompatibleConfig, []string, error) {
	required := []string{
		s3SmokeEndpointEnv,
		s3SmokeAccessKeyIDEnv,
		s3SmokeSecretAccessKeyEnv,
		s3SmokeBucketEnv,
		s3SmokeAccessDomainEnv,
	}
	missing := make([]string, 0, len(required))
	env := func(name string) string {
		return strings.TrimSpace(os.Getenv(name))
	}
	for _, name := range required {
		if env(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, missing, nil
	}

	forcePathStyle, err := optionalBoolEnv(s3SmokeForcePathStyleEnv)
	if err != nil {
		return nil, nil, err
	}

	return &S3CompatibleConfig{
		Enabled:         true,
		ProfileID:       "api-test-s3-compatible-smoke",
		AccessKeyID:     env(s3SmokeAccessKeyIDEnv),
		SecretAccessKey: env(s3SmokeSecretAccessKeyEnv),
		BucketName:      env(s3SmokeBucketEnv),
		Endpoint:        env(s3SmokeEndpointEnv),
		RegionID:        env(s3SmokeRegionEnv),
		AccessDomain:    strings.TrimRight(env(s3SmokeAccessDomainEnv), "/"),
		ForcePathStyle:  forcePathStyle,
	}, nil, nil
}

func optionalBoolEnv(name string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	switch strings.ToLower(raw) {
	case "":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean value", name)
	}
}

func assertStoredSmokeObject(t *testing.T, got *StoredUploadObject, wantKey string, wantData []byte) {
	t.Helper()
	if got == nil {
		t.Fatal("stored object is nil")
		return
	}
	if got.ObjectKey != wantKey {
		t.Fatalf("stored object key=%q, want %q", got.ObjectKey, wantKey)
	}
	if !bytes.Equal(got.Data, wantData) {
		t.Fatalf("stored object data mismatch: got %d bytes, want %d bytes", len(got.Data), len(wantData))
	}
}

func assertPublicSmokeObjectReadable(ctx context.Context, t *testing.T, rawURL string, wantData []byte) {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatalf("build public object request failed: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(len(wantData)+1)))
			closeErr := resp.Body.Close()
			switch {
			case readErr != nil:
				lastErr = readErr
			case closeErr != nil:
				lastErr = closeErr
			case resp.StatusCode != http.StatusOK:
				lastErr = fmt.Errorf("status=%d body_bytes=%d", resp.StatusCode, len(body))
			case !bytes.Equal(body, wantData):
				lastErr = fmt.Errorf("body mismatch: got %d bytes, want %d bytes", len(body), len(wantData))
			default:
				return
			}
		}

		if ctx.Err() != nil {
			t.Fatalf("public object URL did not return uploaded payload before context ended: %v", ctx.Err())
		}
		if time.Now().After(deadline) {
			t.Fatalf("public object URL did not return uploaded payload: %v", lastErr)
		}
		time.Sleep(2 * time.Second)
	}
}

func s3SmokePNGPayload() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x60, 0x60, 0xf8, 0x0f,
		0x00, 0x01, 0x05, 0x01, 0x02, 0xa5, 0x27, 0xde,
		0xfc, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
