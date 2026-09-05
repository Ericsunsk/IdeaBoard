package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/accountpool/providerendpoints/domain"
	"ideaboard/api/internal/accountpool/providerendpoints/ports"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

const (
	ProviderEndpointNameUniqueIndex       = "uq_provider_endpoints_name_lower"
	ProviderEndpointAPITypeBaseURLUniqIdx = "uq_provider_endpoints_api_type_base_url"
	managedImageEndpointExtraConfig       = `{"managed_by":"image-processing-endpoint"}`
)

var _ ports.Repository = (*Repository)(nil)

type Repository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) Create(ctx context.Context, endpoint *domain.Endpoint) error {
	protocol, capabilities, err := domain.NormalizeProtocolCapabilities(endpoint.Protocol, endpoint.Capabilities)
	if err != nil {
		return err
	}
	if err := domain.ValidateProtocolAPIType(protocol, endpoint.APIType); err != nil {
		return err
	}
	if err := validateManagedEndpointAPIKey(protocol, endpoint.APIKey); err != nil {
		return err
	}
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
	err = dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		id, err := q.CreateProviderEndpoint(ctx, dbsqlc.CreateProviderEndpointParams{
			ApiType:  endpoint.APIType,
			Protocol: protocol,
			Name:     endpoint.Name,
			BaseUrl:  endpoint.BaseURL,
			Status:   endpoint.Status,
		})
		if err != nil {
			return err
		}
		endpoint.ID = int64(id)
		if err := replaceProviderEndpointCapabilities(ctx, q, endpoint.ID, capabilities); err != nil {
			return err
		}
		return syncManagedEndpointAccount(ctx, q, endpoint)
	})
	if err != nil {
		return ProviderEndpointConflictError(err)
	}
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
	endpoint.APIKeyConfigured = domain.IsManagedImageEndpointProtocol(protocol)
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*domain.Endpoint, error) {
	row, err := r.q.GetProviderEndpointByID(ctx, id)
	if err != nil {
		return nil, err
	}
	endpoint := domain.Endpoint{
		ID:               row.ID,
		APIType:          row.ApiType,
		Protocol:         row.Protocol,
		Name:             row.Name,
		BaseURL:          row.BaseUrl,
		APIKeyConfigured: row.ApiKeyConfigured,
		Status:           row.Status,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
	capabilities, err := r.capabilities(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	endpoint.Capabilities = capabilities[id]
	return &endpoint, nil
}

func (r *Repository) List(ctx context.Context) ([]*domain.Endpoint, error) {
	rows, err := r.q.ListProviderEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	endpoints := make([]*domain.Endpoint, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		endpoint := domain.Endpoint{
			ID:               row.ID,
			APIType:          row.ApiType,
			Protocol:         row.Protocol,
			Name:             row.Name,
			BaseURL:          row.BaseUrl,
			APIKeyConfigured: row.ApiKeyConfigured,
			Status:           row.Status,
			CreatedAt:        row.CreatedAt,
			UpdatedAt:        row.UpdatedAt,
		}
		endpoints = append(endpoints, &endpoint)
		ids = append(ids, row.ID)
	}
	capabilities, err := r.capabilities(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		endpoint.Capabilities = capabilities[endpoint.ID]
	}
	return endpoints, nil
}

func (r *Repository) Update(ctx context.Context, endpoint *domain.Endpoint) error {
	protocol, capabilities, err := domain.NormalizeProtocolCapabilities(endpoint.Protocol, endpoint.Capabilities)
	if err != nil {
		return err
	}
	if err := domain.ValidateProtocolAPIType(protocol, endpoint.APIType); err != nil {
		return err
	}
	if err := validateManagedEndpointAPIKey(protocol, endpoint.APIKey); err != nil {
		return err
	}
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
	err = dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.UpdateProviderEndpoint(ctx, dbsqlc.UpdateProviderEndpointParams{
			ApiType:  endpoint.APIType,
			Protocol: protocol,
			Name:     endpoint.Name,
			BaseUrl:  endpoint.BaseURL,
			Status:   endpoint.Status,
			ID:       endpoint.ID,
		})
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		if err := replaceProviderEndpointCapabilities(ctx, q, endpoint.ID, capabilities); err != nil {
			return err
		}
		return syncManagedEndpointAccount(ctx, q, endpoint)
	})
	if err != nil {
		return ProviderEndpointConflictError(err)
	}
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
	endpoint.APIKeyConfigured = domain.IsManagedImageEndpointProtocol(protocol)
	return nil
}

