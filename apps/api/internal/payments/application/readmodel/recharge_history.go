package readmodel

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"

	"github.com/google/uuid"
)

const MaxAdminPaymentOrderCursorLen = 512

// AdminPaymentOrderCursor is the immutable keyset position for admin order reads.
type AdminPaymentOrderCursor struct {
	CreatedAt string `json:"created_at"`
	OrderID   string `json:"order_id"`
}

func DecodeAdminPaymentOrderCursor(value string) (*AdminPaymentOrderCursor, error) {
	if len(value) > MaxAdminPaymentOrderCursorLen {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != value {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	var cursor AdminPaymentOrderCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.CreatedAt == "" || cursor.OrderID == "" {
		return nil, paymentcontracts.ErrAdminPaymentOrderCursor
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
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
	return &cursor, nil
}

func EncodeAdminPaymentOrderCursor(createdAt time.Time, orderID uuid.UUID) (string, error) {
	raw, err := json.Marshal(AdminPaymentOrderCursor{CreatedAt: createdAt.UTC().Format(time.RFC3339Nano), OrderID: orderID.String()})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

const (
	// DefaultRechargeHistoryLimit is the default page size for a user's recharge history.
	DefaultRechargeHistoryLimit = 20
	// MaxRechargeHistoryLimit bounds the number of rows returned by one page.
	MaxRechargeHistoryLimit = 20
	// MaxRechargeHistoryCursorLen bounds opaque cursor input before decoding.
	MaxRechargeHistoryCursorLen = 512
)

// RechargeHistoryCursor is the immutable keyset position for recharge history.
// It is an application read-model value and is never persisted as a domain fact.
type RechargeHistoryCursor struct {
	OccurredAt string `json:"occurred_at"`
	OrderID    string `json:"order_id"`
}

func DecodeRechargeHistoryCursor(value string) (*RechargeHistoryCursor, error) {
	if len(value) > MaxRechargeHistoryCursorLen {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	var cursor RechargeHistoryCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.OccurredAt == "" || cursor.OrderID == "" {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.OccurredAt); err != nil {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	if _, err := uuid.Parse(cursor.OrderID); err != nil {
		return nil, paymentcontracts.ErrRechargeHistoryCursor
	}
	return &cursor, nil
}

func EncodeRechargeHistoryCursor(occurredAt time.Time, orderID uuid.UUID) (string, error) {
	raw, err := json.Marshal(RechargeHistoryCursor{OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), OrderID: orderID.String()})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
