package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const storageSecretAccessKeyName = "secret_access_key"

type storageProfileSecretPolicy int

const (
	storageProfileSecretMetadataOnly storageProfileSecretPolicy = iota
	storageProfileSecretPlaintext
)

type storageProfileSaveOptions struct {
	rotateSecret bool
}

type Repository struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

var _ ports.ProfileRepository = (*Repository)(nil)

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) ListProfiles(ctx context.Context, includeDeleted bool) ([]domain.StorageProfile, error) {
	return r.listProfiles(ctx, includeDeleted, storageProfileSecretMetadataOnly)
}

func (r *Repository) ListProfilesWithSecrets(ctx context.Context, includeDeleted bool) ([]domain.StorageProfile, error) {
	return r.listProfiles(ctx, includeDeleted, storageProfileSecretPlaintext)
}

func (r *Repository) listProfiles(ctx context.Context, includeDeleted bool, secretPolicy storageProfileSecretPolicy) ([]domain.StorageProfile, error) {
	rows, err := r.q.ListStorageProfiles(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	profiles := make([]domain.StorageProfile, 0, len(rows))
	for _, row := range rows {
		profile, err := r.profileFromRow(ctx, row, secretPolicy)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (r *Repository) SaveProfile(ctx context.Context, profile domain.StorageProfile) error {
	return r.saveProfile(ctx, profile, storageProfileSaveOptions{
		rotateSecret: storageProfileHasRealSecret(profile),
	})
}

func (r *Repository) SaveProfileWithOptions(ctx context.Context, profile domain.StorageProfile, rotateSecret bool) error {
	return r.saveProfile(ctx, profile, storageProfileSaveOptions{
		rotateSecret: rotateSecret,
	})
}

func (r *Repository) saveProfile(ctx context.Context, profile domain.StorageProfile, options storageProfileSaveOptions) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return saveProfileTx(ctx, q, profile, options)
	})
}

func (r *Repository) SaveAndApplyProfile(ctx context.Context, profile domain.StorageProfile) error {
	return r.saveAndApplyProfile(ctx, profile, storageProfileSaveOptions{
		rotateSecret: storageProfileHasRealSecret(profile),
	})
}

func (r *Repository) SaveAndApplyProfileWithOptions(ctx context.Context, profile domain.StorageProfile, rotateSecret bool) error {
	return r.saveAndApplyProfile(ctx, profile, storageProfileSaveOptions{
		rotateSecret: rotateSecret,
	})
}

func (r *Repository) saveAndApplyProfile(ctx context.Context, profile domain.StorageProfile, options storageProfileSaveOptions) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		saved := profile
		saved.IsCurrent = false
		if err := saveProfileTx(ctx, q, saved, options); err != nil {
			return err
		}
		return applyStorageProfileTx(ctx, q, profile.ID)
	})
}

