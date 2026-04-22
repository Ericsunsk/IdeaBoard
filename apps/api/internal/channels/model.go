package channels

import "time"

// Channel 渠道商
type Channel struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Type      string    `gorm:"type:varchar(50);not null;default:'banana'" json:"type"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	ApiURL    string    `gorm:"type:varchar(500);not null" json:"api_url"`
	Models    string    `gorm:"type:varchar(500);default:''" json:"models"`
	Status    int8      `gorm:"type:tinyint;default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateChannelRequest struct {
	Type   string `json:"type" binding:"required"`
	Name   string `json:"name" binding:"required"`
	ApiURL string `json:"api_url" binding:"required"`
	Models string `json:"models"`
}

type UpdateChannelRequest struct {
	Type   string `json:"type" binding:"required"`
	Name   string `json:"name" binding:"required"`
	ApiURL string `json:"api_url" binding:"required"`
	Models string `json:"models"`
}
