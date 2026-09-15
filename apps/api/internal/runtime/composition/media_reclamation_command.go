package composition

import (
	"context"
	"log/slog"
	"time"

	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/platform/database"
)

func runMediaReclamation(staleAfter time.Duration) {
	if staleAfter <= 0 {
		fatal("usage: api media-reclamation scan [--stale-after=30m]")
	}
	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer connectCancel()
	db, err := database.NewPostgres(connectCtx, cfg.Database)
	if err != nil {
		fatal("failed to connect to database for media reclamation scan", "err", err)
	}
	defer db.Close()

	report, err := reclamation.Scan(context.Background(), db, staleAfter)
	if err != nil {
		fatal("media reclamation scan failed", "err", err)
	}
	slog.Info("media reclamation scan completed",
		"stale_uploading_assets", report.StaleUploadingAssets,
		"stale_processing_derivatives", report.StaleProcessingDerivatives,
		"orphan_assets", report.OrphanAssets,
		"pending_deletion_assets", report.PendingDeletionAssets,
		"stale_after", staleAfter.String(),
	)
}
