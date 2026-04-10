package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type PatternStyleRepository struct {
	db *gorm.DB
}

func NewPatternStyleRepository(db *gorm.DB) *PatternStyleRepository {
	return &PatternStyleRepository{db: db}
}

func (r *PatternStyleRepository) Create(ps *model.PatternStyle) error {
	return r.db.Create(ps).Error
}

func (r *PatternStyleRepository) GetByID(id uint64) (*model.PatternStyle, error) {
	var ps model.PatternStyle
	if err := r.db.Where("id = ?", id).First(&ps).Error; err != nil {
		return nil, err
	}
	return &ps, nil
}

func (r *PatternStyleRepository) List(page, pageSize int) ([]*model.PatternStyle, int64, error) {
	var list []*model.PatternStyle
	var total int64
	if err := r.db.Model(&model.PatternStyle{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := r.db.Offset(offset).Limit(pageSize).Order("sort_order ASC, id ASC").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListEnabled 获取所有已上架的风格模型
func (r *PatternStyleRepository) ListEnabled() ([]*model.PatternStyle, error) {
	var list []*model.PatternStyle
	err := r.db.Where("is_enabled = ?", true).Order("category ASC, sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

// ListEnabledByCategory 按分类获取已上架的风格模型
func (r *PatternStyleRepository) ListEnabledByCategory(category string) ([]*model.PatternStyle, error) {
	var list []*model.PatternStyle
	err := r.db.Where("is_enabled = ? AND category = ?", true, category).Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *PatternStyleRepository) Update(ps *model.PatternStyle) error {
	return r.db.Save(ps).Error
}

func (r *PatternStyleRepository) Delete(id uint64) error {
	return r.db.Delete(&model.PatternStyle{}, id).Error
}
