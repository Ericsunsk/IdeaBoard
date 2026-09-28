package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/outboundhttp"
)

// Models.dev reference seam: read-only price/context lookup for admin
// display. Results are reference data only, never billing input: billing
// charges solely on the billing SKU UnitCredits. Failures degrade to an
// empty result with an explicit error code and never block catalog flows.

// ModelsReferenceSource names the upstream of every reference result.
const ModelsReferenceSource = "models.dev"

// modelsReferenceNote marks every result as non-settlement data.
const modelsReferenceNote = msgPackage

// Machine-readable error codes for the degraded reference result.
const (
	// ModelsReferenceCodeAllowlist means models.dev is not in the egress
	// allowlist: the admin must add it under the outbound policy.
	ModelsReferenceCodeAllowlist = "allowlist_missing"
	// ModelsReferenceCodeUpstream means the upstream fetch failed: the
	// result is empty, the catalog flow is unaffected.
	ModelsReferenceCodeUpstream = "upstream_error"
	// ModelsReferenceCodeNotFound means the catalog answered but holds no
	// entry for the requested model.
	ModelsReferenceCodeNotFound = "not_found"
)

// ModelReferenceModalities mirrors the upstream modalities object verbatim
// ({input, output} string lists). Nil means the upstream entry carried no
// modalities: callers render 暂无数据 instead of guessing.
type ModelReferenceModalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// ModelReference is one models.dev lookup outcome. Found=false carries an
// ErrorCode explaining why (allowlist/upstream/not_found) instead of an
// HTTP error, so callers can render the hint inline.
// Enrichment fields (capability booleans, modalities, knowledge,
// release_date, family, status) are upstream原名直透 and all optional:
// nil/empty means unknown and the frontend renders 暂无数据. description
// is deliberately never透传: gateway运营文案优先.
type ModelReference struct {
	ModelID          string                    `json:"model_id"`
	Provider         string                    `json:"provider,omitempty"`
	Name             string                    `json:"name,omitempty"`
	InputUSDPerMTok  float64                   `json:"input_usd_per_mtok,omitempty"`
	OutputUSDPerMTok float64                   `json:"output_usd_per_mtok,omitempty"`
	ContextLimit     int                       `json:"context_limit,omitempty"`
	LastUpdated      string                    `json:"last_updated,omitempty"`
	Reasoning        *bool                     `json:"reasoning,omitempty"`
	ToolCall         *bool                     `json:"tool_call,omitempty"`
	Temperature      *bool                     `json:"temperature,omitempty"`
	Attachment       *bool                     `json:"attachment,omitempty"`
	StructuredOutput *bool                     `json:"structured_output,omitempty"`
	OpenWeights      *bool                     `json:"open_weights,omitempty"`
	Modalities       *ModelReferenceModalities `json:"modalities,omitempty"`
	Knowledge        string                    `json:"knowledge,omitempty"`
	ReleaseDate      string                    `json:"release_date,omitempty"`
	Family           string                    `json:"family,omitempty"`
	Status           string                    `json:"status,omitempty"`
	Source           string                    `json:"source"`
	Note             string                    `json:"note"`
	Found            bool                      `json:"found"`
	ErrorCode        string                    `json:"error_code,omitempty"`
	Message          string                    `json:"message,omitempty"`
}

// modelsDevCost mirrors the per-million-token USD pricing of one entry.
type modelsDevCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// modelsDevLimit mirrors the token limits of one entry.
type modelsDevLimit struct {
	Context int `json:"context"`
}

// modelsDevModalities mirrors the upstream modalities object verbatim.
type modelsDevModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// modelsDevModelEntry is one model row inside a provider slice. Only the
// fields mapped into ModelReference are decoded (description is never
// 透传); unknown upstream fields are ignored so feed additions don't break
// parsing.
type modelsDevModelEntry struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	LastUpdated      string               `json:"last_updated"`
	Cost             modelsDevCost        `json:"cost"`
	Limit            modelsDevLimit       `json:"limit"`
	Reasoning        *bool                `json:"reasoning"`
	ToolCall         *bool                `json:"tool_call"`
	Temperature      *bool                `json:"temperature"`
	Attachment       *bool                `json:"attachment"`
	StructuredOutput *bool                `json:"structured_output"`
	OpenWeights      *bool                `json:"open_weights"`
	Modalities       *modelsDevModalities `json:"modalities"`
	Knowledge        string               `json:"knowledge"`
	ReleaseDate      string               `json:"release_date"`
	Family           string               `json:"family"`
	Status           string               `json:"status"`
}

