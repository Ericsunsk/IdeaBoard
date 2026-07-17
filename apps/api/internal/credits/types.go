package credits

import (
	"context"
	"errors"
	"time"
)

type CommandType string

const (
	CommandGrant      CommandType = "grant"
	CommandHold       CommandType = "hold"
	CommandCapture    CommandType = "capture"
	CommandRelease    CommandType = "release"
	CommandRefund     CommandType = "refund"
	CommandDeduction  CommandType = "deduction"
	CommandCorrection CommandType = "correction"
)

type AccountRole string

const (
	AccountUserAvailable  AccountRole = "user_available"
	AccountUserReserved   AccountRole = "user_reserved"
	AccountSystemIssuance AccountRole = "system_issuance"
	AccountSystemRevenue  AccountRole = "system_revenue"
	AccountSystemSink     AccountRole = "system_sink"
)

type ActorType string

const (
	ActorAdmin   ActorType = "admin"
	ActorSystem  ActorType = "system"
	ActorService ActorType = "service"
	ActorUser    ActorType = "user"
)

var (
	ErrInvalidCommand             = errors.New("invalid credit command")
	ErrWalletNotFound             = errors.New("credit wallet not found")
	ErrWalletFrozen               = errors.New("credit wallet is frozen")
	ErrWalletRestricted           = errors.New("credit wallet is restricted")
	ErrWalletRestrictionNotFound  = errors.New("credit wallet restriction not found")
	ErrInsufficientCredits        = errors.New("insufficient credits")
	ErrIdempotencyConflict        = errors.New("credit idempotency key conflicts with a different request")
	ErrHoldNotFound               = errors.New("credit hold not found")
	ErrHoldSettled                = errors.New("credit hold is already settled")
	ErrChargeNotFound             = errors.New("credit charge not found")
	ErrRefundExceedsCharge        = errors.New("credit refund exceeds remaining charge")
	ErrWelcomeIdentityKeyMismatch = errors.New("welcome identity key does not match the permanently bound key")
)

type Actor struct {
	Type ActorType `json:"type"`
	ID   string    `json:"id"`
}

type BusinessReference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type CommandMetadata struct {
	IdempotencyKey    string
	Actor             Actor
	Source            string
	Reason            string
	BusinessReference BusinessReference
}

type GrantCommand struct {
	UserID   int64
	Amount   CreditAmount
	Metadata CommandMetadata
}

// DeductionCommand records a discretionary administrative debit. It can only
// consume available credits and always creates a new immutable transaction.
type DeductionCommand struct {
	UserID   int64
	Amount   CreditAmount
	Metadata CommandMetadata
}

// CorrectionCommand partially or fully reverses one prior administrative
// Grant, Deduction, or Refund. Capture corrections remain linked Refunds.
type CorrectionCommand struct {
	UserID                int64
	OriginalTransactionID string
	Amount                CreditAmount
	Metadata              CommandMetadata
}

type HoldCommand struct {
	UserID   int64
	Amount   CreditAmount
	Metadata CommandMetadata
}

type CaptureCommand struct {
	UserID            int64
	HoldTransactionID string
	Metadata          CommandMetadata
}

type ReleaseCommand struct {
	UserID            int64
	HoldTransactionID string
	Metadata          CommandMetadata
}

type RefundCommand struct {
	UserID              int64
	ChargeTransactionID string
	Amount              CreditAmount
	Metadata            CommandMetadata
}

type Commands interface {
	Grant(context.Context, GrantCommand) (CommandResult, error)
	Deduct(context.Context, DeductionCommand) (CommandResult, error)
	Correct(context.Context, CorrectionCommand) (CommandResult, error)
	Hold(context.Context, HoldCommand) (CommandResult, error)
	Capture(context.Context, CaptureCommand) (CommandResult, error)
	Release(context.Context, ReleaseCommand) (CommandResult, error)
	Refund(context.Context, RefundCommand) (CommandResult, error)
}

type WalletLifecycle interface {
	BindWelcomeIdentityKey(context.Context, string) error
	ProvisionWallet(context.Context, int64) error
	SetWalletFrozen(context.Context, int64, bool) error
	ClaimWelcomeGrant(context.Context, string, GrantCommand) (CommandResult, bool, error)
}

