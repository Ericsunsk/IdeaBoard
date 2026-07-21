package rewards

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCommand     = errors.New("invalid reward command")
	ErrCodeCollision      = errors.New("reward Code is not unique")
	ErrRewardCodeConflict = errors.New("reward Code idempotency key conflicts with another payload")
	ErrCodeUnavailable    = errors.New("reward Code is invalid or unavailable")
)

type RewardCodeStatus string

const (
	RewardCodeStatusExpired   RewardCodeStatus = "expired"
	RewardCodeStatusExhausted RewardCodeStatus = "exhausted"
	RewardCodeStatusDisabled  RewardCodeStatus = "disabled"
	RewardCodeStatusEnabled   RewardCodeStatus = "enabled"
)

type RewardCode struct {
	ID              int64            `json:"id"`
	DisplayName     string           `json:"display_name"`
	MaskedCodeHint  string           `json:"masked_code_hint"`
	CreditAmount    int64            `json:"credit_amount"`
	MaxRedemptions  int64            `json:"max_redemptions"`
	RedemptionCount int64            `json:"redemption_count"`
	ExpiresAt       *time.Time       `json:"expires_at,omitempty"`
	Enabled         bool             `json:"enabled"`
	Status          RewardCodeStatus `json:"status"`
	CreatedBy       int64            `json:"created_by"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type CreatedRewardCode struct {
	RewardCode RewardCode `json:"reward_code"`
	Plaintext  string     `json:"plaintext"`
	Replayed   bool       `json:"replayed"`
}

type CreateRewardCodeCommand struct {
	DisplayName    string
	Code           string
	CreditAmount   int64
	MaxRedemptions int64
	ExpiresAt      *time.Time
	IdempotencyKey string
	ActorUserID    int64
}

type ListRewardCodesInput struct {
	BeforeID int64
	Limit    int
}

type RewardCodePage struct {
	RewardCodes  []RewardCode `json:"reward_codes"`
	NextBeforeID int64        `json:"next_before_id,omitempty"`
}

type Redemption struct {
	ID                    int64     `json:"id"`
	RewardCodeID          int64     `json:"reward_code_id"`
	UserID                int64     `json:"user_id"`
	CreditAmount          int64     `json:"credit_amount"`
	CreditTransactionID   string    `json:"credit_transaction_id"`
	AvailableBalanceAfter int64     `json:"available_balance_after"`
	RedeemedAt            time.Time `json:"redeemed_at"`
}

type RedeemResult struct {
	Redemption       Redemption            `json:"redemption"`
	HistoryItem      RedemptionHistoryItem `json:"history_item"`
	CreditedAmount   int64                 `json:"credited_amount"`
	AvailableBalance int64                 `json:"available_balance"`
	Replayed         bool                  `json:"replayed"`
}

type RedemptionHistoryItem struct {
	ID                    int64     `json:"id"`
	RewardCodeDisplayName string    `json:"reward_code_display_name"`
	CreditedAmount        int64     `json:"credited_amount"`
	MaskedCodeHint        string    `json:"masked_code_hint"`
	RedeemedAt            time.Time `json:"redeemed_at"`
}

type ListRedemptionsInput struct {
	UserID   int64
	BeforeID int64
	Limit    int
}

type RedemptionHistoryPage struct {
	Redemptions  []RedemptionHistoryItem `json:"redemptions"`
	NextBeforeID int64                   `json:"next_before_id,omitempty"`
}

type RedeemCommand struct {
	UserID int64
	Code   string
}

type AdminCommands interface {
	CreateRewardCode(context.Context, CreateRewardCodeCommand) (CreatedRewardCode, error)
}

type AdminReads interface {
	ListRewardCodes(context.Context, ListRewardCodesInput) (RewardCodePage, error)
}

type UserCommands interface {
	Redeem(context.Context, RedeemCommand) (RedeemResult, error)
}

type UserReads interface {
	ListRedemptions(context.Context, ListRedemptionsInput) (RedemptionHistoryPage, error)
}
