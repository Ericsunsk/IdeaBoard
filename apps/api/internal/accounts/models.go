package accounts

import "time"

type ApiAccount struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	ChannelID     int64     `json:"channel_id"`
	Platform      string    `json:"platform"`
	ApiType       string    `json:"api_type"`
	ModelSet      string    `json:"model_set"`
	ApiKey        string    `json:"-"`
	ProxyURL      string    `json:"proxy_url"`
	MaxConcurrent int       `json:"max_concurrent"`
	Timeout       int       `json:"timeout"`
	ExtraConfig   string    `json:"extra_config"`
	Status        int16     `json:"status"`
	FailCount     int       `json:"fail_count"`
	Weight        int       `json:"weight"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// 请求结构
type CreateAccountRequest struct {
	Name          string `json:"name"`
	ChannelID     int64  `json:"channel_id" binding:"required"`
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
	ChannelID     int64  `json:"channel_id"`
	Platform      string `json:"platform"`
	ApiType       string `json:"api_type"`
	ModelSet      string `json:"model_set"`
	ApiKey        string `json:"api_key"`
	ProxyURL      string `json:"proxy_url"`
	MaxConcurrent int    `json:"max_concurrent"`
	Timeout       int    `json:"timeout"`
	ExtraConfig   string `json:"extra_config"`
	Weight        int    `json:"weight"`
	Status        int16  `json:"status"`
}

type AccountLimitRange struct {
	Min     int `json:"min"`
	Max     int `json:"max"`
	Default int `json:"default"`
}

type AccountLimits struct {
	MaxConcurrent  AccountLimitRange `json:"max_concurrent"`
	TimeoutSeconds AccountLimitRange `json:"timeout_seconds"`
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
