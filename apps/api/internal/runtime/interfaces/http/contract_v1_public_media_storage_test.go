package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/tenancy"
)

func TestPublicMediaUsesActiveStorageProfile(t *testing.T) {
	const bucket = "public-images"
	for _, tc := range []struct {
		name       string
		profiler   imgproxy.ProfileResolver
		wantStatus int
		wantSource string
	}{
		{"local", fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal, Enabled: true}}, http.StatusOK, "local:///"},
		{"s3 canonical key", fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderS3Compatible, Enabled: true, Config: map[string]interface{}{"bucket_name": bucket}}}, http.StatusOK, "s3://" + bucket + "/"},
		{"no resolver", nil, http.StatusConflict, ""},
		{"resolver error", fakeV1StorageProfiler{err: context.DeadlineExceeded}, http.StatusConflict, ""},
		{"disabled", fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal}}, http.StatusConflict, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const avatarKey = "uploads/avatars/AVTR01/abcdef1234567890.png"
			const logoKey = "uploads/site/assets/logo_abcdef1234567890.png"
			engine := newV1MediaTestEngine(tenancy.NewInMemoryStoreForTests(), newStaticUserDirectory(), ContractV1Deps{
				MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), tc.profiler),
				AvatarProfile: &fakeAvatarUID{uid: "AVTR01", avatar: avatarKey},
			})
			for _, asset := range []struct{ route, key string }{
				{"/api/v1/public/users/AVTR01/avatar", avatarKey},
				{"/api/v1/public/branding/logo" + "?key=" + url.QueryEscape(logoKey), logoKey},
			} {
				status, headers, body := doV1RoundTrip(t, engine, v1Call{method: "GET", path: asset.route}, "public-media-")
				if status != tc.wantStatus {
					t.Fatalf("%s status = %d, want %d", asset.route, status, tc.wantStatus)
				}
				if status == http.StatusOK {
					if !strings.Contains(body, "/plain/"+tc.wantSource+asset.key+"@webp") {
						t.Fatalf("wrong source: %s", body)
					}
					if headers.Get("Cache-Control") != "no-store" {
						t.Fatal("mutable image must not be cached")
					}
				}
			}
		})
	}
}
