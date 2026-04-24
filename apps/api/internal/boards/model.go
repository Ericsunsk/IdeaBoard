package boards

import "time"

type UserBoard struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BoardID   string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_user_board" json:"board_id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_user_board;index:idx_user_updated,priority:1" json:"user_id"`
	Name      string    `gorm:"type:varchar(60);not null" json:"name"`
	State     string    `gorm:"type:longtext;not null" json:"state"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;index:idx_user_updated,priority:2,sort:desc" json:"updated_at"`
}
