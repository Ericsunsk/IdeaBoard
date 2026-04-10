package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type PoseTemplateRepository struct {
	db *gorm.DB
}

func NewPoseTemplateRepository(db *gorm.DB) *PoseTemplateRepository {
	return &PoseTemplateRepository{db: db}
}

func (r *PoseTemplateRepository) Create(tpl *model.PoseTemplate) error {
	return r.db.Create(tpl).Error
}

func (r *PoseTemplateRepository) GetByID(id uint64) (*model.PoseTemplate, error) {
	var tpl model.PoseTemplate
	if err := findByID(r.db, id, &tpl); err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *PoseTemplateRepository) List(page, pageSize int) ([]*model.PoseTemplate, int64, error) {
	var list []*model.PoseTemplate
	total, err := listWithPagination(r.db, page, pageSize, &list)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *PoseTemplateRepository) ListEnabled() ([]*model.PoseTemplate, error) {
	var list []*model.PoseTemplate
	err := listEnabledByCategory(r.db, &list)
	return list, err
}

func (r *PoseTemplateRepository) Update(tpl *model.PoseTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *PoseTemplateRepository) Delete(id uint64) error {
	return deleteByID[model.PoseTemplate](r.db, id)
}
