package channels

import (
		"gorm.io/gorm"
)

type ChannelRepository struct {
	db *gorm.DB
}

func NewChannelRepository(db *gorm.DB) *ChannelRepository {
	return &ChannelRepository{db: db}
}

func (r *ChannelRepository) Create(channel *Channel) error {
	return r.db.Create(channel).Error
}

func (r *ChannelRepository) GetByID(id uint64) (*Channel, error) {
	var channel Channel
	if err := r.db.First(&channel, id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *ChannelRepository) List() ([]*Channel, error) {
	var channels []*Channel
	if err := r.db.Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *ChannelRepository) Update(channel *Channel) error {
	return r.db.Save(channel).Error
}

func (r *ChannelRepository) Delete(id uint64) error {
	return r.db.Delete(&Channel{}, id).Error
}