type WalletRestrictionCommands interface {
	RestrictWallet(context.Context, RestrictWalletCommand) (WalletRestrictionChange, error)
	ReleaseWalletRestriction(context.Context, ReleaseWalletRestrictionCommand) (WalletRestrictionChange, error)
}

type WalletReader interface {
	GetWallet(context.Context, int64) (WalletDetails, error)
	ListLedger(context.Context, ListLedgerInput) (LedgerPage, error)
}

type WalletBalance struct {
	UserID    int64 `json:"user_id"`
	Available int64 `json:"available_credits"`
	Reserved  int64 `json:"reserved_credits"`
}

// WalletDetails is the current administrative read model. CommandResult keeps
// WalletBalance only, preserving its transaction-time balance snapshot.
type WalletDetails struct {
	WalletBalance
	Status             WalletStatus        `json:"status"`
	ActiveRestrictions []WalletRestriction `json:"active_restrictions"`
}

type WalletStatus string

const (
	WalletStatusActive WalletStatus = "active"
	WalletStatusFrozen WalletStatus = "frozen"
)

type WalletRestriction struct {
	RestrictionID string            `json:"restriction_id"`
	UserID        int64             `json:"user_id"`
	Type          string            `json:"type"`
	Source        string            `json:"source"`
	RelatedObject BusinessReference `json:"related_object"`
	Reason        string            `json:"reason"`
	CreatedBy     Actor             `json:"created_by"`
	CreatedAt     time.Time         `json:"created_at"`
}

type RestrictWalletCommand struct {
	UserID        int64
	Type          string
	Source        string
	RelatedObject BusinessReference
	Reason        string
	Actor         Actor
}

type ReleaseWalletRestrictionCommand struct {
	RestrictionID string
	Reason        string
	Actor         Actor
}

type WalletRestrictionChange struct {
	RestrictionID string `json:"restriction_id"`
	Replayed      bool   `json:"replayed"`
}

type CommandResult struct {
	TransactionID string        `json:"transaction_id"`
	CommandType   CommandType   `json:"command_type"`
	Amount        int64         `json:"amount_credits"`
	Wallet        WalletBalance `json:"wallet"`
	Replayed      bool          `json:"replayed"`
	CreatedAt     time.Time     `json:"created_at"`
}

type Posting struct {
	AccountRole AccountRole `json:"account_role"`
	Amount      int64       `json:"amount_credits"`
}

type LedgerTransaction struct {
	SequenceID            int64             `json:"sequence_id"`
	TransactionID         string            `json:"transaction_id"`
	CommandType           CommandType       `json:"command_type"`
	Amount                int64             `json:"amount_credits"`
	Actor                 Actor             `json:"actor"`
	Source                string            `json:"source"`
	Reason                string            `json:"reason"`
	BusinessReference     BusinessReference `json:"business_reference"`
	RelatedTransactionID  string            `json:"related_transaction_id,omitempty"`
	AvailableBalanceAfter int64             `json:"available_credits_after"`
	ReservedBalanceAfter  int64             `json:"reserved_credits_after"`
	CreatedAt             time.Time         `json:"created_at"`
	Postings              []Posting         `json:"postings"`
	Hold                  *HoldReference    `json:"hold,omitempty"`
	Charge                *ChargeReference  `json:"charge,omitempty"`
}

// HoldReference and ChargeReference make settlement facts explicit in Admin
// ledger reads; related_transaction_id remains the immutable audit link.
type HoldReference struct {
	Amount                int64  `json:"amount_credits"`
	Status                string `json:"status"`
	TerminalTransactionID string `json:"terminal_transaction_id,omitempty"`
}

type ChargeReference struct {
	Amount              int64 `json:"amount_credits"`
	RefundedAmount      int64 `json:"refunded_credits"`
	RemainingRefundable int64 `json:"remaining_refundable_credits"`
}

type ListLedgerInput struct {
	UserID         int64
	BeforeSequence int64
	Limit          int
}

type LedgerPage struct {
	Transactions       []LedgerTransaction `json:"transactions"`
	NextBeforeSequence int64               `json:"next_before_sequence,omitempty"`
}
