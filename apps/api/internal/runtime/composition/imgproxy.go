package composition

import (
	"fmt"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/platform/config"
)

// imgproxyConfigFromApp builds the signing config for image rendering
// from the loaded application config.
func imgproxyConfigFromApp(cfg *config.Config) imgproxy.Config {
	if cfg == nil {
		return imgproxy.Config{}
	}
	return imgproxy.Config{
		BaseURL: cfg.Imgproxy.BaseURL,
		KeyHex:  cfg.Imgproxy.KeyHex,
		SaltHex: cfg.Imgproxy.SaltHex,
	}
}

// validateImgproxyOrFatal requires a usable internal renderer binding at startup.
func validateImgproxyOrFatal(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	return imgproxyConfigFromApp(cfg).Validate()
}
