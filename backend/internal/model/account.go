package model

import (
	"encoding/json"
	"time"
)

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

type Task struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"task_id"`
	UserID      uint64     `gorm:"not null;index" json:"user_id"`
	ChannelID   uint64     `gorm:"not null" json:"channel_id"`
	AccountID   uint64     `gorm:"index" json:"account_id"`
	Prompt      string     `gorm:"type:text" json:"prompt"`      // 完整提示词（含关键词）
	UserPrompt  string     `gorm:"type:text" json:"user_prompt"` // 用户原始输入
	Params      string     `gorm:"type:text" json:"params"`
	Status      string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	RetryCount  int        `gorm:"type:int;default:0" json:"retry_count"`
	ErrorMsg    string     `gorm:"type:text" json:"error_msg"`
	Result      string     `gorm:"type:text" json:"result"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// ToUserView 返回用户安全的任务视图，隐藏内部关键词和敏感信息
func (t *Task) ToUserView() map[string]interface{} {
	// 清理 result JSON 中的敏感字段
	safeResult := t.Result
	if safeResult != "" {
		var resultMap map[string]interface{}
		if err := json.Unmarshal([]byte(safeResult), &resultMap); err == nil {
			delete(resultMap, "prompt")
			delete(resultMap, "account_id")
			delete(resultMap, "channel_id")
			delete(resultMap, "channel_name")
			delete(resultMap, "platform")
			delete(resultMap, "api_type")
			delete(resultMap, "model_set")
			if b, err := json.Marshal(resultMap); err == nil {
				safeResult = string(b)
			}
		}
	}

	return map[string]interface{}{
		"id":           t.ID,
		"task_id":      t.TaskID,
		"user_id":      t.UserID,
		"prompt":       t.UserPrompt,
		"status":       t.Status,
		"error_msg":    t.ErrorMsg,
		"result":       safeResult,
		"created_at":   t.CreatedAt,
		"updated_at":   t.UpdatedAt,
		"completed_at": t.CompletedAt,
	}
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

type SubmitTaskRequest struct {
	Prompt  string                 `json:"prompt" binding:"required"`
	CardIDs []uint64               `json:"card_ids"` // 用户选中的风格卡片 ID
	Params  map[string]interface{} `json:"params"`
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

type TaskListRequest struct {
	AccountID uint64 `form:"account_id"`
	Status    string `form:"status"`
	Page      int    `form:"page,default=1"`
	PageSize  int    `form:"page_size,default=20"`
}
