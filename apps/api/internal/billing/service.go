package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) CreateSKU(ctx context.Context, input CreateSKUInput) (CatalogItem, error) {
	sku, err := CanonicalizeSKU(input.SKU)
	if err != nil || !input.Action.IsValid() {
		return CatalogItem{}, ErrInvalidCatalogEntry
	}
	displayName, err := normalizeDisplayName(input.DisplayName)
	if err != nil {
		return CatalogItem{}, err
	}
	description, err := normalizeDescription(input.Description)
	if err != nil {
		return CatalogItem{}, err
	}
	if err := s.requirePool(); err != nil {
		return CatalogItem{}, err
	}
	if _, err := s.pool.Exec(ctx, `SELECT create_billing_sku($1, $2, $3, $4, $5, $6)`, sku, displayName, description, input.Enabled, input.Action.ActorID(), input.Action.Reason()); err != nil {
		return CatalogItem{}, mapDatabaseError("create SKU", err)
	}
	return s.GetSKU(ctx, string(sku))
}

func (s *Service) UpdateSKU(ctx context.Context, input UpdateSKUInput) (CatalogItem, error) {
	sku, err := CanonicalizeSKU(input.SKU)
	if err != nil || !input.Action.IsValid() || (input.DisplayName == nil && input.Description == nil && input.Enabled == nil) {
		return CatalogItem{}, ErrInvalidCatalogEntry
	}
	var displayName *string
	if input.DisplayName != nil {
		value, err := normalizeDisplayName(*input.DisplayName)
		if err != nil {
			return CatalogItem{}, err
		}
		displayName = &value
	}
	var description *string
	if input.Description != nil {
		value, err := normalizeDescription(*input.Description)
		if err != nil {
			return CatalogItem{}, err
		}
		description = &value
	}
	if err := s.requirePool(); err != nil {
		return CatalogItem{}, err
	}
	if _, err := s.pool.Exec(ctx, `SELECT update_billing_sku($1, $2, $3, $4, $5, $6)`, sku, displayName, description, input.Enabled, input.Action.ActorID(), input.Action.Reason()); err != nil {
		return CatalogItem{}, mapDatabaseError("update SKU", err)
	}
	return s.GetSKU(ctx, string(sku))
}

func (s *Service) CreatePrice(ctx context.Context, input CreatePriceInput) (PriceInfo, error) {
	sku, err := CanonicalizeSKU(input.SKU)
	if err != nil || input.UnitCredits < 0 || input.UnitCredits > MaxQuoteCredits || !input.Action.IsValid() {
		return PriceInfo{}, ErrInvalidCatalogEntry
	}
	if err := s.requirePool(); err != nil {
		return PriceInfo{}, err
	}
	var price PriceInfo
	err = s.pool.QueryRow(ctx, `SELECT `+priceInfoColumns+`
FROM create_billing_price_version($1, $2, $3, $4, $5)`, sku, input.UnitCredits, input.Enabled, input.Action.ActorID(), input.Action.Reason()).
		Scan(priceInfoScanTargets(&price)...)
	if err != nil {
		return PriceInfo{}, mapDatabaseError("create price version", err)
	}
	return price, nil
}

func (s *Service) SetPriceEnabled(ctx context.Context, input SetPriceEnabledInput) (PriceInfo, error) {
	sku, err := CanonicalizeSKU(input.SKU)
	if err != nil || input.Version <= 0 || !input.Action.IsValid() {
		return PriceInfo{}, ErrInvalidCatalogEntry
	}
	if err := s.requirePool(); err != nil {
		return PriceInfo{}, err
	}
	var price PriceInfo
	err = s.pool.QueryRow(ctx, `SELECT `+priceInfoColumns+`
FROM set_billing_price_version_enabled($1, $2, $3, $4, $5)`, sku, input.Version, input.Enabled, input.Action.ActorID(), input.Action.Reason()).
		Scan(priceInfoScanTargets(&price)...)
	if err != nil {
		return PriceInfo{}, mapDatabaseError("set price version state", err)
	}
	return price, nil
}

func (s *Service) Quote(ctx context.Context, rawSKU string, quantity int64) (Quote, error) {
	if err := s.requirePool(); err != nil {
		return Quote{}, err
	}
	return s.quote(ctx, s.pool, rawSKU, quantity)
}

// QuoteInTx locks a submission to the price visible in its caller-owned
// PostgreSQL transaction.
func (s *Service) QuoteInTx(ctx context.Context, tx pgx.Tx, rawSKU string, quantity int64) (Quote, error) {
	if tx == nil {
		return Quote{}, ErrServiceUnavailable
	}
	return s.quote(ctx, tx, rawSKU, quantity)
}

type quoteQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) quote(ctx context.Context, querier quoteQuerier, rawSKU string, quantity int64) (Quote, error) {
	sku, err := CanonicalizeSKU(rawSKU)
	if err != nil {
		return Quote{}, err
	}
	if quantity <= 0 {
		return Quote{}, ErrInvalidQuantity
	}
	if querier == nil {
		return Quote{}, ErrServiceUnavailable
	}
	var skuEnabled bool
	var displayName string
	var priceVersionID pgtype.Int8
	var priceVersion pgtype.Int8
	var unitCredits pgtype.Int8
	var priceEnabled pgtype.Bool
	var quotedAt pgtype.Timestamptz
	err = querier.QueryRow(ctx, `
SELECT skus.enabled, skus.display_name, prices.price_version_id, prices.version, prices.unit_credits,
       states.enabled, statement_timestamp()
FROM billing_skus skus
LEFT JOIN billing_price_versions prices
  ON prices.sku = skus.sku AND prices.price_version_id = skus.current_price_version_id
LEFT JOIN billing_price_version_states states USING (price_version_id)
WHERE skus.sku = $1`, sku).Scan(
		&skuEnabled, &displayName, &priceVersionID, &priceVersion, &unitCredits, &priceEnabled, &quotedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrSKUNotFound
	}
	if err != nil {
		return Quote{}, fmt.Errorf("billing quote: %w", err)
	}
	if !skuEnabled || !priceVersionID.Valid || !priceVersion.Valid || !unitCredits.Valid || !priceEnabled.Valid || !priceEnabled.Bool || !quotedAt.Valid {
		return Quote{}, ErrPricingUnavailable
	}
	snapshot := PriceSnapshot{
		SKU: sku, DisplayName: displayName, PriceVersionID: priceVersionID.Int64, PriceVersion: priceVersion.Int64,
		UnitCredits: unitCredits.Int64, QuotedAt: quotedAt.Time,
	}
	total, err := snapshot.Total(quantity)
	if err != nil {
		return Quote{}, err
	}
	return Quote{PriceSnapshot: snapshot, Quantity: quantity, TotalCredits: total}, nil
}

func (s *Service) GetSKU(ctx context.Context, rawSKU string) (CatalogItem, error) {
	sku, err := CanonicalizeSKU(rawSKU)
	if err != nil {
		return CatalogItem{}, err
	}
	if err := s.requirePool(); err != nil {
		return CatalogItem{}, err
	}
	row := s.pool.QueryRow(ctx, catalogSelectSQL+` WHERE skus.sku = $1`, sku)
	item, err := scanCatalogItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CatalogItem{}, ErrSKUNotFound
	}
	if err != nil {
		return CatalogItem{}, fmt.Errorf("get billing SKU: %w", err)
	}
	return item, nil
}

func (s *Service) GetSKUs(ctx context.Context, rawSKUs []string) ([]CatalogItem, error) {
	if len(rawSKUs) == 0 {
		return []CatalogItem{}, nil
	}
	skus := make([]string, 0, len(rawSKUs))
	seen := make(map[SKU]struct{}, len(rawSKUs))
	for _, rawSKU := range rawSKUs {
		sku, err := CanonicalizeSKU(rawSKU)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[sku]; exists {
			continue
		}
		seen[sku] = struct{}{}
		skus = append(skus, string(sku))
	}
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, catalogSelectSQL+`
WHERE skus.sku = ANY($1::text[])
ORDER BY skus.sku`, skus)
	if err != nil {
		return nil, fmt.Errorf("get billing SKUs: %w", err)
	}
	defer rows.Close()
	items := make([]CatalogItem, 0, len(skus))
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan billing SKUs: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read billing SKUs: %w", err)
	}
	return items, nil
}

func (s *Service) ListCatalog(ctx context.Context, input ListCatalogInput) (CatalogPage, error) {
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return CatalogPage{}, err
	}
	afterSKU := ""
	if input.AfterSKU != "" {
		canonical, err := CanonicalizeSKU(input.AfterSKU)
		if err != nil {
			return CatalogPage{}, err
		}
		afterSKU = string(canonical)
	}
	if err := s.requirePool(); err != nil {
		return CatalogPage{}, err
	}
	rows, err := s.pool.Query(ctx, catalogSelectSQL+`
WHERE skus.sku > $1
ORDER BY skus.sku
LIMIT $2`, afterSKU, limit+1)
	if err != nil {
		return CatalogPage{}, fmt.Errorf("list billing catalog: %w", err)
	}
	defer rows.Close()
	items := make([]CatalogItem, 0, limit)
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			return CatalogPage{}, fmt.Errorf("scan billing catalog: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CatalogPage{}, fmt.Errorf("read billing catalog: %w", err)
	}
	page := CatalogPage{Items: items}
	if len(items) > limit {
		next := items[limit-1].SKU
		page.Items = items[:limit]
		page.NextAfterSKU = &next
	}
	return page, nil
}

