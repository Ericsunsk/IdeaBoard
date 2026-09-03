package refund

import (
	"errors"
	"strings"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
)

// CreatePendingPaymentRefundInput is the durable refund reservation command.
type CreatePendingPaymentRefundInput struct {
	ActorID        int64
	OrderNumber    string
	IdeaQuantity   int64
	IdempotencyKey string
	Reason         string
	Now            time.Time
}

// MapCredentialError normalizes credential lookup/decryption failures to the
// public application error taxonomy.
func MapCredentialError(err error) error {
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) || errors.Is(err, paymentcontracts.ErrRefundOrderNotEligible) {
		return err
	}
	return paymentcontracts.ErrRefundOrderNotEligible
}

// NormalizePaymentRefundCommand validates and canonicalizes user refund input
// before it reaches persistence or a provider adapter.
func NormalizePaymentRefundCommand(command paymentcontracts.CreatePaymentRefundCommand) (paymentcontracts.CreatePaymentRefundCommand, error) {
	command.OrderNumber = strings.TrimSpace(command.OrderNumber)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ActorID <= 0 || command.OrderNumber == "" || command.IdeaQuantity <= 0 {
		return paymentcontracts.CreatePaymentRefundCommand{}, paymentcontracts.ErrRefundAmountInvalid
	}
	if len(command.IdempotencyKey) < 8 || len(command.IdempotencyKey) > 160 {
		return paymentcontracts.CreatePaymentRefundCommand{}, paymentcontracts.ErrRefundIdempotency
	}
	return command, nil
}
