package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
)

type AccountRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type RunningTaskFinalizerTx func(ctx context.Context, q *dbsqlc.Queries, completedAt time.Time) ([]string, error)

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db, q: dbsqlc.New(db)}
}

func (r *AccountRepository) Create(ctx context.Context, account *ApiAccount) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		writeAccount := accountForWrite(account)
		id, err := q.CreateAccount(ctx, dbsqlc.CreateAccountParams{
			Name:          writeAccount.Name,
			ChannelID:     writeAccount.ChannelID,
			Platform:      writeAccount.Platform,
			ApiType:       writeAccount.ApiType,
			ProxyUrl:      writeAccount.ProxyURL,
			MaxConcurrent: int32(writeAccount.MaxConcurrent),
			Timeout:       int32(writeAccount.Timeout),
			ExtraConfig:   writeAccount.ExtraConfig,
			Status:        writeAccount.Status,
			FailCount:     int32(writeAccount.FailCount),
			Weight:        int32(writeAccount.Weight),
		})
		if err != nil {
			return err
		}
		writeAccount.ID = int64(id)
		if account != nil {
			account.ID = writeAccount.ID
		}
		if err := replaceAccountModels(ctx, q, writeAccount.ID, accountModelsForWrite(writeAccount)); err != nil {
			return err
		}
		return rotateAccountSecret(ctx, q, writeAccount.ID, writeAccount.ApiKey)
	})
}

func (r *AccountRepository) GetByID(ctx context.Context, id int64) (*ApiAccount, error) {
	row, err := r.q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account := accountFromProjection(accountProjectionFromGet(row))
	if err := r.attachAccountModels(ctx, []*ApiAccount{&account}); err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) GetRuntimeByIDWithSecret(ctx context.Context, id int64) (*ApiAccount, error) {
	row, err := r.q.GetRuntimeAccountByIDWithSecret(ctx, id)
	if err != nil {
		return nil, err
	}
	projection, err := accountProjectionFromRuntimeGet(row)
	if err != nil {
		return nil, err
	}
	account := accountFromProjection(projection)
	if err := r.attachAccountModels(ctx, []*ApiAccount{&account}); err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) List(ctx context.Context, keyword, platform string, page, pageSize int) ([]*ApiAccount, int64, error) {
	keyword = strings.TrimSpace(keyword)
	platform = strings.TrimSpace(platform)
	keywordLike := "%" + keyword + "%"
	hasKeyword := keyword != ""
	hasPlatform := platform != ""
	params := dbsqlc.CountAccountsParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasPlatform: hasPlatform,
		Platform:    platform,
	}
	total, err := r.q.CountAccounts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListAccounts(ctx, dbsqlc.ListAccountsParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasPlatform: hasPlatform,
		Platform:    platform,
		PageLimit:   int32(pageSize),
		PageOffset:  int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	accounts := accountsFromListRows(rows)
	if err := r.attachAccountModels(ctx, accounts); err != nil {
		return nil, 0, err
	}
	return accounts, total, nil
}

func (r *AccountRepository) GetAllEnabled(ctx context.Context) ([]*ApiAccount, error) {
	rows, err := r.q.ListEnabledAccounts(ctx)
	if err != nil {
		return nil, err
	}
	accounts := accountsFromEnabledRows(rows)
	if err := r.attachAccountModels(ctx, accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *AccountRepository) ListAccountIDs(ctx context.Context) ([]int64, error) {
	return r.q.ListAccountIDs(ctx)
}

func (r *AccountRepository) Update(ctx context.Context, account *ApiAccount) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		writeAccount := accountForWrite(account)
		updated, err := q.UpdateAccount(ctx, dbsqlc.UpdateAccountParams{
			Name:          writeAccount.Name,
			Platform:      writeAccount.Platform,
			ApiType:       writeAccount.ApiType,
			ProxyUrl:      writeAccount.ProxyURL,
			MaxConcurrent: int32(writeAccount.MaxConcurrent),
			Timeout:       int32(writeAccount.Timeout),
			ExtraConfig:   writeAccount.ExtraConfig,
			FailCount:     int32(writeAccount.FailCount),
			Weight:        int32(writeAccount.Weight),
			ID:            writeAccount.ID,
		})
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		if err := replaceAccountModels(ctx, q, writeAccount.ID, accountModelsForWrite(writeAccount)); err != nil {
			return err
		}
		if strings.TrimSpace(writeAccount.ApiKey) == "" {
			return nil
		}
		return rotateAccountSecret(ctx, q, writeAccount.ID, writeAccount.ApiKey)
	})
}

