package billingoverview

import (
	"context"

	"ideaboard/api/internal/db/dbutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

func PostgresSnapshot(pool *pgxpool.Pool) Snapshot {
	return func(ctx context.Context, read func(context.Context) error) error {
		return dbutil.WithReadSnapshot(ctx, pool, read)
	}
}
