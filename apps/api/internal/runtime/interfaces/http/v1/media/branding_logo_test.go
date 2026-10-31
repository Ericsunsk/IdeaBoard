package media

import (
	"net/http"
	"net/url"
	"testing"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

func newBrandingLogoEngine(cfg imgproxy.Config) *gin.Engine {
	return newV1MediaTestEngine(tenancy.NewInMemoryStoreForTests(), newStaticUserDirectory(), ContractV1Deps{
		Media: ContractV1MediaDeps{
			MediaRenderer: imgproxy.NewRenderer(cfg, fakeV1StorageProfiler{profile: &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal, Enabled: true}}),
		},
	})
}

func TestPublicBrandingLogoRoute(t *testing.T) {
	cfg := v1TestImgproxyConfig(t)
	engine := newBrandingLogoEngine(cfg)
	for _, key := range []string{"uploads/site/assets/logo_draft1.png", "uploads/site/assets/logo_draft2.png"} {
		route := "/api/v1/public/branding/logo" + "?key=" + url.QueryEscape(key)
		assertImgproxyImage(t, engine, cfg, v1Call{method: "GET", path: route}, "logo-", key, "no-store", "/plain/local:///"+key+"@webp")
	}
	for _, key := range []string{"", "/uploads/site/assets/logo.png", "uploads/site/assets/../secret.png", "uploads/avatars/user/avatar.png", "https://example.com/logo.png"} {
		status, _, _ := doV1RoundTrip(t, engine, v1Call{method: "GET", path: "/api/v1/public/branding/logo" + "?key=" + url.QueryEscape(key)}, "logo-")
		if status != http.StatusBadRequest {
			t.Fatalf("invalid key %q status = %d", key, status)
		}
	}
}

func TestPublicBrandingLogoRouteFailClosed(t *testing.T) {
	engine := newBrandingLogoEngine(imgproxy.Config{})
	status, _, _ := doV1RoundTrip(t, engine, v1Call{method: "GET", path: "/api/v1/public/branding/logo" + "?key=uploads/site/assets/logo.png"}, "logo-")
	if status != http.StatusConflict {
		t.Fatalf("invalid renderer status = %d", status)
	}
}
