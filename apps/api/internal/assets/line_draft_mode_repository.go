package assets

import (
	"context"

	"gorm.io/gorm"
)

type LineDraftModeRepository struct {
	db *gorm.DB
}

func NewLineDraftModeRepository(db *gorm.DB) *LineDraftModeRepository {
	return &LineDraftModeRepository{db: db}
}

func (r *LineDraftModeRepository) Create(ctx context.Context, mode *LineDraftMode) error {
	return r.db.WithContext(ctx).Create(mode).Error
}

func (r *LineDraftModeRepository) GetByID(ctx context.Context, id uint64) (*LineDraftMode, error) {
	var mode LineDraftMode
	err := findByID(r.db.WithContext(ctx), id, &mode)
	if err != nil {
		return nil, err
	}
	return &mode, nil
}

func (r *LineDraftModeRepository) List(ctx context.Context) ([]*LineDraftMode, error) {
	var modes []*LineDraftMode
	err := listAllOrdered(r.db.WithContext(ctx), &modes)
	return modes, err
}

func (r *LineDraftModeRepository) ListEnabled(ctx context.Context) ([]*LineDraftMode, error) {
	var modes []*LineDraftMode
	err := listEnabledOrdered(r.db.WithContext(ctx), &modes)
	return modes, err
}

func (r *LineDraftModeRepository) Update(ctx context.Context, mode *LineDraftMode) error {
	return r.db.WithContext(ctx).Save(mode).Error
}

func (r *LineDraftModeRepository) Delete(ctx context.Context, id uint64) error {
	return deleteByID[LineDraftMode](r.db.WithContext(ctx), id)
}
