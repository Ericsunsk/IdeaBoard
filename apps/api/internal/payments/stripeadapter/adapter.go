package stripeadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/payments"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
)

const (
	productResourceKey   = "ideaboard_resource"
	productResourceValue = "idea_purchase"
)

type Adapter struct {
	newClient func(string) *stripe.Client
}

func New() *Adapter {
	return &Adapter{newClient: func(key string) *stripe.Client { return stripe.NewClient(key) }}
}

func (a *Adapter) PrepareConfiguration(ctx context.Context, input payments.PrepareStripeConfiguration) (payments.PreparedStripeConfiguration, error) {
	if a == nil || a.newClient == nil {
		return payments.PreparedStripeConfiguration{}, payments.NewGatewayError(payments.GatewayErrorProviderUnavailable, "Stripe client unavailable", nil)
	}
	client := a.newClient(input.APIKey)
	account, err := client.V1Accounts.Retrieve(ctx, nil)
	if err != nil {
		return payments.PreparedStripeConfiguration{}, mapStripeError("验证 Stripe 账户", err)
	}
	if account == nil || strings.TrimSpace(account.ID) == "" {
		return payments.PreparedStripeConfiguration{}, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe 账户响应无效", nil)
	}
	if !account.ChargesEnabled {
		return payments.PreparedStripeConfiguration{}, payments.NewGatewayError(payments.GatewayErrorCapabilityMissing, "Stripe 账户尚未启用收款", nil)
	}

	product, err := ensureProduct(ctx, client, input, account.ID)
	if err != nil {
		return payments.PreparedStripeConfiguration{}, err
	}
	endpoint, secret, err := ensureWebhookEndpoint(ctx, client, input, account.ID)
	if err != nil {
		return payments.PreparedStripeConfiguration{}, err
	}
	return payments.PreparedStripeConfiguration{
		AccountID: account.ID, Mode: input.Mode, ChargesEnabled: account.ChargesEnabled,
		ProductID: product.ID, WebhookEndpointID: endpoint.ID, WebhookSecret: secret,
	}, nil
}

func (a *Adapter) Verify(_ context.Context, raw []byte, signature, secret string, tolerance time.Duration, _ time.Time) (payments.StripeWebhookEvent, error) {
	event, err := webhook.ConstructEventWithOptions(raw, signature, secret, webhook.ConstructEventOptions{
		Tolerance: tolerance,
	})
	if err != nil {
		switch {
		case errors.Is(err, webhook.ErrTooOld):
			return payments.StripeWebhookEvent{}, payments.ErrWebhookSignatureExpired
		case errors.Is(err, webhook.ErrInvalidHeader), errors.Is(err, webhook.ErrNoValidSignature), errors.Is(err, webhook.ErrNotSigned):
			return payments.StripeWebhookEvent{}, payments.ErrInvalidWebhookSignature
		default:
			return payments.StripeWebhookEvent{}, payments.ErrMalformedWebhookPayload
		}
	}
	var object struct {
		ID string `json:"id"`
	}
	if len(event.Data.Raw) > 0 {
		if err := json.Unmarshal(event.Data.Raw, &object); err != nil {
			return payments.StripeWebhookEvent{}, payments.ErrMalformedWebhookPayload
		}
	}
	return payments.StripeWebhookEvent{
		ID: event.ID, Type: string(event.Type), ObjectID: object.ID,
		Livemode: event.Livemode, CreatedAt: time.Unix(event.Created, 0).UTC(),
	}, nil
}

func ensureProduct(ctx context.Context, client *stripe.Client, input payments.PrepareStripeConfiguration, accountID string) (*stripe.Product, error) {
	if input.ExistingAccountID == accountID && strings.TrimSpace(input.ExistingProductID) != "" {
		product, err := client.V1Products.Retrieve(ctx, input.ExistingProductID, nil)
		if err == nil && productMatchesMode(product, input.Mode) {
			return updateProduct(ctx, client, product.ID, input.ProductTaxCode)
		}
		if err != nil && !isStripeNotFound(err) {
			return nil, mapStripeError("读取 Stripe Product", err)
		}
	}

	list := client.V1Products.List(ctx, &stripe.ProductListParams{
		ListParams: stripe.ListParams{Limit: stripe.Int64(100), Single: true}, Active: stripe.Bool(true),
	})
	for _, product := range list.Data() {
		if product != nil && product.Metadata[productResourceKey] == productResourceValue && productMatchesMode(product, input.Mode) {
			return updateProduct(ctx, client, product.ID, input.ProductTaxCode)
		}
	}
	if err := list.Err(); err != nil {
		return nil, mapStripeError("查找 Stripe Product", err)
	}

	params := &stripe.ProductCreateParams{
		Name: stripe.String("Idea"), Description: stripe.String("IdeaBoard Idea purchase"),
		TaxCode: stripe.String(input.ProductTaxCode), Active: stripe.Bool(true), UnitLabel: stripe.String("Idea"),
		Metadata: map[string]string{productResourceKey: productResourceValue},
	}
	params.SetIdempotencyKey("ideaboard-product-" + accountID)
	product, err := client.V1Products.Create(ctx, params)
	if err != nil {
		return nil, mapStripeError("创建 Stripe Product", err)
	}
	if product == nil || strings.TrimSpace(product.ID) == "" {
		return nil, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Product 响应无效", nil)
	}
	return product, nil
}

