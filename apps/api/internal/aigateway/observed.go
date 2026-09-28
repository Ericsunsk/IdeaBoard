package aigateway

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ObservedWindowDays is the fixed trailing window for first-party model
// observations. Latency percentiles stay out of scope: no latency column
// exists on task_execution_attempts, adding one needs its own migration.
const ObservedWindowDays = 7

// Cost source and margin status markers for observed SKU margin rows.
// Gateway receipts carry log/event IDs only (no cost fields) and no gateway
// log fetcher exists, so actual cost is always unavailable today. Rows carry
// the explicit marker instead of a fabricated number.
const (
	// ObservedCostSourceUnavailable marks margin rows whose gateway actual
	// cost has no receipt/log source.
	ObservedCostSourceUnavailable = "unavailable"
	// ObservedMarginUnknownNoCost marks margins that cannot be computed
	// because the gateway actual cost is unavailable.
	ObservedMarginUnknownNoCost = "unknown_no_cost"
)

// ModelAttemptAggregate is one (model, provider) terminal-attempt total over
// the observation window. Only completed/failed attempts count: running,
// requeued and cancelled attempts carry no outcome signal.
type ModelAttemptAggregate struct {
	Model     string
	Provider  string
	Total     int64
	Completed int64
	Failed    int64
	CacheHits int64
}

// ModelFailureCount is one (model, provider, failure_stage,
// failure_category) failure total over the observation window.
type ModelFailureCount struct {
	Model           string
	Provider        string
	FailureStage    string
	FailureCategory string
	Count           int64
}

// AttemptObservation is the raw DB read behind one models-observed response.
type AttemptObservation struct {
	Aggregates []ModelAttemptAggregate
	Failures   []ModelFailureCount
}

// ObservedAttemptStore reads terminal execution-attempt facts. Production
// uses ObservedRepository (raw SQL, no sqlc, no migration); tests inject
// fakes.
type ObservedAttemptStore interface {
	ListAttemptObservations(ctx context.Context, since time.Time) (AttemptObservation, error)
}

// SKUCatalogCard is the narrow billing projection the observed margin rows
// need: display name, on-sale status and current UnitCredits by variant.
// Composition adapts BillableItemCatalog.List to it so aigateway never
// imports the billing application package.
type SKUCatalogCard struct {
	SKU                  string
	DisplayName          string
	OnSale               bool
	UnitCreditsByVariant map[string]int64
}

// SKUCatalogReader lists the billing SKU catalog projection for margin rows.
type SKUCatalogReader interface {
	ListSKUCatalog(ctx context.Context) ([]SKUCatalogCard, error)
}

// observedMappingReader is the gateway mapping surface the observed service
// needs. ConfigRepository (via configStore) satisfies it.
type observedMappingReader interface {
	ListModelMappings(ctx context.Context) (map[string]string, error)
}

// ModelSKUMargin is one on-sale (or formerly on-sale) SKU charge row for an
// observed model. ActualCostCredits stays nil until a receipt/log cost source
// exists; MarginCredits stays nil with it. Both nils serialize as JSON null
// so callers render 缺省 instead of zero.
type ModelSKUMargin struct {
	SKU                  string           `json:"sku"`
	DisplayName          string           `json:"display_name"`
	Feature              string           `json:"feature"`
	OnSale               bool             `json:"on_sale"`
	UnitCreditsByVariant map[string]int64 `json:"unit_credits_by_variant"`
	ActualCostCredits    *int64           `json:"actual_cost_credits"`
	HasActualCost        bool             `json:"has_actual_cost"`
	CostSource           string           `json:"cost_source"`
	CostNote             string           `json:"cost_note"`
	MarginCredits        *int64           `json:"margin_credits"`
	MarginStatus         string           `json:"margin_status"`
}

// ModelObservedRow is one model's first-party 7-day observation plus its SKU
// margin rows. SuccessRate/CacheHitRate are nil when the model has no
// terminal attempts in the window; callers treat missing rows the same way.
type ModelObservedRow struct {
	Model           string           `json:"model"`
	Provider        string           `json:"provider"`
	WindowDays      int              `json:"window_days"`
	WindowSince     time.Time        `json:"window_since"`
	AttemptsTotal   int64            `json:"attempts_total"`
	Completed       int64            `json:"completed"`
	Failed          int64            `json:"failed"`
	CacheHits       int64            `json:"cache_hits"`
	SuccessRate     *float64         `json:"success_rate"`
	CacheHitRate    *float64         `json:"cache_hit_rate"`
	TopFailureStage string           `json:"top_failure_stage"`
	TopFailureCat   string           `json:"top_failure_category"`
	TopFailureCount int64            `json:"top_failure_count"`
	HasObservations bool             `json:"has_observations"`
	HasSKUMargin    bool             `json:"has_sku_margin"`
	SKUMargins      []ModelSKUMargin `json:"sku_margins"`
}

