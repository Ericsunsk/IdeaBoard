package v1

import (
	"context"
	"errors"

	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/google/uuid"
)

type invitationIdentityReader struct {
	display V1MemberDisplayProvider
	lookup  V1MemberLookupProvider
	users   runtimehttp.V1UserDirectory
}

// NewInvitationIdentityReader adapts the existing trusted identity readers to
// the Invitation module; identity failure policy belongs to that module.
func NewInvitationIdentityReader(display V1MemberDisplayProvider, lookup V1MemberLookupProvider, users runtimehttp.V1UserDirectory) invitations.IdentityReader {
	return invitationIdentityReader{display: display, lookup: lookup, users: users}
}

func (r invitationIdentityReader) Email(ctx context.Context, userID int64) (string, error) {
	if r.display == nil {
		return "", errors.New("invitation identity: display reader unavailable")
	}
	displays, err := r.display.GetMemberDisplay(ctx, []int64{userID})
	if err != nil {
		return "", err
	}
	return displays[userID].Email, nil
}

func (r invitationIdentityReader) FindUser(ctx context.Context, email string) (int64, error) {
	if r.lookup == nil || r.users == nil {
		return 0, errors.New("invitation identity: user lookup unavailable")
	}
	publicID, err := r.lookup.LookupUserPublicID(ctx, email)
	if err != nil || publicID == uuid.Nil {
		return 0, err
	}
	return r.users.ResolveUserID(ctx, publicID)
}
