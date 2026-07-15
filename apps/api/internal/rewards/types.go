package rewards

import (
	"context"
	"errors"
	"time"
)

type CampaignMode string

const (
	CampaignModePublic    CampaignMode = "public"
	CampaignModeSingleUse CampaignMode = "single_use"
)

type CampaignLifecycleStatus string

const (
	CampaignLifecycleDraft     CampaignLifecycleStatus = "draft"
	CampaignLifecycleActive    CampaignLifecycleStatus = "active"
	CampaignLifecyclePaused    CampaignLifecycleStatus = "paused"
	CampaignLifecycleCancelled CampaignLifecycleStatus = "cancelled"
)

type CampaignStatus string

const (
	CampaignStatusDraft     CampaignStatus = "draft"
	CampaignStatusCancelled CampaignStatus = "cancelled"
	CampaignStatusExpired   CampaignStatus = "expired"
	CampaignStatusExhausted CampaignStatus = "exhausted"
	CampaignStatusPaused    CampaignStatus = "paused"
	CampaignStatusScheduled CampaignStatus = "scheduled"
	CampaignStatusActive    CampaignStatus = "active"
)

var (
	ErrInvalidCommand       = errors.New("invalid reward command")
	ErrCampaignNotFound     = errors.New("reward campaign not found")
	ErrCampaignNotDraft     = errors.New("reward campaign is not draft")
	ErrCampaignNotActive    = errors.New("reward campaign is not active")
	ErrCampaignNotPaused    = errors.New("reward campaign is not paused")
	ErrCampaignRulesFrozen  = errors.New("reward campaign rules are frozen")
	ErrCampaignNotEmpty     = errors.New("reward campaign is not an empty draft")
	ErrCampaignExpired      = errors.New("reward campaign is expired")
	ErrPublicCodeExists     = errors.New("public reward code already exists")
	ErrPublicCodeRequired   = errors.New("public reward code is required")
	ErrCampaignModeMismatch = errors.New("reward campaign mode does not support this Code operation")
	ErrCodeBatchLimit       = errors.New("reward Code Batch exceeds the synchronous generation limit")
	ErrCodeBatchConflict    = errors.New("reward Code Batch idempotency key conflicts with another payload")
	ErrCodeBatchNotFound    = errors.New("reward Code Batch not found")
	ErrCodeCollision        = errors.New("generated reward Code is not unique")
	ErrCodeUnavailable      = errors.New("reward code is invalid or unavailable")
	ErrCampaignExhausted    = errors.New("reward campaign is exhausted")
	ErrAlreadyRedeemed      = errors.New("reward campaign already redeemed")
)

type Campaign struct {
	ID                   int64                   `json:"id"`
	DisplayName          string                  `json:"display_name"`
	InternalNote         string                  `json:"internal_note"`
	CreditAmount         int64                   `json:"credit_amount"`
	Mode                 CampaignMode            `json:"mode"`
	LifecycleStatus      CampaignLifecycleStatus `json:"lifecycle_status"`
	Status               CampaignStatus          `json:"status"`
	MaxRedemptions       int64                   `json:"max_redemptions"`
	RedemptionCount      int64                   `json:"redemption_count"`
	IssuedCodeCount      int64                   `json:"issued_code_count"`
	ValidUnusedCodeCount int64                   `json:"valid_unused_code_count"`
	StartsAt             *time.Time              `json:"starts_at,omitempty"`
	ExpiresAt            *time.Time              `json:"expires_at,omitempty"`
	CreatedBy            int64                   `json:"created_by"`
	CreatedAt            time.Time               `json:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at"`
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

type CodeBatch struct {
	ID                 int64      `json:"id"`
	CampaignID         int64      `json:"campaign_id"`
	IdempotencyKey     string     `json:"idempotency_key"`
	CodeCount          int        `json:"code_count"`
	ValidUnusedCount   int        `json:"valid_unused_count"`
	RedeemedCount      int        `json:"redeemed_count"`
	RevokedUnusedCount int        `json:"revoked_unused_count"`
	IssuedBy           int64      `json:"issued_by"`
	IssuedAt           time.Time  `json:"issued_at"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	RevokedBy          *int64     `json:"revoked_by,omitempty"`
}

type GeneratedCodeBatch struct {
	Batch             CodeBatch `json:"batch"`
	Plaintexts        []string  `json:"plaintexts,omitempty"`
	ExportUnavailable bool      `json:"export_unavailable"`
	Replayed          bool      `json:"replayed"`
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
	Mode           CampaignMode
	MaxRedemptions int64
	StartsAt       *time.Time
	ExpiresAt      *time.Time
	ActorUserID    int64
	Reason         string
}

type GenerateCodeBatchCommand struct {
	CampaignID     int64
	Count          int
	IdempotencyKey string
	ActorUserID    int64
	Reason         string
}

type RevokeCodeBatchCommand struct {
	CampaignID  int64
	BatchID     int64
	ActorUserID int64
	Confirmed   bool
	Reason      string
}

type ListCodeBatchesInput struct {
	CampaignID int64
	BeforeID   int64
	Limit      int
}

type CodeBatchPage struct {
	Batches      []CodeBatch `json:"batches"`
	NextBeforeID int64       `json:"next_before_id,omitempty"`
}

type IssuePublicCodeCommand struct {
	CampaignID  int64
	Code        string
	ActorUserID int64
	Reason      string
}

type UpdateCampaignCommand struct {
	CampaignID     int64
	DisplayName    string
	InternalNote   string
	CreditAmount   int64
	MaxRedemptions int64
	StartsAt       *time.Time
	ExpiresAt      *time.Time
	ActorUserID    int64
	Reason         string
}

type ActivateCampaignCommand struct {
	CampaignID  int64
	ActorUserID int64
	Reason      string
}

type PauseCampaignCommand struct {
	CampaignID  int64
	ActorUserID int64
	Reason      string
}

type ResumeCampaignCommand struct {
	CampaignID  int64
	ActorUserID int64
	Reason      string
}

type DeleteCampaignCommand struct {
	CampaignID  int64
	ActorUserID int64
	Reason      string
}

type CancelCampaignCommand struct {
	CampaignID  int64
	ActorUserID int64
	Confirmed   bool
	Reason      string
}

type RedeemCommand struct {
	UserID int64
	Code   string
}

type AdminCommands interface {
	CreateCampaign(context.Context, CreateCampaignCommand) (Campaign, error)
	UpdateCampaign(context.Context, UpdateCampaignCommand) (Campaign, error)
	IssuePublicCode(context.Context, IssuePublicCodeCommand) (IssuedPublicCode, error)
	GenerateCodeBatch(context.Context, GenerateCodeBatchCommand) (GeneratedCodeBatch, error)
	RevokeCodeBatch(context.Context, RevokeCodeBatchCommand) (CodeBatch, error)
	ActivateCampaign(context.Context, ActivateCampaignCommand) (Campaign, error)
	PauseCampaign(context.Context, PauseCampaignCommand) (Campaign, error)
	ResumeCampaign(context.Context, ResumeCampaignCommand) (Campaign, error)
	DeleteCampaign(context.Context, DeleteCampaignCommand) error
	CancelCampaign(context.Context, CancelCampaignCommand) (Campaign, error)
}

type UserCommands interface {
	Redeem(context.Context, RedeemCommand) (RedeemResult, error)
}

type AdminReads interface {
	ListCampaigns(context.Context, ListCampaignsInput) (CampaignPage, error)
	GetCampaign(context.Context, int64) (Campaign, error)
	ListCodeBatches(context.Context, ListCodeBatchesInput) (CodeBatchPage, error)
}
