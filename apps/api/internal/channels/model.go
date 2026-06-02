package channels

import "time"

// Channel 渠道商
type Channel struct {
	ID        uint64    `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	ApiURL    string    `json:"api_url"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateChannelRequest struct {
	Type   string `json:"type" binding:"required"`
	Name   string `json:"name" binding:"required"`
	ApiURL string `json:"api_url" binding:"required"`
}

type UpdateChannelRequest struct {
	Type   string `json:"type" binding:"required"`
	Name   string `json:"name" binding:"required"`
	ApiURL string `json:"api_url" binding:"required"`
}

type DetectModelsRequest struct {
	BaseURL string `json:"base_url" binding:"required"`
	APIKey  string `json:"api_key" binding:"required"`
	APIType string `json:"api_type"`
}
