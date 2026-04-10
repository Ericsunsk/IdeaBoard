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
	if err := r.db.Where("id = ?", id).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *PoseTemplateRepository) List(page, pageSize int) ([]*model.PoseTemplate, int64, error) {
	var list []*model.PoseTemplate
	var total int64
	if err := r.db.Model(&model.PoseTemplate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := r.db.Offset(offset).Limit(pageSize).Order("sort_order ASC, id ASC").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *PoseTemplateRepository) ListEnabled() ([]*model.PoseTemplate, error) {
	var list []*model.PoseTemplate
	err := r.db.Where("is_enabled = ?", true).Order("category ASC, sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *PoseTemplateRepository) Update(tpl *model.PoseTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *PoseTemplateRepository) Delete(id uint64) error {
	return r.db.Delete(&model.PoseTemplate{}, id).Error
}
