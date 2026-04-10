package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type LineDraftModeRepository struct {
	db *gorm.DB
}

func NewLineDraftModeRepository(db *gorm.DB) *LineDraftModeRepository {
	return &LineDraftModeRepository{db: db}
}

func (r *LineDraftModeRepository) Create(mode *model.LineDraftMode) error {
	return r.db.Create(mode).Error
}

func (r *LineDraftModeRepository) GetByID(id uint64) (*model.LineDraftMode, error) {
	var mode model.LineDraftMode
	err := findByID(r.db, id, &mode)
	if err != nil {
		return nil, err
	}
	return &mode, nil
}

func (r *LineDraftModeRepository) List() ([]*model.LineDraftMode, error) {
	var modes []*model.LineDraftMode
	err := listAllOrdered(r.db, &modes)
	return modes, err
}

func (r *LineDraftModeRepository) ListEnabled() ([]*model.LineDraftMode, error) {
	var modes []*model.LineDraftMode
	err := listEnabledOrdered(r.db, &modes)
	return modes, err
}

func (r *LineDraftModeRepository) Update(mode *model.LineDraftMode) error {
	return r.db.Save(mode).Error
}

func (r *LineDraftModeRepository) Delete(id uint64) error {
	return deleteByID[model.LineDraftMode](r.db, id)
}
