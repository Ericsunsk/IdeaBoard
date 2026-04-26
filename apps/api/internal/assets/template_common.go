package assets

import "gorm.io/gorm"

const (
	orderBySortAndID = "sort_order ASC, id ASC"
)

func findByID[T any](db *gorm.DB, id uint64, out *T) error {
	return db.Where("id = ?", id).First(out).Error
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
