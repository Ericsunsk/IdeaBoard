package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SystemConfigRepository struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

type systemConfigContextDB struct {
	ctx context.Context
	db  *pgxpool.Pool
}

func NewSystemConfigRepository(db *pgxpool.Pool) *SystemConfigRepository {
	return &SystemConfigRepository{db: db, q: dbsqlc.New(db)}
}

func (r *SystemConfigRepository) dbWithContext(ctx context.Context) systemConfigContextDB {
	return systemConfigContextDB{ctx: ctx, db: r.db}
}

func (db systemConfigContextDB) withTx(run func(*dbsqlc.Queries, pgx.Tx) error) error {
	return dbutil.WithTxDBTX(db.ctx, db.db, run)
}

type SystemConfigValue struct {
	Key   string
	Value string
}

type SystemConfigAdminAudit struct {
	ActionType string
	ActorID    int64
	Metadata   json.RawMessage
}

// SystemConfigWriteTx exposes transaction-bound settings operations to a
// context that must calculate and apply one atomic config mutation.
type SystemConfigWriteTx interface {
	Get(context.Context, string) (string, error)
	GetByPrefix(context.Context, string) ([]SystemConfigValue, error)
	SetMany(context.Context, []SystemConfigValue) error
	DeleteMany(context.Context, []string) error
}

type systemConfigWriteTx struct {
	q *dbsqlc.Queries
}

func (tx systemConfigWriteTx) Get(ctx context.Context, key string) (string, error) {
	value, err := tx.q.GetSystemConfig(ctx, key)
	if err != nil {
		return "", err
	}
	return value.Value, nil
}

func (tx systemConfigWriteTx) GetByPrefix(ctx context.Context, prefix string) ([]SystemConfigValue, error) {
	values, err := tx.q.ListSystemConfigsByPrefix(ctx, prefix+"%")
	if err != nil {
		return nil, err
	}
	result := make([]SystemConfigValue, len(values))
	for index, value := range values {
		result[index] = SystemConfigValue{Key: value.ConfigKey, Value: value.Value}
	}
	return result, nil
}

func (tx systemConfigWriteTx) SetMany(ctx context.Context, values []SystemConfigValue) error {
	for _, value := range values {
		if err := tx.q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: value.Key, Value: value.Value}); err != nil {
			return err
		}
	}
	return nil
}

func (tx systemConfigWriteTx) DeleteMany(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return tx.q.DeleteSystemConfigsByKeys(ctx, keys)
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
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ pgx.Tx) error {
		for _, value := range values {
			if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: value.Key, Value: value.Value}); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetManyWithAdminAudit keeps typed configuration and its append-only audit record atomic.
func (r *SystemConfigRepository) SetManyWithAdminAudit(ctx context.Context, values []SystemConfigValue, audit SystemConfigAdminAudit) error {
	if r == nil || r.db == nil || r.q == nil {
		return ErrRuntimeSettingsRepositoryUnavailable
	}
	if len(values) == 0 {
		return nil
	}
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		if err := upsertSystemConfigs(ctx, q, values); err != nil {
			return err
		}
		return insertSystemConfigAdminAction(ctx, tx, audit)
	})
}

// WithAIGatewayCatalogWrite serializes one AI Gateway catalog read-modify-write
// and commits its audit record atomically. The underlying advisory lock keeps
// its historical namespace so old and new binaries remain mutually exclusive
// during a rolling deployment.
func (r *SystemConfigRepository) WithAIGatewayCatalogWrite(
	ctx context.Context,
	mutate func(SystemConfigWriteTx) (SystemConfigAdminAudit, error),
) error {
	if r == nil || r.db == nil || r.q == nil {
		return ErrRuntimeSettingsRepositoryUnavailable
	}
	if mutate == nil {
		return errors.New("system config mutation is nil")
	}
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		if err := q.LockAIGatewayCatalogWrite(ctx); err != nil {
			return err
		}
		audit, err := mutate(systemConfigWriteTx{q: q})
		if err != nil {
			return err
		}
		return insertSystemConfigAdminAction(ctx, tx, audit)
	})
}

// upsertSystemConfigs writes one batch of typed configuration rows.
func upsertSystemConfigs(ctx context.Context, q *dbsqlc.Queries, values []SystemConfigValue) error {
	for _, value := range values {
		if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{ConfigKey: value.Key, Value: value.Value}); err != nil {
			return err
		}
	}
	return nil
}

// insertSystemConfigAdminAction appends one audit record for an admin settings write.
func insertSystemConfigAdminAction(ctx context.Context, tx pgx.Tx, audit SystemConfigAdminAudit) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO system_settings_admin_actions (action_type, actor_id, metadata)
VALUES ($1, $2, $3::jsonb)`, audit.ActionType, audit.ActorID, audit.Metadata); err != nil {
		return fmt.Errorf("insert system settings admin action: %w", err)
	}
	return nil
}
