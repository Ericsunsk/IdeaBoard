package providerendpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
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
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewProviderEndpointRepository(db *sql.DB) *ProviderEndpointRepository {
	return &ProviderEndpointRepository{db: db, q: dbsqlc.New(db)}
}

func (r *ProviderEndpointRepository) Create(ctx context.Context, endpoint *ProviderEndpoint) error {
	protocol, capabilities, err := NormalizeProtocolCapabilities(endpoint.Protocol, endpoint.Capabilities)
	if err != nil {
		return err
	}
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
		return replaceProviderEndpointCapabilities(ctx, q, endpoint.ID, capabilities)
	})
	if err != nil {
		return providerEndpointConflictError(err)
	}
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
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
		Protocol:  row.Protocol,
		Name:      row.Name,
		BaseURL:   row.BaseUrl,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	capabilities, err := r.capabilities(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	endpoint.Capabilities = capabilities[id]
	return &endpoint, nil
}

func (r *ProviderEndpointRepository) List(ctx context.Context) ([]*ProviderEndpoint, error) {
	rows, err := r.q.ListProviderEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	endpoints := make([]*ProviderEndpoint, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		endpoint := ProviderEndpoint{
			ID:        row.ID,
			APIType:   row.ApiType,
			Protocol:  row.Protocol,
			Name:      row.Name,
			BaseURL:   row.BaseUrl,
			Status:    row.Status,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
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

func (r *ProviderEndpointRepository) Update(ctx context.Context, endpoint *ProviderEndpoint) error {
	protocol, capabilities, err := NormalizeProtocolCapabilities(endpoint.Protocol, endpoint.Capabilities)
	if err != nil {
		return err
	}
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
		return replaceProviderEndpointCapabilities(ctx, q, endpoint.ID, capabilities)
	})
	if err != nil {
		return providerEndpointConflictError(err)
	}
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
	return nil
}

func (r *ProviderEndpointRepository) capabilities(ctx context.Context, ids []int64) (map[int64][]string, error) {
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