func (r *Repository) DeleteProfile(ctx context.Context, id string) error {
	rows, err := r.q.MarkStorageProfileDeleted(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *Repository) ApplyProfile(ctx context.Context, id string) error {
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
		UpdatedBy: pgtype.Int8{},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func saveProfileTx(ctx context.Context, q *dbsqlc.Queries, profile domain.StorageProfile, options storageProfileSaveOptions) error {
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
	rotateSecret := options.rotateSecret && secret != ""
	fingerprint := ""
	if rotateSecret {
		fingerprint = storageSecretFingerprint(secret)
		if err := lockStorageProfileSecretVersionRotation(ctx, q, id, storageSecretAccessKeyName); err != nil {
			return err
		}
		active, err := q.GetActiveStorageProfileSecretVersion(ctx, dbsqlc.GetActiveStorageProfileSecretVersionParams{
			ProfileID:  id,
			SecretName: storageSecretAccessKeyName,
		})
		if err != nil && !database.IsNotFound(err) {
			return err
		}
		if err == nil && active.Fingerprint == fingerprint {
			rotateSecret = false
		}
	}
	rows, err := q.UpsertStorageProfile(ctx, dbsqlc.UpsertStorageProfileParams{
		ProfileID: id,
		Name:      strings.TrimSpace(profile.Name),
		Provider:  string(profile.Provider),
		Enabled:   profile.Enabled,
		IsCurrent: profile.IsCurrent,
		Builtin:   profile.Builtin,
		Status:    status,
		Config:    configJSON,
		CreatedBy: pgtype.Int8{},
		UpdatedBy: pgtype.Int8{},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrStorageProfileLocatorImmutable
	}
	if !rotateSecret {
		return nil
	}
	encrypted, err := secrets.EncryptString(secret)
	if err != nil {
		return fmt.Errorf("加密存储密钥失败: %w", err)
	}
	if _, err := q.RetireActiveStorageProfileSecretVersion(ctx, dbsqlc.RetireActiveStorageProfileSecretVersionParams{
		ProfileID:  id,
		SecretName: storageSecretAccessKeyName,
	}); err != nil {
		return err
	}
	_, err = q.InsertStorageProfileSecretVersion(ctx, dbsqlc.InsertStorageProfileSecretVersionParams{
		ProfileID:          id,
		SecretName:         storageSecretAccessKeyName,
		SecretMaterialKind: secrets.MaterialKindEncrypted,
		SecretRef:          "",
		SecretPayload:      encrypted,
		Fingerprint:        fingerprint,
		KeyVersion:         "",
		RotatedAt:          pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ExpiresAt:          pgtype.Timestamptz{},
	})
	if database.IsDuplicateConstraintError(err) {
		return domain.ErrStorageSecretVersionConflict
	}
	return err
}

func lockStorageProfileSecretVersionRotation(ctx context.Context, q *dbsqlc.Queries, profileID, secretName string) error {
	locked, err := q.LockStorageProfileSecretVersionRotation(ctx, dbsqlc.LockStorageProfileSecretVersionRotationParams{
		ProfileID:  profileID,
		SecretName: secretName,
	})
	if err != nil {
		return fmt.Errorf("lock storage secret version rotation: %w", err)
	}
	if !locked {
		return domain.ErrStorageSecretVersionConflict
	}
	return nil
}

func (r *Repository) profileFromRow(ctx context.Context, row dbsqlc.StorageProfile, secretPolicy storageProfileSecretPolicy) (domain.StorageProfile, error) {
	var config map[string]interface{}
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &config); err != nil {
			return domain.StorageProfile{}, err
		}
	}
	if config == nil {
		config = map[string]interface{}{}
	}

	versionRows, err := r.q.ListStorageProfileSecretVersions(ctx, dbsqlc.ListStorageProfileSecretVersionsParams{
		ProfileID:  row.ProfileID,
		SecretName: storageSecretAccessKeyName,
	})
	if err != nil {
		return domain.StorageProfile{}, err
	}
	secretVersions := make([]domain.StorageSecretVersion, 0, len(versionRows))
	for _, versionRow := range versionRows {
		secretVersions = append(secretVersions, storageSecretVersionFromRow(versionRow))
	}

	secretConfigured := false
	switch secretPolicy {
	case storageProfileSecretPlaintext:
		secret, err := r.q.GetActiveStorageProfileSecretVersion(ctx, dbsqlc.GetActiveStorageProfileSecretVersionParams{
			ProfileID:  row.ProfileID,
			SecretName: storageSecretAccessKeyName,
		})
		if err != nil && !database.IsNotFound(err) {
			return domain.StorageProfile{}, err
		}
		if err == nil {
			plain, err := secrets.DecryptMaterialString(secret.SecretPayload, secret.SecretMaterialKind)
			if err != nil {
				return domain.StorageProfile{}, fmt.Errorf("解密存储密钥失败: %w", err)
			}
			if strings.TrimSpace(plain) != "" {
				config[storageSecretAccessKeyName] = plain
				secretConfigured = true
			}
		}
	case storageProfileSecretMetadataOnly:
		configured, err := r.q.HasActiveStorageProfileSecretVersion(ctx, dbsqlc.HasActiveStorageProfileSecretVersionParams{
			ProfileID:  row.ProfileID,
			SecretName: storageSecretAccessKeyName,
		})
		if err != nil {
			return domain.StorageProfile{}, err
		}
		secretConfigured = configured
	default:
		return domain.StorageProfile{}, fmt.Errorf("unknown storage profile secret policy: %d", secretPolicy)
	}

	return domain.StorageProfile{
		ID:               row.ProfileID,
		Name:             row.Name,
		Provider:         domain.StorageProvider(row.Provider),
		Enabled:          row.Enabled,
		IsCurrent:        row.IsCurrent,
		Builtin:          row.Builtin,
		SecretConfigured: secretConfigured,
		SecretVersions:   secretVersions,
		Config:           config,
	}, nil
}

func storageSecretVersionFromRow(row dbsqlc.StorageProfileSecretVersion) domain.StorageSecretVersion {
	return domain.StorageSecretVersion{
		ID:             row.ID,
		SecretName:     row.SecretName,
		VersionNumber:  row.VersionNumber,
		Status:         row.Status,
		RotatedAt:      dbutil.TimeValue(row.RotatedAt),
		ExpiresAt:      timePtr(row.ExpiresAt),
		RetiredAt:      timePtr(row.RetiredAt),
		RevokedAt:      timePtr(row.RevokedAt),
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:      dbutil.TimeValue(row.UpdatedAt),
		HasSecretValue: strings.TrimSpace(row.SecretPayload) != "" || strings.TrimSpace(row.SecretRef) != "",
	}
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	return dbutil.TimePtr(value)
}

func storageProfileConfigForDB(profile domain.StorageProfile) (json.RawMessage, string, error) {
	config := cleanConfigMap(profile.Provider, profile.Config)
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

func cleanConfigMap(provider domain.StorageProvider, cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		cloned[k] = v
	}
	return cloned
}

func storageProfileHasRealSecret(profile domain.StorageProfile) bool {
	return storageProfileRealSecretValue(profile) != ""
}

func storageProfileRealSecretValue(profile domain.StorageProfile) string {
	if profile.Provider != domain.StorageProviderS3Compatible {
		return ""
	}
	secret := strings.TrimSpace(configString(profile.Config, storageSecretAccessKeyName))
	if secret == "" || strings.Contains(secret, "*") {
		return ""
	}
	return secret
}

func storageSecretFingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func configString(cfg map[string]interface{}, key string) string {
	if cfg == nil {
		return ""
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}
