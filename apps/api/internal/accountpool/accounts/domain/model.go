package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrAccountNotFound      = errors.New("account not found")
	ErrAccountNameConflict  = errors.New("account name conflict")
	ErrAccountModelNotFound = errors.New("account model not found")
)

type ApiAccount struct {
	ID                 int64          `json:"id"`
	Name               string         `json:"name"`
	ProviderEndpointID int64          `json:"provider_endpoint_id"`
	ApiType            string         `json:"provider_endpoint_api_type"`
	Protocol           string         `json:"provider_endpoint_protocol"`
	Capabilities       []string       `json:"provider_endpoint_capabilities"`
	Models             []AccountModel `json:"models"`
	ApiKey             string         `json:"-"`
	SecretConfigured   bool           `json:"secret_configured"`
	ProxyURL           string         `json:"provider_endpoint_base_url"`
	MaxConcurrent      int            `json:"max_concurrent"`
	Timeout            int            `json:"timeout"`
	ExtraConfig        string         `json:"extra_config"`
	Status             int16          `json:"status"`
	FailCount          int            `json:"fail_count"`
	Weight             int            `json:"weight"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type RuntimeCredential struct {
	ID                 int64
	AccountID          int64
	APIKey             string
	SecretMaterialKind string
	KeyVersion         string
	Fingerprint        string
	Status             string
}

// RuntimeAccountAccess is the live gate for an already-resolved assignment.
// Provider routing facts intentionally do not cross this boundary: the worker
// must execute the immutable assignment snapshot, not current Admin config.
type RuntimeAccountAccess struct {
	ID      int64
	Name    string
	APIKey  string
	Timeout int
}

type AccountModel struct {
	ID            int64  `json:"id,omitempty"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	Weight        int    `json:"weight"`
	MaxConcurrent *int   `json:"max_concurrent,omitempty"`
	SortOrder     int    `json:"sort_order"`
	Supports2K    bool   `json:"supports_2k"`
	Supports4K    bool   `json:"supports_4k"`
}

type AccountModelInput struct {
	Name          string `json:"name"`
	Enabled       *bool  `json:"enabled,omitempty"`
	Weight        int    `json:"weight,omitempty"`
	MaxConcurrent *int   `json:"max_concurrent,omitempty"`
	Supports2K    *bool  `json:"supports_2k,omitempty"`
	Supports4K    *bool  `json:"supports_4k,omitempty"`
}

type NullableIntPatch struct {
	Set   bool
	Valid bool
	Value int
}

func (p *NullableIntPatch) UnmarshalJSON(data []byte) error {
	p.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		p.Valid = false
		p.Value = 0
		return nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	p.Valid = true
	p.Value = value
	return nil
}

type CreateAccountRequest struct {
	Name               string              `json:"name"`
	ProviderEndpointID int64               `json:"provider_endpoint_id" binding:"required"`
	Models             []AccountModelInput `json:"models"`
	Secret             string              `json:"secret" binding:"required"`
	MaxConcurrent      int                 `json:"max_concurrent"`
	Timeout            int                 `json:"timeout"`
	ExtraConfig        string              `json:"extra_config"`
	Weight             int                 `json:"weight"`
}

type UpdateAccountRequest struct {
	Name          *string              `json:"name,omitempty"`
	Models        *[]AccountModelInput `json:"models,omitempty"`
	Secret        *string              `json:"secret,omitempty"`
	MaxConcurrent *int                 `json:"max_concurrent,omitempty"`
	Timeout       *int                 `json:"timeout,omitempty"`
	ExtraConfig   *string              `json:"extra_config,omitempty"`
	Weight        *int                 `json:"weight,omitempty"`
}

type PatchAccountModelRequest struct {
	Name          string           `json:"name" binding:"required"`
	Enabled       *bool            `json:"enabled,omitempty"`
	Weight        *int             `json:"weight,omitempty"`
	MaxConcurrent NullableIntPatch `json:"max_concurrent,omitempty"`
	SortOrder     *int             `json:"sort_order,omitempty"`
	Supports2K    *bool            `json:"supports_2k,omitempty"`
	Supports4K    *bool            `json:"supports_4k,omitempty"`
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

type AccountListRequest struct {
	Keyword  string `form:"keyword"`
	ApiType  string `form:"api_type"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
}

type AccountWithLoad struct {
	ApiAccount
	CurrentConcurrent int    `json:"current_concurrent"`
	WaitQueueCount    int    `json:"wait_queue_count"`
	RunningStatus     string `json:"running_status"`
}
