package ports

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ideaboard/api/internal/accountpool/accounts/domain"
)

var (
	ErrFinalizerNotConfigured            = errors.New("running task finalizer is not configured")
	ErrRunningTaskFinalizerNotConfigured = ErrFinalizerNotConfigured
)

type AccountRepository interface {
	ProviderEndpointConfig(ctx context.Context, endpointID int64) (string, []string, error)
	Create(ctx context.Context, account *domain.ApiAccount) error
	GetByID(ctx context.Context, id int64) (*domain.ApiAccount, error)
	GetRuntimeByIDWithSecret(ctx context.Context, id int64) (*domain.ApiAccount, error)
	GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*domain.RuntimeAccountAccess, error)
	GetRuntimeCredentialByID(ctx context.Context, accountID int64, credentialID int64) (*domain.RuntimeCredential, error)
	List(ctx context.Context, keyword, apiType string, page, pageSize int) ([]*domain.ApiAccount, int64, error)
	GetAllEnabled(ctx context.Context) ([]*domain.ApiAccount, error)
	ListAccountIDs(ctx context.Context) ([]int64, error)
	Update(ctx context.Context, account *domain.ApiAccount) error
	UpdateFields(ctx context.Context, account *domain.ApiAccount) error
	UpsertModel(ctx context.Context, accountID int64, model domain.AccountModel) error
	UpdateModelEnabled(ctx context.Context, accountID int64, modelName string, enabled bool) error
	DeleteModel(ctx context.Context, accountID int64, modelName string) error
	Delete(ctx context.Context, id int64) error
	ArchiveAndFinalizeTasks(ctx context.Context, id int64, reason string) ([]string, error)
	UpdateStatus(ctx context.Context, id int64, status int16) error
	StopAndFinalizeTasks(ctx context.Context, id int64, reason string) ([]string, error)
	IncrementFailCount(ctx context.Context, id int64) error
	ResetFailCount(ctx context.Context, id int64) error
}

type RunningTaskFinalizer interface {
	FailRunningByAccountIDWithTx(ctx context.Context, tx *sql.Tx, accountID int64, errMsg string, completedAt time.Time) ([]string, error)
}

type AccountTaskStore interface {
	ListTaskIDsByAccountIDAndStatuses(ctx context.Context, accountID int64, statuses ...string) ([]string, error)
}