// skuForFeature maps one gateway mapping feature to its billing SKU. Vision
// rides ai-search:vision; every workflow feature rides ai-image:<slug> (see
// billingregistry). Unknown future features still get a deterministic SKU so
// margin rows degrade to not-on-sale instead of vanishing.
func skuForFeature(feature string) string {
	if strings.TrimSpace(feature) == visionMappingFeature {
		return "ai-search:vision"
	}
	return "ai-image:" + strings.TrimSpace(feature)
}

// providerFromModelID splits the provider prefix off a provider/model
// identifier. Mapping validation guarantees the slash for mapped models;
// bare IDs yield an empty provider instead of a guess.
func providerFromModelID(model string) string {
	if provider, _, found := strings.Cut(strings.TrimSpace(model), "/"); found && strings.TrimSpace(provider) != "" {
		return strings.TrimSpace(provider)
	}
	return ""
}

// AggregateModelObservations folds raw attempt facts, feature mappings and
// SKU catalog cards into per-model observed rows. Rows are the union of
// observed models and currently mapped models so mapped-but-idle models
// report HasObservations=false instead of disappearing. Pure function of its
// inputs for unit tests; the service supplies the window start.
func AggregateModelObservations(obs AttemptObservation, mappings map[string]string, skuCards []SKUCatalogCard, since time.Time) []ModelObservedRow {
	aggregateByKey := make(map[string]ModelAttemptAggregate, len(obs.Aggregates))
	for _, aggregate := range obs.Aggregates {
		model := strings.TrimSpace(aggregate.Model)
		if model == "" {
			continue
		}
		key := model + "\x00" + strings.TrimSpace(aggregate.Provider)
		aggregateByKey[key] = aggregate
	}
	failuresByKey := make(map[string][]ModelFailureCount)
	for _, failure := range obs.Failures {
		model := strings.TrimSpace(failure.Model)
		if model == "" || strings.TrimSpace(failure.FailureStage) == "" || failure.Count <= 0 {
			continue
		}
		key := model + "\x00" + strings.TrimSpace(failure.Provider)
		failuresByKey[key] = append(failuresByKey[key], failure)
	}
	featuresByModel := make(map[string][]string)
	for feature, model := range mappings {
		feature = strings.TrimSpace(feature)
		model = strings.TrimSpace(model)
		if feature == "" || model == "" {
			continue
		}
		featuresByModel[model] = append(featuresByModel[model], feature)
	}
	for model := range featuresByModel {
		sort.Strings(featuresByModel[model])
	}
	skuBySKU := make(map[string]SKUCatalogCard, len(skuCards))
	for _, card := range skuCards {
		if strings.TrimSpace(card.SKU) == "" {
			continue
		}
		skuBySKU[strings.TrimSpace(card.SKU)] = card
	}

	keys := make(map[string]struct{}, len(aggregateByKey)+len(featuresByModel))
	modelByKey := make(map[string]string, len(aggregateByKey)+len(featuresByModel))
	providerByKey := make(map[string]string, len(aggregateByKey)+len(featuresByModel))
	for key, aggregate := range aggregateByKey {
		keys[key] = struct{}{}
		modelByKey[key] = strings.TrimSpace(aggregate.Model)
		providerByKey[key] = strings.TrimSpace(aggregate.Provider)
	}
	for model := range featuresByModel {
		provider := providerFromModelID(model)
		key := model + "\x00" + provider
		if _, exists := keys[key]; !exists {
			keys[key] = struct{}{}
			modelByKey[key] = model
			providerByKey[key] = provider
		}
	}

	rows := make([]ModelObservedRow, 0, len(keys))
	for key := range keys {
		model := modelByKey[key]
		provider := providerByKey[key]
		row := ModelObservedRow{
			Model:       model,
			Provider:    provider,
			WindowDays:  ObservedWindowDays,
			WindowSince: since,
			SKUMargins:  []ModelSKUMargin{},
		}
		if aggregate, observed := aggregateByKey[key]; observed {
			row.AttemptsTotal = aggregate.Total
			row.Completed = aggregate.Completed
			row.Failed = aggregate.Failed
			row.CacheHits = aggregate.CacheHits
			terminal := aggregate.Completed + aggregate.Failed
			if terminal > 0 {
				row.HasObservations = true
				successRate := float64(aggregate.Completed) / float64(terminal)
				row.SuccessRate = &successRate
				cacheHitRate := float64(aggregate.CacheHits) / float64(terminal)
				row.CacheHitRate = &cacheHitRate
			}
			row.TopFailureStage, row.TopFailureCat, row.TopFailureCount = topFailure(failuresByKey[key])
		}
		for _, feature := range featuresByModel[model] {
			row.SKUMargins = append(row.SKUMargins, buildSKUMargin(feature, skuBySKU))
		}
		row.HasSKUMargin = len(row.SKUMargins) > 0
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AttemptsTotal != rows[j].AttemptsTotal {
			return rows[i].AttemptsTotal > rows[j].AttemptsTotal
		}
		if rows[i].Model != rows[j].Model {
			return rows[i].Model < rows[j].Model
		}
		return rows[i].Provider < rows[j].Provider
	})
	return rows
}

