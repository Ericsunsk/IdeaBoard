package stripe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net/http"
	"strings"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/stripe/stripe-go/v86"
)

const (
	productResourceKey   = "ideaboard_resource"
	productResourceValue = "idea_purchase"
)

type Adapter struct {
	newClient func(string) *stripe.Client
	recorder  *Recorder
}

func New() *Adapter {
	return &Adapter{
		newClient: func(key string) *stripe.Client { return stripe.NewClient(key) },
		recorder:  NewRecorder(),
	}
}

// observe records one finished SDK call in outbound RED. It stays nil-safe
// for Adapters built without New (test stubs).
func (a *Adapter) observe(err error, startedAt time.Time) {
	if a == nil || a.recorder == nil {
		NewRecorder().Observe(err, time.Since(startedAt))
		return
	}
	a.recorder.Observe(err, time.Since(startedAt))
}

func (a *Adapter) PrepareConfiguration(ctx context.Context, input paymentports.PrepareStripeConfiguration) (paymentports.PreparedStripeConfiguration, error) {
	if a == nil || a.newClient == nil {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, "Stripe client unavailable", nil)
	}
	client := a.newClient(input.APIKey)
	startedAt := time.Now()
	account, err := client.V1Accounts.Retrieve(ctx, nil)
	a.observe(err, startedAt)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, mapStripeError(errorrender.MsgPrepareConfiguration, err)
	}
	if account == nil || strings.TrimSpace(account.ID) == "" {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgPrepareConfiguration2, nil)
	}
	if !account.ChargesEnabled {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorCapabilityMissing, errorrender.MsgPrepareConfiguration3, nil)
	}
	startedAt = time.Now()
	paymentMethodConfiguration, err := client.V1PaymentMethodConfigurations.Retrieve(ctx, input.PaymentMethodConfigurationID, nil)
	a.observe(err, startedAt)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, mapStripeError(errorrender.MsgPrepareConfiguration4, err)
	}
	if paymentMethodConfiguration == nil || paymentMethodConfiguration.ID != input.PaymentMethodConfigurationID ||
		paymentMethodConfiguration.Livemode != (input.Mode == paymentports.StripeModeLive) || !paymentMethodConfiguration.Active {
		return paymentports.PreparedStripeConfiguration{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgPrepareConfiguration5, nil)
	}
	product, err := a.ensureProduct(ctx, client, input, account.ID)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, err
	}
	endpoint, secret, err := a.ensureWebhookEndpoint(ctx, client, input, account.ID)
	if err != nil {
		return paymentports.PreparedStripeConfiguration{}, err
	}
	return paymentports.PreparedStripeConfiguration{
		AccountID: account.ID, Mode: input.Mode, ChargesEnabled: account.ChargesEnabled,
		PaymentMethodConfigurationID: paymentMethodConfiguration.ID,
		ProductID:                    product.ID, WebhookEndpointID: endpoint.ID, WebhookSecret: secret,
	}, nil
}

func (a *Adapter) ensureProduct(ctx context.Context, client *stripe.Client, input paymentports.PrepareStripeConfiguration, accountID string) (*stripe.Product, error) {
	if input.ExistingAccountID == accountID && strings.TrimSpace(input.ExistingProductID) != "" {
		startedAt := time.Now()
		product, err := client.V1Products.Retrieve(ctx, input.ExistingProductID, nil)
		a.observe(err, startedAt)
		if err == nil && productMatchesMode(product, input.Mode) {
			return a.updateProduct(ctx, client, product.ID, input.ProductTaxCode)
		}
		if err != nil && !isStripeNotFound(err) {
			return nil, mapStripeError(errorrender.MsgEnsureProduct, err)
		}
	}

	listStartedAt := time.Now()
	list := client.V1Products.List(ctx, &stripe.ProductListParams{
		ListParams: stripe.ListParams{Limit: stripe.Int64(100), Single: true}, Active: stripe.Bool(true),
	})
	for _, product := range list.Data() {
		if product != nil && product.Metadata[productResourceKey] == productResourceValue && productMatchesMode(product, input.Mode) {
			a.observe(list.Err(), listStartedAt)
			return a.updateProduct(ctx, client, product.ID, input.ProductTaxCode)
		}
	}
	if err := list.Err(); err != nil {
		a.observe(err, listStartedAt)
		return nil, mapStripeError(errorrender.MsgEnsureProduct2, err)
	}
	a.observe(nil, listStartedAt)

	params := &stripe.ProductCreateParams{
		Name: stripe.String("Idea"), Description: stripe.String("IdeaBoard Idea purchase"),
		TaxCode: stripe.String(input.ProductTaxCode), Active: stripe.Bool(true), UnitLabel: stripe.String("Idea"),
		Metadata: map[string]string{productResourceKey: productResourceValue},
	}
	params.SetIdempotencyKey("ideaboard-product-" + accountID)
	createStartedAt := time.Now()
	product, err := client.V1Products.Create(ctx, params)
	a.observe(err, createStartedAt)
	if err != nil {
		return nil, mapStripeError(errorrender.MsgEnsureProduct3, err)
	}
	if product == nil || strings.TrimSpace(product.ID) == "" {
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgEnsureProduct4, nil)
	}
	return product, nil
}

