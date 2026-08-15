package providerendpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var ErrProviderEndpointNameConflict = errors.New("provider endpoint name conflict")
var ErrProviderEndpointURLConflict = errors.New("provider endpoint api type/base url conflict")

const (
	providerEndpointNameUniqueIndex       = "uq_provider_endpoints_name_lower"
	providerEndpointAPITypeBaseURLUniqIdx = "uq_provider_endpoints_api_type_base_url"
)

type ProviderEndpointRepository struct {
	q *dbsqlc.Queries
}

func NewProviderEndpointRepository(db *sql.DB) *ProviderEndpointRepository {
	return &ProviderEndpointRepository{q: dbsqlc.New(db)}
}

func (r *ProviderEndpointRepository) Create(ctx context.Context, endpoint *ProviderEndpoint) error {
	id, err := r.q.CreateProviderEndpoint(ctx, dbsqlc.CreateProviderEndpointParams{
		ApiType: endpoint.APIType,
		Name:    endpoint.Name,
		BaseUrl: endpoint.BaseURL,
		Status:  endpoint.Status,
	})
	if err != nil {
		return providerEndpointConflictError(err)
	}
	endpoint.ID = int64(id)
	return nil
}

func (r *ProviderEndpointRepository) GetByID(ctx context.Context, id int64) (*ProviderEndpoint, error) {
	row, err := r.q.GetProviderEndpointByID(ctx, id)
	if err != nil {
		return nil, err
	}
	endpoint := ProviderEndpoint{
		ID:        row.ID,
		APIType:   row.ApiType,
		Name:      row.Name,
		BaseURL:   row.BaseUrl,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	return &endpoint, nil
}

func (r *ProviderEndpointRepository) List(ctx context.Context) ([]*ProviderEndpoint, error) {
	rows, err := r.q.ListProviderEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	endpoints := make([]*ProviderEndpoint, 0, len(rows))
	for _, row := range rows {
		endpoint := ProviderEndpoint{
			ID:        row.ID,
			APIType:   row.ApiType,
			Name:      row.Name,
			BaseURL:   row.BaseUrl,
			Status:    row.Status,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}
		endpoints = append(endpoints, &endpoint)
	}
	return endpoints, nil
}

func (r *ProviderEndpointRepository) Update(ctx context.Context, endpoint *ProviderEndpoint) error {
	updated, err := r.q.UpdateProviderEndpoint(ctx, dbsqlc.UpdateProviderEndpointParams{
		ApiType: endpoint.APIType,
		Name:    endpoint.Name,
		BaseUrl: endpoint.BaseURL,
		Status:  endpoint.Status,
		ID:      endpoint.ID,
	})
	if err != nil {
		return providerEndpointConflictError(err)
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *ProviderEndpointRepository) Delete(ctx context.Context, id int64) error {
	updated, err := r.q.DeleteProviderEndpoint(ctx, id)
	if err != nil {
		return err
	}
	if updated == 0 {
		return database.ErrNotFound
	}
	return nil
}

func providerEndpointConflictError(err error) error {
	constraintName := providerEndpointUniqueConstraintName(err)
	switch constraintName {
	case providerEndpointNameUniqueIndex:
		return fmt.Errorf("%w: %v", ErrProviderEndpointNameConflict, err)
	case providerEndpointAPITypeBaseURLUniqIdx:
		return fmt.Errorf("%w: %v", ErrProviderEndpointURLConflict, err)
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