func (r *AccountRepository) Delete(ctx context.Context, id int64) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.ArchiveAccount(ctx, id)
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		return q.RevokeAccountSecretsForArchive(ctx, id)
	})
}

func (r *AccountRepository) ArchiveAccountAndFinalizeRunningTasks(ctx context.Context, id int64, finalizer RunningTaskFinalizerTx) ([]string, error) {
	var failedTaskIDs []string
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.ArchiveAccount(ctx, id)
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		if err := q.RevokeAccountSecretsForArchive(ctx, id); err != nil {
			return err
		}
		if finalizer == nil {
			return nil
		}
		failedTaskIDs, err = finalizer(ctx, q, time.Now())
		return err
	})
	return failedTaskIDs, err
}

func (r *AccountRepository) UpdateStatus(ctx context.Context, id int64, status int16) error {
	updated, err := r.q.UpdateAccountStatus(ctx, dbsqlc.UpdateAccountStatusParams{Status: status, ID: id})
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *AccountRepository) StopAccountAndFinalizeRunningTasks(ctx context.Context, id int64, finalizer RunningTaskFinalizerTx) ([]string, error) {
	var failedTaskIDs []string
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.UpdateAccountStatus(ctx, dbsqlc.UpdateAccountStatusParams{Status: 0, ID: id})
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		if finalizer == nil {
			return nil
		}
		failedTaskIDs, err = finalizer(ctx, q, time.Now())
		return err
	})
	return failedTaskIDs, err
}

