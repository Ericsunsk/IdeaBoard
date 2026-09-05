package domain

import "time"

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
