package bootstrap

import (
	"context"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/users"

	"github.com/google/uuid"
)

// creditsUserResolver adapts the identity repository without making the
// credits package depend on the users lifecycle package.
type creditsUserResolver struct {
	repo *users.UserRepository
}

func (r creditsUserResolver) ResolvePublicID(ctx context.Context, publicID string) (credits.UserIdentity, error) {
	user, err := r.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return credits.UserIdentity{}, err
	}
	return credits.UserIdentity{ID: user.ID, PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

func (r creditsUserResolver) ResolveID(ctx context.Context, id int64) (credits.UserIdentity, error) {
	user, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return credits.UserIdentity{}, err
	}
	return credits.UserIdentity{ID: user.ID, PublicID: user.PublicID, DisplayName: user.DisplayName}, nil
}

type taskUserResolver struct {
	repo *users.UserRepository
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