func (r *AccountRepository) IncrementFailCount(ctx context.Context, id int64) error {
	updated, err := r.q.IncrementAccountFailCount(ctx, id)
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *AccountRepository) ResetFailCount(ctx context.Context, id int64) error {
	updated, err := r.q.ResetAccountFailCount(ctx, id)
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

type accountProjection struct {
	ID               int64
	Name             string
	ChannelID        int64
	Platform         string
	ApiType          string
	ApiKey           string
	SecretConfigured bool
	ProxyURL         string
	MaxConcurrent    int32
	Timeout          int32
	ExtraConfig      string
	Status           int16
	FailCount        int32
	Weight           int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func accountFromProjection(row accountProjection) ApiAccount {
	return ApiAccount{
		ID:               row.ID,
		Name:             row.Name,
		ChannelID:        row.ChannelID,
		Platform:         row.Platform,
		ApiType:          row.ApiType,
		ApiKey:           row.ApiKey,
		SecretConfigured: row.SecretConfigured,
		ProxyURL:         row.ProxyURL,
		MaxConcurrent:    int(row.MaxConcurrent),
		Timeout:          int(row.Timeout),
		ExtraConfig:      row.ExtraConfig,
		Status:           row.Status,
		FailCount:        int(row.FailCount),
		Weight:           int(row.Weight),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func accountProjectionFromGet(row dbsqlc.GetAccountByIDRow) accountProjection {
	return accountProjection{
		ID:               row.ID,
		Name:             row.Name,
		ChannelID:        row.ChannelID,
		Platform:         row.Platform,
		ApiType:          row.ApiType,
		ApiKey:           row.ApiKey,
		SecretConfigured: row.SecretConfigured,
		ProxyURL:         row.ProxyUrl,
		MaxConcurrent:    row.MaxConcurrent,
		Timeout:          row.Timeout,
		ExtraConfig:      row.ExtraConfig,
		Status:           row.Status,
		FailCount:        row.FailCount,
		Weight:           row.Weight,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func accountProjectionFromRuntimeGet(row dbsqlc.GetRuntimeAccountByIDWithSecretRow) (accountProjection, error) {
	apiKey, err := decryptAPIKey(row.ApiKey, row.ApiKeyMaterialKind)
	if err != nil {
		return accountProjection{}, err
	}
	return accountProjection{
		ID:               row.ID,
		Name:             row.Name,
		ChannelID:        row.ChannelID,
		Platform:         row.Platform,
		ApiType:          row.ApiType,
		ApiKey:           apiKey,
		SecretConfigured: row.SecretConfigured,
		ProxyURL:         row.ProxyUrl,
		MaxConcurrent:    row.MaxConcurrent,
		Timeout:          row.Timeout,
		ExtraConfig:      row.ExtraConfig,
		Status:           row.Status,
		FailCount:        row.FailCount,
		Weight:           row.Weight,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}

func accountProjectionFromList(row dbsqlc.ListAccountsRow) accountProjection {
	return accountProjection{
		ID:               row.ID,
		Name:             row.Name,
		ChannelID:        row.ChannelID,
		Platform:         row.Platform,
		ApiType:          row.ApiType,
		ApiKey:           row.ApiKey,
		SecretConfigured: row.SecretConfigured,
		ProxyURL:         row.ProxyUrl,
		MaxConcurrent:    row.MaxConcurrent,
		Timeout:          row.Timeout,
		ExtraConfig:      row.ExtraConfig,
		Status:           row.Status,
		FailCount:        row.FailCount,
		Weight:           row.Weight,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func accountProjectionFromEnabled(row dbsqlc.ListEnabledAccountsRow) accountProjection {
	return accountProjection{
		ID:               row.ID,
		Name:             row.Name,
		ChannelID:        row.ChannelID,
		Platform:         row.Platform,
		ApiType:          row.ApiType,
		ApiKey:           row.ApiKey,
		SecretConfigured: row.SecretConfigured,
		ProxyURL:         row.ProxyUrl,
		MaxConcurrent:    row.MaxConcurrent,
		Timeout:          row.Timeout,
		ExtraConfig:      row.ExtraConfig,
		Status:           row.Status,
		FailCount:        row.FailCount,
		Weight:           row.Weight,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func accountsFromListRows(rows []dbsqlc.ListAccountsRow) []*ApiAccount {
	return accountsFromRows(rows, accountProjectionFromList)
}

func accountsFromEnabledRows(rows []dbsqlc.ListEnabledAccountsRow) []*ApiAccount {
	return accountsFromRows(rows, accountProjectionFromEnabled)
}

func accountsFromRows[T any](rows []T, project func(T) accountProjection) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(rows))
	for _, row := range rows {
		account := accountFromProjection(project(row))
		result = append(result, &account)
	}
	return result
}

func (r *AccountRepository) attachAccountModels(ctx context.Context, accounts []*ApiAccount) error {
	accountIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountIDs = append(accountIDs, account.ID)
		}
	}
	if len(accountIDs) == 0 {
		return nil
	}
	rows, err := r.q.ListAccountModelsByAccountIDs(ctx, accountIDs)
	if err != nil {
		return err
	}
	modelsByAccountID := make(map[int64][]AccountModel, len(accountIDs))
	for _, row := range rows {
		model, err := accountModelFromSQLC(row)
		if err != nil {
			return err
		}
		modelsByAccountID[row.AccountID] = append(modelsByAccountID[row.AccountID], model)
	}
	for _, account := range accounts {
		if account != nil {
			account.Models = modelsByAccountID[account.ID]
		}
	}
	return nil
}

func accountModelFromSQLC(row dbsqlc.ApiAccountModel) (AccountModel, error) {
	var slugs []string
	if len(row.FeatureSlugs) > 0 {
		if err := json.Unmarshal(row.FeatureSlugs, &slugs); err != nil {
			return AccountModel{}, fmt.Errorf("解析账号模型 %q 的 feature_slugs 失败: %w", row.ModelName, err)
		}
	}
	return AccountModel{
		ID:            row.ID,
		Name:          row.ModelName,
		Enabled:       row.Enabled,
		Weight:        int(row.Weight),
		MaxConcurrent: intPtr(row.MaxConcurrent),
		FeatureSlugs:  slugs,
		SortOrder:     int(row.SortOrder),
	}, nil
}

func accountModelsForWrite(account *ApiAccount) []AccountModel {
	if account == nil {
		return nil
	}
	return account.Models
}

func accountForWrite(account *ApiAccount) *ApiAccount {
	if account == nil {
		return &ApiAccount{}
	}
	normalized := *account
	normalized.MaxConcurrent = accountlimits.NormalizeMaxConcurrent(normalized.MaxConcurrent)
	if normalized.Timeout <= 0 {
		normalized.Timeout = 120
	}
	if normalized.Weight <= 0 {
		normalized.Weight = 1
	}
	if normalized.FailCount < 0 {
		normalized.FailCount = 0
	}
	if normalized.Status != 0 && normalized.Status != 1 {
		normalized.Status = 1
	}
	return &normalized
}

func replaceAccountModels(ctx context.Context, q *dbsqlc.Queries, accountID int64, models []AccountModel) error {
	if err := q.DeleteAccountModelsByAccountID(ctx, accountID); err != nil {
		return err
	}
	for i, model := range models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		weight := model.Weight
		if weight <= 0 {
			weight = 1
		}
		featureSlugs, err := accountModelFeatureSlugsJSON(model.FeatureSlugs)
		if err != nil {
			return err
		}
		sortOrder := model.SortOrder
		if sortOrder < 0 {
			sortOrder = i
		}
		if _, err := q.UpsertAccountModel(ctx, dbsqlc.UpsertAccountModelParams{
			AccountID:     accountID,
			ModelName:     name,
			Enabled:       model.Enabled,
			Weight:        int32(weight),
			MaxConcurrent: nullInt32Ptr(model.MaxConcurrent),
			FeatureSlugs:  featureSlugs,
			Metadata:      json.RawMessage("{}"),
			SortOrder:     int32(sortOrder),
		}); err != nil {
			return err
		}
	}
	return nil
}

func accountModelFeatureSlugsJSON(slugs []string) (json.RawMessage, error) {
	if slugs == nil {
		slugs = []string{}
	}
	return json.Marshal(slugs)
}

func rotateAccountSecret(ctx context.Context, q *dbsqlc.Queries, accountID int64, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	if err := secrets.RequireEncryptionEnabled(); err != nil {
		return err
	}
	if err := q.RetireActiveAccountSecrets(ctx, accountID); err != nil {
		return err
	}

	payload, err := secrets.EncryptString(apiKey)
	if err != nil {
		return fmt.Errorf("加密 API Key 失败: %w", err)
	}

	_, err = q.CreateAccountSecret(ctx, dbsqlc.CreateAccountSecretParams{
		AccountID:          accountID,
		SecretMaterialKind: secrets.MaterialKindEncrypted,
		SecretRef:          "",
		SecretPayload:      payload,
		KeyVersion:         "",
		Fingerprint:        "",
		Status:             "active",
		ExpiresAt:          sql.NullTime{},
	})
	return err
}

func decryptAPIKey(apiKey, materialKind string) (string, error) {
	plain, err := secrets.DecryptMaterialString(apiKey, materialKind)
	if err != nil {
		return "", fmt.Errorf("解密 API Key 失败: %w", err)
	}
	return plain, nil
}

func nullInt32Ptr(value *int) sql.NullInt32 {
	if value == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*value), Valid: true}
}

func intPtr(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int32)
	return &v
}
