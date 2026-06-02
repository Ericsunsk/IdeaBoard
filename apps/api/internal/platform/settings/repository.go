package settings

import (
	"context"
	"database/sql"
	"strings"

	dbsqlc "api/internal/db/sqlc"
)

type SystemConfigRepository struct {
	q *dbsqlc.Queries
}

func NewSystemConfigRepository(db *sql.DB) *SystemConfigRepository {
	return &SystemConfigRepository{q: dbsqlc.New(db)}
}

func (r *SystemConfigRepository) Get(ctx context.Context, key string) (string, error) {
	cfg, err := r.q.GetSystemConfig(ctx, key)
	if err != nil {
		return "", err
	}
	return cfg.Value, nil
}

func (r *SystemConfigRepository) GetByPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	cfgs, err := r.q.ListSystemConfigsByPrefix(ctx, prefix+"%")
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(cfgs))
	for _, c := range cfgs {
		key := strings.TrimPrefix(c.ConfigKey, prefix)
		result[key] = c.Value
	}
	return result, nil
}

func (r *SystemConfigRepository) Set(ctx context.Context, key, value string) error {
	return r.q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: key, Value: value})
}
