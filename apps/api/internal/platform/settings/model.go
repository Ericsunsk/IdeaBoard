package settings

import "time"

type SystemConfig struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Key       string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
