// Package aigateway owns the Cloudflare AI Gateway runtime configuration:
// the gateway identity (account plus gateway IDs) and the gateway token.
// Values live in platform system settings so rotation needs no redeploy;
// the token is encrypted at rest and never leaves the process in plaintext
// except toward Cloudflare. Reads are masked summaries; request-time reads
// resolve the full credential or fail closed.
package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ErrAIGatewayNotConfigured reports missing gateway configuration. Callers
// fail closed on it: no fallback to direct provider egress exists.
var ErrAIGatewayNotConfigured = errors.New("AI gateway is not configured")

var (
	gatewayAccountIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	gatewayIDPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// GatewaySummary is the masked admin read model. It never carries secrets.
type GatewaySummary struct {
	BaseURL         string     `json:"base_url,omitempty"`
	AccountID       string     `json:"account_id,omitempty"`
	GatewayID       string     `json:"gateway_id,omitempty"`
	TokenConfigured bool       `json:"token_configured"`
	IsFirstParty    bool       `json:"is_first_party"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// ResolvedGateway is the request-time credential handed to the gateway client.
type ResolvedGateway struct {
	BaseURL   string
	AccountID string
	GatewayID string
	Token     string
}

// UpdateInput carries an admin token rotation. A nil Token keeps the stored
// secret; there is no clear-token operation (rotation means replacement).
type UpdateInput struct {
	BaseURL   string
	Token     *string
	AccountID string
	GatewayID string
}

// ConfigRepository persists gateway configuration in system settings.
type ConfigRepository struct {
	settings *settings.SystemConfigRepository
}

// NewConfigRepository builds the repository over platform settings.
func NewConfigRepository(settingsRepository *settings.SystemConfigRepository) *ConfigRepository {
	return &ConfigRepository{settings: settingsRepository}
}

// GetSummary returns the masked configuration for admin display.
func (r *ConfigRepository) GetSummary(ctx context.Context) (GatewaySummary, error) {
	if r == nil || r.settings == nil {
		return GatewaySummary{}, errors.New("AI gateway config repository is unavailable")
	}
	values, err := r.settings.GetMany(ctx, []string{
		configKeyBaseURL, configKeyAccountID, configKeyGatewayID, configKeyTokenCiphertext, configKeyUpdatedAt,
	})
	if err != nil {
		return GatewaySummary{}, fmt.Errorf("load AI gateway config: %w", err)
	}
	baseURL := strings.TrimSpace(values[configKeyBaseURL])
	accountID := strings.TrimSpace(values[configKeyAccountID])
	gatewayID := strings.TrimSpace(values[configKeyGatewayID])
	if baseURL == "" && accountID != "" && gatewayID != "" {
		baseURL = gatewayBaseURL(accountID, gatewayID)
	}
	summary := GatewaySummary{
		BaseURL:         baseURL,
		AccountID:       accountID,
		GatewayID:       gatewayID,
		TokenConfigured: strings.TrimSpace(values[configKeyTokenCiphertext]) != "",
	}
	// 身份派生归后端：沿用包内网关 URL 正则，自定义网关恒为 false。
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
func (r *ConfigRepository) Resolve(ctx context.Context) (ResolvedGateway, error) {
	if r == nil || r.settings == nil {
		return ResolvedGateway{}, errors.New("AI gateway config repository is unavailable")
	}
	values, err := r.settings.GetMany(ctx, []string{
		configKeyBaseURL, configKeyAccountID, configKeyGatewayID, configKeyTokenCiphertext, configKeyTokenKind,
	})
	if err != nil {
		return ResolvedGateway{}, fmt.Errorf("load AI gateway config: %w", err)
	}
	baseURL := strings.TrimSpace(values[configKeyBaseURL])
	accountID := strings.TrimSpace(values[configKeyAccountID])
	gatewayID := strings.TrimSpace(values[configKeyGatewayID])
	if baseURL == "" && accountID != "" && gatewayID != "" {
		baseURL = gatewayBaseURL(accountID, gatewayID)
	}
	ciphertext := strings.TrimSpace(values[configKeyTokenCiphertext])
	if baseURL == "" || ciphertext == "" {
		return ResolvedGateway{}, ErrAIGatewayNotConfigured
	}
	token, err := secrets.DecryptMaterialString(ciphertext, strings.TrimSpace(values[configKeyTokenKind]))
	if err != nil {
		return ResolvedGateway{}, fmt.Errorf("decrypt AI gateway token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return ResolvedGateway{}, ErrAIGatewayNotConfigured
	}
	return ResolvedGateway{
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

// validateUpdateInput checks actor identity and gateway identity without
// touching storage. Errors are safe to display.
func validateUpdateInput(input UpdateInput, actorID int64) (baseURL, accountID, gatewayID string, token string, rotate bool, err error) {
	if actorID <= 0 {
		return "", "", "", "", false, errors.New("admin actor is required")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	accountID = strings.ToLower(strings.TrimSpace(input.AccountID))
	gatewayID = strings.ToLower(strings.TrimSpace(input.GatewayID))

	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return "", "", "", "", false, errors.New("网关 Base URL 必须是合法的 http 或 https 地址")
		}
		parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "v1" {
			if accountID == "" && gatewayAccountIDPattern.MatchString(parts[1]) {
				accountID = parts[1]
			}
			if gatewayID == "" && gatewayIDPattern.MatchString(parts[2]) {
				gatewayID = parts[2]
			}
		}
		if gatewayID == "" {
			// 自定义 egress 没有网关身份：只有符合网关 ID 形态的 hostname
			// 才回填，避免带点主机名污染后续的身份匹配。
			if gatewayIDPattern.MatchString(parsed.Hostname()) {
				gatewayID = strings.ToLower(parsed.Hostname())
			}
		}
	} else {
		if !gatewayAccountIDPattern.MatchString(accountID) {
			return "", "", "", "", false, errors.New("网关 account id 必须是 32 位小写十六进制")
		}
		if !gatewayIDPattern.MatchString(gatewayID) {
			return "", "", "", "", false, errors.New("网关 id 仅允许小写字母、数字与连字符")
		}
		baseURL = gatewayBaseURL(accountID, gatewayID)
	}

	if input.Token != nil {
		token = strings.TrimSpace(*input.Token)
		if token == "" {
			return "", "", "", "", false, errors.New("网关 token 不能为空")
		}
		rotate = true
	}
	return baseURL, accountID, gatewayID, token, rotate, nil
}

// Update validates and persists gateway configuration atomically with its
// admin audit record. Validation errors are safe to display.
//
// Merge rule: an empty base_url keeps the stored one (the Card form only
// edits CF identity), so switching egress flavor requires an explicit
// base_url through the dialog form. Token rotates only when provided.
func (r *ConfigRepository) Update(ctx context.Context, input UpdateInput, actorID int64) error {
	if r == nil || r.settings == nil {
		return errors.New("AI gateway config repository is unavailable")
	}
	if strings.TrimSpace(input.BaseURL) == "" {
		stored, err := r.GetSummary(ctx)
		if err == nil && strings.TrimSpace(stored.BaseURL) != "" {
			input.BaseURL = stored.BaseURL
		}
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
		"operation":     "save",
		"base_url":      baseURL,
		"gateway_id":    gatewayID,
		"token_rotated": tokenRotated,
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
