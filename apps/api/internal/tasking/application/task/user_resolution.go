package task

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/platform/validation"

	"github.com/google/uuid"
)

type AdminTaskUserResolver interface {
	ResolveUser(context.Context, int64) (uuid.UUID, string, error)
}

func (s *Service) ResolveAdminTaskUserID(ctx context.Context, rawPublicID string) (int64, error) {
	if s == nil || s.userResolver == nil {
		return 0, validation.New("user_public_id 无效")
	}
	publicID, err := uuid.Parse(strings.TrimSpace(rawPublicID))
	if err != nil || publicID == uuid.Nil {
		return 0, validation.New("user_public_id 无效")
	}
	// The resolver is intentionally keyed by the internal id for read models;
	// public-id parsing and lookup stay at the HTTP boundary.
	return s.resolveUserIDByPublicID(ctx, publicID)
}

type adminTaskPublicIDResolver interface {
	ResolvePublicID(context.Context, string) (int64, error)
}

func (s *Service) resolveUserIDByPublicID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	resolver, ok := s.userResolver.(adminTaskPublicIDResolver)
	if !ok {
		return 0, validation.New("user_public_id 无效")
	}
	return resolver.ResolvePublicID(ctx, publicID.String())
}

func (s *Service) resolveAdminTaskUser(ctx context.Context, userID int64) (uuid.UUID, string, error) {
	if s == nil || s.userResolver == nil || userID <= 0 {
		return uuid.Nil, "", nil
	}
	publicID, displayName, err := s.userResolver.ResolveUser(ctx, userID)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("resolve task user identity: %w", err)
	}
	return publicID, displayName, nil
}

func (s *Service) enrichAdminTaskListItem(ctx context.Context, item *AdminMonitorListItem) error {
	if item == nil {
		return nil
	}
	publicID, displayName, err := s.resolveAdminTaskUser(ctx, item.internalUserID)
	if err != nil {
		return err
	}
	item.UserPublicID = publicID
	item.UserDisplayName = displayName
	return nil
}

func (s *Service) enrichAdminTaskDetail(ctx context.Context, detail *AdminMonitorDetail) error {
	if detail == nil {
		return nil
	}
	publicID, displayName, err := s.resolveAdminTaskUser(ctx, detail.internalUserID)
	if err != nil {
		return err
	}
	detail.UserPublicID = publicID
	detail.UserDisplayName = displayName
	return nil
}
