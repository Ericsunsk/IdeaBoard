package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type ModelTemplateRepository struct {
	db *gorm.DB
}

func NewModelTemplateRepository(db *gorm.DB) *ModelTemplateRepository {
	return &ModelTemplateRepository{db: db}
}

func (r *ModelTemplateRepository) Create(tpl *model.ModelTemplate) error {
	return r.db.Create(tpl).Error
}

func (r *ModelTemplateRepository) GetByID(id uint64) (*model.ModelTemplate, error) {
	var tpl model.ModelTemplate
	if err := r.db.Where("id = ?", id).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *ModelTemplateRepository) List(page, pageSize int) ([]*model.ModelTemplate, int64, error) {
	var list []*model.ModelTemplate
	var total int64
	if err := r.db.Model(&model.ModelTemplate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := r.db.Offset(offset).Limit(pageSize).Order("sort_order ASC, id ASC").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ModelTemplateRepository) ListEnabled() ([]*model.ModelTemplate, error) {
	var list []*model.ModelTemplate
	err := r.db.Where("is_enabled = ?", true).Order("category ASC, sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *ModelTemplateRepository) Update(tpl *model.ModelTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *ModelTemplateRepository) Delete(id uint64) error {
	return r.db.Delete(&model.ModelTemplate{}, id).Error
}
