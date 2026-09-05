package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/accountpool/accounts/ports"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
)

type Repository struct {
	db                   *sql.DB
	q                    *dbsqlc.Queries
	runningTaskFinalizer ports.RunningTaskFinalizer
}

var _ ports.AccountRepository = (*Repository)(nil)

func (r *Repository) ProviderEndpointConfig(ctx context.Context, endpointID int64) (string, []string, error) {
	row, err := r.q.GetProviderEndpointByID(ctx, endpointID)
	if err != nil {
		return "", nil, err
	}
	rows, err := r.q.ListProviderEndpointCapabilities(ctx, []int64{endpointID})
	if err != nil {
		return "", nil, err
	}
	capabilities := make([]string, 0, len(rows))
	for _, capability := range rows {
		capabilities = append(capabilities, capability.Capability)
	}
	return row.Protocol, capabilities, nil
}

func NewRepository(db *sql.DB, finalizer ...ports.RunningTaskFinalizer) *Repository {
	var taskFinalizer ports.RunningTaskFinalizer
	if len(finalizer) > 0 {
		taskFinalizer = finalizer[0]
	}
	return &Repository{db: db, q: dbsqlc.New(db), runningTaskFinalizer: taskFinalizer}
}

func (r *Repository) Create(ctx context.Context, account *domain.ApiAccount) error {
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
		if err := replaceAccountModels(ctx, q, writeAccount.ID, writeAccount.ProviderEndpointID, accountModelsForWrite(writeAccount)); err != nil {
			return err
		}
		return rotateAccountSecret(ctx, q, writeAccount.ID, writeAccount.ApiKey)
	})
	if database.IsDuplicateConstraintError(err) {
		return domain.ErrAccountNameConflict
	}
	return err
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*domain.ApiAccount, error) {
	row, err := r.q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row.ApiAccount, row.ApiType, row.Protocol, row.Capabilities, row.ApiKey, row.SecretConfigured, row.ProxyUrl)
	if err := r.attachAccountModels(ctx, []*domain.ApiAccount{&account}); err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *Repository) GetRuntimeByIDWithSecret(ctx context.Context, id int64) (*domain.ApiAccount, error) {
	row, err := r.q.GetRuntimeAccountByIDWithSecret(ctx, id)
	if err != nil {
		return nil, err
	}
	apiKey, err := decryptAPIKey(row.ApiKey, row.ApiKeyMaterialKind)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row.ApiAccount, row.ApiType, row.Protocol, row.Capabilities, apiKey, row.SecretConfigured, row.ProxyUrl)
	if err := r.attachAccountModels(ctx, []*domain.ApiAccount{&account}); err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *Repository) GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*domain.RuntimeAccountAccess, error) {
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
	return &domain.RuntimeAccountAccess{
		ID:      int64(row.ApiAccount.ID),
		Name:    row.ApiAccount.Name,
		APIKey:  apiKey,
		Timeout: int(row.ApiAccount.Timeout),
	}, nil
}

func (r *Repository) GetRuntimeCredentialByID(ctx context.Context, accountID int64, credentialID int64) (*domain.RuntimeCredential, error) {
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
	return &domain.RuntimeCredential{
		ID:                 row.ID,
		AccountID:          row.AccountID,
		APIKey:             apiKey,
		SecretMaterialKind: row.SecretMaterialKind,
		KeyVersion:         row.KeyVersion,
		Fingerprint:        row.Fingerprint,
		Status:             row.Status,
	}, nil
}

func (r *Repository) List(ctx context.Context, keyword, apiType string, page, pageSize int) ([]*domain.ApiAccount, int64, error) {
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

func (r *Repository) GetAllEnabled(ctx context.Context) ([]*domain.ApiAccount, error) {
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

func (r *Repository) ListAccountIDs(ctx context.Context) ([]int64, error) {
	return r.q.ListAccountIDs(ctx)
}

func (r *Repository) Update(ctx context.Context, account *domain.ApiAccount) error {
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
		if err := replaceAccountModels(ctx, q, writeAccount.ID, writeAccount.ProviderEndpointID, accountModelsForWrite(writeAccount)); err != nil {
			return err
		}
		if strings.TrimSpace(writeAccount.ApiKey) == "" {
			return nil
		}
		return rotateAccountSecret(ctx, q, writeAccount.ID, writeAccount.ApiKey)
	})
	if database.IsDuplicateConstraintError(err) {
		return domain.ErrAccountNameConflict
	}
	return err
}

func (r *Repository) UpdateFields(ctx context.Context, account *domain.ApiAccount) error {
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
		return domain.ErrAccountNameConflict
	}
	return err
}

func (r *Repository) UpsertModel(ctx context.Context, accountID int64, model domain.AccountModel) error {
	params, err := accountModelUpsertParams(accountID, model, 0)
	if err != nil {
		return err
	}
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		account, err := q.GetAccountByID(ctx, accountID)
		if errors.Is(err, sql.ErrNoRows) {
			return database.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := q.UpsertProviderEndpointModel(ctx, dbsqlc.UpsertProviderEndpointModelParams{
			ProviderEndpointID: account.ApiAccount.ProviderEndpointID,
			ModelName:          params.ModelName,
			Supports2k:         model.Supports2K,
			Supports4k:         model.Supports4K,
		}); err != nil {
			return err
		}
		_, err = q.UpsertAccountModel(ctx, params)
		if errors.Is(err, sql.ErrNoRows) {
			return database.ErrNotFound
		}
		return err
	})
}

