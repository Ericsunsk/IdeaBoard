package rewards

import (
	"context"
	"errors"
	"time"
)

type CampaignMode string

const CampaignModePublic CampaignMode = "public"

type CampaignStatus string

const (
	CampaignStatusDraft  CampaignStatus = "draft"
	CampaignStatusActive CampaignStatus = "active"
)

var (
	ErrInvalidCommand     = errors.New("invalid reward command")
	ErrCampaignNotFound   = errors.New("reward campaign not found")
	ErrCampaignNotDraft   = errors.New("reward campaign is not draft")
	ErrPublicCodeExists   = errors.New("public reward code already exists")
	ErrPublicCodeRequired = errors.New("public reward code is required")
	ErrCodeUnavailable    = errors.New("reward code is invalid or unavailable")
	ErrCampaignExhausted  = errors.New("reward campaign is exhausted")
	ErrAlreadyRedeemed    = errors.New("reward campaign already redeemed")
)

type Campaign struct {
	ID              int64          `json:"id"`
	DisplayName     string         `json:"display_name"`
	InternalNote    string         `json:"internal_note"`
	CreditAmount    int64          `json:"credit_amount"`
	Mode            CampaignMode   `json:"mode"`
	Status          CampaignStatus `json:"status"`
	MaxRedemptions  int64          `json:"max_redemptions"`
	RedemptionCount int64          `json:"redemption_count"`
	CreatedBy       int64          `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type RewardCode struct {
	ID         int64     `json:"id"`
	CampaignID int64     `json:"campaign_id"`
	Hint       string    `json:"hint"`
	IssuedBy   int64     `json:"issued_by"`
	IssuedAt   time.Time `json:"issued_at"`
}

type IssuedPublicCode struct {
	Code      RewardCode `json:"code"`
	Plaintext string     `json:"plaintext"`
}

type Redemption struct {
	ID                    int64     `json:"id"`
	CampaignID            int64     `json:"campaign_id"`
	CodeID                int64     `json:"code_id"`
	UserID                int64     `json:"user_id"`
	CreditAmount          int64     `json:"credit_amount"`
	CreditTransactionID   string    `json:"credit_transaction_id"`
	AvailableBalanceAfter int64     `json:"available_balance_after"`
	RedeemedAt            time.Time `json:"redeemed_at"`
}

type RedeemResult struct {
	Redemption       Redemption `json:"redemption"`
	CreditedAmount   int64      `json:"credited_amount"`
	AvailableBalance int64      `json:"available_balance"`
	Replayed         bool       `json:"replayed"`
}

type ListCampaignsInput struct {
	BeforeID int64
	Limit    int
}

type CampaignPage struct {
	Campaigns    []Campaign `json:"campaigns"`
	NextBeforeID int64      `json:"next_before_id,omitempty"`
}

type CreateCampaignCommand struct {
	DisplayName    string
	InternalNote   string
	CreditAmount   int64
	MaxRedemptions int64
	ActorUserID    int64
	Reason         string
}

type IssuePublicCodeCommand struct {
	CampaignID  int64
	Code        string
	ActorUserID int64
	Reason      string
}

type ActivateCampaignCommand struct {
	CampaignID  int64
	ActorUserID int64
	Reason      string
}

type RedeemCommand struct {
	UserID int64
	Code   string
}

type AdminCommands interface {
	CreateCampaign(context.Context, CreateCampaignCommand) (Campaign, error)
	IssuePublicCode(context.Context, IssuePublicCodeCommand) (IssuedPublicCode, error)
	ActivateCampaign(context.Context, ActivateCampaignCommand) (Campaign, error)
}

type UserCommands interface {
	Redeem(context.Context, RedeemCommand) (RedeemResult, error)
}

type AdminReads interface {
	ListCampaigns(context.Context, ListCampaignsInput) (CampaignPage, error)
	GetCampaign(context.Context, int64) (Campaign, error)
}
