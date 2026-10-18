package v1

import (
	"context"
	"strings"

	identityports "ideaboard/api/internal/identity/users/ports"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
)

// V1MemberDisplay is the workspace-visible display projection for one
// member: the Auth-owned email (invite search/display assistance) plus
// the avatar public URL (runtimehttp.V1PublicAvatarURLForRow projection). Either
// field is empty when unavailable and never fails the request.
type V1MemberDisplay struct {
	Email  string
	Avatar string
}

// V1MemberDisplayProvider resolves workspace-visible display facts for
// the member list/write shapes. Optional seam: nil fails closed to no
// facts (fields omitted), never fails the request. Production performs
// ONE users.GetByIDs for the rows, one Auth hop for emails, and the
// avatar projection off the same rows; unknown users or missing facts
// are omitted per member. Callers only pass member ids from the
// requested workspace, so facts never leak across workspaces.
type V1MemberDisplayProvider interface {
	GetMemberDisplay(ctx context.Context, userIDs []int64) (map[int64]V1MemberDisplay, error)
}

// memberDisplayService is the single member-display implementation:
// one users.GetByIDs batch resolves the rows, the Auth reader resolves
// the Auth-owned emails, and runtimehttp.V1PublicAvatarURLForRow projects the
// business-owned avatars off the same rows.
type memberDisplayService struct {
	repo   identityports.UserRepository
	emails identityports.UserEmailReader
}

// NewMemberDisplayService builds the production V1MemberDisplayProvider
// over the users repository plus the Auth email reader. A nil repo
// resolves to no facts; a nil reader resolves emails to none while
// avatars still project.
func NewMemberDisplayService(repo identityports.UserRepository, emails identityports.UserEmailReader) V1MemberDisplayProvider {
	return &memberDisplayService{repo: repo, emails: emails}
}

func (s *memberDisplayService) GetMemberDisplay(ctx context.Context, userIDs []int64) (map[int64]V1MemberDisplay, error) {
	out := make(map[int64]V1MemberDisplay)
	if s == nil || s.repo == nil {
		return out, nil
	}
	seen := make(map[int64]struct{}, len(userIDs))
	deduped := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return out, nil
	}
	usersByID, err := s.repo.GetByIDs(ctx, deduped)
	if err != nil {
		return nil, err
	}
	for numeric, user := range usersByID {
		if user == nil {
			continue
		}
		if url := runtimehttp.V1PublicAvatarURLForRow(user.StorageUID, user.Avatar); url != "" {
			entry := out[numeric]
			entry.Avatar = url
			out[numeric] = entry
		}
	}
	authIDs := make([]string, 0, len(usersByID))
	authSeen := make(map[string]struct{}, len(usersByID))
	authByNumeric := make(map[int64]string, len(usersByID))
	for numeric, user := range usersByID {
		if user == nil || user.AuthUserID == nil {
			continue
		}
		authID := strings.TrimSpace(*user.AuthUserID)
		if authID == "" {
			continue
		}
		authByNumeric[numeric] = authID
		if _, ok := authSeen[authID]; !ok {
			authSeen[authID] = struct{}{}
			authIDs = append(authIDs, authID)
		}
	}
	if len(authIDs) == 0 || s.emails == nil {
		return out, nil
	}
	identities, err := s.emails.GetUserEmails(ctx, authIDs)
	if err != nil {
		return nil, err
	}
	for numeric, authID := range authByNumeric {
		identity, ok := identities[authID]
		if !ok {
			continue
		}
		if email := strings.TrimSpace(identity.Email); email != "" {
			entry := out[numeric]
			entry.Email = email
			out[numeric] = entry
		}
	}
	return out, nil
}
