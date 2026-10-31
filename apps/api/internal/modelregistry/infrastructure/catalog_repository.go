package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/platform/database"
	"strings"

	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/workflow"
)

// visionMappingKey isolates the vision-search model. Fresh DB: no legacy
// rows exist.
const visionMappingKey = "ai_gateway_vision_model"

// ResolveVisionModel returns the admin-configured vision search model.
// Storage is isolated at visionMappingKey. Unset means fail closed.
func (r *ConfigRepository) ResolveVisionModel(ctx context.Context) (string, error) {
	if r == nil || r.settings == nil {
		return "", errors.New("AI gateway config repository is unavailable")
	}
	raw, err := r.settings.Get(ctx, visionMappingKey)
	if err != nil {
		return "", fmt.Errorf("load AI gateway vision model mapping: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", domain.ErrVisionModelMissing
	}
	return strings.TrimSpace(raw), nil
}

// catalogConfigKey stores the JSON array of available model items.
const catalogConfigKey = "ai_gateway_model_catalog"

// ListCatalog returns the configured model catalog items, falling back to
// the empty default catalog when nothing is configured. Stored JSON that no
// longer parses is a data-integrity error, never silently replaced: replacing
// it with defaults would let the next save permanently erase operator data.
func (r *ConfigRepository) ListCatalog(ctx context.Context) ([]domain.CatalogItem, error) {
	if r == nil || r.settings == nil {
		return nil, errors.New("AI gateway config repository is unavailable")
	}
	raw, err := r.settings.Get(ctx, catalogConfigKey)
	if err != nil {
		if database.IsNotFound(err) {
			return domain.DefaultCatalog(), nil
		}
		return nil, fmt.Errorf("load AI gateway model catalog: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return domain.DefaultCatalog(), nil
	}
	var items []domain.CatalogItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("stored AI gateway model catalog is corrupt: %w", err)
	}
	return items, nil
}

// SaveCatalog validates and persists the model catalog atomically with an audit action.
const (
	maxCatalogItems        = 500
	maxCatalogIDLength     = 128
	maxCatalogTextField    = 2000
	maxCatalogCapabilities = 32
)

func validateCatalogItemBounds(index int, item domain.CatalogItem) error {
	for field, value := range map[string]string{
		"name": item.Name, "provider": item.Provider, "mode": item.Mode,
		"description": item.Description, "context_or_resolution": item.ContextOrResolution,
		"hosting": item.Hosting, "terms_url": item.TermsURL, "docs_url": item.DocsURL,
		"custom_path": item.CustomPath, "custom_params": item.CustomParams,
	} {
		if len(value) > maxCatalogTextField {
			return fmt.Errorf("item %d %s exceeds %d characters", index, field, maxCatalogTextField)
		}
	}
	if len(item.Capabilities) > maxCatalogCapabilities {
		return fmt.Errorf("item %d exceeds %d capabilities", index, maxCatalogCapabilities)
	}
	return nil
}

// normalizeCatalogItems is the catalog write validation seam, independent of storage.
func normalizeCatalogItems(items []domain.CatalogItem) ([]domain.CatalogItem, error) {
	if len(items) > maxCatalogItems {
		return nil, fmt.Errorf("model catalog exceeds %d items", maxCatalogItems)
	}
	seenModels := make(map[string]struct{}, len(items))
	validated := make([]domain.CatalogItem, 0, len(items))
	for i := range items {
		item := items[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Provider = strings.ToLower(strings.TrimSpace(item.Provider))
		if item.ID == "" {
			return nil, fmt.Errorf("item %d is missing ID", i)
		}
		if len(item.ID) > maxCatalogIDLength {
			return nil, fmt.Errorf("item %d ID exceeds %d characters", i, maxCatalogIDLength)
		}
		if item.Provider == "" || strings.ContainsAny(item.Provider, "/@ \t\n") {
			return nil, fmt.Errorf("item %d requires a provider slug", i)
		}
		key := item.Provider + "\x00" + strings.ToLower(item.ID)
		if _, dup := seenModels[key]; dup {
			return nil, fmt.Errorf("duplicate model %q", workflow.FormatPoolModel(item.Provider, item.ID))
		}
		seenModels[key] = struct{}{}
		if item.Name == "" {
			item.Name = item.ID
		}
		if err := validateCatalogItemBounds(i, item); err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.CustomParams) != "" {
			var decoded any
			if err := json.Unmarshal([]byte(item.CustomParams), &decoded); err != nil {
				return nil, fmt.Errorf("item %d custom_params is not valid JSON: %w", i, err)
			}
			if _, ok := decoded.(map[string]any); !ok {
				return nil, fmt.Errorf("item %d custom_params must be a JSON object", i)
			}
		}
		validated = append(validated, item)
	}
	return validated, nil
}

func (r *ConfigRepository) SaveCatalog(ctx context.Context, items []domain.CatalogItem, actorID int64) error {
	if r == nil || r.settings == nil {
		return errors.New("AI gateway config repository is unavailable")
	}
	if actorID <= 0 {
		return errors.New("admin actor is required")
	}
	validated, err := normalizeCatalogItems(items)
	if err != nil {
		return err
	}
	copy(items, validated)
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode AI gateway model catalog: %w", err)
	}
	return r.settings.WithAIGatewayCatalogWrite(ctx, func(tx settings.SystemConfigWriteTx) (settings.SystemConfigAdminAudit, error) {
		// Read and prune under the same catalog-write lock so a valid mapping
		// update cannot be deleted using a stale pre-lock snapshot.
		removedMappingKeys, err := staleMappingKeysForRemovedModels(ctx, tx, items)
		if err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("read AI gateway catalog mappings: %w", err)
		}
		metadata, err := json.Marshal(map[string]any{"operation": "save_catalog", "count": len(items), "removed_mappings": removedMappingKeys})
		if err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("encode AI gateway catalog audit: %w", err)
		}
		if err := tx.SetMany(ctx, []settings.SystemConfigValue{{Key: catalogConfigKey, Value: string(encoded)}}); err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("save AI gateway model catalog: %w", err)
		}
		if err := tx.DeleteMany(ctx, removedMappingKeys); err != nil {
			return settings.SystemConfigAdminAudit{}, fmt.Errorf("delete stale AI gateway vision mapping: %w", err)
		}
		return settings.SystemConfigAdminAudit{
			ActionType: "update_ai_gateway_model",
			ActorID:    actorID,
			Metadata:   metadata,
		}, nil
	})
}

