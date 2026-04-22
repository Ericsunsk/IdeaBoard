package assets

import (
	
	"gorm.io/gorm"
)

type PatternStyleRepository struct {
	db *gorm.DB
}

func NewPatternStyleRepository(db *gorm.DB) *PatternStyleRepository {
	return &PatternStyleRepository{db: db}
}

func (r *PatternStyleRepository) Create(ps *PatternStyle) error {
	return r.db.Create(ps).Error
}

func (r *PatternStyleRepository) GetByID(id uint64) (*PatternStyle, error) {
	var ps PatternStyle
	if err := findByID(r.db, id, &ps); err != nil {
		return nil, err
	}
	return &ps, nil
}

func (r *PatternStyleRepository) List(page, pageSize int) ([]*PatternStyle, int64, error) {
	var list []*PatternStyle
	total, err := listWithPagination(r.db, page, pageSize, &list)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListEnabled 获取所有已上架的风格模型
func (r *PatternStyleRepository) ListEnabled() ([]*PatternStyle, error) {
	var list []*PatternStyle
	err := listEnabledByCategory(r.db, &list)
	return list, err
}

// ListEnabledByCategory 按分类获取已上架的风格模型
func (r *PatternStyleRepository) ListEnabledByCategory(category string) ([]*PatternStyle, error) {
	var list []*PatternStyle
	err := r.db.Where("is_enabled = ? AND category = ?", true, category).Order(orderBySortAndID).Find(&list).Error
	return list, err
}

func (r *PatternStyleRepository) Update(ps *PatternStyle) error {
	return r.db.Save(ps).Error
}

func (r *PatternStyleRepository) Delete(id uint64) error {
	return deleteByID[PatternStyle](r.db, id)
}
