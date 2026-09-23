package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/billing"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) PublishPrice(ctx context.Context, input billing.PublishPriceInput) (billing.PriceInfo, error) {
	variants := input.Definition.ResolvedPricingVariants()
	if len(variants) == 0 {
		return billing.PriceInfo{}, billing.ErrInvalidCatalogEntry
	}
	prices, err := s.PublishPrices(ctx, billing.PublishPricesInput{
		Definition: input.Definition,
		Prices:     map[billing.OutputResolution]int64{variants[0]: input.UnitCredits},
		ActorID:    input.ActorID,
	})
	return prices[variants[0]], err
}

func (s *Store) PublishPrices(ctx context.Context, input billing.PublishPricesInput) (map[billing.OutputResolution]billing.PriceInfo, error) {
	command, err := preparePublishPrices(input)
	if err != nil {
		return nil, err
	}
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	encodedPrices, err := json.Marshal(command.prices)
	if err != nil {
		return nil, fmt.Errorf("encode prices for publish: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT output_resolution, price_version_id, version, unit_credits, created_by, created_at
	FROM publish_billing_prices($1, $2, $3, $4::jsonb, $5)
	ORDER BY CASE output_resolution WHEN 'default' THEN 1 WHEN '1k' THEN 2 WHEN '2k' THEN 3 ELSE 4 END`, command.sku, command.displayName, command.description, string(encodedPrices), command.actorID)
	if err != nil {
		return nil, mapDatabaseError("publish price version", err)
	}
	defer rows.Close()
	published, err := scanPublishedPrices(rows, len(command.prices))
	if err != nil {
		return nil, err
	}
	if _, ok := published[command.requiredVariant]; !ok {
		return nil, billing.ErrInvalidCatalogEntry
	}
	return published, nil
}

type publishPricesCommand struct {
	sku             billing.SKU
	requiredVariant billing.OutputResolution
	displayName     string
	description     string
	prices          map[string]int64
	actorID         int64
}

func preparePublishPrices(input billing.PublishPricesInput) (publishPricesCommand, error) {
	definition := input.Definition
	sku, err := billing.CanonicalizeSKU(string(definition.SKU))
	variants := definition.ResolvedPricingVariants()
	if err != nil || input.ActorID <= 0 || len(variants) == 0 {
		return publishPricesCommand{}, billing.ErrInvalidCatalogEntry
	}
	prices, err := normalizePublishedPrices(definition, variants[0], input.Prices)
	if err != nil {
		return publishPricesCommand{}, err
	}
	displayName, err := billing.NormalizeDisplayName(definition.DisplayName)
	if err != nil {
		return publishPricesCommand{}, err
	}
	description, err := billing.NormalizeDescription(definition.Description)
	if err != nil {
		return publishPricesCommand{}, err
	}
	return publishPricesCommand{
		sku:             sku,
		requiredVariant: variants[0],
		displayName:     displayName,
		description:     description,
		prices:          prices,
		actorID:         input.ActorID,
	}, nil
}

func normalizePublishedPrices(definition billing.BillableItemDefinition, requiredVariant billing.OutputResolution, input map[billing.OutputResolution]int64) (map[string]int64, error) {
	requiredPrice, hasRequired := input[requiredVariant]
	if !hasRequired || requiredPrice < 0 || requiredPrice > billing.MaxQuoteCredits {
		return nil, billing.ErrInvalidCatalogEntry
	}
	prices := make(map[string]int64, len(input))
	for resolution, unitCredits := range input {
		if err := billing.ValidatePricingVariant(definition, resolution); err != nil || unitCredits < 0 || unitCredits > billing.MaxQuoteCredits {
			return nil, billing.ErrInvalidCatalogEntry
		}
		prices[string(resolution)] = unitCredits
	}
	return prices, nil
}

func scanPublishedPrices(rows pgx.Rows, capacity int) (map[billing.OutputResolution]billing.PriceInfo, error) {
	published := make(map[billing.OutputResolution]billing.PriceInfo, capacity)
	for rows.Next() {
		var candidate billing.PriceInfo
		if err := rows.Scan(&candidate.OutputResolution, &candidate.PriceVersionID, &candidate.Version, &candidate.UnitCredits, &candidate.CreatedBy, &candidate.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan published price version: %w", err)
		}
		published[candidate.OutputResolution] = candidate
	}
	if err := rows.Err(); err != nil {
		return nil, mapDatabaseError("publish price version", err)
	}
	return published, nil
}

func (s *Store) SetAvailability(ctx context.Context, rawSKU string, enabled bool, actorID int64) (billing.AvailabilityInfo, error) {
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil || actorID <= 0 {
		return billing.AvailabilityInfo{}, billing.ErrInvalidCatalogEntry
	}
	if err := s.requirePool(); err != nil {
		return billing.AvailabilityInfo{}, err
	}
	var info billing.AvailabilityInfo
	var persistedEnabled bool
	err = s.pool.QueryRow(ctx, `
SELECT sku, enabled, updated_at
FROM set_billing_item_availability($1, $2, $3)`, sku, enabled, actorID).Scan(
		&info.SKU, &persistedEnabled, &info.UpdatedAt,
	)
	if err != nil {
		return billing.AvailabilityInfo{}, mapDatabaseError("set item availability", err)
	}
	if persistedEnabled {
		info.Status = billing.BillableItemStatusActive
	} else {
		info.Status = billing.BillableItemStatusDisabled
	}
	return info, nil
}

func (s *Store) QuoteAtResolution(ctx context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	if err := s.requirePool(); err != nil {
		return billing.Quote{}, err
	}
	return s.quote(ctx, s.pool, rawSKU, resolution, quantity)
}

func (s *Store) QuoteAtResolutionInTx(ctx context.Context, tx pgx.Tx, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	if tx == nil {
		return billing.Quote{}, billing.ErrServiceUnavailable
	}
	return s.quote(ctx, tx, rawSKU, resolution, quantity)
}

type quoteQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) quote(ctx context.Context, querier quoteQuerier, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	if quantity <= 0 {
		return billing.Quote{}, billing.ErrInvalidQuantity
	}
	if _, err := billing.NormalizeOutputResolution(string(resolution)); err != nil {
		return billing.Quote{}, err
	}
	if querier == nil {
		return billing.Quote{}, billing.ErrServiceUnavailable
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
LEFT JOIN billing_current_prices current_prices
  ON current_prices.sku = skus.sku AND current_prices.output_resolution = $2
LEFT JOIN billing_price_versions prices
  ON prices.sku = current_prices.sku AND prices.output_resolution = current_prices.output_resolution
 AND prices.price_version_id = current_prices.price_version_id
	WHERE skus.sku = $1`, sku, resolution).Scan(
		&skuEnabled, &displayName, &priceVersionID, &priceVersion, &unitCredits, &quotedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.Quote{}, billing.ErrSKUNotFound
	}
	if err != nil {
		return billing.Quote{}, fmt.Errorf("billing quote: %w", err)
	}
	if !skuEnabled || !priceVersionID.Valid || !priceVersion.Valid || !unitCredits.Valid || !quotedAt.Valid {
		return billing.Quote{}, billing.ErrPricingUnavailable
	}
	snapshot := billing.PriceSnapshot{
		SKU: sku, DisplayName: displayName, PriceVersionID: priceVersionID.Int64, PriceVersion: priceVersion.Int64,
		OutputResolution: resolution, UnitCredits: unitCredits.Int64, QuotedAt: quotedAt.Time,
	}
	total, err := snapshot.Total(quantity)
	if err != nil {
		return billing.Quote{}, err
	}
	return billing.Quote{PriceSnapshot: snapshot, Quantity: quantity, TotalCredits: total}, nil
}

// ListCatalogFacts returns the complete persisted projection used by the
// bounded Billable Item Registry merge.
func (s *Store) ListCatalogFacts(ctx context.Context) ([]billing.CatalogItem, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, catalogSelectSQL+` ORDER BY skus.sku, current_prices.output_resolution`)
	if err != nil {
		return nil, fmt.Errorf("list billing catalog facts: %w", err)
	}
	defer rows.Close()
	items := make([]billing.CatalogItem, 0)
	indexBySKU := make(map[billing.SKU]int)
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan billing catalog facts: %w", err)
		}
		index, exists := indexBySKU[item.SKU]
		if !exists {
			indexBySKU[item.SKU] = len(items)
			items = append(items, item)
			continue
		}
		for resolution, price := range item.CurrentPrices {
			if items[index].CurrentPrices == nil {
				items[index].CurrentPrices = make(map[billing.OutputResolution]billing.PriceInfo)
			}
			items[index].CurrentPrices[resolution] = price
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read billing catalog facts: %w", err)
	}
	return items, nil
}

