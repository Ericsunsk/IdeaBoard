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
	err := r.db.Where("id = ?", id).First(&mode).Error
	if err != nil {
		return nil, err
	}
	return &mode, nil
}

func (r *LineDraftModeRepository) List() ([]*model.LineDraftMode, error) {
	var modes []*model.LineDraftMode
	err := r.db.Order("sort_order ASC, id ASC").Find(&modes).Error
	return modes, err
}

func (r *LineDraftModeRepository) ListEnabled() ([]*model.LineDraftMode, error) {
	var modes []*model.LineDraftMode
	err := r.db.Where("is_enabled = ?", true).Order("sort_order ASC, id ASC").Find(&modes).Error
	return modes, err
}

func (r *LineDraftModeRepository) Update(mode *model.LineDraftMode) error {
	return r.db.Save(mode).Error
}

func (r *LineDraftModeRepository) Delete(id uint64) error {
	return r.db.Delete(&model.LineDraftMode{}, id).Error
}