func updateProduct(ctx context.Context, client *stripe.Client, productID, taxCode string) (*stripe.Product, error) {
	product, err := client.V1Products.Update(ctx, productID, &stripe.ProductUpdateParams{
		Name: stripe.String("Idea"), Description: stripe.String("IdeaBoard Idea purchase"),
		TaxCode: stripe.String(taxCode), Active: stripe.Bool(true), UnitLabel: stripe.String("Idea"),
		Metadata: map[string]string{productResourceKey: productResourceValue},
	})
	if err != nil {
		return nil, mapStripeError("更新 Stripe Product", err)
	}
	if product == nil || strings.TrimSpace(product.ID) == "" {
		return nil, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Product 响应无效", nil)
	}
	return product, nil
}

func ensureWebhookEndpoint(ctx context.Context, client *stripe.Client, input payments.PrepareStripeConfiguration, accountID string) (*stripe.WebhookEndpoint, string, error) {
	if input.ExistingAccountID == accountID && strings.TrimSpace(input.ExistingWebhookEndpointID) != "" && strings.TrimSpace(input.ExistingWebhookSecret) != "" {
		endpoint, err := client.V1WebhookEndpoints.Retrieve(ctx, input.ExistingWebhookEndpointID, nil)
		if err == nil && endpoint != nil && endpoint.APIVersion == stripe.APIVersion && endpointMatchesMode(endpoint, input.Mode) {
			updated, updateErr := updateWebhookEndpoint(ctx, client, endpoint.ID, input.WebhookURL)
			if updateErr != nil {
				return nil, "", updateErr
			}
			return updated, input.ExistingWebhookSecret, nil
		}
		if err != nil && !isStripeNotFound(err) {
			return nil, "", mapStripeError("读取 Stripe Webhook Endpoint", err)
		}
	}

	params := &stripe.WebhookEndpointCreateParams{
		URL: stripe.String(input.WebhookURL), APIVersion: stripe.String(stripe.APIVersion),
		Description: stripe.String("IdeaBoard payment events"), EnabledEvents: stringPointers(payments.SupportedStripeWebhookEventTypes()),
		Metadata: map[string]string{productResourceKey: "payment_webhook"},
	}
	params.SetIdempotencyKey(webhookIdempotencyKey(accountID, input.WebhookURL))
	endpoint, err := client.V1WebhookEndpoints.Create(ctx, params)
	if err != nil {
		return nil, "", mapStripeError("创建 Stripe Webhook Endpoint", err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ID) == "" || strings.TrimSpace(endpoint.Secret) == "" {
		return nil, "", payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Webhook Endpoint 响应无效", nil)
	}
	return endpoint, endpoint.Secret, nil
}

func updateWebhookEndpoint(ctx context.Context, client *stripe.Client, endpointID, webhookURL string) (*stripe.WebhookEndpoint, error) {
	endpoint, err := client.V1WebhookEndpoints.Update(ctx, endpointID, &stripe.WebhookEndpointUpdateParams{
		URL: stripe.String(webhookURL), Description: stripe.String("IdeaBoard payment events"),
		Disabled: stripe.Bool(false), EnabledEvents: stringPointers(payments.SupportedStripeWebhookEventTypes()),
		Metadata: map[string]string{productResourceKey: "payment_webhook"},
	})
	if err != nil {
		return nil, mapStripeError("更新 Stripe Webhook Endpoint", err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ID) == "" {
		return nil, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Webhook Endpoint 响应无效", nil)
	}
	return endpoint, nil
}

func productMatchesMode(product *stripe.Product, mode payments.StripeMode) bool {
	return product != nil && product.Livemode == (mode == payments.StripeModeLive)
}

func endpointMatchesMode(endpoint *stripe.WebhookEndpoint, mode payments.StripeMode) bool {
	return endpoint != nil && endpoint.Livemode == (mode == payments.StripeModeLive)
}

func stringPointers(values []string) []*string {
	result := make([]*string, 0, len(values))
	for _, value := range values {
		result = append(result, stripe.String(value))
	}
	return result
}

func webhookIdempotencyKey(accountID, webhookURL string) string {
	sum := sha256.Sum256([]byte(webhookURL))
	return fmt.Sprintf("ideaboard-webhook-%s-%s", accountID, hex.EncodeToString(sum[:8]))
}

func isStripeNotFound(err error) bool {
	var stripeErr *stripe.Error
	return errors.As(err, &stripeErr) && stripeErr.HTTPStatusCode == http.StatusNotFound
}

func mapStripeError(operation string, err error) error {
	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) {
		return payments.NewGatewayError(payments.GatewayErrorProviderUnavailable, operation+"失败，请稍后重试", err)
	}
	code := payments.GatewayErrorProviderUnavailable
	message := operation + "失败，请稍后重试"
	switch stripeErr.HTTPStatusCode {
	case http.StatusUnauthorized:
		code, message = payments.GatewayErrorInvalidKey, "Stripe API Key 无效"
	case http.StatusForbidden:
		code, message = payments.GatewayErrorPermissionDenied, "Stripe API Key 权限不足"
	case http.StatusBadRequest:
		code, message = payments.GatewayErrorInvalidResponse, "Stripe 拒绝了当前配置"
	}
	return payments.NewGatewayError(code, message, err)
}
