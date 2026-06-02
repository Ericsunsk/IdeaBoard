package settings

import "time"

type SystemConfig struct {
	ID        int64     `json:"id"`
	ConfigKey string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