// UpdateStatus updates the scheduling state without touching endpoint configuration,
// capabilities, or managed account secrets.
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status int16) (*domain.Endpoint, error) {
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.UpdateProviderEndpointStatus(ctx, dbsqlc.UpdateProviderEndpointStatusParams{
			ID:     id,
			Status: status,
		})
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		return q.UpdateManagedProviderEndpointAccountStatus(ctx, dbsqlc.UpdateManagedProviderEndpointAccountStatusParams{
			ProviderEndpointID: id,
			Status:             status,
		})
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *Repository) capabilities(ctx context.Context, ids []int64) (map[int64][]string, error) {
	result := make(map[int64][]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.q.ListProviderEndpointCapabilities(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ProviderEndpointID] = append(result[row.ProviderEndpointID], row.Capability)
	}
	return result, nil
}

func replaceProviderEndpointCapabilities(ctx context.Context, q *dbsqlc.Queries, endpointID int64, capabilities []string) error {
	if err := q.ReplaceProviderEndpointCapabilities(ctx, endpointID); err != nil {
		return err
	}
	for _, capability := range capabilities {
		if err := q.AddProviderEndpointCapability(ctx, dbsqlc.AddProviderEndpointCapabilityParams{
			ProviderEndpointID: endpointID,
			Capability:         capability,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if _, err := q.DeleteManagedProviderEndpointAccounts(ctx, id); err != nil {
			return err
		}
		updated, err := q.DeleteProviderEndpoint(ctx, id)
		if err != nil {
			return err
		}
		if updated == 0 {
			return database.ErrNotFound
		}
		return nil
	})
}

func validateManagedEndpointAPIKey(protocol, apiKey string) error {
	if domain.IsManagedImageEndpointProtocol(protocol) && strings.TrimSpace(apiKey) == "" {
		return domain.ErrAPIKeyRequired
	}
	return nil
}

func syncManagedEndpointAccount(ctx context.Context, q *dbsqlc.Queries, endpoint *domain.Endpoint) error {
	if !domain.IsManagedImageEndpointProtocol(endpoint.Protocol) {
		_, err := q.DeleteManagedProviderEndpointAccounts(ctx, endpoint.ID)
		return err
	}

	accountID, err := q.GetManagedProviderEndpointAccountID(ctx, endpoint.ID)
	if errors.Is(err, sql.ErrNoRows) {
		accountID, err = q.CreateAccount(ctx, dbsqlc.CreateAccountParams{
			Name:               endpoint.Name,
			ProviderEndpointID: endpoint.ID,
			MaxConcurrent:      int32(accountlimits.DefaultMaxConcurrent),
			Timeout:            int32(accountlimits.DefaultTimeoutSeconds),
			ExtraConfig:        managedImageEndpointExtraConfig,
			Status:             endpoint.Status,
			FailCount:          0,
			Weight:             1,
		})
	} else if err == nil {
		err = q.UpdateManagedProviderEndpointAccount(ctx, dbsqlc.UpdateManagedProviderEndpointAccountParams{
			Name:               endpoint.Name,
			Status:             endpoint.Status,
			MaxConcurrent:      int32(accountlimits.DefaultMaxConcurrent),
			Timeout:            int32(accountlimits.DefaultTimeoutSeconds),
			ID:                 accountID,
			ProviderEndpointID: endpoint.ID,
		})
	}
	if err != nil {
		return err
	}

	payload, err := secrets.EncryptString(strings.TrimSpace(endpoint.APIKey))
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

func ProviderEndpointConflictError(err error) error {
	constraintName := providerEndpointUniqueConstraintName(err)
	switch constraintName {
	case ProviderEndpointNameUniqueIndex:
		return fmt.Errorf("%w: %v", domain.ErrNameConflict, err)
	case ProviderEndpointAPITypeBaseURLUniqIdx:
		return fmt.Errorf("%w: %v", domain.ErrURLConflict, err)
	default:
		return err
	}
}

func providerEndpointUniqueConstraintName(err error) string {
	if !database.IsDuplicateConstraintError(err) {
		return ""
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.ToLower(strings.TrimSpace(pgErr.ConstraintName))
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return strings.ToLower(strings.TrimSpace(pqErr.Constraint))
	}
	return ""
}
