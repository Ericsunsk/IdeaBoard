package media

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	identitydomain "ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
)

// DiceBear avatars project to a public route that delivers imgproxy bytes;
// signed internal requests never become browser-facing URLs.
func TestV1UserDicebearAvatarProxiedViaImgproxy(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	const dicebear = "https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=user-1"
	const oauth = "https://cdn.discordapp.com/avatars/123/abc.png"
	const evil = "https://api.dicebear.com.evil.com/9.x/big-ears-neutral/svg?seed=evil"
	repo := &avatarProfileRepository{users: map[int64]*identitydomain.User{
		f.owner:  {ID: f.owner, StorageUID: "USER01", Avatar: dicebear, Role: "user", Status: identitydomain.StatusActive},
		f.member: {ID: f.member, StorageUID: "USER02", Avatar: oauth, Role: "user", Status: identitydomain.StatusActive},
		f.other:  {ID: f.other, StorageUID: "USER03", Avatar: evil, Role: "user", Status: identitydomain.StatusActive},
		100:      {ID: 100, StorageUID: "USER04", Avatar: "", Role: "user", Status: identitydomain.StatusActive},
	}}
	var renderCalls atomic.Int32
	var requestedSize atomic.Int32
	cfg := imgproxy.Config{KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderCalls.Add(1)
		signature, path, ok := strings.Cut(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
		want, err := cfg.SignPath("/" + path)
		if !ok || err != nil || signature != want {
			t.Error("invalid internal render signature")
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
		_, escapedSource, ok := strings.Cut(path, "/plain/")
		source, err := url.PathUnescape(strings.TrimSuffix(escapedSource, "@webp"))
		if !ok || err != nil || source != dicebear || r.URL.RawQuery != "" {
			t.Errorf("internal render source = %q (%v), want original seed URL", source, err)
		}
		if strings.Contains(path, "/exp:") {
			t.Error("permanent seed avatar must keep a stable render URL")
		}
		if !strings.HasPrefix(path, fmt.Sprintf("rs:fill:%d:%d/plain/", requestedSize.Load(), requestedSize.Load())) {
			t.Errorf("internal render path = %q, want requested avatar size", path)
		}
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte("rendered avatar"))
	}))
	defer upstream.Close()
	cfg.BaseURL = upstream.URL
	engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{
		UserProfile: runtimehttp.NewV1UserProfileDirectory(repo),
		Media:       ContractV1MediaDeps{MediaRenderer: imgproxy.NewRenderer(cfg, nil)},
	})

	status, _, payload := doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("GET /user = %d %v, want 200", status, payload)
	}
	proxied, _ := v1Data(t, payload).(map[string]any)["avatar"].(string)
	if !strings.HasPrefix(proxied, "/api/v1/public/users/USER01/avatar?v=") {
		t.Fatalf("dicebear avatar = %q, want a versioned same-origin public route", proxied)
	}
	if renderCalls.Load() != 0 {
		t.Fatal("profile projection must not render avatar bytes")
	}
	for _, size := range []int{32, 64, 128, 256} {
		requestedSize.Store(int32(size))
		request := httptest.NewRequest(http.MethodGet, proxied+fmt.Sprintf("&size=%d", size), nil)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != "rendered avatar" {
			t.Fatalf("public avatar size %d = %d %q", size, response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Type") != "image/webp" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("public avatar headers = %v", response.Header())
		}
	}
	if renderCalls.Load() != 4 {
		t.Fatalf("internal renders = %d, want 4", renderCalls.Load())
	}

	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.member})
	if status != http.StatusOK || v1Data(t, payload).(map[string]any)["avatar"] != oauth {
		t.Fatalf("OAuth avatar = %d %v, want verbatim passthrough", status, payload)
	}

	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.other})
	if status != http.StatusOK || v1Data(t, payload).(map[string]any)["avatar"] != evil {
		t.Fatalf("lookalike-host avatar = %d %v, want verbatim passthrough", status, payload)
	}

	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: 100})
	if status != http.StatusOK {
		t.Fatalf("GET /user = %d %v, want 200", status, payload)
	}
	if _, present := v1Data(t, payload).(map[string]any)["avatar"]; present {
		t.Fatalf("empty avatar = %v, want the field omitted", payload)
	}
}

// Profile projections keep the public route even when the renderer is unavailable.
func TestV1UserDicebearAvatarWithoutRenderer(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	const dicebear = "https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=user-1"
	repo := &avatarProfileRepository{users: map[int64]*identitydomain.User{
		f.owner: {ID: f.owner, StorageUID: "USER01", Avatar: dicebear, Role: "user", Status: identitydomain.StatusActive},
	}}
	engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{
		UserProfile: runtimehttp.NewV1UserProfileDirectory(repo),
	})
	status, _, payload := doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("GET /user = %d %v", status, payload)
	}
	avatar, _ := v1Data(t, payload).(map[string]any)["avatar"].(string)
	if !strings.HasPrefix(avatar, "/api/v1/public/users/USER01/avatar?v=") {
		t.Fatalf("renderer-less avatar = %q, want public route", avatar)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, avatar, nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("renderer-less public avatar = %d, want 503", response.Code)
	}
}
