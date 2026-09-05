package tests

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/providerendpoints/infrastructure/postgres"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

func TestProviderEndpointConflictErrorMapsUniqueConstraintNames(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "pgx duplicate name",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: postgres.ProviderEndpointNameUniqueIndex},
			want: ErrNameConflict,
		},
		{
			name: "pgx duplicate api type and base url",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: postgres.ProviderEndpointAPITypeBaseURLUniqIdx},
			want: ErrURLConflict,
		},
		{
			name: "pq duplicate name",
			err:  &pq.Error{Code: "23505", Constraint: postgres.ProviderEndpointNameUniqueIndex},
			want: ErrNameConflict,
		},
		{
			name: "pq duplicate api type and base url",
			err:  &pq.Error{Code: "23505", Constraint: postgres.ProviderEndpointAPITypeBaseURLUniqIdx},
			want: ErrURLConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := postgres.ProviderEndpointConflictError(tt.err); !errors.Is(err, tt.want) {
				t.Fatalf("mapped error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreateProviderEndpointReturnsNameConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Duplicate Endpoint",
		BaseURL: "https://example.com/provider-endpoint-name-a",
		Status:  1,
	})

	err := repo.Create(ctx, &Endpoint{
		APIType:  "openai-compatible",
		Protocol: ProtocolModelAPI,
		Name:     "duplicate endpoint",
		BaseURL:  "https://example.com/provider-endpoint-name-b",
		Status:   1,
	})
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("duplicate provider endpoint name error = %v, want ErrNameConflict", err)
	}
}

