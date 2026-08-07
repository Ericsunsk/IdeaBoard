package payments

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type StripeMode string

const (
	StripeModeTest StripeMode = "test"
	StripeModeLive StripeMode = "live"
)

var (
	ErrStripeConfigurationNotFound = errors.New("尚未完成 Stripe 配置")
	ErrStripeAPIKeyRequired        = errors.New("请填写 Stripe API Key")
	ErrInvalidStripeAPIKey         = errors.New("无效的 Stripe API Key 格式")
	ErrInvalidProductTaxCode       = errors.New("无效的 Stripe Product Tax Code 格式")
	ErrInvalidWebhookURL           = errors.New("无效的 Stripe Webhook URL")
	ErrStripePermissionDenied      = errors.New("当前 Stripe API Key 权限不足")
	ErrStripeAccountUnavailable    = errors.New("当前 Stripe 账户不可用")
	ErrStripeCapabilityUnavailable = errors.New("当前 Stripe 账户暂不具备收款能力")
	ErrStripeProviderUnavailable   = errors.New("当前 Stripe 服务暂时不可用")
	ErrStripeConfigurationInvalid  = errors.New("收到的 Stripe 配置不完整")
)

var productTaxCodePattern = regexp.MustCompile(`^txcd_[0-9]{8}$`)

const maxStripeAPIKeyLength = 512

type StripeConfigurationSummary struct {
	Configured              bool       `json:"configured"`
	APIKeyConfigured        bool       `json:"api_key_configured"`
	WebhookSecretConfigured bool       `json:"webhook_secret_configured"`
	AccountID               string     `json:"account_id,omitempty"`
	Mode                    StripeMode `json:"mode,omitempty"`
	ChargesEnabled          bool       `json:"charges_enabled"`
	ProductTaxCode          string     `json:"product_tax_code"`
	ProductReady            bool       `json:"product_ready"`
	WebhookReady            bool       `json:"webhook_ready"`
	PurchasesEnabled        bool       `json:"purchases_enabled"`
	UpdatedAt               time.Time  `json:"updated_at,omitempty"`
}

type ConfigureStripeCommand struct {
	ActorID          int64
	APIKey           string
	ProductTaxCode   string
	PurchasesEnabled bool
	WebhookURL       string
}

type PrepareStripeConfiguration struct {
	APIKey                    string
	Mode                      StripeMode
	PurchasesEnabled          bool
	ProductTaxCode            string
	WebhookURL                string
	ExistingAccountID         string
	ExistingProductID         string
	ExistingWebhookEndpointID string
	ExistingWebhookSecret     string
}

type PreparedStripeConfiguration struct {
	AccountID         string
	Mode              StripeMode
	ChargesEnabled    bool
	ProductID         string
	WebhookEndpointID string
	WebhookSecret     string
}

type StripeGateway interface {
	PrepareConfiguration(ctx context.Context, input PrepareStripeConfiguration) (PreparedStripeConfiguration, error)
}

type GatewayErrorCode string

const (
	GatewayErrorInvalidKey          GatewayErrorCode = "invalid_key"
	GatewayErrorPermissionDenied    GatewayErrorCode = "permission_denied"
	GatewayErrorAccountUnavailable  GatewayErrorCode = "account_unavailable"
	GatewayErrorCapabilityMissing   GatewayErrorCode = "capability_missing"
	GatewayErrorProviderUnavailable GatewayErrorCode = "provider_unavailable"
	GatewayErrorInvalidResponse     GatewayErrorCode = "invalid_response"
	GatewayErrorRequestRejected     GatewayErrorCode = "request_rejected"
)

type GatewayError struct {
	code        GatewayErrorCode
	safeMessage string
	cause       error
}

func NewGatewayError(code GatewayErrorCode, safeMessage string, cause error) *GatewayError {
	return &GatewayError{code: code, safeMessage: strings.TrimSpace(safeMessage), cause: cause}
}

func (e *GatewayError) Error() string {
	if e == nil || e.safeMessage == "" {
		return "Stripe operation failed"
	}
	return e.safeMessage
}

func (e *GatewayError) Code() GatewayErrorCode {
	if e == nil {
		return GatewayErrorProviderUnavailable
	}
	return e.code
}

func ClassifyStripeAPIKey(raw string) (StripeMode, error) {
	key := strings.TrimSpace(raw)
	if len(key) > maxStripeAPIKeyLength {
		return "", ErrInvalidStripeAPIKey
	}
	switch {
	case strings.HasPrefix(key, "sk_test_") && len(key) > len("sk_test_"):
		return StripeModeTest, nil
	case strings.HasPrefix(key, "rk_live_") && len(key) > len("rk_live_"):
		return StripeModeLive, nil
	default:
		return "", ErrInvalidStripeAPIKey
	}
}

func normalizeProductTaxCode(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !productTaxCodePattern.MatchString(value) {
		return "", ErrInvalidProductTaxCode
	}
	return value, nil
}

func normalizeWebhookURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidWebhookURL
	}
	if parsed.Path != "/api/webhooks/stripe" {
		return "", ErrInvalidWebhookURL
	}
	hostname := strings.ToLower(parsed.Hostname())
	isLoopback := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopback) {
		return "", ErrInvalidWebhookURL
	}
	return parsed.String(), nil
}

func validatePreparedConfiguration(prepared PreparedStripeConfiguration, expectedMode StripeMode) error {
	if strings.TrimSpace(prepared.AccountID) == "" || prepared.Mode != expectedMode ||
		strings.TrimSpace(prepared.ProductID) == "" || strings.TrimSpace(prepared.WebhookEndpointID) == "" ||
		strings.TrimSpace(prepared.WebhookSecret) == "" {
		return ErrStripeConfigurationInvalid
	}
	if !prepared.ChargesEnabled {
		return ErrStripeCapabilityUnavailable
	}
	return nil
}

func gatewayFailure(err error) (GatewayErrorCode, string, error) {
	var gatewayErr *GatewayError
	if !errors.As(err, &gatewayErr) {
		return GatewayErrorProviderUnavailable, "Stripe 服务暂时不可用", ErrStripeProviderUnavailable
	}
	summary := gatewayErr.Error()
	if summary == "" {
		summary = "Stripe operation failed"
	}
	switch gatewayErr.Code() {
	case GatewayErrorInvalidKey:
		return gatewayErr.Code(), summary, ErrInvalidStripeAPIKey
	case GatewayErrorPermissionDenied:
		return gatewayErr.Code(), summary, ErrStripePermissionDenied
	case GatewayErrorAccountUnavailable:
		return gatewayErr.Code(), summary, ErrStripeAccountUnavailable
	case GatewayErrorCapabilityMissing:
		return gatewayErr.Code(), summary, ErrStripeCapabilityUnavailable
	case GatewayErrorInvalidResponse:
		return gatewayErr.Code(), summary, ErrStripeConfigurationInvalid
	default:
		return GatewayErrorProviderUnavailable, summary, ErrStripeProviderUnavailable
	}
}

func wrapPublicError(public error, summary string) error {
	if strings.TrimSpace(summary) == "" {
		return public
	}
	return fmt.Errorf("%w: %s", public, summary)
}
