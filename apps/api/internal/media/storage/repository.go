package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
)

const storageSecretAccessKeyName = "secret_access_key"

type storageProfileSecretPolicy int

const (
	storageProfileSecretMetadataOnly storageProfileSecretPolicy = iota
	storageProfileSecretPlaintext
)

type StorageProfileRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewStorageProfileRepository(db *sql.DB) *StorageProfileRepository {
	return &StorageProfileRepository{db: db, q: dbsqlc.New(db)}
}

func (r *StorageProfileRepository) ListProfiles(ctx context.Context, includeDeleted bool) ([]StorageProfile, error) {
	return r.listProfiles(ctx, includeDeleted, storageProfileSecretMetadataOnly)
}

func (r *StorageProfileRepository) ListProfilesWithSecrets(ctx context.Context, includeDeleted bool) ([]StorageProfile, error) {
	return r.listProfiles(ctx, includeDeleted, storageProfileSecretPlaintext)
}

func (r *StorageProfileRepository) listProfiles(ctx context.Context, includeDeleted bool, secretPolicy storageProfileSecretPolicy) ([]StorageProfile, error) {
	rows, err := r.q.ListStorageProfiles(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	profiles := make([]StorageProfile, 0, len(rows))
	for _, row := range rows {
		profile, err := r.profileFromRow(ctx, row, secretPolicy)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (r *StorageProfileRepository) SaveProfile(ctx context.Context, profile StorageProfile) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return saveProfileTx(ctx, q, profile)
	})
}

func (r *StorageProfileRepository) SaveAndApplyProfile(ctx context.Context, profile StorageProfile) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		saved := profile
		saved.IsCurrent = false
		if err := saveProfileTx(ctx, q, saved); err != nil {
			return err
		}
		return applyStorageProfileTx(ctx, q, profile.ID)
	})
}

func (r *StorageProfileRepository) DeleteProfile(ctx context.Context, id string) error {
	rows, err := r.q.MarkStorageProfileDeleted(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *StorageProfileRepository) ApplyProfile(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return database.ErrNotFound
	}
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return applyStorageProfileTx(ctx, q, id)
	})
}

func applyStorageProfileTx(ctx context.Context, q *dbsqlc.Queries, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return database.ErrNotFound
	}
	if err := q.ClearCurrentStorageProfiles(ctx); err != nil {
		return err
	}
	rows, err := q.SetCurrentStorageProfile(ctx, dbsqlc.SetCurrentStorageProfileParams{
		ProfileID: id,
		UpdatedBy: sql.NullInt64{},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func saveProfileTx(ctx context.Context, q *dbsqlc.Queries, profile StorageProfile) error {
	id := strings.TrimSpace(profile.ID)
	if id == "" {
		return nil
	}
	configJSON, secret, err := storageProfileConfigForDB(profile)
	if err != nil {
		return err
	}
	status := "active"
	if !profile.Enabled {
		status = "disabled"
	}
	if err := q.UpsertStorageProfile(ctx, dbsqlc.UpsertStorageProfileParams{
		ProfileID: id,
		Name:      strings.TrimSpace(profile.Name),
		Provider:  string(canonicalizeStorageProvider(profile.Provider)),
		Enabled:   profile.Enabled,
		IsCurrent: profile.IsCurrent,
		Builtin:   profile.Builtin,
		Status:    status,
		Config:    configJSON,
		CreatedBy: sql.NullInt64{},
		UpdatedBy: sql.NullInt64{},
	}); err != nil {
		return err
	}
	if secret == "" {
		return nil
	}
	fingerprint := storageSecretFingerprint(secret)
	encrypted, err := secrets.EncryptString(secret)
	if err != nil {
		return fmt.Errorf("加密存储密钥失败: %w", err)
	}
	return q.UpsertStorageProfileSecret(ctx, dbsqlc.UpsertStorageProfileSecretParams{
		ProfileID:          id,
		SecretName:         storageSecretAccessKeyName,
		SecretMaterialKind: secrets.MaterialKindEncrypted,
		SecretRef:          "",
		SecretPayload:      encrypted,
		Fingerprint:        fingerprint,
		KeyVersion:         "",
		Status:             "active",
		RotatedAt:          sql.NullTime{Time: time.Now(), Valid: true},
		ExpiresAt:          sql.NullTime{},
		RetiredAt:          sql.NullTime{},
	})
}

func (r *StorageProfileRepository) profileFromRow(ctx context.Context, row dbsqlc.StorageProfile, secretPolicy storageProfileSecretPolicy) (StorageProfile, error) {
	var config map[string]interface{}
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &config); err != nil {
			return StorageProfile{}, err
		}
	}
	if config == nil {
		config = map[string]interface{}{}
	}

	secretConfigured := false
	switch secretPolicy {
	case storageProfileSecretPlaintext:
		secret, err := r.q.GetStorageProfileSecret(ctx, dbsqlc.GetStorageProfileSecretParams{
			ProfileID:  row.ProfileID,
			SecretName: storageSecretAccessKeyName,
		})
		if err != nil && !database.IsNotFound(err) {
			return StorageProfile{}, err
		}
		if err == nil {
			plain, err := secrets.DecryptMaterialString(secret.SecretPayload, secret.SecretMaterialKind)
			if err != nil {
				return StorageProfile{}, fmt.Errorf("解密存储密钥失败: %w", err)
			}
			if strings.TrimSpace(plain) != "" {
				config[storageSecretAccessKeyName] = plain
				secretConfigured = true
			}
		}
	case storageProfileSecretMetadataOnly:
		configured, err := r.q.HasStorageProfileSecret(ctx, dbsqlc.HasStorageProfileSecretParams{
			ProfileID:  row.ProfileID,
			SecretName: storageSecretAccessKeyName,
		})
		if err != nil {
			return StorageProfile{}, err
		}
		secretConfigured = configured
	default:
		return StorageProfile{}, fmt.Errorf("unknown storage profile secret policy: %d", secretPolicy)
	}

	return StorageProfile{
		ID:               row.ProfileID,
		Name:             row.Name,
		Provider:         StorageProvider(row.Provider),
		Enabled:          row.Enabled,
		IsCurrent:        row.IsCurrent,
		Builtin:          row.Builtin,
		SecretConfigured: secretConfigured,
		Config:           config,
	}, nil
}

func storageProfileConfigForDB(profile StorageProfile) (json.RawMessage, string, error) {
	config := storageProfileConfigMap(profile.Provider, profile.Config)
	secret := strings.TrimSpace(configString(config, storageSecretAccessKeyName))
	delete(config, storageSecretAccessKeyName)
	if config == nil {
		config = map[string]interface{}{}
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, "", err
	}
	return json.RawMessage(raw), secret, nil
}

func storageSecretFingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