func TestCreateImageEndpointStoresManagedAPIKeyInSameTransaction(t *testing.T) {
	for _, test := range []struct {
		name       string
		protocol   string
		capability string
	}{
		{name: "single image transform", protocol: ProtocolImagePipeline, capability: CapabilityCutout},
		{name: "image fill", protocol: ProtocolImageFill, capability: CapabilityFill},
	} {
		t.Run(test.name, func(t *testing.T) {
			secretstest.EnableEncryption(t)
			ctx := context.Background()
			db := newProviderEndpointRepositoryTestDB(t)
			repo := NewRepository(db)
			endpoint := &Endpoint{
				APIType:      "openai-compatible",
				Protocol:     test.protocol,
				Capabilities: []string{test.capability},
				Name:         "Modal " + test.name,
				BaseURL:      "https://example.com/modal-" + strings.ReplaceAll(test.name, " ", "-"),
				APIKey:       "modal-secret",
				Status:       1,
			}

			if err := repo.Create(ctx, endpoint); err != nil {
				t.Fatalf("create image endpoint failed: %v", err)
			}
			if !endpoint.APIKeyConfigured {
				t.Fatal("created image endpoint should report configured API key")
			}
			var capabilityCount int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_endpoint_capabilities WHERE provider_endpoint_id = $1`, endpoint.ID).Scan(&capabilityCount); err != nil {
				t.Fatalf("count derived capabilities: %v", err)
			}
			if capabilityCount != 1 {
				t.Fatalf("derived capability count = %d, want 1", capabilityCount)
			}
			var accountID int64
			if err := db.QueryRowContext(ctx, `SELECT id FROM api_accounts WHERE provider_endpoint_id = $1`, endpoint.ID).Scan(&accountID); err != nil {
				t.Fatalf("load managed account: %v", err)
			}
			var materialKind, payload string
			if err := db.QueryRowContext(ctx, `SELECT secret_material_kind, secret_payload FROM api_account_secrets WHERE account_id = $1 AND status = 'active'`, accountID).Scan(&materialKind, &payload); err != nil {
				t.Fatalf("load managed secret: %v", err)
			}
			if materialKind != secrets.MaterialKindEncrypted || payload == "modal-secret" {
				t.Fatalf("managed secret storage = (%q, %q)", materialKind, payload)
			}
		})
	}
}

func TestUpdateProviderEndpointStatusPreservesEndpointConfigurationAndSecret(t *testing.T) {
	for _, test := range []struct {
		name       string
		protocol   string
		capability string
		apiKey     string
	}{
		{name: "model endpoint", protocol: ProtocolModelAPI},
		{
			name:       "managed image endpoint",
			protocol:   ProtocolImagePipeline,
			capability: CapabilityCutout,
			apiKey:     "modal-secret",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.apiKey != "" {
				secretstest.EnableEncryption(t)
			}
			ctx := context.Background()
			db := newProviderEndpointRepositoryTestDB(t)
			repo := NewRepository(db)
			endpoint := seedProviderEndpoint(t, ctx, repo, &Endpoint{
				APIType:      "openai-compatible",
				Protocol:     test.protocol,
				Capabilities: []string{test.capability},
				Name:         "Status " + test.name,
				BaseURL:      "https://example.com/status-" + strings.ReplaceAll(test.name, " ", "-"),
				APIKey:       test.apiKey,
				Status:       1,
			})
			before := *endpoint

			var managedSecret string
			if test.apiKey != "" {
				if err := db.QueryRowContext(ctx, `
SELECT secret_payload
FROM api_account_secrets
WHERE account_id = (SELECT id FROM api_accounts WHERE provider_endpoint_id = $1)
  AND status = 'active'`, endpoint.ID).Scan(&managedSecret); err != nil {
					t.Fatalf("load managed secret before status update: %v", err)
				}
			}

			updated, err := repo.UpdateStatus(ctx, endpoint.ID, 0)
			if err != nil {
				t.Fatalf("disable provider endpoint: %v", err)
			}
			if updated.Status != 0 {
				t.Fatalf("returned status = %d, want 0", updated.Status)
			}
			stored, err := repo.GetByID(ctx, endpoint.ID)
			if err != nil {
				t.Fatalf("reload provider endpoint: %v", err)
			}
			assertEndpointConfigUnchanged(t, &before, stored)
			if test.apiKey != "" {
				assertManagedAccountSecretUnchanged(t, ctx, db, endpoint.ID, managedSecret)
			}
		})
	}
}

func assertEndpointConfigUnchanged(t *testing.T, before, stored *Endpoint) {
	t.Helper()
	if stored.APIType != before.APIType || stored.Protocol != before.Protocol || stored.Name != before.Name || stored.BaseURL != before.BaseURL {
		t.Fatalf("status update changed endpoint configuration: before=%+v after=%+v", before, stored)
	}
	if strings.Join(stored.Capabilities, ",") != strings.Join(before.Capabilities, ",") {
		t.Fatalf("status update changed endpoint capabilities: before=%+v after=%+v", before, stored)
	}
}

func assertManagedAccountSecretUnchanged(t *testing.T, ctx context.Context, db *sql.DB, endpointID int64, wantSecret string) {
	t.Helper()
	var accountStatus int16
	var currentSecret string
	if err := db.QueryRowContext(ctx, `
SELECT account.status, secret.secret_payload
FROM api_accounts AS account
JOIN api_account_secrets AS secret ON secret.account_id = account.id AND secret.status = 'active'
WHERE account.provider_endpoint_id = $1`, endpointID).Scan(&accountStatus, &currentSecret); err != nil {
		t.Fatalf("load managed account after status update: %v", err)
	}
	if accountStatus != 0 || currentSecret != wantSecret {
		t.Fatalf("managed account status/secret = (%d, %q), want (0, unchanged)", accountStatus, currentSecret)
	}
}

func TestCreateProviderEndpointReturnsURLConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "URL A",
		BaseURL: "https://example.com/provider-endpoint-url",
		Status:  1,
	})

	err := repo.Create(ctx, &Endpoint{
		APIType:  "openai-compatible",
		Protocol: ProtocolModelAPI,
		Name:     "URL B",
		BaseURL:  "https://example.com/provider-endpoint-url",
		Status:   1,
	})
	if !errors.Is(err, ErrURLConflict) {
		t.Fatalf("duplicate provider endpoint url error = %v, want ErrURLConflict", err)
	}
}

func TestDeleteProviderEndpointCascadesBoundAccounts(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	endpoint := seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Endpoint with API key",
		BaseURL: "https://example.com/provider-endpoint-delete",
		Status:  1,
	})

	var accountID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO api_accounts (name, provider_endpoint_id, extra_config)
VALUES ('bound-api-key', $1, '{"managed_by":"image-processing-endpoint"}')
RETURNING id`, endpoint.ID).Scan(&accountID); err != nil {
		t.Fatalf("seed bound APIKey: %v", err)
	}

	if err := repo.Delete(ctx, endpoint.ID); err != nil {
		t.Fatalf("delete ProviderEndpoint with bound APIKey: %v", err)
	}

	var accountCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_accounts WHERE id = $1`, accountID).Scan(&accountCount); err != nil {
		t.Fatalf("count cascaded APIKey: %v", err)
	}
	if accountCount != 0 {
		t.Fatalf("cascaded APIKey rows = %d, want 0", accountCount)
	}
}

func TestUpdateProviderEndpointReturnsNameConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	first := seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Primary Endpoint",
		BaseURL: "https://example.com/provider-endpoint-update-name-a",
		Status:  1,
	})
	second := seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Secondary Endpoint",
		BaseURL: "https://example.com/provider-endpoint-update-name-b",
		Status:  1,
	})

	second.Name = first.Name
	if err := repo.Update(ctx, second); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("duplicate provider endpoint update name error = %v, want ErrNameConflict", err)
	}
}

func TestUpdateProviderEndpointReturnsURLConflict(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	first := seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Primary URL",
		BaseURL: "https://example.com/provider-endpoint-update-url-a",
		Status:  1,
	})
	second := seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Secondary URL",
		BaseURL: "https://example.com/provider-endpoint-update-url-b",
		Status:  1,
	})

	second.BaseURL = first.BaseURL
	if err := repo.Update(ctx, second); !errors.Is(err, ErrURLConflict) {
		t.Fatalf("duplicate provider endpoint update url error = %v, want ErrURLConflict", err)
	}
}

func TestServiceGetByID(t *testing.T) {
	ctx := context.Background()
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)
	endpoint := seedProviderEndpoint(t, ctx, repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Service GetByID Endpoint",
		BaseURL: "https://example.com/service-getbyid",
		Status:  1,
	})

	got, err := svc.GetByID(ctx, endpoint.ID)
	if err != nil {
		t.Fatalf("svc.GetByID failed: %v", err)
	}
	if got.ID != endpoint.ID || got.Name != endpoint.Name {
		t.Fatalf("svc.GetByID returned %+v, want %+v", got, endpoint)
	}
}
