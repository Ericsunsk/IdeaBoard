package billing

import (
	"context"
	"encoding/json"
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
	variants := input.Definition.pricingVariants()
	if len(variants) == 0 {
		return PriceInfo{}, ErrInvalidCatalogEntry
	}
	prices, err := s.PublishPrices(ctx, PublishPricesInput{
		Definition: input.Definition,
		Prices:     map[OutputResolution]int64{variants[0]: input.UnitCredits},
		ActorID:    input.ActorID,
	})
	return prices[variants[0]], err
}

func (s *Service) PublishPrices(ctx context.Context, input PublishPricesInput) (map[OutputResolution]PriceInfo, error) {
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
		return nil, ErrInvalidCatalogEntry
	}
	return published, nil
}

type publishPricesCommand struct {
	sku             SKU
	requiredVariant OutputResolution
	displayName     string
	description     string
	prices          map[string]int64
	actorID         int64
}

func preparePublishPrices(input PublishPricesInput) (publishPricesCommand, error) {
	definition := input.Definition
	sku, err := CanonicalizeSKU(string(definition.SKU))
	variants := definition.pricingVariants()
	if err != nil || input.ActorID <= 0 || len(variants) == 0 {
		return publishPricesCommand{}, ErrInvalidCatalogEntry
	}
	prices, err := normalizePublishedPrices(definition, variants[0], input.Prices)
	if err != nil {
		return publishPricesCommand{}, err
	}
	displayName, err := normalizeDisplayName(definition.DisplayName)
	if err != nil {
		return publishPricesCommand{}, err
	}
	description, err := normalizeDescription(definition.Description)
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

func normalizePublishedPrices(definition BillableItemDefinition, requiredVariant OutputResolution, input map[OutputResolution]int64) (map[string]int64, error) {
	requiredPrice, hasRequired := input[requiredVariant]
	if !hasRequired || requiredPrice < 0 || requiredPrice > MaxQuoteCredits {
		return nil, ErrInvalidCatalogEntry
	}
	prices := make(map[string]int64, len(input))
	for resolution, unitCredits := range input {
		if err := validatePricingVariant(definition, resolution); err != nil || unitCredits < 0 || unitCredits > MaxQuoteCredits {
			return nil, ErrInvalidCatalogEntry
		}
		prices[string(resolution)] = unitCredits
	}
	return prices, nil
}

func scanPublishedPrices(rows pgx.Rows, capacity int) (map[OutputResolution]PriceInfo, error) {
	published := make(map[OutputResolution]PriceInfo, capacity)
	for rows.Next() {
		var candidate PriceInfo
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

func (s *Service) QuoteAtResolution(ctx context.Context, rawSKU string, resolution OutputResolution, quantity int64) (Quote, error) {
	if err := s.requirePool(); err != nil {
		return Quote{}, err
	}
	return s.quote(ctx, s.pool, rawSKU, resolution, quantity)
}

func (s *Service) QuoteAtResolutionInTx(ctx context.Context, tx pgx.Tx, rawSKU string, resolution OutputResolution, quantity int64) (Quote, error) {
	if tx == nil {
		return Quote{}, ErrServiceUnavailable
	}
	return s.quote(ctx, tx, rawSKU, resolution, quantity)
}

type quoteQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) quote(ctx context.Context, querier quoteQuerier, rawSKU string, resolution OutputResolution, quantity int64) (Quote, error) {
	sku, err := CanonicalizeSKU(rawSKU)
	if err != nil {
		return Quote{}, err
	}
	if quantity <= 0 {
		return Quote{}, ErrInvalidQuantity
	}
	if _, err := NormalizeOutputResolution(string(resolution)); err != nil {
		return Quote{}, err
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
LEFT JOIN billing_current_prices current_prices
  ON current_prices.sku = skus.sku AND current_prices.output_resolution = $2
LEFT JOIN billing_price_versions prices
  ON prices.sku = current_prices.sku AND prices.output_resolution = current_prices.output_resolution
 AND prices.price_version_id = current_prices.price_version_id
	WHERE skus.sku = $1`, sku, resolution).Scan(
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
		OutputResolution: resolution, UnitCredits: unitCredits.Int64, QuotedAt: quotedAt.Time,
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
	rows, err := s.pool.Query(ctx, catalogSelectSQL+` ORDER BY skus.sku, current_prices.output_resolution`)
	if err != nil {
		return nil, fmt.Errorf("list billing catalog facts: %w", err)
	}
	defer rows.Close()
	items := make([]CatalogItem, 0)
	indexBySKU := make(map[SKU]int)
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
				items[index].CurrentPrices = make(map[OutputResolution]PriceInfo)
			}
			items[index].CurrentPrices[resolution] = price
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read billing catalog facts: %w", err)
	}
	return items, nil
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

func scanCatalogItem(row rowScanner) (CatalogItem, error) {
	var item CatalogItem
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
		return CatalogItem{}, err
	}
	if priceID.Valid && version.Valid && unitCredits.Valid && createdBy.Valid && priceCreatedAt.Valid && resolution.Valid {
		candidate := PriceInfo{
			PriceVersionID: priceID.Int64, Version: version.Int64, OutputResolution: OutputResolution(resolution.String), UnitCredits: unitCredits.Int64,
			CreatedBy: createdBy.Int64, CreatedAt: priceCreatedAt.Time,
		}
		if item.CurrentPrices == nil {
			item.CurrentPrices = make(map[OutputResolution]PriceInfo)
		}
		item.CurrentPrices[candidate.OutputResolution] = candidate
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
