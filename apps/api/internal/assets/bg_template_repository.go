package assets

import (
	
	"gorm.io/gorm"
)

type BgTemplateRepository struct {
	db *gorm.DB
}

func NewBgTemplateRepository(db *gorm.DB) *BgTemplateRepository {
	return &BgTemplateRepository{db: db}
}

func (r *BgTemplateRepository) Create(tpl *BgTemplate) error {
	return r.db.Create(tpl).Error
}

func (r *BgTemplateRepository) GetByID(id uint64) (*BgTemplate, error) {
	var tpl BgTemplate
	err := findByID(r.db, id, &tpl)
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *BgTemplateRepository) List(page, pageSize int) ([]*BgTemplate, int64, error) {
	var list []*BgTemplate
	total, err := listWithPagination(r.db, page, pageSize, &list)
	if err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// ListEnabled 获取已上架的背景模板（用户端）
func (r *BgTemplateRepository) ListEnabled() ([]*BgTemplate, error) {
	var list []*BgTemplate
	err := listEnabledByCategory(r.db, &list)
	return list, err
}

func (r *BgTemplateRepository) Update(tpl *BgTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *BgTemplateRepository) Delete(id uint64) error {
	return deleteByID[BgTemplate](r.db, id)
}
