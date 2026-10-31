// Package infrastructure owns the modelregistry persistence adapters: the
// gateway configuration repository over platform system settings. Domain
// owns the DTOs and narrow seams; this package implements them.
package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/platform/errorrender"
	"net/url"
	"regexp"
	"strings"
	"time"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/platform/settings"
)

// System settings keys. Dev and prod are separate databases, so one key set
// serves both: per-environment isolation falls out of deployment topology.
const (
	configKeyBaseURL         = "ai_gateway_base_url"
	configKeyAccountID       = "ai_gateway_account_id"
	configKeyGatewayID       = "ai_gateway_gateway_id"
	configKeyTokenCiphertext = "ai_gateway_token_ciphertext"
	configKeyTokenKind       = "ai_gateway_token_material_kind"
	configKeyUpdatedAt       = "ai_gateway_updated_at"
)

var (
	gatewayAccountIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	gatewayIDPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// ConfigRepository persists gateway configuration in system settings.
type ConfigRepository struct {
	settings *settings.SystemConfigRepository
}

// NewConfigRepository builds the repository over platform settings.
func NewConfigRepository(settingsRepository *settings.SystemConfigRepository) *ConfigRepository {
	return &ConfigRepository{settings: settingsRepository}
}

// GetSummary returns the masked configuration for admin display.
func (r *ConfigRepository) GetSummary(ctx context.Context) (domain.GatewaySummary, error) {
	if r == nil || r.settings == nil {
		return domain.GatewaySummary{}, errors.New("AI gateway config repository is unavailable")
	}
	values, err := r.settings.GetMany(ctx, []string{
		configKeyBaseURL, configKeyAccountID, configKeyGatewayID, configKeyTokenCiphertext, configKeyUpdatedAt,
	})
	if err != nil {
		return domain.GatewaySummary{}, fmt.Errorf("load AI gateway config: %w", err)
	}
	baseURL := strings.TrimSpace(values[configKeyBaseURL])
	accountID := strings.TrimSpace(values[configKeyAccountID])
	gatewayID := strings.TrimSpace(values[configKeyGatewayID])
	if baseURL == "" && accountID != "" && gatewayID != "" {
		baseURL = gatewayBaseURL(accountID, gatewayID)
	}
	summary := domain.GatewaySummary{
		BaseURL:         baseURL,
		AccountID:       accountID,
		GatewayID:       gatewayID,
		TokenConfigured: strings.TrimSpace(values[configKeyTokenCiphertext]) != "",
	}
	// Gateway identity derives from the stored URL path
	// /v1/{account}/{gateway}; custom hosts stay non-first-party.
	if accountID, gatewayID := extractGatewayIdentity(baseURL); accountID != "" && gatewayID != "" {
		summary.IsFirstParty = true
	}
	if raw := strings.TrimSpace(values[configKeyUpdatedAt]); raw != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			summary.UpdatedAt = &parsed
		}
	}
	return summary, nil
}

// Resolve loads the full credential for request-time use. Missing or
// undecryptable configuration fails closed.
func (r *ConfigRepository) Resolve(ctx context.Context) (domain.ResolvedGateway, error) {
	if r == nil || r.settings == nil {
		return domain.ResolvedGateway{}, errors.New("AI gateway config repository is unavailable")
	}
	values, err := r.settings.GetMany(ctx, []string{
		configKeyBaseURL, configKeyAccountID, configKeyGatewayID, configKeyTokenCiphertext, configKeyTokenKind,
	})
	if err != nil {
		return domain.ResolvedGateway{}, fmt.Errorf("load AI gateway config: %w", err)
	}
	baseURL := strings.TrimSpace(values[configKeyBaseURL])
	accountID := strings.TrimSpace(values[configKeyAccountID])
	gatewayID := strings.TrimSpace(values[configKeyGatewayID])
	if baseURL == "" && accountID != "" && gatewayID != "" {
		baseURL = gatewayBaseURL(accountID, gatewayID)
	}
	ciphertext := strings.TrimSpace(values[configKeyTokenCiphertext])
	if baseURL == "" || ciphertext == "" {
		return domain.ResolvedGateway{}, domain.ErrAIGatewayNotConfigured
	}
	token, err := secrets.DecryptMaterialString(ciphertext, strings.TrimSpace(values[configKeyTokenKind]))
	if err != nil {
		return domain.ResolvedGateway{}, fmt.Errorf("decrypt AI gateway token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return domain.ResolvedGateway{}, domain.ErrAIGatewayNotConfigured
	}
	return domain.ResolvedGateway{
		BaseURL:   baseURL,
		AccountID: accountID,
		GatewayID: gatewayID,
		Token:     token,
	}, nil
}

// gatewayBaseURL renders the gateway root for one account/gateway pair.
func gatewayBaseURL(accountID, gatewayID string) string {
	return "https://gateway.ai.cloudflare.com/v1/" + accountID + "/" + gatewayID
}

// gatewayHostForAudit returns the lowercase hostname of a base URL, or ""
// when it does not parse. Audit-only: never carries secrets.
func gatewayHostForAudit(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(parsed.Hostname()), "."))
}

