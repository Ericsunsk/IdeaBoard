package assets

import "time"

type LineDraftMode struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	CoverImage string    `json:"cover_image"`
	Prompt     string    `json:"prompt"` // 发送给 API 的提示词模板
	SortOrder  int       `json:"sort_order"`
	IsEnabled  bool      `json:"is_enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
