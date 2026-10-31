package composition

import (
	"context"
	"log/slog"

	"ideaboard/api/internal/boards/previewscene"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

func newBackfillBoardPreviewsCommand() *cobra.Command {
	var batchSize int
	var maxBoards int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "backfill-board-previews",
		Short: "Assign missing board content versions and scene projections",
		Long: `Assigns missing content identities and scene projections from stored documents.
Idempotent and resumable; document content and ordering remain unchanged.
With --dry-run only the first batch is reported and no writes are issued.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			runBackfillBoardPreviews(batchSize, maxBoards, dryRun)
			return nil
		},
	}
	cmd.Flags().IntVar(&batchSize, "batch-size", 200, "rows scanned per page (1..1000)")
	cmd.Flags().IntVar(&maxBoards, "max-boards", 0, "stop after N rows (0 = no cap)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing")
	return cmd
}

func runBackfillBoardPreviews(batchSize, maxBoards int, dryRun bool) {
	cfg := loadBackfillConfigOrFatal()
	configureRuntimeLogging(cfg)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, database.PostgresDSN(cfg.Database))
	if err != nil {
		fatal("failed to connect to database", "err", err)
	}
	defer pool.Close()
	stats, err := previewscene.BackfillBoardPreviews(ctx, dbsqlc.New(pool), batchSize, maxBoards, dryRun)
	if err != nil {
		fatal("failed to backfill board previews", "err", err)
	}
	slog.Info("board preview backfill completed",
		"scanned", stats.Scanned,
		"updated", stats.Updated,
		"scenes", stats.Scenes,
		"dry_run", dryRun,
	)
}
