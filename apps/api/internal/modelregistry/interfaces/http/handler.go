// Package http is the thin inbound adapter for the modelregistry context.
// The handler implementation stays in application; this package gives
// composition a stable seam that depends on ports (ConfigRepository,
// ObservedReader) instead of the concrete application wiring.
package http

import (
	"ideaboard/api/internal/modelregistry/application"
	"ideaboard/api/internal/modelregistry/ports"
)

// Handler serves the Admin AI gateway settings endpoints.
// It aliases the application handler so the guard and wiring program to
// this adapter package without importing application directly.
type Handler = application.Handler

// NewHandler builds the admin handler over the config seam.
func NewHandler(store ports.ConfigRepository) *Handler {
	return application.NewHandler(store)
}
