package adminbranding

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/validation"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Scope string

const (
	ScopeProviderEndpoint Scope = "provider-endpoint"
	ScopeStorageProfile   Scope = "storage-profile"
	ScopeSearchProvider   Scope = "search-provider"
	ScopeOAuthProvider    Scope = "oauth-provider"
)

var validTargetKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

var allowedLogoKeys = map[string]struct{}{
	"flux": {}, "midjourney": {}, "stability": {}, "fal": {}, "replicate": {}, "ideogram": {}, "recraft": {}, "runway": {},
	"zhipu": {}, "aliyun": {}, "doubao": {}, "moonshot": {}, "minimax": {}, "baidu": {}, "tencent": {}, "lingyi": {},
	"openai": {}, "gemini": {}, "anthropic": {}, "deepseek": {}, "cloudflare": {}, "siliconflow": {}, "mistral": {},
	"groq": {}, "together": {}, "fireworks": {}, "huggingface": {}, "perplexity": {}, "cohere": {}, "novita": {},
	"lepton": {}, "google": {}, "azure": {}, "bedrock": {}, "ollama": {}, "amazons3": {}, "cloudflare-r2": {},
	"alibabacloud": {}, "tencent-cos": {}, "minio": {}, "unsplash": {}, "pexels": {}, "github": {}, "google-oauth": {},
	"discord": {},
}

type Override struct {
	Scope     Scope     `json:"scope"`
	TargetKey string    `json:"target_key"`
	LogoKey   string    `json:"logo_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store interface {
	List(context.Context) ([]Override, error)
	Upsert(context.Context, Scope, string, string) (Override, error)
	Delete(context.Context, Scope, string) (bool, error)
}

type Repository struct{ db *pgxpool.Pool }

type repositoryContextDB struct {
	ctx context.Context
	db  *pgxpool.Pool
}

func (db repositoryContextDB) queries() *dbsqlc.Queries { return dbsqlc.New(db.db) }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) dbWithContext(ctx context.Context) repositoryContextDB {
	return repositoryContextDB{ctx: ctx, db: r.db}
}

func (r *Repository) List(ctx context.Context) ([]Override, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("admin brand logo override repository is unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().ListAdminBrandLogoOverrides(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admin brand logo overrides: %w", err)
	}
	items := make([]Override, 0, len(rows))
	for _, row := range rows {
		items = append(items, Override{Scope: Scope(row.Scope), TargetKey: row.TargetKey, LogoKey: row.LogoKey, CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt)})
	}
	return items, nil
}

func (r *Repository) Upsert(ctx context.Context, scope Scope, targetKey, logoKey string) (Override, error) {
	if r == nil || r.db == nil {
		return Override{}, errors.New("admin brand logo override repository is unavailable")
	}
	if err := Validate(scope, targetKey, logoKey); err != nil {
		return Override{}, err
	}
	row, err := r.dbWithContext(ctx).queries().UpsertAdminBrandLogoOverride(ctx, dbsqlc.UpsertAdminBrandLogoOverrideParams{Scope: string(scope), TargetKey: targetKey, LogoKey: logoKey})
	if err != nil {
		return Override{}, fmt.Errorf("upsert admin brand logo override: %w", err)
	}
	return Override{Scope: Scope(row.Scope), TargetKey: row.TargetKey, LogoKey: row.LogoKey, CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt)}, nil
}

func (r *Repository) Delete(ctx context.Context, scope Scope, targetKey string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("admin brand logo override repository is unavailable")
	}
	if err := ValidateTarget(scope, targetKey); err != nil {
		return false, err
	}
	deleted, err := r.dbWithContext(ctx).queries().DeleteAdminBrandLogoOverride(ctx, dbsqlc.DeleteAdminBrandLogoOverrideParams{Scope: string(scope), TargetKey: targetKey})
	if err != nil {
		return false, fmt.Errorf("delete admin brand logo override: %w", err)
	}
	return deleted > 0, nil
}

func Validate(scope Scope, targetKey, logoKey string) error {
	if err := ValidateTarget(scope, targetKey); err != nil {
		return err
	}
	if _, ok := allowedLogoKeys[strings.TrimSpace(logoKey)]; !ok {
		return validation.New(msgValidate)
	}
	return nil
}

func ValidateTarget(scope Scope, targetKey string) error {
	if !scope.Valid() {
		return validation.New(msgValidateTarget)
	}
	if !validTargetKey.MatchString(strings.TrimSpace(targetKey)) {
		return validation.New(msgValidateTarget2)
	}
	return nil
}

func (s Scope) Valid() bool {
	return s == ScopeProviderEndpoint || s == ScopeStorageProfile || s == ScopeSearchProvider || s == ScopeOAuthProvider
}
