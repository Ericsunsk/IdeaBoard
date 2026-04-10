package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type FaceTemplateRepository struct {
	db *gorm.DB
}

func NewFaceTemplateRepository(db *gorm.DB) *FaceTemplateRepository {
	return &FaceTemplateRepository{db: db}
}

func (r *FaceTemplateRepository) Create(tpl *model.FaceTemplate) error {
	return r.db.Create(tpl).Error
}

func (r *FaceTemplateRepository) GetByID(id uint64) (*model.FaceTemplate, error) {
	var tpl model.FaceTemplate
	err := findByID(r.db, id, &tpl)
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *FaceTemplateRepository) List(page, pageSize int) ([]*model.FaceTemplate, int64, error) {
	var list []*model.FaceTemplate
	total, err := listWithPagination(r.db, page, pageSize, &list)
	if err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// ListEnabled 获取已上架的人脸模板（用户端）
func (r *FaceTemplateRepository) ListEnabled() ([]*model.FaceTemplate, error) {
	var list []*model.FaceTemplate
	err := listEnabledByCategory(r.db, &list)
	return list, err
}

func (r *FaceTemplateRepository) Update(tpl *model.FaceTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *FaceTemplateRepository) Delete(id uint64) error {
	return deleteByID[model.FaceTemplate](r.db, id)
}
