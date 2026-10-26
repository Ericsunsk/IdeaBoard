package wiring

import (
	"ideaboard/api/internal/runtime/infrastructure/migration"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
)

func NewDatabaseMigrator() *migration.Runner {
	return migration.New(workerriver.MigrateRiver, runtimePrivilegePolicies()...)
}
