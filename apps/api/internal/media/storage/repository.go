package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
)

const storageSecretAccessKeyName = "secret_access_key"

var ErrStorageSecretVersionConflict = errors.New("storage secret version rotation conflict: retry the request")
var ErrStorageProfileLocatorImmutable = errors.New("storage profile locator is immutable after media assets reference it")

type storageProfileSecretPolicy int

const (
	storageProfileSecretMetadataOnly storageProfileSecretPolicy = iota
	storageProfileSecretPlaintext
)

type storageProfileSaveOptions struct {
	rotateSecret bool
}

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
	return r.saveProfile(ctx, profile, storageProfileSaveOptions{
		rotateSecret: storageProfileHasRealSecret(profile),
	})
}

func (r *StorageProfileRepository) saveProfile(ctx context.Context, profile StorageProfile, options storageProfileSaveOptions) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return saveProfileTx(ctx, q, profile, options)
	})
}

func (r *StorageProfileRepository) SaveAndApplyProfile(ctx context.Context, profile StorageProfile) error {
	return r.saveAndApplyProfile(ctx, profile, storageProfileSaveOptions{
		rotateSecret: storageProfileHasRealSecret(profile),
	})
}

func (r *StorageProfileRepository) saveAndApplyProfile(ctx context.Context, profile StorageProfile, options storageProfileSaveOptions) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		saved := profile
		saved.IsCurrent = false
		if err := saveProfileTx(ctx, q, saved, options); err != nil {
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

func saveProfileTx(ctx context.Context, q *dbsqlc.Queries, profile StorageProfile, options storageProfileSaveOptions) error {
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
		Provider:  string(canonicalizeStorageProvider(profile.Provider)),
		Enabled:   profile.Enabled,
		IsCurrent: profile.IsCurrent,
		Builtin:   profile.Builtin,
		Status:    status,
		Config:    configJSON,
		CreatedBy: sql.NullInt64{},
		UpdatedBy: sql.NullInt64{},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrStorageProfileLocatorImmutable
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
		RotatedAt:          sql.NullTime{Time: time.Now(), Valid: true},
		ExpiresAt:          sql.NullTime{},
	})
	if database.IsDuplicateConstraintError(err) {
		return ErrStorageSecretVersionConflict
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
		return ErrStorageSecretVersionConflict
	}
	return nil
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

	versionRows, err := r.q.ListStorageProfileSecretVersions(ctx, dbsqlc.ListStorageProfileSecretVersionsParams{
		ProfileID:  row.ProfileID,
		SecretName: storageSecretAccessKeyName,
	})
	if err != nil {
		return StorageProfile{}, err
	}
	secretVersions := make([]StorageSecretVersion, 0, len(versionRows))
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
		configured, err := r.q.HasActiveStorageProfileSecretVersion(ctx, dbsqlc.HasActiveStorageProfileSecretVersionParams{
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
		SecretVersions:   secretVersions,
		Config:           config,
	}, nil
}

func storageSecretVersionFromRow(row dbsqlc.StorageProfileSecretVersion) StorageSecretVersion {
	return StorageSecretVersion{
		ID:             row.ID,
		SecretName:     row.SecretName,
		VersionNumber:  row.VersionNumber,
		Status:         row.Status,
		RotatedAt:      row.RotatedAt,
		ExpiresAt:      timePtr(row.ExpiresAt),
		RetiredAt:      timePtr(row.RetiredAt),
		RevokedAt:      timePtr(row.RevokedAt),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		HasSecretValue: strings.TrimSpace(row.SecretPayload) != "" || strings.TrimSpace(row.SecretRef) != "",
	}
}

func timePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	out := value.Time
	return &out
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

func storageProfileHasRealSecret(profile StorageProfile) bool {
	return storageProfileRealSecretValue(profile) != ""
}

func storageProfileRealSecretValue(profile StorageProfile) string {
	if canonicalizeStorageProvider(profile.Provider) != StorageProviderS3Compatible {
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
