package v1

import (
	"context"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"

	"github.com/getkin/kin-openapi/openapi3"
)

type avatarProfileRepository struct {
	identityports.UserRepository
	users             map[int64]*identitydomain.User
	readErr, writeErr error
}

// errAvatarProfileUserNotFound is the fixture's unknown-uid signal. It
// aliases the platform not-found sentinel so the public avatar route
// converges it to 404 exactly like production.
var errAvatarProfileUserNotFound = database.ErrNotFound

func (r *avatarProfileRepository) GetByID(_ context.Context, id int64) (*identitydomain.User, error) {
	return r.users[id], r.readErr
}

func (r *avatarProfileRepository) GetByStorageUID(_ context.Context, storageUID string) (*identitydomain.User, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	for _, user := range r.users {
		if user != nil && strings.TrimSpace(user.StorageUID) == strings.TrimSpace(storageUID) {
			return user, nil
		}
	}
	return nil, errAvatarProfileUserNotFound
}

func (r *avatarProfileRepository) UpdateAvatarByID(_ context.Context, id int64, avatar string) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.users[id].Avatar = avatar
	return nil
}

func TestV1AvatarProfilePersistsAcrossHandlers(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	repo := &avatarProfileRepository{users: map[int64]*identitydomain.User{
		f.owner: {ID: f.owner, StorageUID: "AVTR01"},
		f.other: {ID: f.other, StorageUID: "AVTR02"},
	}}
	deps := ContractV1Deps{Media: f.media, MediaObjects: f.objects, AvatarObjects: f.objects,
		AvatarUID: NewV1AvatarDirectory(repo), AvatarProfile: runtimehttp.NewV1AvatarProfileDirectory(repo)}
	engine := newV1MediaTestEngine(f.store, f.dir, deps)
	call := v1Call{method: "PUT", path: "/api/v1/user/avatar", userID: f.owner,
		body:    string(v1GapsPNGBytes(512)),
		headers: map[string]string{"Content-Type": "image/png", "Idempotency-Key": "persist-avatar"}}
	repo.writeErr = errors.New("profile write unavailable")
	if status, _, payload := doV1(t, engine, call); status != http.StatusServiceUnavailable {
		t.Fatalf("failed avatar persistence = %d %v, want 503", status, payload)
	}
	if repo.users[f.owner].Avatar != "" {
		t.Fatal("failed write changed profile projection")
	}
	repo.writeErr = nil
	status, _, payload := doV1(t, engine, call)
	if status != http.StatusOK {
		t.Fatalf("avatar persistence retry = %d %v, want 200", status, payload)
	}
	avatar := v1Data(t, payload).(map[string]any)["avatar_url"]
	// A fresh handler has no upload replay cache and reads only the user record.
	engine = newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{
		AvatarProfile: runtimehttp.NewV1AvatarProfileDirectory(repo),
	})
	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusOK || v1Data(t, payload).(map[string]any)["avatar"] != avatar {
		t.Fatalf("fresh profile read = %d %v, want persisted avatar", status, payload)
	}
	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.other})
	if _, leaked := v1Data(t, payload).(map[string]any)["avatar"]; status != http.StatusOK || leaked {
		t.Fatalf("other user profile = %d %v, want no avatar", status, payload)
	}
	repo.readErr = errors.New("profile read unavailable")
	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("unavailable profile = %d %v, want 503", status, payload)
	}
}

func TestV1AvatarProfileWriterRequired(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{
		Media: f.media, MediaObjects: f.objects,
		AvatarUID: &fakeAvatarUID{uid: "AVTR01"},
	})
	status, _, payload := doV1(t, engine, v1Call{method: "PUT", path: "/api/v1/user/avatar",
		userID: f.owner, body: string(v1GapsPNGBytes(512)), headers: map[string]string{"Content-Type": "image/png"}})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("missing profile writer = %d %v, want 503", status, payload)
	}
	if len(f.objects.objects) != 0 {
		t.Fatal("missing profile writer must not write avatar bytes")
	}
}

