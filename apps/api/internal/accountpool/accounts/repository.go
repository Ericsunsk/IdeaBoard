package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
)

var errRunningTaskFinalizerNotConfigured = errors.New("running task finalizer is not configured")

type AccountRepository struct {
	db                   *sql.DB
	q                    *dbsqlc.Queries
	runningTaskFinalizer RunningTaskFinalizer
}

type RunningTaskFinalizer interface {
	FailRunningByAccountIDWithTx(ctx context.Context, tx *sql.Tx, accountID int64, errMsg string, completedAt time.Time) ([]string, error)
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db, q: dbsqlc.New(db)}
}

func NewAccountRepositoryWithRunningTaskFinalizer(db *sql.DB, finalizer RunningTaskFinalizer) *AccountRepository {
	return &AccountRepository{db: db, q: dbsqlc.New(db), runningTaskFinalizer: finalizer}
}

func (r *AccountRepository) Create(ctx context.Context, account *ApiAccount) error {
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		writeAccount := accountForWrite(account)
		id, err := q.CreateAccount(ctx, dbsqlc.CreateAccountParams{
			Name:               writeAccount.Name,
			ProviderEndpointID: writeAccount.ProviderEndpointID,
			MaxConcurrent:      int32(writeAccount.MaxConcurrent),
			Timeout:            int32(writeAccount.Timeout),
			ExtraConfig:        writeAccount.ExtraConfig,
			Status:             writeAccount.Status,
			FailCount:          int32(writeAccount.FailCount),
			Weight:             int32(writeAccount.Weight),
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
	if database.IsDuplicateConstraintError(err) {
		return ErrAccountNameConflict
	}
	return err
}

func (r *AccountRepository) GetByID(ctx context.Context, id int64) (*ApiAccount, error) {
	row, err := r.q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row.ApiAccount, row.ApiType, row.ApiKey, row.SecretConfigured, row.ProxyUrl)
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
	apiKey, err := decryptAPIKey(row.ApiKey, row.ApiKeyMaterialKind)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row.ApiAccount, row.ApiType, apiKey, row.SecretConfigured, row.ProxyUrl)
	if err := r.attachAccountModels(ctx, []*ApiAccount{&account}); err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*ApiAccount, error) {
	row, err := r.q.GetRuntimeAccountForAssignment(ctx, dbsqlc.GetRuntimeAccountForAssignmentParams{
		AccountID:          accountID,
		ProviderEndpointID: providerEndpointID,
		CredentialID:       credentialID,
	})
	if err != nil {
		return nil, err
	}
	apiKey, err := decryptAPIKey(row.ApiKey, row.ApiKeyMaterialKind)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row.ApiAccount, row.ApiType, apiKey, row.SecretConfigured, row.ProxyUrl)
	if err := r.attachAccountModels(ctx, []*ApiAccount{&account}); err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) GetRuntimeCredentialByID(ctx context.Context, accountID int64, credentialID int64) (*RuntimeCredential, error) {
	row, err := r.q.GetRuntimeCredentialByID(ctx, dbsqlc.GetRuntimeCredentialByIDParams{
		ID:        credentialID,
		AccountID: accountID,
	})
	if err != nil {
		return nil, err
	}
	apiKey, err := decryptAPIKey(row.SecretPayload, row.SecretMaterialKind)
	if err != nil {
		return nil, err
	}
	return &RuntimeCredential{
		ID:                 row.ID,
		AccountID:          row.AccountID,
		APIKey:             apiKey,
		SecretMaterialKind: row.SecretMaterialKind,
		KeyVersion:         row.KeyVersion,
		Fingerprint:        row.Fingerprint,
		Status:             row.Status,
	}, nil
}

