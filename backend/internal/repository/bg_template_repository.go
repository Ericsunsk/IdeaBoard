package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type BgTemplateRepository struct {
	db *gorm.DB
}

func NewBgTemplateRepository(db *gorm.DB) *BgTemplateRepository {
	return &BgTemplateRepository{db: db}
}

func (r *BgTemplateRepository) Create(tpl *model.BgTemplate) error {
	return r.db.Create(tpl).Error
}

func (r *BgTemplateRepository) GetByID(id uint64) (*model.BgTemplate, error) {
	var tpl model.BgTemplate
	err := r.db.Where("id = ?", id).First(&tpl).Error
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *BgTemplateRepository) List(page, pageSize int) ([]*model.BgTemplate, int64, error) {
	var list []*model.BgTemplate
	var total int64

	if err := r.db.Model(&model.BgTemplate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := r.db.Offset(offset).Limit(pageSize).Order("sort_order ASC, id ASC").Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// ListEnabled 获取已上架的背景模板（用户端）
func (r *BgTemplateRepository) ListEnabled() ([]*model.BgTemplate, error) {
	var list []*model.BgTemplate
	err := r.db.Where("is_enabled = ?", true).Order("category ASC, sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *BgTemplateRepository) Update(tpl *model.BgTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *BgTemplateRepository) Delete(id uint64) error {
	return r.db.Delete(&model.BgTemplate{}, id).Error
}