// topFailure picks the (stage, category) failure with the highest count.
// Ties break on stage then category ascending so output is deterministic.
func topFailure(failures []ModelFailureCount) (stage, category string, count int64) {
	for _, failure := range failures {
		if failure.Count > count ||
			(failure.Count == count && (strings.TrimSpace(failure.FailureStage) < stage ||
				(strings.TrimSpace(failure.FailureStage) == stage && strings.TrimSpace(failure.FailureCategory) < category))) {
			stage = strings.TrimSpace(failure.FailureStage)
			category = strings.TrimSpace(failure.FailureCategory)
			count = failure.Count
		}
	}
	return stage, category, count
}

// buildSKUMargin renders one margin row for a mapped feature. The charged
// side comes from the on-sale catalog card; the gateway actual cost has no
// receipt/log source, so the cost side stays an explicit unavailable marker.
func buildSKUMargin(feature string, skuBySKU map[string]SKUCatalogCard) ModelSKUMargin {
	sku := skuForFeature(feature)
	margin := ModelSKUMargin{
		SKU:                  sku,
		Feature:              feature,
		UnitCreditsByVariant: map[string]int64{},
		CostSource:           ObservedCostSourceUnavailable,
		CostNote:             msgBuildSKUMargin,
		MarginStatus:         ObservedMarginUnknownNoCost,
	}
	if card, ok := skuBySKU[sku]; ok {
		margin.DisplayName = card.DisplayName
		margin.OnSale = card.OnSale
		for variant, credits := range card.UnitCreditsByVariant {
			margin.UnitCreditsByVariant[variant] = credits
		}
	}
	return margin
}

// ObservedService composes attempt facts, gateway mappings and the billing
// SKU catalog into the admin models-observed read model.
type ObservedService struct {
	attempts ObservedAttemptStore
	mappings observedMappingReader
	billing  SKUCatalogReader
}

// NewObservedService builds the service over its three narrow seams.
func NewObservedService(attempts ObservedAttemptStore, mappings observedMappingReader, billing SKUCatalogReader) *ObservedService {
	return &ObservedService{attempts: attempts, mappings: mappings, billing: billing}
}

// ListModelsObserved returns per-model 7-day first-party observations with
// SKU margin rows. Models absent from the result have neither attempts nor
// mappings in the window; callers render them as 暂无数据.
func (s *ObservedService) ListModelsObserved(ctx context.Context) ([]ModelObservedRow, error) {
	if s == nil || s.attempts == nil || s.mappings == nil || s.billing == nil {
		return nil, errors.New("model observation service is unavailable")
	}
	since := time.Now().UTC().Add(-ObservedWindowDays * 24 * time.Hour)
	obs, err := s.attempts.ListAttemptObservations(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("list model attempt observations: %w", err)
	}
	mappings, err := s.mappings.ListModelMappings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list gateway model mappings: %w", err)
	}
	skuCards, err := s.billing.ListSKUCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billing SKU catalog: %w", err)
	}
	return AggregateModelObservations(obs, mappings, skuCards, since), nil
}