// TestV1UserProjectsProfileFields pins the restored /user profile
// facts: storage namespace (users.storage_uid), RFC3339 timestamps,
// numeric status, and the persisted avatar projection.
func TestV1UserProjectsProfileFields(t *testing.T) {
	f := newV1MediaGapsFixture(t, nil)
	created := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	repo := &avatarProfileRepository{users: map[int64]*identitydomain.User{
		f.owner: {ID: f.owner, StorageUID: "USER01", Avatar: "https://cdn.example/a.png",
			Role: "user", Status: identitydomain.StatusActive, CreatedAt: created, UpdatedAt: updated},
		f.other: {ID: f.other, StorageUID: "USER02",
			Role: "user", Status: identitydomain.StatusBanned, CreatedAt: created, UpdatedAt: updated},
	}}
	engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{
		AvatarProfile: runtimehttp.NewV1AvatarProfileDirectory(repo),
	})
	status, _, payload := doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("GET /user = %d %v, want 200", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["storage_namespace"] != "USER01" {
		t.Fatalf("storage_namespace = %v, want USER01", data["storage_namespace"])
	}
	if data["created_at"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("created_at = %v, want RFC3339", data["created_at"])
	}
	if data["updated_at"] != "2026-02-03T04:05:06Z" {
		t.Fatalf("updated_at = %v, want RFC3339", data["updated_at"])
	}
	if data["status"] != float64(identitydomain.StatusActive) {
		t.Fatalf("status = %v, want %d", data["status"], identitydomain.StatusActive)
	}
	if data["avatar"] != "https://cdn.example/a.png" {
		t.Fatalf("avatar = %v, want the persisted URL", data["avatar"])
	}
	// Banned rows keep their numeric status so the client can render
	// the deactivated badge (0 is never dropped by omitempty).
	status, _, payload = doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.other})
	if status != http.StatusOK {
		t.Fatalf("GET /user (banned row) = %d %v, want 200", status, payload)
	}
	banned := v1Data(t, payload).(map[string]any)
	if banned["status"] != float64(identitydomain.StatusBanned) {
		t.Fatalf("status = %v, want %d", banned["status"], identitydomain.StatusBanned)
	}
	if banned["storage_namespace"] != "USER02" {
		t.Fatalf("storage_namespace = %v, want USER02", banned["storage_namespace"])
	}
}

func TestV1UserResponseMatchesPublicOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "..", "openapi.public.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema := doc.Paths.Value("/user").Get.Responses.Status(http.StatusOK).Value.Content["application/json"].Schema.Value
	f := newV1MediaGapsFixture(t, nil)
	created := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	for _, tt := range []struct {
		name    string
		profile *identitydomain.User
	}{
		{"active profile", &identitydomain.User{ID: f.owner, StorageUID: "USER01", Avatar: "https://cdn.example/a.png",
			Status: identitydomain.StatusActive, CreatedAt: created, UpdatedAt: created}},
		{"banned profile", &identitydomain.User{ID: f.owner, StorageUID: "USER01",
			Status: identitydomain.StatusBanned, CreatedAt: created, UpdatedAt: created}},
		{"profile reader absent", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := ContractV1Deps{}
			if tt.profile != nil {
				repo := &avatarProfileRepository{users: map[int64]*identitydomain.User{f.owner: tt.profile}}
				deps.AvatarProfile = runtimehttp.NewV1AvatarProfileDirectory(repo)
			}
			engine := newV1MediaTestEngine(f.store, f.dir, deps)
			status, _, payload := doV1(t, engine, v1Call{method: "GET", path: "/api/v1/user", userID: f.owner})
			if status != http.StatusOK {
				t.Fatalf("GET /user = %d, want 200", status)
			}
			if err := schema.VisitJSON(payload); err != nil {
				t.Fatalf("actual /user response rejected by Public OpenAPI: %v", err)
			}
			// Profile facts may be absent, while numeric status must always
			// be present, including the banned zero value.
			data := v1Data(t, payload).(map[string]any)
			for _, field := range []string{"storage_namespace", "created_at", "updated_at"} {
				delete(data, field)
			}
			if err := schema.VisitJSON(payload); err != nil {
				t.Fatalf("optional profile facts required by Public OpenAPI: %v", err)
			}
			delete(data, "status")
			if err := schema.VisitJSON(payload); err == nil {
				t.Fatal("Public OpenAPI accepted a /user response without status")
			}
		})
	}
}
