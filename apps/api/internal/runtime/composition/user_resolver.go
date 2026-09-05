package composition

import (
	"context"
	credithttp "ideaboard/api/internal/credits/interfaces/http"

	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/rewards"

	"github.com/google/uuid"
)

// creditsUserResolver adapts the identity repository without making the
// credits package depend on the users lifecycle package.
type creditsUserResolver struct {
	repo ports.UserRepository
}

func (r creditsUserResolver) ResolvePublicID(ctx context.Context, publicID string) (credithttp.UserIdentity, error) {
	user, err := r.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return credithttp.UserIdentity{}, err
	}
	return credithttp.UserIdentity{ID: user.ID, PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

func (r creditsUserResolver) ResolveID(ctx context.Context, id int64) (credithttp.UserIdentity, error) {
	user, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return credithttp.UserIdentity{}, err
	}
	return credithttp.UserIdentity{ID: user.ID, PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

type taskUserResolver struct {
	repo ports.UserRepository
}

type rewardIdentityProjector struct {
	repo ports.UserRepository
}

func (r rewardIdentityProjector) ResolveIdentity(
	ctx context.Context,
	userID int64,
) (rewards.IdentityProjection, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return rewards.IdentityProjection{}, err
	}
	return rewards.IdentityProjection{PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

func (r rewardIdentityProjector) ResolveIdentities(
	ctx context.Context,
	userIDs []int64,
) (map[int64]rewards.IdentityProjection, error) {
	usersByID, err := r.repo.GetByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	identities := make(map[int64]rewards.IdentityProjection, len(usersByID))
	for userID, user := range usersByID {
		identities[userID] = rewards.IdentityProjection{PublicID: user.PublicID, DisplayName: user.DisplayName}
	}
	return identities, nil
}

func (r taskUserResolver) ResolvePublicID(ctx context.Context, publicID string) (int64, error) {
	user, err := r.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

func (r taskUserResolver) ResolveUser(ctx context.Context, userID int64) (uuid.UUID, string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return uuid.Nil, "", err
	}
	return user.PublicID, user.DisplayName, nil
}
