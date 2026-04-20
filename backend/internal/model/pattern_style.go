package model

import "time"

// PatternStyle 图案工作室风格模型
type PatternStyle struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	ImageURL  string    `gorm:"type:text;not null" json:"image_url"`
	Category  string    `gorm:"type:varchar(40);not null;index" json:"category"` // pattern-gen / other active pattern categories
	SortOrder int       `gorm:"type:int;default:0" json:"sort_order"`
	IsEnabled bool      `gorm:"type:tinyint;default:1" json:"is_enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type CreatePatternStyleRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	ImageURL  string `json:"image_url" binding:"required"`
	Category  string `json:"category" binding:"required"`
	SortOrder int    `json:"sort_order"`
	IsEnabled bool   `json:"is_enabled"`
}

type UpdatePatternStyleRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	ImageURL  string `json:"image_url" binding:"required"`
	Category  string `json:"category" binding:"required"`
	SortOrder int    `json:"sort_order"`
	IsEnabled bool   `json:"is_enabled"`
}