// staleMappingKeysForRemovedModels returns storage keys that point at models
// absent from the incoming catalog. Corrupt catalog JSON skips pruning so an
// admin can replace it; database errors abort the transaction.
func staleMappingKeysForRemovedModels(ctx context.Context, tx settings.SystemConfigWriteTx, items []domain.CatalogItem) ([]string, error) {
	keep := make(map[string]struct{}, len(items))
	for _, item := range items {
		keep[strings.ToLower(workflow.FormatPoolModel(item.Provider, item.ID))] = struct{}{}
	}
	rawCatalog, err := tx.Get(ctx, catalogConfigKey)
	oldItems := domain.DefaultCatalog()
	if err != nil && !database.IsNotFound(err) {
		return nil, fmt.Errorf("load current catalog: %w", err)
	}
	if err == nil && strings.TrimSpace(rawCatalog) != "" {
		if err := json.Unmarshal([]byte(rawCatalog), &oldItems); err != nil {
			oldItems = nil
		}
	}
	removed := make(map[string]struct{})
	for _, item := range oldItems {
		if id := strings.ToLower(workflow.FormatPoolModel(item.Provider, item.ID)); strings.TrimSpace(item.ID) != "" {
			if _, ok := keep[id]; !ok {
				removed[id] = struct{}{}
			}
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}
	visionModel, err := tx.Get(ctx, visionMappingKey)
	if err != nil && !database.IsNotFound(err) {
		return nil, fmt.Errorf("load current vision mapping: %w", err)
	}
	if err == nil {
		if _, ok := removed[strings.ToLower(strings.TrimSpace(visionModel))]; ok {
			return []string{visionMappingKey}, nil
		}
	}
	return nil, nil
}
