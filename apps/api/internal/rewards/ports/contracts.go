// Package ports defines the service interfaces (seams) and public DTOs
// that drive Rewards context operations. Application services implement these,
// and callers (HTTP handlers, bootstrap wiring) depend on these ports.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrWalletNotFound = errors.New("rewards credit wallet not found")

// CodeLocker defines transaction-scoped advisory lock operations for reward codes.
type CodeLocker interface {
	LockRewardCodeID(ctx context.Context, tx pgx.Tx, rewardCodeID int64) error
	LockRewardCodeIdentity(ctx context.Context, tx pgx.Tx, normalized string) error
}

type RewardCodeStatus string

const (
	RewardCodeStatusExpired   RewardCodeStatus = "expired"
	RewardCodeStatusExhausted RewardCodeStatus = "exhausted"
	RewardCodeStatusDisabled  RewardCodeStatus = "disabled"
	RewardCodeStatusEnabled   RewardCodeStatus = "enabled"
)

// AdminCommands exposes reward-code administrative write operations.
type AdminCommands interface {
	CreateRewardCode(context.Context, CreateRewardCodeCommand) (CreatedRewardCode, error)
	UpdateRewardCode(context.Context, UpdateRewardCodeCommand) (UpdatedRewardCode, error)
	SetRewardCodeEnabled(context.Context, SetRewardCodeEnabledCommand) (RewardCode, error)
	DeleteRewardCode(context.Context, DeleteRewardCodeCommand) (DeletedRewardCode, error)
}

// AdminReads exposes reward-code administrative read operations.
type AdminReads interface {
	SearchRewardCodes(context.Context, SearchRewardCodesInput) (RewardCodePage, error)
	ListAdminRedemptions(context.Context, ListAdminRedemptionsInput) (AdminRedemptionPage, error)
	GetAdminRedemption(context.Context, GetAdminRedemptionInput) (AdminRedemptionDetail, error)
}

// UserCommands exposes end-user reward operations.
type UserCommands interface {
	Redeem(context.Context, RedeemCommand) (RedeemResult, error)
}

// UserReads exposes end-user reward history read operations.
type UserReads interface {
	ListRedemptions(context.Context, ListRedemptionsInput) (RedemptionHistoryPage, error)
}

// UserService combines user-facing read and write seams.
type UserService interface {
	UserCommands
	UserReads
}

// AdminCommandService exposes the admin command seam.
type AdminCommandService interface {
	AdminCommands
}

// AdminReadService exposes the admin read seam.
type AdminReadService interface {
	AdminReads
}

// IdentityProjectionReader resolves user identity details for rewards display.
type IdentityProjectionReader interface {
	ResolveIdentity(context.Context, int64) (IdentityProjection, error)
	ResolveIdentities(context.Context, []int64) (map[int64]IdentityProjection, error)
}

// IdentityProjection is a read-only user identity snapshot used by rewards.
type IdentityProjection struct {
	PublicID    uuid.UUID
	DisplayName string
}

// --- Reward Code Admin DTOs ---

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
	CreatedBy       int64            `json:"-"`
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

type UpdateRewardCodeCommand struct {
	RewardCodeID   int64
	DisplayName    string
	CreditAmount   int64
	MaxRedemptions int64
	ExpiresAt      *time.Time
	Code           *string
	ActorUserID    int64
}

type UpdatedRewardCode struct {
	RewardCode RewardCode `json:"reward_code"`
	Plaintext  string     `json:"plaintext,omitempty"`
	Replayed   *bool      `json:"replayed,omitempty"`
}

type SetRewardCodeEnabledCommand struct {
	RewardCodeID int64
	Enabled      bool
	ActorUserID  int64
}

type DeleteRewardCodeCommand struct {
	RewardCodeID int64
	ActorUserID  int64
}

type DeletedRewardCode struct {
	RewardCodeID int64 `json:"reward_code_id"`
}

type SearchRewardCodesInput struct {
	Query    string
	Status   RewardCodeStatus
	BeforeID int64
	Limit    int
}

type RewardCodePage struct {
	RewardCodes  []RewardCode `json:"reward_codes"`
	Total        int64        `json:"total"`
	NextBeforeID int64        `json:"next_before_id,omitempty"`
}

type AdminRedemptionSummary struct {
	ID             int64     `json:"id"`
	RewardCodeID   int64     `json:"reward_code_id"`
	UserID         int64     `json:"-"`
	UserPublicID   uuid.UUID `json:"user_public_id"`
	DisplayName    string    `json:"display_name"`
	CreditedAmount int64     `json:"credited_amount"`
	RedeemedAt     time.Time `json:"redeemed_at"`
}

type ListAdminRedemptionsInput struct {
	RewardCodeID int64
	BeforeID     int64
	Limit        int
}

type AdminRedemptionPage struct {
	Redemptions  []AdminRedemptionSummary `json:"redemptions"`
	NextBeforeID int64                    `json:"next_before_id,omitempty"`
}

type GetAdminRedemptionInput struct {
	RewardCodeID int64
	RedemptionID int64
}

type AdminRedemptionDetail struct {
	ID                  int64     `json:"id"`
	RewardCodeID        int64     `json:"reward_code_id"`
	UserID              int64     `json:"-"`
	UserPublicID        uuid.UUID `json:"user_public_id"`
	DisplayName         string    `json:"display_name"`
	CreditedAmount      int64     `json:"credited_amount"`
	RedeemedAt          time.Time `json:"redeemed_at"`
	CreditTransactionID string    `json:"credit_transaction_id"`
	MaskedCodeHint      string    `json:"masked_code_hint"`
}

// --- Redemption User DTOs ---

type RedeemCommand struct {
	UserID int64
	Code   string
}

type Redemption struct {
	ID                    int64     `json:"id"`
	RewardCodeID          int64     `json:"reward_code_id"`
	UserID                int64     `json:"-"`
	UserPublicID          uuid.UUID `json:"user_public_id"`
	DisplayName           string    `json:"display_name"`
	RewardCodeDisplayName string    `json:"-"`
	MaskedCodeHint        string    `json:"-"`
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
	// BillingAccountID is the internal beneficiary Billing Account that
	// received the Grant: the caller's deterministic initial personal
	// account (#916/#921). The HTTP edge maps it to the ba_ public ID so
	// the UI can name the actual credited account; the requesting
	// workspace is authZ scope only and never changes the beneficiary.
	BillingAccountID int64 `json:"billing_account_id"`
	Replayed         bool  `json:"replayed"`
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
