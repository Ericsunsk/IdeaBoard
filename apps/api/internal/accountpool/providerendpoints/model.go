package providerendpoints

import "time"

type ProviderEndpoint struct {
	ID        int64     `json:"id"`
	APIType   string    `json:"api_type"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	Status    int16     `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateProviderEndpointRequest struct {
	APIType string `json:"api_type" binding:"required"`
	Name    string `json:"name" binding:"required"`
	BaseURL string `json:"base_url" binding:"required"`
	Status  int16  `json:"status"`
}

type UpdateProviderEndpointRequest struct {
	APIType string `json:"api_type" binding:"required"`
	Name    string `json:"name" binding:"required"`
	BaseURL string `json:"base_url" binding:"required"`
	Status  int16  `json:"status"`
}

type DetectModelsRequest struct {
	BaseURL string `json:"base_url" binding:"required"`
	APIKey  string `json:"api_key" binding:"required"`
	APIType string `json:"api_type"`
}
