package readmodel

import (
	"strings"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/pagination"

	"github.com/google/uuid"
)

const (
	// DefaultAdminPaymentOrderLimit tracks the shared pagination standard.
	DefaultAdminPaymentOrderLimit = pagination.DefaultLimit
	// MaxAdminPaymentOrderLimit tracks the shared pagination standard.
	MaxAdminPaymentOrderLimit = pagination.MaxLimit
	// MaxAdminPaymentOrderCursorLen tracks the shared pagination standard.
	MaxAdminPaymentOrderCursorLen = pagination.MaxCursorLen
)

// AdminPaymentOrderCursor is the immutable keyset position for admin order reads.
type AdminPaymentOrderCursor struct {
	CreatedAt string `json:"created_at"`
	OrderID   string `json:"order_id"`
}

func DecodeAdminPaymentOrderCursor(value string) (*AdminPaymentOrderCursor, error) {
	// Transport integrity lives in the shared pagination helper; this
	// wrapper only enforces the admin keyset position fields.
	cursor, err := pagination.DecodeCursor[AdminPaymentOrderCursor](value)
	if err != nil {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	if cursor == nil {
		return nil, nil
	}
	if cursor.CreatedAt == "" || cursor.OrderID == "" {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil || createdAt.UTC().Format(time.RFC3339Nano) != cursor.CreatedAt {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	parsedID, err := uuid.Parse(cursor.OrderID)
	if err != nil || parsedID.String() != strings.ToLower(cursor.OrderID) {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	return cursor, nil
}

func EncodeAdminPaymentOrderCursor(createdAt time.Time, orderID uuid.UUID) (string, error) {
	return pagination.EncodeCursor(AdminPaymentOrderCursor{CreatedAt: createdAt.UTC().Format(time.RFC3339Nano), OrderID: orderID.String()})
}

const (
	// DefaultRechargeHistoryLimit is the default page size for a user's recharge history.
	// It tracks the shared pagination standard.
	DefaultRechargeHistoryLimit = pagination.DefaultLimit
	// MaxRechargeHistoryLimit bounds the number of rows returned by one page.
	// It tracks the shared pagination standard.
	MaxRechargeHistoryLimit = pagination.MaxLimit
	// MaxRechargeHistoryCursorLen bounds opaque cursor input before decoding.
	MaxRechargeHistoryCursorLen = pagination.MaxCursorLen
)

// RechargeHistoryCursor is the immutable keyset position for recharge history.
// It is an application read-model value and is never persisted as a domain fact.
type RechargeHistoryCursor struct {
	OccurredAt string `json:"occurred_at"`
	OrderID    string `json:"order_id"`
}

func DecodeRechargeHistoryCursor(value string) (*RechargeHistoryCursor, error) {
	// The legacy codec stays as the expand-phase seam: callers keep using
	// this function while the transport integrity checks live in the shared
	// pagination helper.
	cursor, err := pagination.DecodeCursor[RechargeHistoryCursor](value)
	if err != nil {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	if cursor == nil {
		return nil, nil
	}
	if cursor.OccurredAt == "" || cursor.OrderID == "" {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.OccurredAt); err != nil {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	if _, err := uuid.Parse(cursor.OrderID); err != nil {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	return cursor, nil
}

func EncodeRechargeHistoryCursor(occurredAt time.Time, orderID uuid.UUID) (string, error) {
	return pagination.EncodeCursor(RechargeHistoryCursor{OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), OrderID: orderID.String()})
}
