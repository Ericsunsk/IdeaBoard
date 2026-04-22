package accounts

import "time"

type ApiAccount struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"type:varchar(100);not null" json:"name"`
	ChannelID     uint64    `gorm:"not null;default:0" json:"channel_id"`
	Platform      string    `gorm:"type:varchar(50);not null" json:"platform"`
	ApiType       string    `gorm:"type:varchar(50);not null" json:"api_type"`
	ModelSet      string    `gorm:"type:varchar(255)" json:"model_set"`
	ApiKey        string    `gorm:"type:text;not null" json:"-"`
	ProxyURL      string    `gorm:"type:text" json:"proxy_url"`
	MaxConcurrent int       `gorm:"type:int;default:3" json:"max_concurrent"`
	Timeout       int       `gorm:"type:int;default:120" json:"timeout"`
	ExtraConfig   string    `gorm:"type:text" json:"extra_config"`
	Status        int8      `gorm:"type:tinyint;default:1" json:"status"`
	FailCount     int       `gorm:"type:int;default:0" json:"fail_count"`
	Weight        int       `gorm:"type:int;default:1" json:"weight"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// 请求结构
type CreateAccountRequest struct {
	Name          string `json:"name"`
	ChannelID     uint64 `json:"channel_id" binding:"required"`
	Platform      string `json:"platform"`
	ApiType       string `json:"api_type"`
	ModelSet      string `json:"model_set"`
	ApiKey        string `json:"api_key" binding:"required"`
	ProxyURL      string `json:"proxy_url"`
	MaxConcurrent int    `json:"max_concurrent"`
	Timeout       int    `json:"timeout"`
	ExtraConfig   string `json:"extra_config"`
	Weight        int    `json:"weight"`
}

type UpdateAccountRequest struct {
	Name          string `json:"name"`
	ChannelID     uint64 `json:"channel_id"`
	Platform      string `json:"platform"`
	ApiType       string `json:"api_type"`
	ModelSet      string `json:"model_set"`
	ApiKey        string `json:"api_key"`
	ProxyURL      string `json:"proxy_url"`
	MaxConcurrent int    `json:"max_concurrent"`
	Timeout       int    `json:"timeout"`
	ExtraConfig   string `json:"extra_config"`
	Weight        int    `json:"weight"`
	Status        int8   `json:"status"`
}

// 列表响应
type AccountListRequest struct {
	Keyword  string `form:"keyword"`
	Platform string `form:"platform"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
}

type AccountWithLoad struct {
	ApiAccount
	CurrentConcurrent int    `json:"current_concurrent"`
	WaitQueueCount    int    `json:"wait_queue_count"`
	RunningStatus     string `json:"running_status"`
}
