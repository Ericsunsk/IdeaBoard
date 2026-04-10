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
	if err := findByID(r.db, id, &tpl); err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *ModelTemplateRepository) List(page, pageSize int) ([]*model.ModelTemplate, int64, error) {
	var list []*model.ModelTemplate
	total, err := listWithPagination(r.db, page, pageSize, &list)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ModelTemplateRepository) ListEnabled() ([]*model.ModelTemplate, error) {
	var list []*model.ModelTemplate
	err := listEnabledByCategory(r.db, &list)
	return list, err
}

func (r *ModelTemplateRepository) Update(tpl *model.ModelTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *ModelTemplateRepository) Delete(id uint64) error {
	return deleteByID[model.ModelTemplate](r.db, id)
}
