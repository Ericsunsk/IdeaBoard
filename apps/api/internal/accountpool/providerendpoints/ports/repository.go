package ports

import (
	"context"

	"ideaboard/api/internal/accountpool/providerendpoints/domain"
)

type Repository interface {
	Create(ctx context.Context, endpoint *domain.Endpoint) error
	GetByID(ctx context.Context, id int64) (*domain.Endpoint, error)
	List(ctx context.Context) ([]*domain.Endpoint, error)
	Update(ctx context.Context, endpoint *domain.Endpoint) error
	UpdateStatus(ctx context.Context, id int64, status int16) (*domain.Endpoint, error)
	Delete(ctx context.Context, id int64) error
}
