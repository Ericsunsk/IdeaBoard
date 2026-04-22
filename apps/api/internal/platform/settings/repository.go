package settings

import (
	"gorm.io/gorm"
)

type SystemConfigRepository struct {
	db *gorm.DB
}

func NewSystemConfigRepository(db *gorm.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db}
}

func (r *SystemConfigRepository) Get(key string) (string, error) {
	var cfg SystemConfig
	err := r.db.Where("`key` = ?", key).First(&cfg).Error
	if err != nil {
		return "", err
	}
	return cfg.Value, nil
}

func (r *SystemConfigRepository) GetByPrefix(prefix string) (map[string]string, error) {
	var cfgs []SystemConfig
	err := r.db.Where("`key` LIKE ?", prefix+"%").Find(&cfgs).Error
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

func (r *SystemConfigRepository) Set(key, value string) error {
	var cfg SystemConfig
	err := r.db.Where("`key` = ?", key).First(&cfg).Error
	if err != nil {
		// 不存在则创建
		cfg = SystemConfig{Key: key, Value: value}
		return r.db.Create(&cfg).Error
	}
	cfg.Value = value
	return r.db.Save(&cfg).Error
}
