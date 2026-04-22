package assets

import "time"

type StyleCard struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	CoverImage  string    `gorm:"type:text" json:"cover_image"`
	Keywords    string    `gorm:"type:text" json:"-"` // 内部关键词，对用户隐藏
	SortOrder   int       `gorm:"type:int;default:0" json:"sort_order"`
	IsEnabled   bool      `gorm:"type:tinyint;default:1" json:"is_enabled"` // 上下架
	IsExclusive bool      `gorm:"type:tinyint;default:0" json:"is_exclusive"` // 是否互斥（单选）
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// ============ 请求结构 ============

type CreateStyleCardRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	CoverImage  string `json:"cover_image"`                        // 封面图 URL
	Keywords    string `json:"keywords" binding:"required"`        // 内部关键词，多个用逗号分隔
	SortOrder   int    `json:"sort_order"`                        // 排序
	IsEnabled   bool   `json:"is_enabled"`                        // 是否上架
	IsExclusive bool   `json:"is_exclusive"`                      // 是否互斥
}

type UpdateStyleCardRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	CoverImage  string `json:"cover_image"`
	Keywords    string `json:"keywords" binding:"required"`
	SortOrder   int    `json:"sort_order"`
	IsEnabled   bool   `json:"is_enabled"`
	IsExclusive bool   `json:"is_exclusive"`
}

type StyleCardListRequest struct {
	Page     int  `form:"page,default=1"`
	PageSize int  `form:"page_size,default=20"`
}
