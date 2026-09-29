package credentials

import (
	"context"
	"errors"
	"testing"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/secrets"
	secretstest "ideaboard/api/internal/testsupport/secretstest"
)

type stubStore struct {
	byID     map[string]paymentconfiguration.StoredStripeConfiguration
	byOrder  map[string]paymentconfiguration.StoredStripeConfiguration
	byNumber map[string]paymentconfiguration.StoredStripeConfiguration
	list     []paymentconfiguration.StoredStripeConfiguration
	err      error
}

func (s *stubStore) GetStripeCredential(_ context.Context, id string) (paymentconfiguration.StoredStripeConfiguration, error) {
	if s.err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, s.err
	}
	stored, ok := s.byID[id]
	if !ok {
		return paymentconfiguration.StoredStripeConfiguration{}, errors.New("missing credential")
	}
	return stored, nil
}

func (s *stubStore) GetStripeCredentialForOrder(_ context.Context, id string) (paymentconfiguration.StoredStripeConfiguration, error) {
	if s.err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, s.err
	}
	stored, ok := s.byOrder[id]
	if !ok {
		return paymentconfiguration.StoredStripeConfiguration{}, errors.New("missing credential")
	}
	return stored, nil
}

func (s *stubStore) GetStripeCredentialForOrderNumber(_ context.Context, number string) (paymentconfiguration.StoredStripeConfiguration, error) {
	if s.err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, s.err
	}
	stored, ok := s.byNumber[number]
	if !ok {
		return paymentconfiguration.StoredStripeConfiguration{}, errors.New("missing credential")
	}
	return stored, nil
}

func (s *stubStore) ListStripeCredentials(context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.list, nil
}

func encryptedStored(t *testing.T, apiKey, webhookSecret string) paymentconfiguration.StoredStripeConfiguration {
	t.Helper()
	secretstest.EnableEncryption(t)
	apiCipher, err := secrets.EncryptString(apiKey)
	if err != nil {
		t.Fatalf("encrypt api key: %v", err)
	}
	webhookCipher, err := secrets.EncryptString(webhookSecret)
	if err != nil {
		t.Fatalf("encrypt webhook secret: %v", err)
	}
	return paymentconfiguration.StoredStripeConfiguration{
		CredentialID: "cred_test", AccountID: "acct_test",
		APIKeyCiphertext: apiCipher, WebhookSecretCiphertext: webhookCipher,
		SecretMaterialKind: secrets.MaterialKindEncrypted,
	}
}

func TestNewResolverRejectsNilStore(t *testing.T) {
	if _, err := NewResolver(nil); err == nil {
		t.Fatal("NewResolver accepted nil store")
	}
}

func TestResolverRecoversAPIKeyThroughOneBoundary(t *testing.T) {
	stored := encryptedStored(t, "sk_test_key", "whsec_test")
	resolver, err := NewResolver(&stubStore{
		byID:     map[string]paymentconfiguration.StoredStripeConfiguration{"cred_test": stored},
		byOrder:  map[string]paymentconfiguration.StoredStripeConfiguration{"order_test": stored},
		byNumber: map[string]paymentconfiguration.StoredStripeConfiguration{"PO-TEST": stored},
		list:     []paymentconfiguration.StoredStripeConfiguration{stored},
	})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	ctx := context.Background()
	if _, key, err := resolver.ForCredentialID(ctx, "cred_test"); err != nil || key != "sk_test_key" {
		t.Fatalf("ForCredentialID key = %q, err = %v", key, err)
	}
	if _, key, err := resolver.ForOrder(ctx, "order_test"); err != nil || key != "sk_test_key" {
		t.Fatalf("ForOrder key = %q, err = %v", key, err)
	}
	if _, key, err := resolver.ForOrderNumber(ctx, "PO-TEST"); err != nil || key != "sk_test_key" {
		t.Fatalf("ForOrderNumber key = %q, err = %v", key, err)
	}
	if secret, err := DecryptWebhookSecret(stored.WebhookSecretCiphertext, stored.SecretMaterialKind); err != nil || secret != "whsec_test" {
		t.Fatalf("DecryptWebhookSecret = %q, err = %v", secret, err)
	}
	if got, err := resolver.ListCredentials(ctx); err != nil || len(got) != 1 {
		t.Fatalf("ListCredentials = %d rows, err = %v", len(got), err)
	}
}

func TestResolverNormalizesLookupFailures(t *testing.T) {
	resolver, err := NewResolver(&stubStore{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	ctx := context.Background()
	if _, _, err := resolver.ForCredentialID(ctx, "missing"); !errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) {
		t.Fatalf("ForCredentialID err = %v, want ErrStripeConfigurationInvalid", err)
	}
	if _, _, err := resolver.ForOrder(ctx, "missing"); !errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) {
		t.Fatalf("ForOrder err = %v, want ErrStripeConfigurationInvalid", err)
	}
	if _, _, err := resolver.ForOrderNumber(ctx, "missing"); !errors.Is(err, paymentcontracts.ErrRefundOrderNotEligible) {
		t.Fatalf("ForOrderNumber err = %v, want ErrRefundOrderNotEligible", err)
	}
}

func TestResolverDecryptFailureStaysInvalid(t *testing.T) {
	broken := paymentconfiguration.StoredStripeConfiguration{CredentialID: "cred_broken", AccountID: "acct_broken", APIKeyCiphertext: "not-ciphertext", SecretMaterialKind: secrets.MaterialKindEncrypted}
	resolver, err := NewResolver(&stubStore{
		byID: map[string]paymentconfiguration.StoredStripeConfiguration{"cred_broken": broken},
	})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if _, _, err := resolver.ForCredentialID(context.Background(), "cred_broken"); !errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) {
		t.Fatalf("decrypt failure err = %v, want ErrStripeConfigurationInvalid", err)
	}
}
