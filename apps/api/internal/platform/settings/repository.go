package settings

import (
	"context"

	"gorm.io/gorm"
)

type SystemConfigRepository struct {
	db *gorm.DB
}

func NewSystemConfigRepository(db *gorm.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db}
}

func (r *SystemConfigRepository) Get(ctx context.Context, key string) (string, error) {
	var cfg SystemConfig
	err := r.db.WithContext(ctx).Where("`key` = ?", key).First(&cfg).Error
	if err != nil {
		return "", err
	}
	return cfg.Value, nil
}

func (r *SystemConfigRepository) GetByPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	var cfgs []SystemConfig
	err := r.db.WithContext(ctx).Where("`key` LIKE ?", prefix+"%").Find(&cfgs).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(cfgs))
	for _, c := range cfgs {
		// 去掉前缀返回
		key := c.Key[len(prefix):]
		result[key] = c.Value
	}
	return result, nil
}

func (r *SystemConfigRepository) Set(ctx context.Context, key, value string) error {
	db := r.db.WithContext(ctx)
	var cfg SystemConfig
	err := db.Where("`key` = ?", key).First(&cfg).Error
	if err != nil {
		// 不存在则创建
		cfg = SystemConfig{Key: key, Value: value}
		return db.Create(&cfg).Error
	}
	cfg.Value = value
	return db.Save(&cfg).Error
}