// ObservedRepository reads attempt facts with raw SQL: the aggregation needs
// no schema change and no new sqlc query, so generated code stays untouched.
// The status+created_at filter matches idx_task_execution_attempts_status_created_at.
type ObservedRepository struct {
	db *pgxpool.Pool
}

// NewObservedRepository builds the repository over the runtime database.
func NewObservedRepository(db *pgxpool.Pool) *ObservedRepository {
	return &ObservedRepository{db: db}
}

type observedContextDB struct {
	ctx context.Context
	db  *pgxpool.Pool
}

func (r *ObservedRepository) dbWithContext(ctx context.Context) observedContextDB {
	return observedContextDB{ctx: ctx, db: r.db}
}

func (q observedContextDB) Query(query string, args ...any) (pgx.Rows, error) {
	return q.db.Query(q.ctx, query, args...)
}

// ListAttemptObservations aggregates terminal (completed/failed) attempts
// created since the window start, grouped by resolved-or-requested model and
// provider, plus the per-model failure stage/category breakdown.
func (r *ObservedRepository) ListAttemptObservations(ctx context.Context, since time.Time) (AttemptObservation, error) {
	if r == nil || r.db == nil {
		return AttemptObservation{}, errors.New("model observation repository is unavailable")
	}
	aggregates, err := r.listAggregates(ctx, since)
	if err != nil {
		return AttemptObservation{}, err
	}
	failures, err := r.listFailures(ctx, since)
	if err != nil {
		return AttemptObservation{}, err
	}
	return AttemptObservation{Aggregates: aggregates, Failures: failures}, nil
}

const listObservedAggregatesSQL = `
SELECT
  COALESCE(NULLIF(resolved_model, ''), requested_model) AS model,
  provider AS provider,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 'completed') AS completed,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed,
  COUNT(*) FILTER (WHERE cache_hit) AS cache_hits
FROM task_execution_attempts
WHERE created_at >= $1
  AND status IN ('completed', 'failed')
  AND (resolved_model <> '' OR requested_model <> '')
GROUP BY 1, 2
ORDER BY 1 ASC, 2 ASC
`

func (r *ObservedRepository) listAggregates(ctx context.Context, since time.Time) ([]ModelAttemptAggregate, error) {
	rows, err := r.dbWithContext(ctx).Query(listObservedAggregatesSQL, since)
	if err != nil {
		return nil, fmt.Errorf("list model attempt aggregates: %w", err)
	}
	defer rows.Close()
	aggregates := make([]ModelAttemptAggregate, 0)
	for rows.Next() {
		var aggregate ModelAttemptAggregate
		if err := rows.Scan(&aggregate.Model, &aggregate.Provider, &aggregate.Total, &aggregate.Completed, &aggregate.Failed, &aggregate.CacheHits); err != nil {
			return nil, fmt.Errorf("scan model attempt aggregate: %w", err)
		}
		aggregates = append(aggregates, aggregate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read model attempt aggregates: %w", err)
	}
	return aggregates, nil
}

const listObservedFailuresSQL = `
SELECT
  COALESCE(NULLIF(resolved_model, ''), requested_model) AS model,
  provider AS provider,
  failure_stage AS failure_stage,
  failure_category AS failure_category,
  COUNT(*) AS total
FROM task_execution_attempts
WHERE created_at >= $1
  AND status = 'failed'
  AND (resolved_model <> '' OR requested_model <> '')
  AND failure_stage <> ''
GROUP BY 1, 2, 3, 4
ORDER BY 1 ASC, 2 ASC, 5 DESC, 3 ASC, 4 ASC
`

func (r *ObservedRepository) listFailures(ctx context.Context, since time.Time) ([]ModelFailureCount, error) {
	rows, err := r.dbWithContext(ctx).Query(listObservedFailuresSQL, since)
	if err != nil {
		return nil, fmt.Errorf("list model failure breakdown: %w", err)
	}
	defer rows.Close()
	failures := make([]ModelFailureCount, 0)
	for rows.Next() {
		var failure ModelFailureCount
		if err := rows.Scan(&failure.Model, &failure.Provider, &failure.FailureStage, &failure.FailureCategory, &failure.Count); err != nil {
			return nil, fmt.Errorf("scan model failure breakdown: %w", err)
		}
		failures = append(failures, failure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read model failure breakdown: %w", err)
	}
	return failures, nil
}
