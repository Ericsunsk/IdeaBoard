package repository

import "gorm.io/gorm"

const (
	orderBySortAndID      = "sort_order ASC, id ASC"
	orderByCategorySortID = "category ASC, sort_order ASC, id ASC"
)

func findByID[T any](db *gorm.DB, id uint64, out *T) error {
	return db.Where("id = ?", id).First(out).Error
}

func listWithPagination[T any](db *gorm.DB, page, pageSize int, out *[]*T) (int64, error) {
	var total int64
	var entity T
	if err := db.Model(&entity).Count(&total).Error; err != nil {
		return 0, err
	}

	offset := (page - 1) * pageSize
	if err := db.Offset(offset).Limit(pageSize).Order(orderBySortAndID).Find(out).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func listEnabledByCategory[T any](db *gorm.DB, out *[]*T) error {
	return db.Where("is_enabled = ?", true).Order(orderByCategorySortID).Find(out).Error
}

func listAllOrdered[T any](db *gorm.DB, out *[]*T) error {
	return db.Order(orderBySortAndID).Find(out).Error
}

func listEnabledOrdered[T any](db *gorm.DB, out *[]*T) error {
	return db.Where("is_enabled = ?", true).Order(orderBySortAndID).Find(out).Error
}

func deleteByID[T any](db *gorm.DB, id uint64) error {
	var entity T
	return db.Delete(&entity, id).Error
}