func (r *Repository) UpdateModelEnabled(ctx context.Context, accountID int64, modelName string, enabled bool) error {
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

func (r *Repository) DeleteModel(ctx context.Context, accountID int64, modelName string) error {
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

func (r *Repository) Delete(ctx context.Context, id int64) error {
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

func (r *Repository) ArchiveAndFinalizeTasks(ctx context.Context, id int64, reason string) ([]string, error) {
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
			return ports.ErrFinalizerNotConfigured
		}
		failedTaskIDs, err = r.runningTaskFinalizer.FailRunningByAccountIDWithTx(ctx, tx, id, reason, time.Now())
		return err
	})
	return failedTaskIDs, err
}

func (r *Repository) UpdateStatus(ctx context.Context, id int64, status int16) error {
	updated, err := r.q.UpdateAccountStatus(ctx, dbsqlc.UpdateAccountStatusParams{Status: status, ID: id})
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *Repository) StopAndFinalizeTasks(ctx context.Context, id int64, reason string) ([]string, error) {
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
			return ports.ErrFinalizerNotConfigured
		}
		failedTaskIDs, err = r.runningTaskFinalizer.FailRunningByAccountIDWithTx(ctx, tx, id, reason, time.Now())
		return err
	})
	return failedTaskIDs, err
}

func (r *Repository) IncrementFailCount(ctx context.Context, id int64) error {
	updated, err := r.q.IncrementAccountFailCount(ctx, id)
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *Repository) ResetFailCount(ctx context.Context, id int64) error {
	updated, err := r.q.ResetAccountFailCount(ctx, id)
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func accountFromSQLC(row dbsqlc.ApiAccount, apiType, protocol, capabilityCSV, apiKey string, secretConfigured bool, proxyURL string) domain.ApiAccount {
	return domain.ApiAccount{
		ID:                 row.ID,
		Name:               row.Name,
		ProviderEndpointID: row.ProviderEndpointID,
		ApiType:            apiType,
		Protocol:           protocol,
		Capabilities:       splitCapabilities(capabilityCSV),
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

func accountFromListRow(row dbsqlc.ListAccountsRow) domain.ApiAccount {
	return accountFromSQLC(row.ApiAccount, row.ApiType, row.Protocol, row.Capabilities, row.ApiKey, row.SecretConfigured, row.ProxyUrl)
}

func accountFromEnabledRow(row dbsqlc.ListEnabledAccountsRow) domain.ApiAccount {
	return accountFromSQLC(row.ApiAccount, row.ApiType, row.Protocol, row.Capabilities, row.ApiKey, row.SecretConfigured, row.ProxyUrl)
}

func splitCapabilities(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	capabilities := make([]string, 0, len(parts))
	for _, part := range parts {
		if capability := strings.TrimSpace(part); capability != "" {
			capabilities = append(capabilities, capability)
		}
	}
	return capabilities
}

func accountsFromListRows(rows []dbsqlc.ListAccountsRow) []*domain.ApiAccount {
	return accountsFromRows(rows, accountFromListRow)
}

func accountsFromEnabledRows(rows []dbsqlc.ListEnabledAccountsRow) []*domain.ApiAccount {
	return accountsFromRows(rows, accountFromEnabledRow)
}

func accountsFromRows[T any](rows []T, convert func(T) domain.ApiAccount) []*domain.ApiAccount {
	result := make([]*domain.ApiAccount, 0, len(rows))
	for _, row := range rows {
		account := convert(row)
		result = append(result, &account)
	}
	return result
}

func (r *Repository) attachAccountModels(ctx context.Context, accounts []*domain.ApiAccount) error {
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
	modelsByAccountID := make(map[int64][]domain.AccountModel, len(accountIDs))
	for _, accountID := range accountIDs {
		modelsByAccountID[accountID] = make([]domain.AccountModel, 0)
	}
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

func accountModelFromSQLC(row dbsqlc.ListAccountModelsByAccountIDsRow) (domain.AccountModel, error) {
	return domain.AccountModel{
		ID:            row.ID,
		Name:          row.ModelName,
		Enabled:       row.Enabled,
		Weight:        int(row.Weight),
		MaxConcurrent: intPtr(row.MaxConcurrent),
		SortOrder:     int(row.SortOrder),
		Supports2K:    row.Supports2k,
		Supports4K:    row.Supports4k,
	}, nil
}

func accountModelsForWrite(account *domain.ApiAccount) []domain.AccountModel {
	if account == nil {
		return nil
	}
	return account.Models
}

func accountForWrite(account *domain.ApiAccount) *domain.ApiAccount {
	if account == nil {
		return &domain.ApiAccount{}
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

func replaceAccountModels(ctx context.Context, q *dbsqlc.Queries, accountID, providerEndpointID int64, models []domain.AccountModel) error {
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
		if err := q.UpsertProviderEndpointModel(ctx, dbsqlc.UpsertProviderEndpointModelParams{
			ProviderEndpointID: providerEndpointID,
			ModelName:          params.ModelName,
			Supports2k:         model.Supports2K,
			Supports4k:         model.Supports4K,
		}); err != nil {
			return err
		}
		if _, err := q.UpsertAccountModel(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

func accountModelUpsertParams(accountID int64, model domain.AccountModel, fallbackSortOrder int) (dbsqlc.UpsertAccountModelParams, error) {
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
	return dbsqlc.UpsertAccountModelParams{
		AccountID:     accountID,
		ModelName:     name,
		Enabled:       model.Enabled,
		Weight:        int32(weight),
		MaxConcurrent: nullInt32Ptr(model.MaxConcurrent),
		Metadata:      json.RawMessage("{}"),
		SortOrder:     int32(sortOrder),
	}, nil
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
