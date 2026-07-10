package credits

import (
	"context"
	"errors"
	"time"
)

type CommandType string

const (
	CommandGrant   CommandType = "grant"
	CommandHold    CommandType = "hold"
	CommandCapture CommandType = "capture"
	CommandRelease CommandType = "release"
	CommandRefund  CommandType = "refund"
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
	ErrInsufficientCredits        = errors.New("insufficient credits")
	ErrIdempotencyConflict        = errors.New("credit idempotency key conflicts with a different request")
	ErrHoldNotFound               = errors.New("credit hold not found")
	ErrHoldSettled                = errors.New("credit hold is already settled")
	ErrChargeNotFound             = errors.New("credit charge not found")
	ErrRefundExceedsCharge        = errors.New("credit refund exceeds remaining charge")
	ErrWelcomeIdentityKeyMismatch = errors.New("Welcome identity key does not match the permanently bound key")
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

type WalletReader interface {
	GetWallet(context.Context, int64) (WalletBalance, error)
	ListLedger(context.Context, ListLedgerInput) (LedgerPage, error)
}

type WalletBalance struct {
	UserID    int64 `json:"user_id"`
	Available int64 `json:"available_credits"`
	Reserved  int64 `json:"reserved_credits"`
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
