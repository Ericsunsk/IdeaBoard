package accounts

import (
	"context"
	"database/sql"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

type AccountRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db, q: dbsqlc.New(db)}
}

func (r *AccountRepository) Create(ctx context.Context, account *ApiAccount) error {
	id, err := r.q.CreateAccount(ctx, dbsqlc.CreateAccountParams{
		Name:          account.Name,
		ChannelID:     account.ChannelID,
		Platform:      account.Platform,
		ApiType:       account.ApiType,
		ModelSet:      account.ModelSet,
		ApiKey:        account.ApiKey,
		ProxyUrl:      account.ProxyURL,
		MaxConcurrent: int32(account.MaxConcurrent),
		Timeout:       int32(account.Timeout),
		ExtraConfig:   account.ExtraConfig,
		Status:        account.Status,
		FailCount:     int32(account.FailCount),
		Weight:        int32(account.Weight),
	})
	if err != nil {
		return err
	}
	account.ID = int64(id)
	return nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id int64) (*ApiAccount, error) {
	row, err := r.q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row)
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
	return accountsFromSQLC(rows), total, nil
}

func (r *AccountRepository) GetAllEnabled(ctx context.Context) ([]*ApiAccount, error) {
	rows, err := r.q.ListEnabledAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return accountsFromSQLC(rows), nil
}

func (r *AccountRepository) ListAccountIDs(ctx context.Context) ([]int64, error) {
	return r.q.ListAccountIDs(ctx)
}

func (r *AccountRepository) Update(ctx context.Context, account *ApiAccount) error {
	return r.q.UpdateAccount(ctx, dbsqlc.UpdateAccountParams{
		Name:          account.Name,
		ChannelID:     account.ChannelID,
		Platform:      account.Platform,
		ApiType:       account.ApiType,
		ModelSet:      account.ModelSet,
		ApiKey:        account.ApiKey,
		ProxyUrl:      account.ProxyURL,
		MaxConcurrent: int32(account.MaxConcurrent),
		Timeout:       int32(account.Timeout),
		ExtraConfig:   account.ExtraConfig,
		Status:        account.Status,
		FailCount:     int32(account.FailCount),
		Weight:        int32(account.Weight),
		ID:            account.ID,
	})
}

func (r *AccountRepository) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteAccount(ctx, id)
}

func (r *AccountRepository) UpdateStatus(ctx context.Context, id int64, status int16) error {
	return r.q.UpdateAccountStatus(ctx, dbsqlc.UpdateAccountStatusParams{Status: status, ID: id})
}

func (r *AccountRepository) IncrementFailCount(ctx context.Context, id int64) error {
	return r.q.IncrementAccountFailCount(ctx, id)
}

func (r *AccountRepository) ResetFailCount(ctx context.Context, id int64) error {
	return r.q.ResetAccountFailCount(ctx, id)
}

func accountFromSQLC(row dbsqlc.ApiAccount) ApiAccount {
	return ApiAccount{
		ID:            row.ID,
		Name:          row.Name,
		ChannelID:     row.ChannelID,
		Platform:      row.Platform,
		ApiType:       row.ApiType,
		ModelSet:      row.ModelSet,
		ApiKey:        row.ApiKey,
		ProxyURL:      row.ProxyUrl,
		MaxConcurrent: int(row.MaxConcurrent),
		Timeout:       int(row.Timeout),
		ExtraConfig:   row.ExtraConfig,
		Status:        row.Status,
		FailCount:     int(row.FailCount),
		Weight:        int(row.Weight),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func accountsFromSQLC(rows []dbsqlc.ApiAccount) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(rows))
	for _, row := range rows {
		account := accountFromSQLC(row)
		result = append(result, &account)
	}
	return result
}
