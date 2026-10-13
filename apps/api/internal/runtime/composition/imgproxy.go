package composition

import (
	"fmt"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/platform/config"
)

// imgproxyConfigFromApp builds the signing config for delivery handlers
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

// validateImgproxyOrFatal rejects boot without a usable on-demand image
// renderer. Display tiers resolve through signed imgproxy URLs, so starting
// without it would serve broken media.
func validateImgproxyOrFatal(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	return imgproxyConfigFromApp(cfg).Validate()
}
