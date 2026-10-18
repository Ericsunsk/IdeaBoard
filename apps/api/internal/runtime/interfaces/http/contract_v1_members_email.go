package httpapi

import (
	"context"
	"strings"

	identityports "ideaboard/api/internal/identity/users/ports"
)

// memberEmailService joins numeric workspace-member ids to Auth-owned emails.
//
// It reuses the Admin enrichUserList join pattern: users.GetByIDs resolves
// the AuthUserID per numeric id, then UserEmailReader.GetUserEmails resolves
// the Auth-owned email identities. Unknown users, missing AuthUserIDs, and
// missing email identities are omitted per member (never fabricated).
// A nil repo or reader fails closed to an empty map with no error so the
// member list never breaks on email unavailability; callers additionally
// ignore provider errors fail-closed.
type memberEmailService struct {
	repo   identityports.UserRepository
	emails identityports.UserEmailReader
}

// NewMemberEmailService builds the production V1MemberEmailProvider over the
// users repository plus the Auth email reader. Either side may be nil in
// tests; lookups then resolve to no email.
func NewMemberEmailService(repo identityports.UserRepository, emails identityports.UserEmailReader) V1MemberEmailProvider {
	return &memberEmailService{repo: repo, emails: emails}
}

func (s *memberEmailService) GetMemberEmails(ctx context.Context, userIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string)
	if s == nil || s.repo == nil || s.emails == nil {
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
	if len(authIDs) == 0 {
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
		email := strings.TrimSpace(identity.Email)
		if email == "" {
			continue
		}
		out[numeric] = email
	}
	return out, nil
}
