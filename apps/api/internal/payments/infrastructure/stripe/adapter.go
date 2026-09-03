package stripe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/stripe/stripe-go/v86"
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

func (a *Adapter) PrepareConfiguration(ctx context.Context, input paymentports.PrepareStripeConfiguration) (paymentports.PreparedStripeConfiguration, error) {
	if a == nil || a.newClient == nil {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, "Stripe client unavailable", nil)
	}
	client := a.newClient(input.APIKey)
	account, err := client.V1Accounts.Retrieve(ctx, nil)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, mapStripeError("验证 Stripe 账户", err)
	}
	if account == nil || strings.TrimSpace(account.ID) == "" {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe 账户响应无效", nil)
	}
	if !account.ChargesEnabled {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorCapabilityMissing, "Stripe 账户尚未启用收款", nil)
	}
	paymentMethodConfiguration, err := client.V1PaymentMethodConfigurations.Retrieve(ctx, input.PaymentMethodConfigurationID, nil)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, mapStripeError("读取 Stripe Payment Method Configuration", err)
	}
	if paymentMethodConfiguration == nil || paymentMethodConfiguration.ID != input.PaymentMethodConfigurationID ||
		paymentMethodConfiguration.Livemode != (input.Mode == paymentports.StripeModeLive) || !paymentMethodConfiguration.Active {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Payment Method Configuration 无效或未启用", nil)
	}
	product, err := ensureProduct(ctx, client, input, account.ID)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, err
	}
	endpoint, secret, err := ensureWebhookEndpoint(ctx, client, input, account.ID)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, err
	}
	return paymentports.PreparedStripeConfiguration{
		AccountID: account.ID, Mode: input.Mode, ChargesEnabled: account.ChargesEnabled,
		PaymentMethodConfigurationID: paymentMethodConfiguration.ID,
		ProductID:                    product.ID, WebhookEndpointID: endpoint.ID, WebhookSecret: secret,
	}, nil
}

func ensureProduct(ctx context.Context, client *stripe.Client, input paymentports.PrepareStripeConfiguration, accountID string) (*stripe.Product, error) {
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
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Product 响应无效", nil)
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
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Product 响应无效", nil)
	}
	return product, nil
}

func ensureWebhookEndpoint(ctx context.Context, client *stripe.Client, input paymentports.PrepareStripeConfiguration, accountID string) (*stripe.WebhookEndpoint, string, error) {
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
		Description: stripe.String("IdeaBoard payment events"), EnabledEvents: stringPointers(paymentports.SupportedStripeWebhookEventTypes()),
		Metadata: map[string]string{productResourceKey: "payment_webhook"},
	}
	params.SetIdempotencyKey(webhookIdempotencyKey(accountID, input.WebhookURL))
	endpoint, err := client.V1WebhookEndpoints.Create(ctx, params)
	if err != nil {
		return nil, "", mapStripeError("创建 Stripe Webhook Endpoint", err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ID) == "" || strings.TrimSpace(endpoint.Secret) == "" {
		return nil, "", paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Webhook Endpoint 响应无效", nil)
	}
	return endpoint, endpoint.Secret, nil
}

func updateWebhookEndpoint(ctx context.Context, client *stripe.Client, endpointID, webhookURL string) (*stripe.WebhookEndpoint, error) {
	endpoint, err := client.V1WebhookEndpoints.Update(ctx, endpointID, &stripe.WebhookEndpointUpdateParams{
		URL: stripe.String(webhookURL), Description: stripe.String("IdeaBoard payment events"),
		Disabled: stripe.Bool(false), EnabledEvents: stringPointers(paymentports.SupportedStripeWebhookEventTypes()),
		Metadata: map[string]string{productResourceKey: "payment_webhook"},
	})
	if err != nil {
		return nil, mapStripeError("更新 Stripe Webhook Endpoint", err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ID) == "" {
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Webhook Endpoint 响应无效", nil)
	}
	return endpoint, nil
}

func productMatchesMode(product *stripe.Product, mode paymentports.StripeMode) bool {
	return product != nil && product.Livemode == (mode == paymentports.StripeModeLive)
}

func endpointMatchesMode(endpoint *stripe.WebhookEndpoint, mode paymentports.StripeMode) bool {
	return endpoint != nil && endpoint.Livemode == (mode == paymentports.StripeModeLive)
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
		return paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, operation+"失败，请稍后重试", err)
	}
	code := paymentports.GatewayErrorProviderUnavailable
	message := operation + "失败，请稍后重试"
	switch stripeErr.HTTPStatusCode {
	case http.StatusUnauthorized:
		code, message = paymentports.GatewayErrorInvalidKey, "Stripe API Key 无效"
	case http.StatusForbidden:
		code, message = paymentports.GatewayErrorPermissionDenied, "Stripe API Key 权限不足"
	case http.StatusBadRequest, http.StatusNotFound:
		code, message = paymentports.GatewayErrorInvalidResponse, "Stripe 拒绝了当前配置"
	}
	return paymentports.NewGatewayError(code, message, err)
}

func mapStripeCreateError(operation string, err error) error {
	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) {
		return mapStripeError(operation, err)
	}
	if stripeErr.Type == stripe.ErrorTypeIdempotency {
		return mapStripeError(operation, err)
	}
	switch stripeErr.HTTPStatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return paymentports.NewGatewayError(
			paymentports.GatewayErrorRequestRejected,
			operation+" 被 Stripe 拒绝",
			err,
		)
	default:
		return mapStripeError(operation, err)
	}
}
