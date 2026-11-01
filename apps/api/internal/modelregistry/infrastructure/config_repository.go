// Package infrastructure owns the modelregistry persistence adapters: the
// gateway configuration repository over platform system settings. Domain
// owns the DTOs and narrow seams; this package implements them.
//
// Vision mapping and model catalog stay isolated here and in
// catalog_repository.go. Connection keys are not part of this repository's
// contract; gateway credentials resolve from Worker env. Pre-existing
// ai_gateway_* rows are inert to this package.
package infrastructure

import (
	"ideaboard/api/internal/platform/settings"
)

// ConfigRepository persists gateway configuration in system settings.
type ConfigRepository struct {
	settings *settings.SystemConfigRepository
}

// NewConfigRepository builds the repository over platform settings.
func NewConfigRepository(settingsRepository *settings.SystemConfigRepository) *ConfigRepository {
	return &ConfigRepository{settings: settingsRepository}
}