func (r *AccountRepository) List(ctx context.Context, keyword, apiType string, page, pageSize int) ([]*ApiAccount, int64, error) {
	keyword = strings.TrimSpace(keyword)
	apiType = strings.TrimSpace(apiType)
	keywordLike := "%" + keyword + "%"
	hasKeyword := keyword != ""
	hasAPIType := apiType != ""
	params := dbsqlc.CountAccountsParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasApiType:  hasAPIType,
		ApiType:     apiType,
	}
	total, err := r.q.CountAccounts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListAccounts(ctx, dbsqlc.ListAccountsParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasApiType:  hasAPIType,
		ApiType:     apiType,
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
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		writeAccount := accountForWrite(account)
		updated, err := q.UpdateAccount(ctx, dbsqlc.UpdateAccountParams{
			Name:          writeAccount.Name,
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
	if database.IsDuplicateConstraintError(err) {
		return ErrAccountNameConflict
	}
	return err
}

func (r *AccountRepository) UpdateFields(ctx context.Context, account *ApiAccount) error {
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		writeAccount := accountForWrite(account)
		updated, err := q.UpdateAccount(ctx, dbsqlc.UpdateAccountParams{
			Name:          writeAccount.Name,
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
		if strings.TrimSpace(writeAccount.ApiKey) == "" {
			return nil
		}
		return rotateAccountSecret(ctx, q, writeAccount.ID, writeAccount.ApiKey)
	})
	if database.IsDuplicateConstraintError(err) {
		return ErrAccountNameConflict
	}
	return err
}

func (r *AccountRepository) UpsertModel(ctx context.Context, accountID int64, model AccountModel) error {
	params, err := accountModelUpsertParams(accountID, model, 0)
	if err != nil {
		return err
	}
	_, err = r.q.UpsertAccountModel(ctx, params)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ErrNotFound
	}
	return err
}

func (r *AccountRepository) UpdateModelEnabled(ctx context.Context, accountID int64, modelName string, enabled bool) error {
	updated, err := r.q.UpdateAccountModelEnabled(ctx, dbsqlc.UpdateAccountModelEnabledParams{
		AccountID: accountID,
		ModelName: modelName,
		Enabled:   enabled,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *AccountRepository) DeleteModel(ctx context.Context, accountID int64, modelName string) error {
	updated, err := r.q.DeleteAccountModel(ctx, dbsqlc.DeleteAccountModelParams{
		AccountID: accountID,
		ModelName: modelName,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
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

func (r *AccountRepository) ArchiveAccountAndFinalizeRunningTasks(ctx context.Context, id int64, reason string) ([]string, error) {
	var failedTaskIDs []string
	err := dbutil.WithTxDBTX(ctx, r.db, func(q *dbsqlc.Queries, tx *sql.Tx) error {
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
		if r.runningTaskFinalizer == nil {
			return errRunningTaskFinalizerNotConfigured
		}
		failedTaskIDs, err = r.runningTaskFinalizer.FailRunningByAccountIDWithTx(ctx, tx, id, reason, time.Now())
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

func (r *AccountRepository) StopAccountAndFinalizeRunningTasks(ctx context.Context, id int64, reason string) ([]string, error) {
	var failedTaskIDs []string
	err := dbutil.WithTxDBTX(ctx, r.db, func(q *dbsqlc.Queries, tx *sql.Tx) error {
		updated, err := q.UpdateAccountStatus(ctx, dbsqlc.UpdateAccountStatusParams{Status: 0, ID: id})
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		if r.runningTaskFinalizer == nil {
			return errRunningTaskFinalizerNotConfigured
		}
		failedTaskIDs, err = r.runningTaskFinalizer.FailRunningByAccountIDWithTx(ctx, tx, id, reason, time.Now())
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

func accountFromSQLC(row dbsqlc.ApiAccount, apiType, apiKey string, secretConfigured bool, proxyURL string) ApiAccount {
	return ApiAccount{
		ID:                 row.ID,
		Name:               row.Name,
		ProviderEndpointID: row.ProviderEndpointID,
		ApiType:            apiType,
		ApiKey:             apiKey,
		SecretConfigured:   secretConfigured,
		ProxyURL:           proxyURL,
		MaxConcurrent:      int(row.MaxConcurrent),
		Timeout:            int(row.Timeout),
		ExtraConfig:        row.ExtraConfig,
		Status:             row.Status,
		FailCount:          int(row.FailCount),
		Weight:             int(row.Weight),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func accountFromListRow(row dbsqlc.ListAccountsRow) ApiAccount {
	return accountFromSQLC(row.ApiAccount, row.ApiType, row.ApiKey, row.SecretConfigured, row.ProxyUrl)
}

func accountFromEnabledRow(row dbsqlc.ListEnabledAccountsRow) ApiAccount {
	return accountFromSQLC(row.ApiAccount, row.ApiType, row.ApiKey, row.SecretConfigured, row.ProxyUrl)
}

func accountsFromListRows(rows []dbsqlc.ListAccountsRow) []*ApiAccount {
	return accountsFromRows(rows, accountFromListRow)
}

func accountsFromEnabledRows(rows []dbsqlc.ListEnabledAccountsRow) []*ApiAccount {
	return accountsFromRows(rows, accountFromEnabledRow)
}

func accountsFromRows[T any](rows []T, convert func(T) ApiAccount) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(rows))
	for _, row := range rows {
		account := convert(row)
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
	normalized.Timeout = accountlimits.NormalizeTimeoutSeconds(normalized.Timeout)
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
		params, err := accountModelUpsertParams(accountID, model, i)
		if err != nil {
			return err
		}
		if params.ModelName == "" {
			continue
		}
		if _, err := q.UpsertAccountModel(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

func accountModelUpsertParams(accountID int64, model AccountModel, fallbackSortOrder int) (dbsqlc.UpsertAccountModelParams, error) {
	name := strings.TrimSpace(model.Name)
	if name == "" {
		return dbsqlc.UpsertAccountModelParams{}, nil
	}
	weight := model.Weight
	if weight <= 0 {
		weight = 1
	}
	sortOrder := model.SortOrder
	if sortOrder < 0 {
		sortOrder = fallbackSortOrder
	}
	featureSlugs, err := accountModelFeatureSlugsJSON(model.FeatureSlugs)
	if err != nil {
		return dbsqlc.UpsertAccountModelParams{}, err
	}
	return dbsqlc.UpsertAccountModelParams{
		AccountID:     accountID,
		ModelName:     name,
		Enabled:       model.Enabled,
		Weight:        int32(weight),
		MaxConcurrent: nullInt32Ptr(model.MaxConcurrent),
		FeatureSlugs:  featureSlugs,
		Metadata:      json.RawMessage("{}"),
		SortOrder:     int32(sortOrder),
	}, nil
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
	payload, err := secrets.EncryptString(apiKey)
	if err != nil {
		return fmt.Errorf("加密 API Key 失败: %w", err)
	}
	if err := q.RetireActiveAccountSecrets(ctx, accountID); err != nil {
		return err
	}

	_, err = q.CreateAccountSecret(ctx, dbsqlc.CreateAccountSecretParams{
		AccountID:          accountID,
		SecretMaterialKind: secrets.MaterialKindEncrypted,
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
