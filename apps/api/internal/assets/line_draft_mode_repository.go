package assets

import (
	
	"gorm.io/gorm"
)

type LineDraftModeRepository struct {
	db *gorm.DB
}

func NewLineDraftModeRepository(db *gorm.DB) *LineDraftModeRepository {
	return &LineDraftModeRepository{db: db}
}

func (r *LineDraftModeRepository) Create(mode *LineDraftMode) error {
	return r.db.Create(mode).Error
}

func (r *LineDraftModeRepository) GetByID(id uint64) (*LineDraftMode, error) {
	var mode LineDraftMode
	err := findByID(r.db, id, &mode)
	if err != nil {
		return nil, err
	}
	return &mode, nil
}

func (r *LineDraftModeRepository) List() ([]*LineDraftMode, error) {
	var modes []*LineDraftMode
	err := listAllOrdered(r.db, &modes)
	return modes, err
}

func (r *LineDraftModeRepository) ListEnabled() ([]*LineDraftMode, error) {
	var modes []*LineDraftMode
	err := listEnabledOrdered(r.db, &modes)
	return modes, err
}

func (r *LineDraftModeRepository) Update(mode *LineDraftMode) error {
	return r.db.Save(mode).Error
}

func (r *LineDraftModeRepository) Delete(id uint64) error {
	return deleteByID[LineDraftMode](r.db, id)
}