// validateUpdateInput checks actor identity and gateway identity without
// touching storage. Errors are safe to display.
//
// Base URL policy is open-host: any http(s) URL with a host passes the
// scheme/host check (Msg3) plus the credential/query/fragment check
// (Msg4). There is no host allowlist; URL changes rely on the admin
// audit record (actor + old/new host) instead of prevention.
func validateUpdateInput(input domain.UpdateInput, actorID int64) (baseURL, accountID, gatewayID string, token string, rotate bool, err error) {
	if actorID <= 0 {
		return "", "", "", "", false, errors.New("admin actor is required")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	accountID = strings.ToLower(strings.TrimSpace(input.AccountID))
	gatewayID = strings.ToLower(strings.TrimSpace(input.GatewayID))
	if accountID != "" && !gatewayAccountIDPattern.MatchString(accountID) {
		return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput)
	}
	if gatewayID != "" && !gatewayIDPattern.MatchString(gatewayID) {
		return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput2)
	}

	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput3)
		}
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput4)
		}
		// Any host passes; /v1/{account}/{gateway} paths fill missing IDs.
		parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "v1" {
			if accountID == "" && gatewayAccountIDPattern.MatchString(parts[1]) {
				accountID = parts[1]
			}
			if gatewayID == "" && gatewayIDPattern.MatchString(parts[2]) {
				gatewayID = parts[2]
			}
		}
	}
	if !gatewayAccountIDPattern.MatchString(accountID) {
		return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput)
	}
	if !gatewayIDPattern.MatchString(gatewayID) {
		return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput2)
	}
	if baseURL == "" {
		baseURL = gatewayBaseURL(accountID, gatewayID)
	}

	if input.Token != nil {
		token = strings.TrimSpace(*input.Token)
		if token == "" {
			return "", "", "", "", false, errors.New(errorrender.MsgValidateUpdateInput5)
		}
		rotate = true
	}
	return baseURL, accountID, gatewayID, token, rotate, nil
}

// Update validates and persists gateway configuration atomically with its
// admin audit record. Validation errors are safe to display.
//
// Merge rule: an empty base_url plus empty identity keeps the stored URL
// (token-only rotation). Explicit account/gateway IDs rebuild the
// Cloudflare URL when no base URL is given. Any http(s) host is accepted;
// URL changes are deterred by audit (actor + old/new host), not by an
// allowlist. Token rotates only when provided; the token value never
// enters logs or audit metadata (only a rotated boolean does).
func (r *ConfigRepository) Update(ctx context.Context, input domain.UpdateInput, actorID int64) error {
	if r == nil || r.settings == nil {
		return errors.New("AI gateway config repository is unavailable")
	}
	stored, loadErr := r.GetSummary(ctx)
	prevHost := ""
	if loadErr == nil {
		prevHost = gatewayHostForAudit(stored.BaseURL)
	}
	if strings.TrimSpace(input.BaseURL) == "" && strings.TrimSpace(input.AccountID) == "" && strings.TrimSpace(input.GatewayID) == "" {
		if loadErr != nil {
			return fmt.Errorf("load existing AI gateway config: %w", loadErr)
		}
		input.BaseURL = stored.BaseURL
	}
	baseURL, accountID, gatewayID, token, tokenRotated, err := validateUpdateInput(input, actorID)
	if err != nil {
		return err
	}
	values := []settings.SystemConfigValue{
		{Key: configKeyBaseURL, Value: baseURL},
		{Key: configKeyAccountID, Value: accountID},
		{Key: configKeyGatewayID, Value: gatewayID},
		{Key: configKeyUpdatedAt, Value: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	if tokenRotated {
		ciphertext, err := secrets.EncryptString(token)
		if err != nil {
			return fmt.Errorf("encrypt AI gateway token: %w", err)
		}
		values = append(values,
			settings.SystemConfigValue{Key: configKeyTokenCiphertext, Value: ciphertext},
			settings.SystemConfigValue{Key: configKeyTokenKind, Value: secrets.MaterialKindEncrypted},
		)
	}
	metadata, err := json.Marshal(map[string]any{
		"operation":          "save",
		"base_url":           baseURL,
		"base_url_host":      gatewayHostForAudit(baseURL),
		"prev_base_url_host": prevHost,
		"account_id":         accountID,
		"gateway_id":         gatewayID,
		"token_rotated":      tokenRotated,
	})
	if err != nil {
		return fmt.Errorf("encode AI gateway audit: %w", err)
	}
	return r.settings.SetManyWithAdminAudit(ctx, values, settings.SystemConfigAdminAudit{
		ActionType: "update_ai_gateway",
		ActorID:    actorID,
		Metadata:   metadata,
	})
}
