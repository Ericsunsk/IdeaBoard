package httpapi

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"

	"github.com/google/uuid"
)

type stubUserRepo struct {
	users map[int64]*domain.User
	err   error
}

func (s *stubUserRepo) Create(_ context.Context, _ *domain.User) error { return nil }
func (s *stubUserRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.users[id], nil
}
func (s *stubUserRepo) GetByIDs(_ context.Context, ids []int64) (map[int64]*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[int64]*domain.User, len(ids))
	for _, id := range ids {
		if user, ok := s.users[id]; ok {
			out[id] = user
		}
	}
	return out, nil
}
func (s *stubUserRepo) GetByPublicID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *stubUserRepo) GetByAuthUserID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *stubUserRepo) GetByStorageUID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *stubUserRepo) Update(_ context.Context, _ *domain.User) error { return nil }
func (s *stubUserRepo) UpdateLastLoginMetadata(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *stubUserRepo) List(_ context.Context, _, _ string, _ int16, _ bool, _ int, _ int) ([]*domain.User, identityports.UserListCounts, error) {
	return nil, identityports.UserListCounts{}, errors.New("not implemented")
}
func (s *stubUserRepo) UpdateStatus(_ context.Context, _ int64, _ int16) error      { return nil }
func (s *stubUserRepo) UpdateRoleByID(_ context.Context, _ int64, _ string) error   { return nil }
func (s *stubUserRepo) UpdateAvatarByID(_ context.Context, _ int64, _ string) error { return nil }
func (s *stubUserRepo) EnsureStorageUIDs(_ context.Context) error                   { return nil }

type stubEmailReader struct {
	identities map[string]authidentity.UserIdentity
	err        error
	calls      [][]string
}

func (s *stubEmailReader) GetUserEmails(_ context.Context, ids []string) (map[string]authidentity.UserIdentity, error) {
	s.calls = append(s.calls, append([]string(nil), ids...))
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]authidentity.UserIdentity, len(ids))
	for _, id := range ids {
		if identity, ok := s.identities[id]; ok {
			out[id] = identity
		}
	}
	return out, nil
}

func strPtr(value string) *string { return &value }

func TestMemberEmailServiceJoinsRepoToAuthEmails(t *testing.T) {
	repo := &stubUserRepo{users: map[int64]*domain.User{
		1: {ID: 1, PublicID: uuid.New(), AuthUserID: strPtr("auth-1")},
		2: {ID: 2, PublicID: uuid.New(), AuthUserID: strPtr("auth-2")},
		3: {ID: 3, PublicID: uuid.New()},
	}}
	reader := &stubEmailReader{identities: map[string]authidentity.UserIdentity{
		"auth-1": {Email: "  Alice@Example.com  ", Providers: []string{"email"}},
	}}
	svc := NewMemberEmailService(repo, reader)
	got, err := svc.GetMemberEmails(context.Background(), []int64{1, 2, 3, 1, -5, 0})
	if err != nil {
		t.Fatalf("GetMemberEmails = %v, want nil", err)
	}
	if got[1] != "Alice@Example.com" {
		t.Fatalf("user 1 email = %q, want trimmed Alice@Example.com", got[1])
	}
	if _, ok := got[2]; ok {
		t.Fatalf("user 2 without email identity must be omitted: %v", got)
	}
	if _, ok := got[3]; ok {
		t.Fatalf("user 3 without AuthUserID must be omitted: %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("emails = %v, want exactly 1 entry", got)
	}
	if len(reader.calls) != 1 || len(reader.calls[0]) != 2 {
		t.Fatalf("email reader calls = %v, want one deduped batch of 2 auth ids", reader.calls)
	}
}

func TestMemberEmailServiceFailClosed(t *testing.T) {
	// Nil sides resolve to empty without error.
	svc := NewMemberEmailService(nil, nil)
	got, err := svc.GetMemberEmails(context.Background(), []int64{1})
	if err != nil || len(got) != 0 {
		t.Fatalf("nil service = %v, %v; want empty, nil", got, err)
	}
	got, err = svc.GetMemberEmails(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty ids = %v, %v; want empty, nil", got, err)
	}
	// Repo and reader errors propagate to the caller (the HTTP layer then
	// fails closed to no email); they must never fabricate addresses.
	repoErr := &stubUserRepo{err: errors.New("db down")}
	svc = NewMemberEmailService(repoErr, &stubEmailReader{})
	if _, err := svc.GetMemberEmails(context.Background(), []int64{1}); err == nil {
		t.Fatal("repo error must propagate")
	}
	repo := &stubUserRepo{users: map[int64]*domain.User{
		1: {ID: 1, PublicID: uuid.New(), AuthUserID: strPtr("auth-1")},
	}}
	svc = NewMemberEmailService(repo, &stubEmailReader{err: errors.New("auth down")})
	if _, err := svc.GetMemberEmails(context.Background(), []int64{1}); err == nil {
		t.Fatal("email reader error must propagate")
	}
}
