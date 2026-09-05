package domain

import (
	"errors"
	"time"
)

var (
	ErrNameConflict         = errors.New("provider endpoint name conflict")
	ErrURLConflict          = errors.New("provider endpoint api type/base url conflict")
	ErrNotFound             = errors.New("provider endpoint not found")
	ErrInvalidStatus        = errors.New("status 只支持 0 或 1")
	ErrAPIKeyRequired       = errors.New("图片处理 API 必须配置 API Key")
	ErrUnsupportedProtocol  = errors.New("不支持的 ProviderEndpoint Protocol")
	ErrImageAPIRequiresFunc = errors.New("图片处理 API 必须选择一个处理功能")
	ErrInvalidBaseURL       = errors.New("base_url 不可用，仅支持可达的公网 http(s) 地址")
	ErrUnsupportedAPIType   = errors.New("接口类型不支持")
)

type Endpoint struct {
	ID               int64     `json:"id"`
	APIType          string    `json:"api_type"`
	Protocol         string    `json:"protocol"`
	Capabilities     []string  `json:"capabilities"`
	Name             string    `json:"name"`
	BaseURL          string    `json:"base_url"`
	APIKey           string    `json:"-"`
	APIKeyConfigured bool      `json:"api_key_configured"`
	Status           int16     `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
