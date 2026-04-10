package model

import "time"

type LineDraftMode struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name       string    `gorm:"type:varchar(100);not null" json:"name"`
	CoverImage string    `gorm:"type:text" json:"cover_image"`
	Prompt     string    `gorm:"type:text" json:"prompt"`    // 发送给 API 的提示词模板
	SortOrder  int       `gorm:"type:int;default:0" json:"sort_order"`
	IsEnabled  bool      `gorm:"type:tinyint;default:1" json:"is_enabled"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type CreateLineDraftModeRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=100"`
	CoverImage string `json:"cover_image"`
	Prompt     string `json:"prompt" binding:"required"`
	SortOrder  int    `json:"sort_order"`
	IsEnabled  bool   `json:"is_enabled"`
}

type UpdateLineDraftModeRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=100"`
	CoverImage string `json:"cover_image"`
	Prompt     string `json:"prompt" binding:"required"`
	SortOrder  int    `json:"sort_order"`
	IsEnabled  bool   `json:"is_enabled"`
}
