package providerendpoints

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

func newProviderEndpointRepositoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func seedProviderEndpoint(t testing.TB, ctx context.Context, repo *ProviderEndpointRepository, endpoint *ProviderEndpoint) *ProviderEndpoint {
	t.Helper()
	if err := repo.Create(ctx, endpoint); err != nil {
		t.Fatalf("seed provider endpoint failed: %v", err)
	}
	return endpoint
}

func TestProviderEndpointConflictErrorMapsUniqueConstraintNames(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "pgx duplicate name",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: providerEndpointNameUniqueIndex},
			want: ErrProviderEndpointNameConflict,
		},
		{
			name: "pgx duplicate api type and base url",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: providerEndpointAPITypeBaseURLUniqIdx},
			want: ErrProviderEndpointURLConflict,
		},
		{
			name: "pq duplicate name",
			err:  &pq.Error{Code: "23505", Constraint: providerEndpointNameUniqueIndex},
			want: ErrProviderEndpointNameConflict,
		},
		{
			name: "pq duplicate api type and base url",
			err:  &pq.Error{Code: "23505", Constraint: providerEndpointAPITypeBaseURLUniqIdx},
			want: ErrProviderEndpointURLConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := providerEndpointConflictError(tt.err); !errors.Is(err, tt.want) {
				t.Fatalf("mapped error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreateProviderEndpointReturnsNameConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewProviderEndpointRepository(db)
	seedProviderEndpoint(t, ctx, repo, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "Duplicate Endpoint",
		BaseURL: "https://example.com/provider-endpoint-name-a",
		Status:  1,
	})

	err := repo.Create(ctx, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "duplicate endpoint",
		BaseURL: "https://example.com/provider-endpoint-name-b",
		Status:  1,
	})
	if !errors.Is(err, ErrProviderEndpointNameConflict) {
		t.Fatalf("duplicate provider endpoint name error = %v, want ErrProviderEndpointNameConflict", err)
	}
}

func TestCreateProviderEndpointReturnsURLConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewProviderEndpointRepository(db)
	seedProviderEndpoint(t, ctx, repo, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "URL A",
		BaseURL: "https://example.com/provider-endpoint-url",
		Status:  1,
	})

	err := repo.Create(ctx, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "URL B",
		BaseURL: "https://example.com/provider-endpoint-url",
		Status:  1,
	})
	if !errors.Is(err, ErrProviderEndpointURLConflict) {
		t.Fatalf("duplicate provider endpoint url error = %v, want ErrProviderEndpointURLConflict", err)
	}
}

func TestUpdateProviderEndpointReturnsNameConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewProviderEndpointRepository(db)
	first := seedProviderEndpoint(t, ctx, repo, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "Primary Endpoint",
		BaseURL: "https://example.com/provider-endpoint-update-name-a",
		Status:  1,
	})
	second := seedProviderEndpoint(t, ctx, repo, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "Secondary Endpoint",
		BaseURL: "https://example.com/provider-endpoint-update-name-b",
		Status:  1,
	})

	second.Name = first.Name
	if err := repo.Update(ctx, second); !errors.Is(err, ErrProviderEndpointNameConflict) {
		t.Fatalf("duplicate provider endpoint update name error = %v, want ErrProviderEndpointNameConflict", err)
	}
}

func TestUpdateProviderEndpointReturnsURLConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewProviderEndpointRepository(db)
	first := seedProviderEndpoint(t, ctx, repo, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "Primary URL",
		BaseURL: "https://example.com/provider-endpoint-update-url-a",
		Status:  1,
	})
	second := seedProviderEndpoint(t, ctx, repo, &ProviderEndpoint{
		APIType: "openai-compatible",
		Name:    "Secondary URL",
		BaseURL: "https://example.com/provider-endpoint-update-url-b",
		Status:  1,
	})

	second.BaseURL = first.BaseURL
	if err := repo.Update(ctx, second); !errors.Is(err, ErrProviderEndpointURLConflict) {
		t.Fatalf("duplicate provider endpoint update url error = %v, want ErrProviderEndpointURLConflict", err)
	}
}
