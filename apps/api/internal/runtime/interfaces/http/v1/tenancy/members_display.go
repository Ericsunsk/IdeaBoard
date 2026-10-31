package tenancy

import (
	"context"
	"strings"

	"ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
)

// V1MemberDisplay is the workspace-visible display projection for one member.
type V1MemberDisplay = shared.MemberDisplay

// V1MemberDisplayProvider resolves workspace-visible display facts for
// the member list/write shapes.
type V1MemberDisplayProvider = shared.MemberDisplayProvider

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
	return memberDisplayForUsers(ctx, usersByID, s.emails)
}

// memberDisplayForUsers shares Auth email and public-avatar projection over
// already-read users, so invitation enrichment needs no second local read.
func memberDisplayForUsers(ctx context.Context, usersByID map[int64]*domain.User, emails identityports.UserEmailReader) (map[int64]V1MemberDisplay, error) {
	out := make(map[int64]V1MemberDisplay, len(usersByID))
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
	if len(authIDs) == 0 || emails == nil {
		return out, nil
	}
	identities, err := emails.GetUserEmails(ctx, authIDs)
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
