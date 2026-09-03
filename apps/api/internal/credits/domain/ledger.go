package domain

import (
	"time"

	"github.com/google/uuid"
)

type CommandType string

const (
	CommandGrant                 CommandType = "grant"
	CommandHold                  CommandType = "hold"
	CommandCapture               CommandType = "capture"
	CommandRelease               CommandType = "release"
	CommandRefund                CommandType = "refund"
	CommandDeduction             CommandType = "deduction"
	CommandCorrection            CommandType = "correction"
	CommandPurchaseCredit        CommandType = "purchase_credit"
	CommandPurchaseRefund        CommandType = "purchase_refund"
	CommandPaymentDisputeDebit   CommandType = "payment_dispute_debit"
	CommandPaymentDisputeRestore CommandType = "payment_dispute_restore"
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

type Actor struct {
	Type ActorType `json:"type"`
	ID   string    `json:"-"`
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
	Amount               int64  `json:"amount_credits"`
	CapturedAmount       int64  `json:"captured_credits"`
	ReleasedAmount       int64  `json:"released_credits"`
	Status               string `json:"status"`
	CaptureTransactionID string `json:"capture_transaction_id,omitempty"`
	ReleaseTransactionID string `json:"release_transaction_id,omitempty"`
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

// WalletBalance.UserID is an internal wallet FK and must never cross the HTTP boundary.
type WalletBalance struct {
	UserID      int64     `json:"-"`
	PublicID    uuid.UUID `json:"public_id"`
	DisplayName string    `json:"display_name"`
	Available   int64     `json:"available_credits"`
	Reserved    int64     `json:"reserved_credits"`
}
