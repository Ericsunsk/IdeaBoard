package bootstrap

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/platform/database"
)

func runMediaReclamation(args []string) {
	if len(args) == 0 || args[0] != "scan" {
		fatal("usage: api media-reclamation scan [--stale-after=30m]")
	}
	fs := flag.NewFlagSet("media-reclamation scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	staleAfter := fs.Duration("stale-after", reclamation.StaleUploadingAfter, "age after which uploading assets are reported as stale")
	if err := fs.Parse(args[1:]); err != nil {
		fatal("failed to parse media reclamation scan arguments", "err", err)
	}
	if len(fs.Args()) > 0 || *staleAfter <= 0 {
		fatal("usage: api media-reclamation scan [--stale-after=30m]")
	}

	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		fatal("failed to connect to database for media reclamation scan", "err", err)
	}
	defer db.Close()

	report, err := reclamation.Scan(context.Background(), db, *staleAfter)
	if err != nil {
		fatal("media reclamation scan failed", "err", err)
	}
	slog.Info("media reclamation scan completed",
		"stale_uploading_assets", report.StaleUploadingAssets,
		"stale_processing_derivatives", report.StaleProcessingDerivatives,
		"missing_derivative_outbox", report.MissingDerivativeOutbox,
		"missing_default_derivatives", report.MissingDefaultDerivatives,
		"orphan_assets", report.OrphanAssets,
		"pending_deletion_assets", report.PendingDeletionAssets,
		"stale_after", staleAfter.String(),
	)
}
