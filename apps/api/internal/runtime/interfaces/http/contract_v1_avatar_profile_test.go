package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
)

type avatarProfileRepository struct {
	identityports.UserRepository
	users             map[int64]*identitydomain.User
	readErr, writeErr error
}

func (r *avatarProfileRepository) GetByID(_ context.Context, id int64) (*identitydomain.User, error) {
	return r.users[id], r.readErr
}

func (r *avatarProfileRepository) UpdateAvatarByID(_ context.Context, id int64, avatar string) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.users[id].Avatar = avatar
	return nil
}

func TestV1AvatarProfilePersistsAcrossHandlers(t *testing.T) {
	f := newV1MediaGapsFixture(t, false, nil)
	repo := &avatarProfileRepository{users: map[int64]*identitydomain.User{
		f.owner: {ID: f.owner, StorageUID: "AVTR01"},
		f.other: {ID: f.other, StorageUID: "AVTR02"},
	}}
	deps := ContractV1Deps{Media: f.media, MediaObjects: f.objects,
		AvatarUID: NewV1AvatarDirectory(repo), AvatarProfile: NewV1AvatarProfileDirectory(repo)}
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
		AvatarProfile: NewV1AvatarProfileDirectory(repo),
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
	f := newV1MediaGapsFixture(t, false, nil)
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
