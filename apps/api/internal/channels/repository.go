package channels

import (
	"context"

	"gorm.io/gorm"
)

type ChannelRepository struct {
	db *gorm.DB
}

func NewChannelRepository(db *gorm.DB) *ChannelRepository {
	return &ChannelRepository{db: db}
}

func (r *ChannelRepository) Create(ctx context.Context, channel *Channel) error {
	return r.db.WithContext(ctx).Create(channel).Error
}

func (r *ChannelRepository) GetByID(ctx context.Context, id uint64) (*Channel, error) {
	var channel Channel
	if err := r.db.WithContext(ctx).First(&channel, id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *ChannelRepository) List(ctx context.Context) ([]*Channel, error) {
	var channels []*Channel
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *ChannelRepository) Update(ctx context.Context, channel *Channel) error {
	return r.db.WithContext(ctx).Save(channel).Error
}

func (r *ChannelRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Channel{}, id).Error
}
