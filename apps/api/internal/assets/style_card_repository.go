package assets

import (
	
	"gorm.io/gorm"
)

type StyleCardRepository struct {
	db *gorm.DB
}

func NewStyleCardRepository(db *gorm.DB) *StyleCardRepository {
	return &StyleCardRepository{db: db}
}

func (r *StyleCardRepository) Create(card *StyleCard) error {
	return r.db.Create(card).Error
}

func (r *StyleCardRepository) GetByID(id uint64) (*StyleCard, error) {
	var card StyleCard
	err := r.db.Where("id = ?", id).First(&card).Error
	if err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *StyleCardRepository) List(page, pageSize int) ([]*StyleCard, int64, error) {
	var cards []*StyleCard
	var total int64

	if err := r.db.Model(&StyleCard{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := r.db.Offset(offset).Limit(pageSize).Order("sort_order ASC, id ASC").Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

func (r *StyleCardRepository) ListEnabled() ([]*StyleCard, error) {
	var cards []*StyleCard
	err := r.db.Where("is_enabled = ?", true).Order("sort_order ASC, id ASC").Find(&cards).Error
	return cards, err
}

func (r *StyleCardRepository) Update(card *StyleCard) error {
	return r.db.Save(card).Error
}

func (r *StyleCardRepository) Delete(id uint64) error {
	return r.db.Delete(&StyleCard{}, id).Error
}

func (r *StyleCardRepository) GetByIDs(ids []uint64) ([]*StyleCard, error) {
	var cards []*StyleCard
	err := r.db.Where("id IN ? AND is_enabled = ?", ids, true).Find(&cards).Error
	return cards, err
}
