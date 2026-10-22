package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const (
	// Fixed fiat-to-Idea recharge schedule: 500~500,000 Ideas at 100
	// Ideas per USD (BaseAmountMinor equals IdeaQuantity in USD cents).
	// This is the Payments-owned fiat boundary and intentionally stays
	// outside the Billing SKU catalog: Billing prices Idea-to-service
	// consumption (unit Credits per SKU/resolution), Payments prices
	// fiat-to-Idea recharge. Quick presets reuse the same 100:1 rate.
	MinimumPurchaseIdeas       int64 = 500
	MaximumPurchaseIdeas       int64 = 500_000
	IdeasPerUSD                int64 = 100
	CurrentTermsVersion              = "2026-10-23"
	CurrentRefundPolicyVersion       = "2026-10-23"
)

var QuickPurchaseIdeaQuantities = [...]int64{1_000, 2_000, 3_000, 5_000, 10_000}

var (
	ErrInvalidIdeaQuantity         = errors.New(msgInvalidIdeaQuantity)
	ErrTermsAcceptanceRequired     = errors.New(msgTermsAcceptanceRequired)
	ErrCheckoutIdempotencyKey      = errors.New(msgCheckoutIdempotencyKey)
	ErrCheckoutIdempotencyConflict = errors.New(msgCheckoutIdempotencyConflict)
	ErrCheckoutRequestRejected     = errors.New(msgCheckoutRequestRejected)
	ErrPurchasesUnavailable        = errors.New(msgPurchasesUnavailable)
	ErrWalletNotRechargeable       = errors.New(msgWalletNotRechargeable)
)

type PurchaseQuote struct {
	IdeaQuantity    int64  `json:"idea_quantity"`
	BaseCurrency    string `json:"base_currency"`
	BaseAmountMinor int64  `json:"base_amount_minor"`
}

type CreateCheckoutCommand struct {
	// Payer funds identity: the Billing Account that owns the unified
	// balance and scopes idempotency, unpaid slots, and settlement.
	BillingAccountID int64
	// Operator attribution: the real User performing the checkout.
	UserID               int64
	ClientIdempotencyKey string
	IdeaQuantity         int64
	TermsAccepted        bool
	TermsVersion         string
	RefundPolicyVersion  string
	ReturnBaseURL        string
}

func NewPurchaseQuote(ideaQuantity int64) (PurchaseQuote, error) {
	if ideaQuantity < MinimumPurchaseIdeas || ideaQuantity > MaximumPurchaseIdeas {
		return PurchaseQuote{}, ErrInvalidIdeaQuantity
	}
	return PurchaseQuote{
		IdeaQuantity:    ideaQuantity,
		BaseCurrency:    "usd",
		BaseAmountMinor: ideaQuantity,
	}, nil
}

func CheckoutRequestFingerprint(command CreateCheckoutCommand) (string, error) {
	if command.BillingAccountID <= 0 || command.UserID <= 0 {
		return "", ErrCheckoutIdempotencyKey
	}
	idempotencyKey := strings.TrimSpace(command.ClientIdempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 160 {
		return "", ErrCheckoutIdempotencyKey
	}
	quote, err := NewPurchaseQuote(command.IdeaQuantity)
	if err != nil {
		return "", err
	}
	if !command.TermsAccepted || strings.TrimSpace(command.TermsVersion) != CurrentTermsVersion ||
		strings.TrimSpace(command.RefundPolicyVersion) != CurrentRefundPolicyVersion {
		return "", ErrTermsAcceptanceRequired
	}
	payload, err := json.Marshal(struct {
		BillingAccountID    int64  `json:"billing_account_id"`
		IdeaQuantity        int64  `json:"idea_quantity"`
		BaseCurrency        string `json:"base_currency"`
		BaseAmountMinor     int64  `json:"base_amount_minor"`
		TermsVersion        string `json:"terms_version"`
		RefundPolicyVersion string `json:"refund_policy_version"`
	}{
		BillingAccountID: command.BillingAccountID, IdeaQuantity: quote.IdeaQuantity,
		BaseCurrency: quote.BaseCurrency, BaseAmountMinor: quote.BaseAmountMinor,
		TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