// modelsDevProviderEntry is one provider slice of the catalog.
type modelsDevProviderEntry struct {
	Models map[string]modelsDevModelEntry `json:"models"`
}

// modelsDevFact is the flattened lookup row cached in memory.
// Capability booleans stay pointers so "upstream omitted" (unknown) and
// false (unsupported) never collapse into each other.
type modelsDevFact struct {
	provider         string
	name             string
	lastUpdated      string
	input            float64
	output           float64
	context          int
	reasoning        *bool
	toolCall         *bool
	temperature      *bool
	attachment       *bool
	structuredOutput *bool
	openWeights      *bool
	modalities       *ModelReferenceModalities
	knowledge        string
	releaseDate      string
	family           string
	status           string
}

var modelsReferenceCache struct {
	sync.RWMutex
	facts     map[string]modelsDevFact
	fetchedAt time.Time
}

// resetModelsReferenceCache clears the in-memory catalog (tests only).
func resetModelsReferenceCache() {
	modelsReferenceCache.Lock()
	modelsReferenceCache.facts = nil
	modelsReferenceCache.fetchedAt = time.Time{}
	modelsReferenceCache.Unlock()
}

// GetModelReference resolves one model's reference facts from models.dev.
// modelID wins when set; otherwise provider+model compose "provider/model".
// Only invalid input returns an error; allowlist gaps, upstream failures
// and misses degrade into a Found=false result with an explicit ErrorCode.
func (r *ConfigRepository) GetModelReference(ctx context.Context, modelID, provider, model string, refresh bool) (ModelReference, error) {
	id := strings.TrimSpace(modelID)
	if id == "" {
		provider = strings.TrimSpace(provider)
		model = strings.TrimSpace(model)
		if model == "" {
			return ModelReference{}, errors.New(msgGetModelReference)
		}
		if provider != "" {
			id = provider + "/" + model
		} else {
			id = model
		}
	}
	base := ModelReference{ModelID: id, Source: ModelsReferenceSource, Note: modelsReferenceNote}

	if !modelsDevAllowlisted(ctx) {
		base.ErrorCode = ModelsReferenceCodeAllowlist
		base.Message = msgGetModelReference2
		return base, nil
	}
	facts, fetchErr := loadModelsReferenceFacts(ctx, refresh)
	if fetchErr != nil {
		base.ErrorCode = ModelsReferenceCodeUpstream
		base.Message = msgGetModelReference3
		//nolint:nilerr // upstream failure degrades to an explicit error_code by contract, never an error
		return base, nil
	}
	fact, ok := lookupModelsReferenceFact(facts, id)
	if !ok {
		base.ErrorCode = ModelsReferenceCodeNotFound
		base.Message = fmt.Sprintf(msgGetModelReference4, id)
		return base, nil
	}
	base.Provider = fact.provider
	base.Name = fact.name
	base.InputUSDPerMTok = fact.input
	base.OutputUSDPerMTok = fact.output
	base.ContextLimit = fact.context
	base.LastUpdated = fact.lastUpdated
	base.Reasoning = cloneReferenceBool(fact.reasoning)
	base.ToolCall = cloneReferenceBool(fact.toolCall)
	base.Temperature = cloneReferenceBool(fact.temperature)
	base.Attachment = cloneReferenceBool(fact.attachment)
	base.StructuredOutput = cloneReferenceBool(fact.structuredOutput)
	base.OpenWeights = cloneReferenceBool(fact.openWeights)
	base.Modalities = cloneReferenceModalities(fact.modalities)
	base.Knowledge = fact.knowledge
	base.ReleaseDate = fact.releaseDate
	base.Family = fact.family
	base.Status = fact.status
	base.Found = true
	return base, nil
}

// lookupModelsReferenceFact finds one row by exact (case-insensitive) ID,
// falling back to a unique "/<id>" suffix match so bare community keys
// (e.g. "gpt-4o") still resolve when unambiguous.
func lookupModelsReferenceFact(facts map[string]modelsDevFact, id string) (modelsDevFact, bool) {
	if fact, ok := facts[strings.ToLower(id)]; ok {
		return fact, true
	}
	suffix := "/" + strings.ToLower(strings.TrimSpace(id))
	var matched modelsDevFact
	count := 0
	for key, fact := range facts {
		if strings.HasSuffix(key, suffix) {
			matched, count = fact, count+1
			if count > 1 {
				return modelsDevFact{}, false
			}
		}
	}
	return matched, count == 1
}

