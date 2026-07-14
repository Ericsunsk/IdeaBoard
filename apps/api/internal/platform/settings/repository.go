package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/adminaction"
)

type SystemConfigRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type systemConfigContextDB struct {
	ctx context.Context
	db  *sql.DB
}

func NewSystemConfigRepository(db *sql.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db, q: dbsqlc.New(db)}
}

func (r *SystemConfigRepository) dbWithContext(ctx context.Context) systemConfigContextDB {
	return systemConfigContextDB{ctx: ctx, db: r.db}
}

func (db systemConfigContextDB) withTx(run func(*dbsqlc.Queries, *sql.Tx) error) error {
	return dbutil.WithTxDBTX(db.ctx, db.db, run)
}

type SystemConfigValue struct {
	Key   string
	Value string
}

type SystemConfigAdminAction struct {
	ActionType string
	Action     adminaction.Metadata
	Metadata   json.RawMessage
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

func (r *SystemConfigRepository) GetMany(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	cfgs, err := r.q.ListSystemConfigsByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(cfgs))
	for _, cfg := range cfgs {
		result[cfg.ConfigKey] = cfg.Value
	}
	return result, nil
}

func (r *SystemConfigRepository) Set(ctx context.Context, key, value string) error {
	return r.q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: key, Value: value})
}

func (r *SystemConfigRepository) SetMany(ctx context.Context, values []SystemConfigValue) error {
	if len(values) == 0 {
		return nil
	}
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ *sql.Tx) error {
		for _, value := range values {
			if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: value.Key, Value: value.Value}); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetManyWithAdminAction keeps the operational configuration and its append-only audit record atomic.
func (r *SystemConfigRepository) SetManyWithAdminAction(ctx context.Context, values []SystemConfigValue, action SystemConfigAdminAction) error {
	if r == nil || r.db == nil || r.q == nil {
		return ErrRuntimeSettingsRepositoryUnavailable
	}
	if len(values) == 0 {
		return nil
	}
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx *sql.Tx) error {
		for _, value := range values {
			if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: value.Key, Value: value.Value}); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO system_settings_admin_actions (action_type, actor_id, reason, metadata)
VALUES ($1, $2, $3, $4::jsonb)`, action.ActionType, action.Action.ActorID(), action.Action.Reason(), action.Metadata); err != nil {
			return fmt.Errorf("insert system settings admin action: %w", err)
		}
		return nil
	})
}
