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

func (s *Service) PublishPrice(ctx context.Context, input PublishPriceInput) (PriceInfo, error) {
	definition := input.Definition
	sku, err := CanonicalizeSKU(string(definition.SKU))
	if err != nil || input.UnitCredits < 0 || input.UnitCredits > MaxQuoteCredits || input.ActorID <= 0 {
		return PriceInfo{}, ErrInvalidCatalogEntry
	}
	displayName, err := normalizeDisplayName(definition.DisplayName)
	if err != nil {
		return PriceInfo{}, err
	}
	description, err := normalizeDescription(definition.Description)
	if err != nil {
		return PriceInfo{}, err
	}
	if err := s.requirePool(); err != nil {
		return PriceInfo{}, err
	}
	var price PriceInfo
	err = s.pool.QueryRow(ctx, `SELECT `+priceInfoColumns+`
	FROM publish_billing_price($1, $2, $3, $4, $5)`, sku, displayName, description, input.UnitCredits, input.ActorID).
		Scan(priceInfoScanTargets(&price)...)
	if err != nil {
		return PriceInfo{}, mapDatabaseError("publish price version", err)
	}
	return price, nil
}

func (s *Service) SetAvailability(ctx context.Context, rawSKU string, enabled bool, actorID int64) (AvailabilityInfo, error) {
	sku, err := CanonicalizeSKU(rawSKU)
	if err != nil || actorID <= 0 {
		return AvailabilityInfo{}, ErrInvalidCatalogEntry
	}
	if err := s.requirePool(); err != nil {
		return AvailabilityInfo{}, err
	}
	var info AvailabilityInfo
	var persistedEnabled bool
	err = s.pool.QueryRow(ctx, `
SELECT sku, enabled, updated_at
FROM set_billing_item_availability($1, $2, $3)`, sku, enabled, actorID).Scan(
		&info.SKU, &persistedEnabled, &info.UpdatedAt,
	)
	if err != nil {
		return AvailabilityInfo{}, mapDatabaseError("set item availability", err)
	}
	if persistedEnabled {
		info.Status = BillableItemStatusActive
	} else {
		info.Status = BillableItemStatusDisabled
	}
	return info, nil
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
	var quotedAt pgtype.Timestamptz
	err = querier.QueryRow(ctx, `
SELECT skus.enabled, skus.display_name, prices.price_version_id, prices.version, prices.unit_credits,
	       statement_timestamp()
FROM billing_skus skus
LEFT JOIN billing_price_versions prices
  ON prices.sku = skus.sku AND prices.price_version_id = skus.current_price_version_id
WHERE skus.sku = $1`, sku).Scan(
		&skuEnabled, &displayName, &priceVersionID, &priceVersion, &unitCredits, &quotedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrSKUNotFound
	}
	if err != nil {
		return Quote{}, fmt.Errorf("billing quote: %w", err)
	}
	if !skuEnabled || !priceVersionID.Valid || !priceVersion.Valid || !unitCredits.Valid || !quotedAt.Valid {
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

// ListCatalogFacts returns the complete persisted projection used by the
// bounded Billable Item Registry merge.
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
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing_skus WHERE sku = $1)`, sku).Scan(&exists); err != nil {
			return PricePage{}, fmt.Errorf("check billing SKU: %w", err)
		}
		if !exists {
			return PricePage{}, ErrSKUNotFound
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

func (s *Service) DeleteArchived(ctx context.Context, rawSKU string, actorID int64) (ArchivedItemDeleteResult, error) {
	sku, err := CanonicalizeSKU(rawSKU)
	if err != nil || actorID <= 0 {
		return ArchivedItemDeleteResult{}, ErrInvalidCatalogEntry
	}
	if err := s.requirePool(); err != nil {
		return ArchivedItemDeleteResult{}, err
	}
	var deleted SKU
	if err := s.pool.QueryRow(ctx, `SELECT delete_archived_billing_item($1, $2)`, sku, actorID).Scan(&deleted); err != nil {
		return ArchivedItemDeleteResult{}, mapDeleteArchivedDatabaseError(err)
	}
	return ArchivedItemDeleteResult{SKU: deleted}, nil
}

func mapDeleteArchivedDatabaseError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrArchivedItemInUse
	}
	return mapDatabaseError("delete archived item", err)
}

const catalogSelectSQL = `
SELECT skus.sku, skus.display_name, skus.description, skus.enabled,
       skus.current_price_version_id, skus.created_at, skus.updated_at,
	   prices.price_version_id, prices.version, prices.unit_credits,
	   prices.created_by, prices.created_at
FROM billing_skus skus
LEFT JOIN billing_price_versions prices
  ON prices.sku = skus.sku AND prices.price_version_id = skus.current_price_version_id`

const priceInfoColumns = `price_version_id, version, unit_credits, created_by, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func priceInfoScanTargets(price *PriceInfo) []any {
	return []any{
		&price.PriceVersionID, &price.Version, &price.UnitCredits, &price.CreatedBy, &price.CreatedAt,
	}
}

func scanCatalogItem(row rowScanner) (CatalogItem, error) {
	var item CatalogItem
	var currentID pgtype.Int8
	var priceID pgtype.Int8
	var version pgtype.Int8
	var unitCredits pgtype.Int8
	var createdBy pgtype.Int8
	var priceCreatedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.SKU, &item.DisplayName, &item.Description, &item.Enabled,
		&currentID, &item.CreatedAt, &item.UpdatedAt,
		&priceID, &version, &unitCredits, &createdBy, &priceCreatedAt,
	); err != nil {
		return CatalogItem{}, err
	}
	if currentID.Valid {
		value := currentID.Int64
		item.CurrentPriceVersionID = &value
	}
	if priceID.Valid && version.Valid && unitCredits.Valid && createdBy.Valid && priceCreatedAt.Valid {
		item.CurrentPrice = &PriceInfo{
			PriceVersionID: priceID.Int64, Version: version.Int64, UnitCredits: unitCredits.Int64,
			CreatedBy: createdBy.Int64, CreatedAt: priceCreatedAt.Time,
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
		case "IB103":
			return ErrSKUNotFound
		case "IB104":
			return ErrPricingUnavailable
		case "IB108":
			return ErrArchivedItemInUse
		}
	}
	return fmt.Errorf("billing %s: %w", action, err)
}
