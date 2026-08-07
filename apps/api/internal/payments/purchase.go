package payments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const (
	MinimumPurchaseIdeas       int64 = 500
	MaximumPurchaseIdeas       int64 = 500_000
	IdeasPerUSD                int64 = 100
	CurrentTermsVersion              = "2026-08-01"
	CurrentRefundPolicyVersion       = "2026-08-01"
)

var QuickPurchaseIdeaQuantities = [...]int64{1_000, 2_000, 3_000, 5_000, 10_000}

var (
	ErrInvalidIdeaQuantity         = errors.New("充值 Idea 数量必须在 500 到 500,000 之间")
	ErrTermsAcceptanceRequired     = errors.New("请先同意服务条款与退款政策")
	ErrCheckoutIdempotencyKey      = errors.New("充值请求标识无效")
	ErrCheckoutIdempotencyConflict = errors.New("充值请求与原请求不一致")
	ErrCheckoutRequestRejected     = errors.New("Stripe 拒绝创建支付页面")
	ErrPurchasesUnavailable        = errors.New("充值服务暂不可用")
	ErrWalletNotRechargeable       = errors.New("当前账户暂不可充值")
)

type PurchaseQuote struct {
	IdeaQuantity    int64  `json:"idea_quantity"`
	BaseCurrency    string `json:"base_currency"`
	BaseAmountMinor int64  `json:"base_amount_minor"`
}

type CreateCheckoutCommand struct {
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
	if command.UserID <= 0 {
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
		UserID              int64  `json:"user_id"`
		IdeaQuantity        int64  `json:"idea_quantity"`
		BaseCurrency        string `json:"base_currency"`
		BaseAmountMinor     int64  `json:"base_amount_minor"`
		TermsVersion        string `json:"terms_version"`
		RefundPolicyVersion string `json:"refund_policy_version"`
	}{
		UserID: command.UserID, IdeaQuantity: quote.IdeaQuantity,
		BaseCurrency: quote.BaseCurrency, BaseAmountMinor: quote.BaseAmountMinor,
		TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