// ListCatalogFacts returns the complete persisted Catalog projection used by
// the Billable Item Read Model. The Admin SKU endpoint keeps its paginated
// contract; only the bounded Registry merge consumes this complete view.
func (s *Service) ListCatalogFacts(ctx context.Context) ([]CatalogItem, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, catalogSelectSQL+` ORDER BY skus.sku`)
	if err != nil {
		return nil, fmt.Errorf("list billing catalog facts: %w", err)
	}
	defer rows.Close()
	items := make([]CatalogItem, 0)
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan billing catalog facts: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read billing catalog facts: %w", err)
	}
	return items, nil
}

func (s *Service) ListPrices(ctx context.Context, input ListPricesInput) (PricePage, error) {
	sku, err := CanonicalizeSKU(input.SKU)
	if err != nil {
		return PricePage{}, err
	}
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return PricePage{}, err
	}
	before := input.BeforeVersion
	if before < 0 {
		return PricePage{}, ErrInvalidCatalogEntry
	}
	if before == 0 {
		before = int64(^uint64(0) >> 1)
	}
	if err := s.requirePool(); err != nil {
		return PricePage{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+priceInfoColumns+`
FROM billing_price_versions prices
JOIN billing_price_version_states states USING (price_version_id)
WHERE prices.sku = $1 AND prices.version < $2
ORDER BY prices.version DESC
LIMIT $3`, sku, before, limit+1)
	if err != nil {
		return PricePage{}, fmt.Errorf("list billing prices: %w", err)
	}
	defer rows.Close()
	prices := make([]PriceInfo, 0, limit)
	for rows.Next() {
		var price PriceInfo
		if err := rows.Scan(priceInfoScanTargets(&price)...); err != nil {
			return PricePage{}, fmt.Errorf("scan billing price: %w", err)
		}
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		return PricePage{}, fmt.Errorf("read billing prices: %w", err)
	}
	if len(prices) == 0 {
		if _, err := s.GetSKU(ctx, string(sku)); err != nil {
			return PricePage{}, err
		}
	}
	page := PricePage{Prices: prices}
	if len(prices) > limit {
		next := prices[limit-1].Version
		page.Prices = prices[:limit]
		page.NextBeforeVersion = &next
	}
	return page, nil
}

const catalogSelectSQL = `
SELECT skus.sku, skus.display_name, skus.description, skus.enabled,
       skus.current_price_version_id, skus.created_at, skus.updated_at,
       prices.price_version_id, prices.version, prices.unit_credits, states.enabled,
       prices.created_by, prices.created_at, states.updated_by, states.updated_at
FROM billing_skus skus
LEFT JOIN billing_price_versions prices
  ON prices.sku = skus.sku AND prices.price_version_id = skus.current_price_version_id
LEFT JOIN billing_price_version_states states USING (price_version_id)`

const priceInfoColumns = `price_version_id, version, unit_credits, enabled, created_by, created_at, updated_by, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func priceInfoScanTargets(price *PriceInfo) []any {
	return []any{
		&price.PriceVersionID, &price.Version, &price.UnitCredits, &price.Enabled,
		&price.CreatedBy, &price.CreatedAt, &price.UpdatedBy, &price.UpdatedAt,
	}
}

func scanCatalogItem(row rowScanner) (CatalogItem, error) {
	var item CatalogItem
	var currentID pgtype.Int8
	var priceID pgtype.Int8
	var version pgtype.Int8
	var unitCredits pgtype.Int8
	var priceEnabled pgtype.Bool
	var createdBy pgtype.Int8
	var priceCreatedAt pgtype.Timestamptz
	var updatedBy pgtype.Int8
	var priceUpdatedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.SKU, &item.DisplayName, &item.Description, &item.Enabled,
		&currentID, &item.CreatedAt, &item.UpdatedAt,
		&priceID, &version, &unitCredits, &priceEnabled,
		&createdBy, &priceCreatedAt, &updatedBy, &priceUpdatedAt,
	); err != nil {
		return CatalogItem{}, err
	}
	if currentID.Valid {
		value := currentID.Int64
		item.CurrentPriceVersionID = &value
	}
	if priceID.Valid && version.Valid && unitCredits.Valid && priceEnabled.Valid && createdBy.Valid && priceCreatedAt.Valid && updatedBy.Valid && priceUpdatedAt.Valid {
		item.CurrentPrice = &PriceInfo{
			PriceVersionID: priceID.Int64, Version: version.Int64, UnitCredits: unitCredits.Int64,
			Enabled: priceEnabled.Bool, CreatedBy: createdBy.Int64, CreatedAt: priceCreatedAt.Time,
			UpdatedBy: updatedBy.Int64, UpdatedAt: priceUpdatedAt.Time,
		}
	}
	return item, nil
}

func (s *Service) requirePool() error {
	if s == nil || s.pool == nil {
		return ErrServiceUnavailable
	}
	return nil
}

func mapDatabaseError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "IB101", "22001", "23514":
			return ErrInvalidCatalogEntry
		case "IB102", "23505":
			return ErrSKUAlreadyExists
		case "IB103":
			return ErrSKUNotFound
		case "IB106":
			return ErrPriceNotFound
		}
	}
	return fmt.Errorf("billing %s: %w", action, err)
}
