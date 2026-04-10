package model

import "time"

// BgTemplate 背景模板（管理员上传）
type BgTemplate struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	ImageURL  string    `gorm:"type:text;not null" json:"image_url"`
	Category  string    `gorm:"type:varchar(20);not null;index" json:"category"` // solid / scene / custom
	SortOrder int       `gorm:"type:int;default:0" json:"sort_order"`
	IsEnabled bool      `gorm:"type:tinyint;default:1" json:"is_enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// ============ 请求结构 ============

type CreateBgTemplateRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	ImageURL  string `json:"image_url" binding:"required"`
	Category  string `json:"category" binding:"required"`
	SortOrder int    `json:"sort_order"`
	IsEnabled bool   `json:"is_enabled"`
}

type UpdateBgTemplateRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	ImageURL  string `json:"image_url" binding:"required"`
	Category  string `json:"category" binding:"required"`
	SortOrder int    `json:"sort_order"`
	IsEnabled bool   `json:"is_enabled"`
}