// loadModelsReferenceFacts returns the flattened catalog, reusing a 24h
// entry unless refresh forces a refetch. Every failure is an error for the
// caller to degrade on.
func loadModelsReferenceFacts(ctx context.Context, refresh bool) (map[string]modelsDevFact, error) {
	if !refresh {
		modelsReferenceCache.RLock()
		if len(modelsReferenceCache.facts) > 0 && modelsDevCacheFresh(modelsReferenceCache.fetchedAt, modelsReferenceTTL) {
			cached := make(map[string]modelsDevFact, len(modelsReferenceCache.facts))
			for key, fact := range modelsReferenceCache.facts {
				cached[key] = fact
			}
			modelsReferenceCache.RUnlock()
			return cached, nil
		}
		modelsReferenceCache.RUnlock()
	}

	// Catalog uses client-default retries (MaxAttempts 0): see modelsDevGet.
	resp, err := modelsDevGet(ctx, modelsDevCatalogURL, 15*time.Second, 0, detectModelsReferenceTarget)
	if err != nil {
		var statusErr *outboundhttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return nil, fmt.Errorf(msgLoadModelsReferenceFacts, statusErr.StatusCode)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(msgLoadModelsReferenceFacts, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, errors.New(msgLoadModelsReferenceFacts2)
	}
	var raw map[string]modelsDevProviderEntry
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New(msgLoadModelsReferenceFacts3)
	}
	facts := make(map[string]modelsDevFact, 1024)
	for providerID, provider := range raw {
		for key, entry := range provider.Models {
			id := strings.TrimSpace(entry.ID)
			if id == "" {
				id = strings.TrimSpace(key)
			}
			if id == "" {
				continue
			}
			facts[strings.ToLower(id)] = modelsDevFact{
				provider:         strings.TrimSpace(providerID),
				name:             strings.TrimSpace(entry.Name),
				lastUpdated:      strings.TrimSpace(entry.LastUpdated),
				input:            entry.Cost.Input,
				output:           entry.Cost.Output,
				context:          entry.Limit.Context,
				reasoning:        cloneReferenceBool(entry.Reasoning),
				toolCall:         cloneReferenceBool(entry.ToolCall),
				temperature:      cloneReferenceBool(entry.Temperature),
				attachment:       cloneReferenceBool(entry.Attachment),
				structuredOutput: cloneReferenceBool(entry.StructuredOutput),
				openWeights:      cloneReferenceBool(entry.OpenWeights),
				modalities:       cloneReferenceModalities(modelsDevModalitiesToReference(entry.Modalities)),
				knowledge:        strings.TrimSpace(entry.Knowledge),
				releaseDate:      strings.TrimSpace(entry.ReleaseDate),
				family:           strings.TrimSpace(entry.Family),
				status:           strings.TrimSpace(entry.Status),
			}
		}
	}
	if len(facts) == 0 {
		return nil, errors.New(msgLoadModelsReferenceFacts4)
	}
	modelsReferenceCache.Lock()
	modelsReferenceCache.facts = facts
	modelsReferenceCache.fetchedAt = time.Now()
	modelsReferenceCache.Unlock()
	return facts, nil
}

// cloneReferenceBool copies an upstream capability bit so cached facts and
// served results never share a mutable pointer. Nil stays nil (unknown).
func cloneReferenceBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// modelsDevModalitiesToReference converts the upstream modalities object,
// dropping empty entries so callers can treat nil as "no data".
func modelsDevModalitiesToReference(modalities *modelsDevModalities) *ModelReferenceModalities {
	if modalities == nil {
		return nil
	}
	converted := &ModelReferenceModalities{
		Input:  append([]string(nil), modalities.Input...),
		Output: append([]string(nil), modalities.Output...),
	}
	if len(converted.Input) == 0 && len(converted.Output) == 0 {
		return nil
	}
	return converted
}

// cloneReferenceModalities deep-copies cached modalities per result.
func cloneReferenceModalities(modalities *ModelReferenceModalities) *ModelReferenceModalities {
	if modalities == nil {
		return nil
	}
	return &ModelReferenceModalities{
		Input:  append([]string(nil), modalities.Input...),
		Output: append([]string(nil), modalities.Output...),
	}
}