func (s *Store) DeleteArchived(ctx context.Context, rawSKU string, actorID int64) (billing.ArchivedItemDeleteResult, error) {
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil || actorID <= 0 {
		return billing.ArchivedItemDeleteResult{}, billing.ErrInvalidCatalogEntry
	}
	if err := s.requirePool(); err != nil {
		return billing.ArchivedItemDeleteResult{}, err
	}
	var deleted billing.SKU
	if err := s.pool.QueryRow(ctx, `SELECT delete_archived_billing_item($1, $2)`, sku, actorID).Scan(&deleted); err != nil {
		return billing.ArchivedItemDeleteResult{}, mapDeleteArchivedDatabaseError(err)
	}
	return billing.ArchivedItemDeleteResult{SKU: deleted}, nil
}

func mapDeleteArchivedDatabaseError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return billing.ErrArchivedItemInUse
	}
	return mapDatabaseError("delete archived item", err)
}

const catalogSelectSQL = `
SELECT skus.sku, skus.display_name, skus.description, skus.enabled,
       skus.created_at, skus.updated_at,
	   current_prices.output_resolution,
	   prices.price_version_id, prices.version, prices.unit_credits,
	   prices.created_by, prices.created_at
FROM billing_skus skus
LEFT JOIN billing_current_prices current_prices ON current_prices.sku = skus.sku
LEFT JOIN billing_price_versions prices
	ON prices.sku = current_prices.sku
 AND prices.output_resolution = current_prices.output_resolution
 AND prices.price_version_id = current_prices.price_version_id`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCatalogItem(row rowScanner) (billing.CatalogItem, error) {
	var item billing.CatalogItem
	var priceID pgtype.Int8
	var version pgtype.Int8
	var unitCredits pgtype.Int8
	var createdBy pgtype.Int8
	var priceCreatedAt pgtype.Timestamptz
	var resolution pgtype.Text
	if err := row.Scan(
		&item.SKU, &item.DisplayName, &item.Description, &item.Enabled,
		&item.CreatedAt, &item.UpdatedAt,
		&resolution, &priceID, &version, &unitCredits, &createdBy, &priceCreatedAt,
	); err != nil {
		return billing.CatalogItem{}, err
	}
	if priceID.Valid && version.Valid && unitCredits.Valid && createdBy.Valid && priceCreatedAt.Valid && resolution.Valid {
		candidate := billing.PriceInfo{
			PriceVersionID: priceID.Int64, Version: version.Int64, OutputResolution: billing.OutputResolution(resolution.String), UnitCredits: unitCredits.Int64,
			CreatedBy: createdBy.Int64, CreatedAt: priceCreatedAt.Time,
		}
		if item.CurrentPrices == nil {
			item.CurrentPrices = make(map[billing.OutputResolution]billing.PriceInfo)
		}
		item.CurrentPrices[candidate.OutputResolution] = candidate
	}
	return item, nil
}

func (s *Store) requirePool() error {
	if s == nil || s.pool == nil {
		return billing.ErrServiceUnavailable
	}
	return nil
}

func mapDatabaseError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "IB101", "22001", "23514":
			return billing.ErrInvalidCatalogEntry
		case "IB103":
			return billing.ErrSKUNotFound
		case "IB104":
			return billing.ErrPricingUnavailable
		case "IB108":
			return billing.ErrArchivedItemInUse
		}
	}
	return fmt.Errorf("billing %s: %w", action, err)
}