func (a *Adapter) updateProduct(ctx context.Context, client *stripe.Client, productID, taxCode string) (*stripe.Product, error) {
	startedAt := time.Now()
	product, err := client.V1Products.Update(ctx, productID, &stripe.ProductUpdateParams{
		Name: stripe.String("Idea"), Description: stripe.String("IdeaBoard Idea purchase"),
		TaxCode: stripe.String(taxCode), Active: stripe.Bool(true), UnitLabel: stripe.String("Idea"),
		Metadata: map[string]string{productResourceKey: productResourceValue},
	})
	a.observe(err, startedAt)
	if err != nil {
		return nil, mapStripeError(errorrender.MsgUpdateProduct, err)
	}
	if product == nil || strings.TrimSpace(product.ID) == "" {
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgEnsureProduct4, nil)
	}
	return product, nil
}

func (a *Adapter) ensureWebhookEndpoint(ctx context.Context, client *stripe.Client, input paymentports.PrepareStripeConfiguration, accountID string) (*stripe.WebhookEndpoint, string, error) {
	if input.ExistingAccountID == accountID && strings.TrimSpace(input.ExistingWebhookEndpointID) != "" && strings.TrimSpace(input.ExistingWebhookSecret) != "" {
		startedAt := time.Now()
		endpoint, err := client.V1WebhookEndpoints.Retrieve(ctx, input.ExistingWebhookEndpointID, nil)
		a.observe(err, startedAt)
		if err == nil && endpoint != nil && endpoint.APIVersion == stripe.APIVersion && endpointMatchesMode(endpoint, input.Mode) {
			updated, updateErr := a.updateWebhookEndpoint(ctx, client, endpoint.ID, input.WebhookURL)
			if updateErr != nil {
				return nil, "", updateErr
			}
			return updated, input.ExistingWebhookSecret, nil
		}
		if err != nil && !isStripeNotFound(err) {
			return nil, "", mapStripeError(errorrender.MsgEnsureWebhookEndpoint, err)
		}
	}

	params := &stripe.WebhookEndpointCreateParams{
		URL: stripe.String(input.WebhookURL), APIVersion: stripe.String(stripe.APIVersion),
		Description: stripe.String("IdeaBoard payment events"), EnabledEvents: stringPointers(paymentports.SupportedStripeWebhookEventTypes()),
		Metadata: map[string]string{productResourceKey: "payment_webhook"},
	}
	params.SetIdempotencyKey(webhookIdempotencyKey(accountID, input.WebhookURL))
	createStartedAt := time.Now()
	endpoint, err := client.V1WebhookEndpoints.Create(ctx, params)
	a.observe(err, createStartedAt)
	if err != nil {
		return nil, "", mapStripeError(errorrender.MsgEnsureWebhookEndpoint2, err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ID) == "" || strings.TrimSpace(endpoint.Secret) == "" {
		return nil, "", paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgEnsureWebhookEndpoint3, nil)
	}
	return endpoint, endpoint.Secret, nil
}

func (a *Adapter) updateWebhookEndpoint(ctx context.Context, client *stripe.Client, endpointID, webhookURL string) (*stripe.WebhookEndpoint, error) {
	startedAt := time.Now()
	endpoint, err := client.V1WebhookEndpoints.Update(ctx, endpointID, &stripe.WebhookEndpointUpdateParams{
		URL: stripe.String(webhookURL), Description: stripe.String("IdeaBoard payment events"),
		Disabled: stripe.Bool(false), EnabledEvents: stringPointers(paymentports.SupportedStripeWebhookEventTypes()),
		Metadata: map[string]string{productResourceKey: "payment_webhook"},
	})
	a.observe(err, startedAt)
	if err != nil {
		return nil, mapStripeError(errorrender.MsgUpdateWebhookEndpoint, err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ID) == "" {
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgEnsureWebhookEndpoint3, nil)
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
		return paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, operation+errorrender.MsgMapStripeError, err)
	}
	code := paymentports.GatewayErrorProviderUnavailable
	message := operation + errorrender.MsgMapStripeError
	switch stripeErr.HTTPStatusCode {
	case http.StatusUnauthorized:
		code, message = paymentports.GatewayErrorInvalidKey, errorrender.MsgMapStripeError2
	case http.StatusForbidden:
		code, message = paymentports.GatewayErrorPermissionDenied, errorrender.MsgMapStripeError3
	case http.StatusBadRequest, http.StatusNotFound:
		code, message = paymentports.GatewayErrorInvalidResponse, errorrender.MsgMapStripeError4
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
			operation+errorrender.MsgMapStripeCreateError,
			err,
		)
	default:
		return mapStripeError(operation, err)
	}
}
