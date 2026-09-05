package composition

import (
	"ideaboard/api/internal/platform/config"
	configinfra "ideaboard/api/internal/runtime/infrastructure/config"
)

func configureRuntimeLogging(cfg *config.Config) {
	configinfra.ConfigureRuntimeLogging(cfg)
}

func requireAuthInternalKeyForProduction(cfg *config.Config) {
	if err := configinfra.ValidateAuthInternalKey(cfg); err != nil {
		fatal(err.Error())
	}
}
