package ports

import (
	"context"
	"time"

	"ideaboard/api/internal/credits/domain"
)

type CommandResult struct {
	TransactionID string               `json:"transaction_id"`
	CommandType   domain.CommandType   `json:"command_type"`
	Amount        int64                `json:"amount_credits"`
	Wallet        domain.WalletBalance `json:"wallet"`
	Replayed      bool                 `json:"replayed"`
	CreatedAt     time.Time            `json:"created_at"`
}

type GrantCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

// PurchaseCommand credits a paid external order. Only the Payments service
// may issue it, and callers should compose it into the Payment Order transaction.
type PurchaseCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type PurchaseRefundHoldCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type PurchaseRefundCommand struct {
	BillingAccountID  int64
	HoldTransactionID string
	Metadata          domain.CommandMetadata
}

// PaymentDisputeDebitCommand is a provider-driven, bounded recovery debit.
// The database routine caps the actual posting at the current available
// balance and returns the unrecovered remainder without creating a negative
// projection.
type PaymentDisputeDebitCommand struct {
	BillingAccountID int64
	RequestedAmount  domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type PaymentDisputeDebitResult struct {
	CommandResult
	RequestedAmount   int64 `json:"requested_amount_credits"`
	UnrecoveredAmount int64 `json:"unrecovered_credits"`
}

type PaymentDisputeRestoreCommand struct {
	BillingAccountID      int64
	OriginalTransactionID string
	Amount                domain.CreditAmount
	Metadata              domain.CommandMetadata
}

// DeductionCommand records a discretionary administrative debit. It can only
// consume available credits and always creates a new immutable transaction.
type DeductionCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

// CorrectionCommand partially or fully reverses one prior administrative
// Grant, Deduction, or Refund. Capture corrections remain linked Refunds.
type CorrectionCommand struct {
	BillingAccountID      int64
	OriginalTransactionID string
	Amount                domain.CreditAmount
	Metadata              domain.CommandMetadata
}

type HoldCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type CaptureCommand struct {
	BillingAccountID  int64
	HoldTransactionID string
	Amount            domain.CreditAmount
	Metadata          domain.CommandMetadata
}

type ReleaseCommand struct {
	BillingAccountID  int64
	HoldTransactionID string
	Amount            domain.CreditAmount
	Metadata          domain.CommandMetadata
}

type RefundCommand struct {
	BillingAccountID    int64
	ChargeTransactionID string
	Amount              domain.CreditAmount
	Metadata            domain.CommandMetadata
}

type Commands interface {
	Grant(context.Context, GrantCommand) (CommandResult, error)
	Purchase(context.Context, PurchaseCommand) (CommandResult, error)
	PurchaseRefundHold(context.Context, PurchaseRefundHoldCommand) (CommandResult, error)
	PurchaseRefund(context.Context, PurchaseRefundCommand) (CommandResult, error)
	PaymentDisputeDebit(context.Context, PaymentDisputeDebitCommand) (PaymentDisputeDebitResult, error)
	PaymentDisputeRestore(context.Context, PaymentDisputeRestoreCommand) (CommandResult, error)
	Deduct(context.Context, DeductionCommand) (CommandResult, error)
	Correct(context.Context, CorrectionCommand) (CommandResult, error)
	Hold(context.Context, HoldCommand) (CommandResult, error)
	Capture(context.Context, CaptureCommand) (CommandResult, error)
	Release(context.Context, ReleaseCommand) (CommandResult, error)
	Refund(context.Context, RefundCommand) (CommandResult, error)
}
